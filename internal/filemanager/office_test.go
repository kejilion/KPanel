package filemanager

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func officeFixture(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	_ = w.SetComment("preserve package comment")
	parts["[Content_Types].xml"] = `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`
	parts["custom/unknown.xml"] = `<unknown><!-- preserve exactly -->secret structure</unknown>`
	for name, content := range parts {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func officeFixtures(t *testing.T) map[string][]byte {
	return map[string][]byte{
		"sample.docx": officeFixture(t, map[string]string{"word/document.xml": `<w:document xmlns:w="` + nsWord + `"><w:body><w:p><w:r><w:rPr><w:b/></w:rPr><w:t>Hello </w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t>world</w:t></w:r></w:p><w:tbl><w:tr><w:tc><w:p><w:r><w:t>Cell</w:t></w:r></w:p></w:tc></w:tr></w:tbl><w:p><w:r><w:fldChar w:fldCharType="begin"/><w:t>field</w:t></w:r></w:p></w:body></w:document>`}),
		"sample.xlsx": officeFixture(t, map[string]string{
			"xl/workbook.xml":            `<workbook xmlns="` + nsSheet + `" xmlns:r="` + nsRel + `"><sheets><sheet name="Data" r:id="r1"/></sheets></workbook>`,
			"xl/_rels/workbook.xml.rels": `<Relationships xmlns="` + nsPackageRel + `"><Relationship Id="r1" Target="worksheets/sheet1.xml"/></Relationships>`,
			"xl/sharedStrings.xml":       `<sst xmlns="` + nsSheet + `"><si><t>Label</t></si></sst>`,
			"xl/worksheets/sheet1.xml":   `<worksheet xmlns="` + nsSheet + `"><sheetData><row r="1"><c r="A1" t="s" s="4"><v>0</v></c><c r="B1"><v>12</v></c><c r="C1"><f>B1*2</f><v>24</v></c><c r="D1"/></row></sheetData><extLst><ext uri="preserved"/></extLst></worksheet>`}),
		"sample.pptx": officeFixture(t, map[string]string{
			"ppt/presentation.xml":            `<p:presentation xmlns:p="` + nsSlide + `" xmlns:r="` + nsRel + `"><p:sldIdLst><p:sldId id="256" r:id="r1"/></p:sldIdLst><p:sldSz cx="9144000" cy="5143500"/></p:presentation>`,
			"ppt/_rels/presentation.xml.rels": `<Relationships xmlns="` + nsPackageRel + `"><Relationship Id="r1" Target="slides/slide1.xml"/></Relationships>`,
			"ppt/slides/slide1.xml":           `<p:sld xmlns:p="` + nsSlide + `" xmlns:a="` + nsDraw + `"><p:cSld><p:spTree><p:sp><p:spPr><a:xfrm><a:off x="100" y="200"/><a:ext cx="4000000" cy="1000000"/></a:xfrm></p:spPr><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:rPr b="1" sz="2400"/><a:t>Title</a:t></a:r><a:endParaRPr/></a:p></p:txBody></p:sp></p:spTree></p:cSld></p:sld>`}),
	}
}
func officeMember(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range z.File {
		if f.Name == name {
			r, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			b, err := io.ReadAll(r)
			if err != nil {
				t.Fatal(err)
			}
			return b
		}
	}
	t.Fatal("missing member", name)
	return nil
}

func TestOfficeRoundTripAndPreservation(t *testing.T) {
	for name, original := range officeFixtures(t) {
		t.Run(name, func(t *testing.T) {
			m, root := newTestManager(t)
			file := filepath.Join(root, name)
			if err := os.WriteFile(file, original, 0600); err != nil {
				t.Fatal(err)
			}
			doc, err := m.ReadOffice(context.Background(), "/"+name)
			if err != nil {
				t.Fatal(err)
			}
			item := doc.Sections[0].Items[0]
			if !item.Editable {
				t.Fatal("ordinary text is not editable", item)
			}
			input := contract.FileWriteRequest{ExpectedResourceVersion: doc.Entry.ResourceVersion, ExpectedContentVersion: doc.ContentVersion, OfficeEdits: []contract.OfficeEdit{{ID: item.ID, Text: "Hello <KPanel> 世界 & friends"}}}
			entry, err := m.WriteOffice(context.Background(), doc.Entry.Path, input)
			if err != nil {
				t.Fatal(err)
			}
			if entry.ResourceVersion == doc.Entry.ResourceVersion {
				t.Fatal("version did not change")
			}
			after, err := m.ReadOffice(context.Background(), doc.Entry.Path)
			if err != nil {
				t.Fatal(err)
			}
			if after.Sections[0].Items[0].Text != input.OfficeEdits[0].Text {
				t.Fatalf("roundtrip: %#v", after)
			}
			updated, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(officeMember(t, original, "custom/unknown.xml"), officeMember(t, updated, "custom/unknown.xml")) {
				t.Fatal("unknown member changed")
			}
			if name == "sample.docx" {
				xml := string(officeMember(t, updated, "word/document.xml"))
				if !strings.Contains(xml, "<w:b/>") || !strings.Contains(xml, "<w:i/>") || !strings.Contains(xml, "<w:t>Cell</w:t>") {
					t.Fatal("formatting/unselected content changed", xml)
				}
				if after.Sections[0].Items[2].Editable {
					t.Fatal("field unexpectedly editable")
				}
			}
			if name == "sample.xlsx" {
				xml := string(officeMember(t, updated, "xl/worksheets/sheet1.xml"))
				if !strings.Contains(xml, `s="4"`) || !strings.Contains(xml, "<f>B1*2</f><v>24</v>") || !strings.Contains(xml, `uri="preserved"`) {
					t.Fatal("cell metadata changed", xml)
				}
				if !strings.Contains(string(officeMember(t, updated, "xl/workbook.xml")), `fullCalcOnLoad="1"`) {
					t.Fatal("missing recalculate flag")
				}
				if after.Sections[0].Items[2].Editable {
					t.Fatal("formula editable")
				}
			}
			if _, err := m.WriteOffice(context.Background(), doc.Entry.Path, input); !errors.Is(err, ErrConflict) {
				t.Fatal("stale save accepted", err)
			}
		})
	}
}

func TestOfficeRejectsInvalidEditsWithoutChangingFile(t *testing.T) {
	m, root := newTestManager(t)
	data := officeFixtures(t)["sample.xlsx"]
	file := filepath.Join(root, "sample.xlsx")
	_ = os.WriteFile(file, data, 0600)
	doc, err := m.ReadOffice(context.Background(), "/sample.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	for _, edits := range [][]contract.OfficeEdit{
		{{ID: "../../outside", Text: "bad"}}, {{ID: doc.Sections[0].Items[2].ID, Text: "99"}},
		{{ID: doc.Sections[0].Items[0].ID, Text: "x"}, {ID: doc.Sections[0].Items[0].ID, Text: "y"}},
		{{ID: doc.Sections[0].Items[0].ID, Text: "\x00"}},
	} {
		_, err := m.WriteOffice(context.Background(), doc.Entry.Path, contract.FileWriteRequest{ExpectedResourceVersion: doc.Entry.ResourceVersion, ExpectedContentVersion: doc.ContentVersion, OfficeEdits: edits})
		if !errors.Is(err, ErrOfficeInvalidEdit) {
			t.Fatal("invalid edit accepted", err)
		}
	}
	unchanged, _ := os.ReadFile(file)
	if !bytes.Equal(unchanged, data) {
		t.Fatal("file changed after rejection")
	}
	input := contract.FileWriteRequest{ExpectedResourceVersion: doc.Entry.ResourceVersion, ExpectedContentVersion: strings.Repeat("0", 64), OfficeEdits: []contract.OfficeEdit{{ID: doc.Sections[0].Items[0].ID, Text: "x"}}}
	if _, err = m.WriteOffice(context.Background(), doc.Entry.Path, input); !errors.Is(err, ErrConflict) {
		t.Fatal("content proof ignored", err)
	}
}
func TestOfficeRejectsUnsafePackagesAndXML(t *testing.T) {
	for _, xml := range []string{`<!DOCTYPE x [<!ENTITY foo SYSTEM "file:///secret">]><x>&foo;</x>`, `<x>` + strings.Repeat("<x>", 70) + strings.Repeat("</x>", 70) + `</x>`, `<x/><x/>`} {
		if _, err := parseOfficeXML(context.Background(), []byte(xml)); err == nil {
			t.Fatal("unsafe XML accepted")
		}
	}
	for _, name := range []string{"../outside.xml", "/absolute.xml", `bad\name.xml`} {
		data := officeFixture(t, map[string]string{name: "bad"})
		if _, err := openOffice(context.Background(), data); err == nil {
			t.Fatal("unsafe zip path accepted", name)
		}
	}
	m, root := newTestManager(t)
	data := officeFixture(t, map[string]string{"word/document.xml": `<w:document xmlns:w="` + nsWord + `"><w:body><w:p><w:r><w:t>signed</w:t></w:r></w:p></w:body></w:document>`, "_xmlsignatures/sig1.xml": "signature"})
	_ = os.WriteFile(filepath.Join(root, "signed.docx"), data, 0600)
	doc, err := m.ReadOffice(context.Background(), "/signed.docx")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Sections[0].Items[0].Editable {
		t.Fatal("signed document editable")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := m.ReadOffice(ctx, "/signed.docx"); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored", err)
	}
	if _, err := m.ReadOffice(context.Background(), "/docker/kpanel/secret.docx"); !errors.Is(err, ErrProtected) {
		t.Fatal("protected path allowed", err)
	}
}

func TestOfficeAttributeValuesAndNumericTypesRemainValid(t *testing.T) {
	input := `<c r="A1" note='contains t="fake" and xml:space="fake"' t="s">`
	if actual := officeRemoveAttrs(input, "t"); actual != `<c r="A1" note='contains t="fake" and xml:space="fake"'>` {
		t.Fatal("unrelated attribute was changed", actual)
	}
	for _, value := range []string{"0", "0.5", "1e3", "0x1p0", "=SUM(A1)", "001"} {
		t.Run(value, func(t *testing.T) {
			m, root := newTestManager(t)
			original := officeFixtures(t)["sample.xlsx"]
			file := filepath.Join(root, "sample.xlsx")
			_ = os.WriteFile(file, original, 0600)
			doc, err := m.ReadOffice(context.Background(), "/sample.xlsx")
			if err != nil {
				t.Fatal(err)
			}
			input := contract.FileWriteRequest{ExpectedResourceVersion: doc.Entry.ResourceVersion, ExpectedContentVersion: doc.ContentVersion, OfficeEdits: []contract.OfficeEdit{{ID: doc.Sections[0].Items[1].ID, Text: value}}}
			if _, err = m.WriteOffice(context.Background(), doc.Entry.Path, input); err != nil {
				t.Fatal(err)
			}
			after, err := m.ReadOffice(context.Background(), doc.Entry.Path)
			if err != nil || after.Sections[0].Items[1].Text != value {
				t.Fatal("roundtrip", value, after, err)
			}
			data, _ := os.ReadFile(file)
			xml := string(officeMember(t, data, "xl/worksheets/sheet1.xml"))
			literal := value == "0x1p0" || value == "=SUM(A1)" || value == "001"
			if strings.Contains(xml, `r="B1" t="inlineStr"`) != literal {
				t.Fatal("numeric/literal type", xml)
			}
		})
	}
}
