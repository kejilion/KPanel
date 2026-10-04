package filemanager

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const MaxOfficeBytes = 16 << 20
const maxOfficeExpanded = 64 << 20
const maxOfficeXML = 4 << 20
const maxOfficeItems = 10_000
const maxOfficeEdits = 256
const maxOfficeDisplayBytes = 2 << 20
const maxOfficeViewBytes = 16 << 20

var ErrOfficeUnsupported = errors.New("office document structure is unsupported")
var ErrOfficeInvalidEdit = errors.New("office edit is invalid or targets unsupported content")

type officePackage struct {
	ctx          context.Context
	zip          *zip.Reader
	files        map[string]*zip.File
	xml          map[string]*officeXML
	targets      map[string]officeTarget
	items        int
	parsedNodes  int
	mediaBytes   int
	displayBytes int
	imageCache   map[string]*officeImage
	signed       bool
}

type officeTarget struct {
	part  string
	nodes []*officeNode
	cell  *officeNode
	text  string
}

func officeKind(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".docx":
		return "docx"
	case ".xlsx":
		return "xlsx"
	case ".pptx":
		return "pptx"
	}
	return ""
}

func openOffice(ctx context.Context, data []byte) (*officePackage, error) {
	if len(data) > MaxOfficeBytes {
		return nil, ErrTooLarge
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, ErrOfficeUnsupported
	}
	if len(z.File) > 2048 {
		return nil, ErrTooLarge
	}
	p := &officePackage{ctx: ctx, zip: z, files: map[string]*zip.File{}, xml: map[string]*officeXML{}, targets: map[string]officeTarget{}, imageCache: map[string]*officeImage{}}
	var expanded uint64
	for _, f := range z.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := f.Name
		if len(name) > 512 {
			return nil, ErrTooLarge
		}
		if !utf8.ValidString(name) || !validOfficeText(name) {
			return nil, ErrOfficeUnsupported
		}
		if name == "" || strings.ContainsAny(name, "\\\x00:") || name == ".." || strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") || path.Clean(name) != strings.TrimSuffix(name, "/") || f.Mode()&os.ModeSymlink != 0 || f.Flags&1 != 0 || (f.Method != zip.Store && f.Method != zip.Deflate) {
			return nil, ErrOfficeUnsupported
		}
		if _, exists := p.files[name]; exists {
			return nil, ErrOfficeUnsupported
		}
		if f.UncompressedSize64 > maxOfficeExpanded || expanded > maxOfficeExpanded-f.UncompressedSize64 {
			return nil, ErrTooLarge
		}
		expanded += f.UncompressedSize64
		p.files[name] = f
		if strings.HasPrefix(name, "_xmlsignatures/") {
			p.signed = true
		}
	}
	if p.files["[Content_Types].xml"] == nil {
		return nil, ErrOfficeUnsupported
	}
	return p, nil
}

func (p *officePackage) read(name string, limit int64) ([]byte, error) {
	if err := p.ctx.Err(); err != nil {
		return nil, err
	}
	f := p.files[name]
	if f == nil {
		return nil, ErrOfficeUnsupported
	}
	if f.UncompressedSize64 > uint64(limit) {
		return nil, ErrTooLarge
	}
	r, err := f.Open()
	if err != nil {
		return nil, ErrOfficeUnsupported
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, ErrOfficeUnsupported
	}
	if int64(len(data)) > limit {
		return nil, ErrTooLarge
	}
	return data, p.ctx.Err()
}

func (p *officePackage) part(name string) (*officeXML, error) {
	if x := p.xml[name]; x != nil {
		return x, nil
	}
	data, err := p.read(name, maxOfficeXML)
	if err != nil {
		return nil, err
	}
	x, err := parseOfficeXML(p.ctx, data)
	if err != nil {
		return nil, err
	}
	p.parsedNodes += x.count
	if p.parsedNodes > 150_000 {
		return nil, ErrTooLarge
	}
	p.xml[name] = x
	return x, nil
}

func (p *officePackage) document(kind string) (contract.OfficeDocument, error) {
	doc := contract.OfficeDocument{Kind: kind, Sections: []contract.OfficeSection{}, Notes: []string{"basic_layout"}}
	var err error
	switch kind {
	case "docx":
		doc.Sections, err = p.word()
	case "xlsx":
		doc.Sections, err = p.sheets()
		doc.Notes = append(doc.Notes, "formula_cached")
	case "pptx":
		doc.Sections, err = p.slides()
		doc.Notes = append(doc.Notes, "slide_objects")
	default:
		return doc, ErrOfficeUnsupported
	}
	if p.signed {
		p.targets = map[string]officeTarget{}
		doc.Notes = append(doc.Notes, "signed_readonly")
		for s := range doc.Sections {
			for i := range doc.Sections[s].Items {
				makeOfficeReadOnly(&doc.Sections[s].Items[i])
			}
		}
	}
	if err != nil {
		return doc, err
	}
	if _, err := officeViewSize(doc); err != nil {
		return doc, err
	}
	return doc, nil
}

func officeViewSize(doc contract.OfficeDocument) (int, error) {
	data, err := json.Marshal(doc)
	if err != nil {
		return 0, ErrOfficeUnsupported
	}
	// Reserve room for the entry metadata attached after parsing.
	if len(data) > maxOfficeViewBytes-(64<<10) {
		return 0, ErrTooLarge
	}
	return len(data), nil
}
func (p *officePackage) displayText(values ...string) error {
	for _, value := range values {
		p.displayBytes += len(value)
	}
	if p.displayBytes > maxOfficeDisplayBytes {
		return ErrTooLarge
	}
	return p.ctx.Err()
}

func makeOfficeReadOnly(item *contract.OfficeItem) {
	item.Editable = false
	for r := range item.Table {
		for c := range item.Table[r] {
			makeOfficeReadOnly(&item.Table[r][c])
		}
	}
}

func officeDigest(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func (m *Manager) ReadOffice(ctx context.Context, virtual string) (contract.OfficeDocument, error) {
	file, entry, err := m.Open(ctx, virtual)
	if err != nil {
		return contract.OfficeDocument{}, err
	}
	defer file.Close()
	if !entry.OfficeEditable {
		return contract.OfficeDocument{}, ErrOfficeUnsupported
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxOfficeBytes+1))
	if err != nil {
		return contract.OfficeDocument{}, err
	}
	p, err := openOffice(ctx, data)
	if err != nil {
		return contract.OfficeDocument{}, err
	}
	doc, err := p.document(officeKind(entry.Name))
	if err != nil {
		return contract.OfficeDocument{}, err
	}
	current, err := m.Stat(virtual)
	if err != nil || current.ResourceVersion != entry.ResourceVersion {
		return contract.OfficeDocument{}, ErrConflict
	}
	doc.Entry, doc.ContentVersion = entry, officeDigest(data)
	return doc, nil
}

// WriteOffice reopens the original archive and patches only validated content
// nodes. All other members keep their raw compressed bytes and metadata.
func (m *Manager) WriteOffice(ctx context.Context, virtual string, input contract.FileWriteRequest) (contract.FileEntry, error) {
	if input.Content != "" || len(input.OfficeEdits) == 0 || len(input.OfficeEdits) > maxOfficeEdits || len(input.ExpectedContentVersion) != 64 {
		return contract.FileEntry{}, ErrOfficeInvalidEdit
	}
	if err := m.writeMu.LockContext(ctx); err != nil {
		return contract.FileEntry{}, err
	}
	defer m.writeMu.Unlock()
	_, normalized, err := m.resolveExisting(virtual)
	if err != nil {
		return contract.FileEntry{}, err
	}
	if err := m.mutationError(normalized); err != nil {
		return contract.FileEntry{}, err
	}
	info, err := m.rootFS.Lstat(rootName(normalized))
	if err != nil {
		return contract.FileEntry{}, err
	}
	current := m.entry(normalized, info)
	if !current.OfficeEditable || !info.Mode().IsRegular() {
		return contract.FileEntry{}, ErrOfficeUnsupported
	}
	if input.ExpectedResourceVersion != current.ResourceVersion {
		return contract.FileEntry{}, ErrConflict
	}
	source, err := m.rootFS.Open(rootName(normalized))
	if err != nil {
		return contract.FileEntry{}, err
	}
	defer source.Close()
	opened, err := source.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return contract.FileEntry{}, ErrConflict
	}
	data, err := io.ReadAll(io.LimitReader(source, MaxOfficeBytes+1))
	if err != nil {
		return contract.FileEntry{}, err
	}
	if officeDigest(data) != input.ExpectedContentVersion {
		return contract.FileEntry{}, ErrConflict
	}
	p, err := openOffice(ctx, data)
	if err != nil {
		return contract.FileEntry{}, err
	}
	doc, err := p.document(officeKind(current.Name))
	if err != nil {
		return contract.FileEntry{}, err
	}
	viewBytes, _ := officeViewSize(doc)
	displayBytes := p.displayBytes
	patches := map[string][]officePatch{}
	seen := map[string]bool{}
	for _, edit := range input.OfficeEdits {
		target, ok := p.targets[edit.ID]
		if !ok || seen[edit.ID] || !utf8.ValidString(edit.Text) || !validOfficeText(edit.Text) || len(edit.Text) > 64<<10 {
			return contract.FileEntry{}, ErrOfficeInvalidEdit
		}
		seen[edit.ID] = true
		displayBytes += len(edit.Text) - len(target.text)
		before, _ := json.Marshal(target.text)
		after, _ := json.Marshal(edit.Text)
		viewBytes += len(after) - len(before)
		if displayBytes > maxOfficeDisplayBytes || viewBytes > maxOfficeViewBytes-(64<<10) {
			return contract.FileEntry{}, ErrTooLarge
		}
		changes, err := p.edit(target, edit.Text)
		if err != nil {
			return contract.FileEntry{}, err
		}
		patches[target.part] = append(patches[target.part], changes...)
	}
	if officeKind(current.Name) == "xlsx" {
		patches["xl/workbook.xml"] = append(patches["xl/workbook.xml"], p.recalculatePatch())
	}
	parent := path.Dir(normalized)
	temp, tempPath, err := m.createTemp(parent, ".kpanel-office-")
	if err != nil {
		return contract.FileEntry{}, err
	}
	success := false
	defer func() {
		temp.Close()
		if !success {
			_ = m.rootFS.Remove(rootName(tempPath))
		}
	}()
	if err := temp.Chmod(info.Mode().Perm()); err != nil {
		return contract.FileEntry{}, err
	}
	if err := preserveFileOwnership(temp, info); err != nil {
		return contract.FileEntry{}, err
	}
	if err := preserveFileExtendedAttributes(temp, source); err != nil {
		return contract.FileEntry{}, err
	}
	w := zip.NewWriter(&officeLimitedWriter{writer: temp, remaining: MaxOfficeBytes})
	if err := w.SetComment(p.zip.Comment); err != nil {
		return contract.FileEntry{}, err
	}
	for _, f := range p.zip.File {
		if err := ctx.Err(); err != nil {
			_ = w.Close()
			return contract.FileEntry{}, err
		}
		if changes := patches[f.Name]; len(changes) > 0 {
			updated, err := applyOfficePatches(p.xml[f.Name].data, changes)
			if err != nil {
				_ = w.Close()
				return contract.FileEntry{}, err
			}
			h := f.FileHeader
			member, err := w.CreateHeader(&h)
			if err != nil {
				_ = w.Close()
				return contract.FileEntry{}, err
			}
			if _, err := member.Write(updated); err != nil {
				_ = w.Close()
				return contract.FileEntry{}, err
			}
		} else if err := w.Copy(f); err != nil {
			_ = w.Close()
			return contract.FileEntry{}, err
		}
	}
	if err := w.Close(); err != nil {
		return contract.FileEntry{}, err
	}
	// The edited file must still fit the same preview budgets before replacing
	// the original, including offset/JSON expansion caused by escaped text.
	if _, err := temp.Seek(0, io.SeekStart); err != nil {
		return contract.FileEntry{}, err
	}
	written, err := io.ReadAll(io.LimitReader(temp, MaxOfficeBytes+1))
	if err != nil {
		return contract.FileEntry{}, err
	}
	checked, err := openOffice(ctx, written)
	if err != nil {
		return contract.FileEntry{}, err
	}
	if _, err = checked.document(officeKind(current.Name)); err != nil {
		return contract.FileEntry{}, err
	}
	if err := temp.Sync(); err != nil {
		return contract.FileEntry{}, err
	}
	if err := temp.Close(); err != nil {
		return contract.FileEntry{}, err
	}
	// Recheck external writes and target replacement immediately before commit.
	if _, _, err := m.resolveExisting(normalized); err != nil {
		return contract.FileEntry{}, err
	}
	latest, err := m.rootFS.Lstat(rootName(normalized))
	if err != nil || !os.SameFile(info, latest) || m.entry(normalized, latest).ResourceVersion != input.ExpectedResourceVersion {
		return contract.FileEntry{}, ErrConflict
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return contract.FileEntry{}, err
	}
	check, err := io.ReadAll(io.LimitReader(source, MaxOfficeBytes+1))
	if err != nil || officeDigest(check) != input.ExpectedContentVersion {
		return contract.FileEntry{}, ErrConflict
	}
	if err := ctx.Err(); err != nil {
		return contract.FileEntry{}, err
	}
	if err := m.rootFS.Rename(rootName(tempPath), rootName(normalized)); err != nil {
		return contract.FileEntry{}, err
	}
	if err := syncRootDirectory(m.rootFS, rootName(parent)); err != nil {
		return contract.FileEntry{}, err
	}
	success = true
	return m.Stat(normalized)
}

type officeLimitedWriter struct {
	writer    io.Writer
	remaining int
}

func (w *officeLimitedWriter) Write(data []byte) (int, error) {
	if len(data) > w.remaining {
		return 0, ErrTooLarge
	}
	n, err := w.writer.Write(data)
	w.remaining -= n
	return n, err
}

type officePatch struct {
	start, end int
	value      string
}

func applyOfficePatches(data []byte, edits []officePatch) ([]byte, error) {
	sort.Slice(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var result bytes.Buffer
	position := 0
	for _, edit := range edits {
		if edit.start < position || edit.end < edit.start || edit.end > len(data) {
			return nil, ErrOfficeInvalidEdit
		}
		result.Write(data[position:edit.start])
		result.WriteString(edit.value)
		position = edit.end
		if result.Len() > maxOfficeXML {
			return nil, ErrTooLarge
		}
	}
	result.Write(data[position:])
	if result.Len() > maxOfficeXML {
		return nil, ErrTooLarge
	}
	return result.Bytes(), nil
}
