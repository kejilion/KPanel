package filemanager

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"
)

const nsWord = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
const nsSheet = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
const nsSlide = "http://schemas.openxmlformats.org/presentationml/2006/main"
const nsDraw = "http://schemas.openxmlformats.org/drawingml/2006/main"
const nsRel = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
const nsPackageRel = "http://schemas.openxmlformats.org/package/2006/relationships"

type officeXML struct {
	data  []byte
	root  *officeNode
	count int
}
type officeNode struct {
	name                            xml.Name
	attrs                           []xml.Attr
	children                        []*officeNode
	text                            string
	buffer                          strings.Builder
	start, openEnd, closeStart, end int
}

func parseOfficeXML(ctx context.Context, data []byte) (*officeXML, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	x := &officeXML{data: data}
	var stack []*officeNode
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		before := int(d.InputOffset())
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, ErrOfficeUnsupported
		}
		after := int(d.InputOffset())
		switch t := token.(type) {
		case xml.StartElement:
			x.count++
			if len(stack) >= 64 || x.count > 100_000 {
				return nil, ErrTooLarge
			}
			n := &officeNode{name: t.Name, attrs: t.Attr, start: before, openEnd: after}
			if len(stack) == 0 {
				if x.root != nil {
					return nil, ErrOfficeUnsupported
				}
				x.root = n
			} else {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, ErrOfficeUnsupported
			}
			n := stack[len(stack)-1]
			n.closeStart, n.end = before, after
			n.text = n.buffer.String()
			n.buffer = strings.Builder{}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				n := stack[len(stack)-1]
				n.buffer.Write(t)
			} else if strings.TrimSpace(string(t)) != "" {
				return nil, ErrOfficeUnsupported
			}
		case xml.Directive:
			return nil, ErrOfficeUnsupported
		case xml.ProcInst:
			if t.Target != "xml" || x.root != nil {
				return nil, ErrOfficeUnsupported
			}
		}
	}
	if x.root == nil || len(stack) != 0 {
		return nil, ErrOfficeUnsupported
	}
	return x, nil
}

func (n *officeNode) is(ns, local string) bool {
	return n != nil && n.name.Space == ns && n.name.Local == local
}
func (n *officeNode) attr(local string) string {
	for _, a := range n.attrs {
		if a.Name.Local == local && a.Name.Space == "" {
			return a.Value
		}
	}
	return ""
}
func (n *officeNode) relID() string {
	for _, a := range n.attrs {
		if a.Name.Local == "id" && a.Name.Space == nsRel {
			return a.Value
		}
	}
	return ""
}
func (n *officeNode) child(ns, local string) *officeNode {
	if n != nil {
		for _, c := range n.children {
			if c.is(ns, local) {
				return c
			}
		}
	}
	return nil
}
func (n *officeNode) all(ns, local string) []*officeNode {
	var found []*officeNode
	var visit func(*officeNode)
	visit = func(v *officeNode) {
		if v.is(ns, local) {
			found = append(found, v)
		}
		for _, c := range v.children {
			visit(c)
		}
	}
	if n != nil {
		visit(n)
	}
	return found
}
func officeText(nodes []*officeNode) string {
	var b strings.Builder
	for _, n := range nodes {
		b.WriteString(n.text)
	}
	return b.String()
}
func validOfficeText(s string) bool {
	for _, r := range s {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' || r == 0xfffe || r == 0xffff {
			return false
		}
	}
	return true
}
func escapedOffice(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// Scan quoted attributes so an attribute-like string inside another value is
// never mistaken for markup. The surrounding XML has already been validated.
func officeRemoveAttrs(open string, names ...string) string {
	remove := map[string]bool{}
	for _, name := range names {
		remove[name] = true
	}
	i := strings.IndexAny(open, " \t\r\n/>")
	if i < 0 {
		return open
	}
	var result strings.Builder
	position := 0
	space := func(b byte) bool { return b == ' ' || b == '\t' || b == '\r' || b == '\n' }
	for i < len(open) {
		start := i
		for i < len(open) && space(open[i]) {
			i++
		}
		if i >= len(open) || open[i] == '/' || open[i] == '>' {
			break
		}
		keyStart := i
		for i < len(open) && !space(open[i]) && open[i] != '=' {
			i++
		}
		key := open[keyStart:i]
		for i < len(open) && space(open[i]) {
			i++
		}
		if i >= len(open) || open[i] != '=' {
			break
		}
		i++
		for i < len(open) && space(open[i]) {
			i++
		}
		if i >= len(open) || (open[i] != '\'' && open[i] != '"') {
			break
		}
		quote := open[i]
		i++
		for i < len(open) && open[i] != quote {
			i++
		}
		if i >= len(open) {
			break
		}
		i++
		if remove[key] {
			result.WriteString(open[position:start])
			position = i
		}
	}
	result.WriteString(open[position:])
	return result.String()
}

func officeOpening(data []byte, n *officeNode) (string, string) {
	op := string(data[n.start:n.openEnd])
	end := strings.IndexAny(op, " \t\r\n/>")
	if end < 1 {
		return "", ""
	}
	return op, op[1:end]
}
func officeNodeContent(data []byte, n *officeNode, content string, preserve bool) officePatch {
	op, qname := officeOpening(data, n)
	op = strings.TrimSuffix(op, ">")
	op = strings.TrimSuffix(op, "/")
	if preserve {
		op = officeRemoveAttrs(op, "xml:space") + ` xml:space="preserve"`
	}
	return officePatch{n.start, n.end, op + ">" + content + "</" + qname + ">"}
}

// Only package-local relationships are used; external targets are never fetched.
func (p *officePackage) relationships(part string) (map[string]string, error) {
	name := path.Join(path.Dir(part), "_rels", path.Base(part)+".rels")
	result := map[string]string{}
	if p.files[name] == nil {
		return result, nil
	}
	x, err := p.part(name)
	if err != nil {
		return nil, err
	}
	if !x.root.is(nsPackageRel, "Relationships") {
		return nil, ErrOfficeUnsupported
	}
	for _, r := range x.root.children {
		if !r.is(nsPackageRel, "Relationship") || r.attr("TargetMode") == "External" {
			continue
		}
		id, target := r.attr("Id"), r.attr("Target")
		if id == "" || target == "" || strings.ContainsAny(target, "\\\x00?#:") {
			continue
		}
		resolved := path.Clean(path.Join(path.Dir(part), target))
		if strings.HasPrefix(target, "/") {
			resolved = path.Clean(strings.TrimPrefix(target, "/"))
		}
		if resolved == ".." || strings.HasPrefix(resolved, "../") || p.files[resolved] == nil {
			continue
		}
		if _, exists := result[id]; exists {
			return nil, ErrOfficeUnsupported
		}
		result[id] = resolved
	}
	return result, nil
}
func (p *officePackage) itemID(part string, n *officeNode) (string, error) {
	p.items++
	if p.items > maxOfficeItems {
		return "", ErrTooLarge
	}
	id := fmt.Sprintf("%s:%d", part, n.start)
	if err := p.displayText(id); err != nil {
		return "", err
	}
	return id, nil
}
func (p *officePackage) edit(target officeTarget, text string) ([]officePatch, error) {
	x := p.xml[target.part]
	if target.cell != nil {
		return p.editCell(x, target.cell, text)
	}
	if len(target.nodes) == 0 || strings.ContainsAny(text, "\r\n\t") {
		return nil, ErrOfficeInvalidEdit
	}
	// Keep the unchanged prefix/suffix in their original runs, including styles.
	old := []rune(officeText(target.nodes))
	next := []rune(text)
	prefix := 0
	for prefix < len(old) && prefix < len(next) && old[prefix] == next[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(old)-prefix && suffix < len(next)-prefix && old[len(old)-1-suffix] == next[len(next)-1-suffix] {
		suffix++
	}
	insert := string(next[prefix : len(next)-suffix])
	position := 0
	inserted := false
	var patches []officePatch
	for _, n := range target.nodes {
		runes := []rune(n.text)
		start, end := position, position+len(runes)
		position = end
		keepBefore := max(0, min(len(runes), prefix-start))
		keepAfter := max(0, min(len(runes)-keepBefore, end-(len(old)-suffix)))
		value := string(runes[:keepBefore])
		if !inserted && end >= prefix {
			value += insert
			inserted = true
		}
		value += string(runes[len(runes)-keepAfter:])
		if value != n.text {
			patches = append(patches, officeNodeContent(x.data, n, escapedOffice(value), true))
		}
	}
	return patches, nil
}
