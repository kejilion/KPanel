package agent

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
)

func TestOfficeContentRouteRoundTripAndRejectedTargets(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	m, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	server.files = m
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	for name, content := range map[string]string{"[Content_Types].xml": "<Types/>", "word/document.xml": `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Original</w:t></w:r></w:p></w:body></w:document>`} {
		member, _ := writer.Create(name)
		_, _ = member.Write([]byte(content))
	}
	_ = writer.Close()
	if err := os.WriteFile(filepath.Join(root, "demo.docx"), data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	read := fileRequest(server, http.MethodGet, "/v1/files/content?path=%2Fdemo.docx&mode=office", "")
	if read.Code != 200 || read.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("read: %d %s", read.Code, read.Body.String())
	}
	var doc contract.OfficeDocument
	if err := json.Unmarshal(read.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	input := contract.FileWriteRequest{ExpectedResourceVersion: doc.Entry.ResourceVersion, ExpectedContentVersion: doc.ContentVersion, OfficeEdits: []contract.OfficeEdit{{ID: doc.Sections[0].Items[0].ID, Text: "Saved"}}}
	body, _ := json.Marshal(input)
	write := fileRequest(server, http.MethodPut, "/v1/files/content?path=%2Fdemo.docx", string(body))
	if write.Code != 200 {
		t.Fatalf("write: %d %s", write.Code, write.Body.String())
	}
	read = fileRequest(server, http.MethodGet, "/v1/files/content?path=%2Fdemo.docx&mode=office", "")
	if !strings.Contains(read.Body.String(), `"text":"Saved"`) {
		t.Fatal("readback failed", read.Body.String())
	}
	stale := fileRequest(server, http.MethodPut, "/v1/files/content?path=%2Fdemo.docx", string(body))
	if stale.Code != 409 {
		t.Fatal("stale write accepted", stale.Code)
	}
	for _, target := range []string{"/v1/files/content?path=%2Fdemo.docx&mode=office&disposition=attachment", "/v1/files/share-content?path=%2Fdemo.docx&mode=office"} {
		rejected := fileRequest(server, http.MethodGet, target, "")
		if rejected.Code < 400 {
			t.Fatal("invalid mode accepted", target, rejected.Code)
		}
	}
}
