package filemanager

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strconv"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func (p *officePackage) word() ([]contract.OfficeSection, error) {
	const part = "word/document.xml"
	x, err := p.part(part)
	if err != nil {
		return nil, err
	}
	if !x.root.is(nsWord, "document") {
		return nil, ErrOfficeUnsupported
	}
	body := x.root.child(nsWord, "body")
	if body == nil {
		return nil, ErrOfficeUnsupported
	}
	rels, err := p.relationships(part)
	if err != nil {
		return nil, err
	}
	section := contract.OfficeSection{Name: "", Items: []contract.OfficeItem{}}
	for _, n := range body.children {
		switch {
		case n.is(nsWord, "p"):
			item, err := p.textItem(part, n, nsWord)
			if err != nil {
				return nil, err
			}
			section.Items = append(section.Items, item)
			for _, blip := range n.all(nsDraw, "blip") {
				img, err := p.imageItem(rels[officeEmbed(blip)])
				if err != nil {
					return nil, err
				}
				if img != nil {
					section.Items = append(section.Items, *img)
				}
			}
		case n.is(nsWord, "tbl"):
			id, err := p.itemID(part, n)
			if err != nil {
				return nil, err
			}
			table := contract.OfficeItem{ID: id, Kind: "table", Table: [][]contract.OfficeItem{}}
			for _, row := range n.children {
				if !row.is(nsWord, "tr") {
					continue
				}
				cells := []contract.OfficeItem{}
				for _, cell := range row.children {
					if !cell.is(nsWord, "tc") {
						continue
					}
					paras := cell.all(nsWord, "p")
					item := contract.OfficeItem{Kind: "text"}
					if len(paras) == 1 {
						item, err = p.textItem(part, paras[0], nsWord)
						if err != nil {
							return nil, err
						}
					} else {
						id, err := p.itemID(part, cell)
						if err != nil {
							return nil, err
						}
						item.ID = id
						for i, para := range paras {
							if i > 0 {
								item.Text += "\n"
							}
							item.Text += officeText(para.all(nsWord, "t"))
						}
						if err := p.displayText(item.Text); err != nil {
							return nil, err
						}
					}
					cells = append(cells, item)
				}
				table.Table = append(table.Table, cells)
			}
			section.Items = append(section.Items, table)
		}
	}
	return []contract.OfficeSection{section}, nil
}

func (p *officePackage) textItem(part string, n *officeNode, ns string) (contract.OfficeItem, error) {
	id, err := p.itemID(part, n)
	if err != nil {
		return contract.OfficeItem{}, err
	}
	nodes := n.all(ns, "t")
	item := contract.OfficeItem{ID: id, Kind: "text", Text: officeText(nodes)}
	if err := p.displayText(item.Text); err != nil {
		return item, err
	}
	// Field results, drawings, breaks and tracked revisions must not be flattened.
	editable := len(nodes) > 0
	var safe func(*officeNode)
	safe = func(v *officeNode) {
		if v.name.Space != ns || (v.name.Local != "p" && v.name.Local != "r" && v.name.Local != "t" && v.name.Local != "rPr" && v.name.Local != "pPr" && v.name.Local != "endParaRPr") {
			editable = false
			return
		}
		if v.name.Local == "pPr" || v.name.Local == "rPr" || v.name.Local == "endParaRPr" {
			return
		}
		for _, c := range v.children {
			safe(c)
		}
	}
	safe(n)
	item.Editable = editable
	if editable {
		p.targets[id] = officeTarget{part: part, nodes: nodes, text: item.Text}
	}
	if ns == nsWord {
		if props := n.child(ns, "pPr"); props != nil {
			if a := props.child(ns, "jc"); a != nil {
				for _, attr := range a.attrs {
					if attr.Name.Local == "val" && attr.Name.Space == ns {
						item.Align = attr.Value
					}
				}
			}
		}
		for _, run := range n.children {
			if !run.is(ns, "r") {
				continue
			}
			props := run.child(ns, "rPr")
			if props == nil {
				break
			}
			item.Bold = officeEnabled(props.child(ns, "b"), ns)
			item.Italic = officeEnabled(props.child(ns, "i"), ns)
			if sz := props.child(ns, "sz"); sz != nil {
				for _, a := range sz.attrs {
					if a.Name.Space == ns && a.Name.Local == "val" {
						value, _ := strconv.ParseFloat(a.Value, 64)
						item.FontSize = max(8, min(72, value/2))
					}
				}
			}
			break
		}
	}
	return item, nil
}
func officeEnabled(n *officeNode, ns string) bool {
	if n == nil {
		return false
	}
	for _, a := range n.attrs {
		if a.Name.Space == ns && a.Name.Local == "val" {
			return a.Value != "0" && a.Value != "false" && a.Value != "off"
		}
	}
	return true
}
func officeEmbed(n *officeNode) string {
	for _, a := range n.attrs {
		if a.Name.Space == nsRel && a.Name.Local == "embed" {
			return a.Value
		}
	}
	return ""
}

type officeImage struct {
	url  string
	size int
}

func (p *officePackage) imageItem(name string) (*contract.OfficeItem, error) {
	if name == "" {
		return nil, nil
	}
	f := p.files[name]
	if f == nil {
		return nil, nil
	}
	if f.UncompressedSize64 > 2<<20 || uint64(p.mediaBytes)+f.UncompressedSize64 > 8<<20 {
		return nil, ErrTooLarge
	}
	cached, known := p.imageCache[name]
	if !known {
		data, err := p.read(name, 2<<20)
		if err != nil {
			return nil, err
		}
		config, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err == nil && config.Width > 0 && config.Height > 0 && int64(config.Width)*int64(config.Height) <= 16_000_000 && (format == "png" || format == "jpeg" || format == "gif") {
			cached = &officeImage{url: "data:image/" + format + ";base64," + base64.StdEncoding.EncodeToString(data), size: len(data)}
		}
		p.imageCache[name] = cached
	}
	if cached == nil {
		return nil, nil
	}
	p.items++
	if p.items > maxOfficeItems {
		return nil, ErrTooLarge
	}
	p.mediaBytes += cached.size
	return &contract.OfficeItem{Kind: "image", Image: cached.url}, nil
}

func officeString(n *officeNode, ns string) string {
	var b strings.Builder
	for _, t := range n.all(ns, "t") {
		b.WriteString(t.text)
	}
	return b.String()
}
