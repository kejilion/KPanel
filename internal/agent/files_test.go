package agent

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
)

func TestFileArchiveDownloadStreamsOneZIPAndRejectsStaleSelections(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "readme.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "site.css"), []byte("body{}"), 0644); err != nil {
		t.Fatal(err)
	}
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	server.files = manager
	readme, err := manager.Stat("/readme.txt")
	if err != nil {
		t.Fatal(err)
	}
	assets, err := manager.Stat("/assets")
	if err != nil {
		t.Fatal(err)
	}
	selection, _ := json.Marshal(contract.FileArchiveDownloadRequest{
		Sources: []string{"/readme.txt", "/assets"},
		ExpectedResourceVersions: map[string]string{
			"/readme.txt": readme.ResourceVersion,
			"/assets":     assets.ResourceVersion,
		},
	})
	response := fileRequest(
		server, http.MethodGet,
		"/v1/files/archive?selection="+url.QueryEscape(string(selection))+"&name=release.zip", "",
	)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "application/zip" ||
		!strings.Contains(response.Header().Get("Content-Disposition"), "release.zip") {
		t.Fatalf("archive status=%d headers=%#v body=%s", response.Code, response.Header(), response.Body.String())
	}
	archive, err := zip.NewReader(bytes.NewReader(response.Body.Bytes()), int64(response.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(archive.File))
	for _, item := range archive.File {
		if !item.FileInfo().IsDir() {
			names = append(names, item.Name)
		}
	}
	if strings.Join(names, ",") != "readme.txt,assets/site.css" {
		t.Fatalf("archive names=%#v", names)
	}

	stale, _ := json.Marshal(contract.FileArchiveDownloadRequest{
		Sources:                  []string{"/readme.txt"},
		ExpectedResourceVersions: map[string]string{"/readme.txt": "sha256:stale"},
	})
	conflict := fileRequest(
		server, http.MethodGet,
		"/v1/files/archive?selection="+url.QueryEscape(string(stale))+"&name=stale.zip", "",
	)
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "file_conflict") {
		t.Fatalf("stale archive status=%d body=%s", conflict.Code, conflict.Body.String())
	}
}

func TestFileThumbnailIsBoundedAndVersionProtected(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	server.files = manager
	source := image.NewNRGBA(image.Rect(0, 0, 640, 360))
	for y := range 360 {
		for x := range 640 {
			source.SetNRGBA(x, y, color.NRGBA{R: uint8(x % 255), G: uint8(y % 255), B: 160, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "photo.png"), encoded.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	entry, err := manager.Stat("/photo.png")
	if err != nil {
		t.Fatal(err)
	}

	response := fileRequest(
		server,
		http.MethodGet,
		"/v1/files/content?path=%2Fphoto.png&disposition=inline&mode=thumbnail&version="+
			url.QueryEscape(entry.ResourceVersion),
		"",
	)
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("thumbnail status=%d type=%q body=%s", response.Code, response.Header().Get("Content-Type"), response.Body.String())
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(response.Body.Bytes()))
	if err != nil || format != "png" || config.Width > fileThumbnailMaxWidth || config.Height > fileThumbnailMaxHeight {
		t.Fatalf("thumbnail config=%#v format=%q err=%v", config, format, err)
	}

	stale := fileRequest(
		server,
		http.MethodGet,
		"/v1/files/content?path=%2Fphoto.png&disposition=inline&mode=thumbnail&version=stale",
		"",
	)
	if stale.Code != http.StatusConflict {
		t.Fatalf("stale thumbnail status=%d body=%s", stale.Code, stale.Body.String())
	}
}

func TestFileTransferExportAndImportDirectory(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "app", "config"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "desktop"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app", "config", "site.conf"), []byte("enabled"), 0644); err != nil {
		t.Fatal(err)
	}
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	server.files = manager
	source, err := manager.Stat("/app")
	if err != nil {
		t.Fatal(err)
	}
	exported := fileRequest(
		server, http.MethodGet,
		"/v1/files/transfer/export?path=%2Fapp&resourceVersion="+url.QueryEscape(source.ResourceVersion), "",
	)
	if exported.Code != http.StatusOK || exported.Header().Get(fileTransferResultTrailer) != "ok" {
		t.Fatalf("export status=%d trailer=%q body=%s", exported.Code, exported.Header().Get(fileTransferResultTrailer), exported.Body.String())
	}
	rawMetadata, err := base64.RawURLEncoding.DecodeString(exported.Header().Get(fileTransferMetadataHeader))
	if err != nil {
		t.Fatal(err)
	}
	var metadata contract.FileTransferMetadata
	if err := json.Unmarshal(rawMetadata, &metadata); err != nil || metadata.Name != "app" || metadata.Kind != "directory" {
		t.Fatalf("metadata=%#v err=%v", metadata, err)
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/files/transfer/import?path=%2Fdesktop&name=app&kind=directory&size=0",
		bytes.NewReader(exported.Body.Bytes()),
	)
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	request.Header.Set("Content-Type", "application/octet-stream")
	imported := httptest.NewRecorder()
	server.ServeHTTP(imported, request)
	if imported.Code != http.StatusCreated {
		t.Fatalf("import status=%d body=%s", imported.Code, imported.Body.String())
	}
	content, err := os.ReadFile(filepath.Join(root, "desktop", "app", "config", "site.conf"))
	if err != nil || string(content) != "enabled" {
		t.Fatalf("imported content=%q err=%v", content, err)
	}
}

type transferErrorReader struct {
	content []byte
	err     error
}

func (r *transferErrorReader) Read(output []byte) (int, error) {
	if len(r.content) > 0 {
		count := copy(output, r.content)
		r.content = r.content[count:]
		return count, nil
	}
	return 0, r.err
}

func TestExactTransferReaderRequiresAuthenticatedEndAfterExpectedBytes(t *testing.T) {
	endError := errors.New("missing authenticated transfer end")
	reader := &exactTransferReader{
		source:    &transferErrorReader{content: []byte("data"), err: endError},
		remaining: 4,
	}
	content, err := io.ReadAll(reader)
	if string(content) != "data" || !errors.Is(err, endError) {
		t.Fatalf("content=%q err=%v", content, err)
	}
}

func TestFileTextReturnsBoundedJSONAndHonorsProtectedPaths(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{
		Root: root, ProtectedVirtual: []string{"/protected"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	server.files = manager
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "protected"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "protected", "secret.txt"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	response := fileRequest(server, http.MethodGet, "/v1/files/text?path=%2Fhello.txt", "")
	if response.Code != http.StatusOK {
		t.Fatalf("text status=%d body=%s", response.Code, response.Body.String())
	}
	var result fileTextResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Content != "hello" || result.ResourceVersion == "" {
		t.Fatalf("text result=%#v err=%v", result, err)
	}
	protected := fileRequest(server, http.MethodGet, "/v1/files/text?path=%2Fprotected%2Fsecret.txt", "")
	if protected.Code != http.StatusForbidden || strings.Contains(protected.Body.String(), "secret") {
		t.Fatalf("protected status=%d body=%s", protected.Code, protected.Body.String())
	}
}

func TestFileEntryReturnsOneProtectedFileManagerEntry(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root, ProtectedVirtual: []string{"/protected"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	server.files = manager
	if err := os.WriteFile(filepath.Join(root, "nginx.conf"), []byte("events {}"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "protected"), 0o700); err != nil {
		t.Fatal(err)
	}

	response := fileRequest(server, http.MethodGet, "/v1/files/entry?path=%2Fnginx.conf", "")
	if response.Code != http.StatusOK {
		t.Fatalf("entry status=%d body=%s", response.Code, response.Body.String())
	}
	var entry contract.FileEntry
	if err := json.Unmarshal(response.Body.Bytes(), &entry); err != nil || entry.Path != "/nginx.conf" || entry.Kind != "file" {
		t.Fatalf("entry=%#v err=%v", entry, err)
	}
	protected := fileRequest(server, http.MethodGet, "/v1/files/entry?path=%2Fprotected", "")
	if protected.Code != http.StatusForbidden {
		t.Fatalf("protected entry status=%d body=%s", protected.Code, protected.Body.String())
	}
	invalid := fileRequest(server, http.MethodGet, "/v1/files/entry?path=%2Fnginx.conf&extra=1", "")
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid query status=%d body=%s", invalid.Code, invalid.Body.String())
	}
}

func TestFileEntriesReturnsBoundedMetadataAndUnavailablePaths(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	server.files = manager
	if err := os.WriteFile(filepath.Join(root, "nginx.conf"), []byte("events {}"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "app"), 0o750); err != nil {
		t.Fatal(err)
	}

	response := fileRequest(server, http.MethodPost, "/v1/files/entries", `{"paths":["/nginx.conf","/missing","/app"]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("entries status=%d body=%s", response.Code, response.Body.String())
	}
	var result contract.FileEntryBatchResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 2 || result.Entries[0].Path != "/nginx.conf" || result.Entries[1].Path != "/app" {
		t.Fatalf("entries=%#v", result.Entries)
	}
	if len(result.Unavailable) != 1 || result.Unavailable[0] != "/missing" {
		t.Fatalf("unavailable=%#v", result.Unavailable)
	}

	duplicate := fileRequest(server, http.MethodPost, "/v1/files/entries", `{"paths":["/app","/app"]}`)
	if duplicate.Code != http.StatusBadRequest {
		t.Fatalf("duplicate status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}
	paths := make([]string, contract.MaxFileEntryBatch+1)
	for index := range paths {
		paths[index] = fmt.Sprintf("/item-%d", index)
	}
	content, err := json.Marshal(contract.FileEntryBatchRequest{Paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	overLimit := fileRequest(server, http.MethodPost, "/v1/files/entries", string(content))
	if overLimit.Code != http.StatusBadRequest {
		t.Fatalf("over-limit status=%d body=%s", overLimit.Code, overLimit.Body.String())
	}
}

func TestFileTailReadsLargeUTF8LogAndHonorsProtectedPaths(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root, ProtectedVirtual: []string{"/protected"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	server.files = manager
	content := strings.Repeat("old line\n", 1000) + "latest error\n"
	if err := os.WriteFile(filepath.Join(root, "nginx.log"), []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "protected"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "protected", "secret.log"), []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	response := fileRequest(server, http.MethodGet, "/v1/files/tail?path=%2Fnginx.log&maxBytes=1024", "")
	if response.Code != http.StatusOK {
		t.Fatalf("tail status=%d body=%s", response.Code, response.Body.String())
	}
	var result fileTailResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || !result.Truncated || !strings.Contains(result.Content, "latest error") || result.ResourceVersion == "" {
		t.Fatalf("tail result=%#v err=%v", result, err)
	}
	protected := fileRequest(server, http.MethodGet, "/v1/files/tail?path=%2Fprotected%2Fsecret.log", "")
	if protected.Code != http.StatusForbidden || strings.Contains(protected.Body.String(), "secret") {
		t.Fatalf("protected tail status=%d body=%s", protected.Code, protected.Body.String())
	}
}

func TestFileEndpointsListWriteUploadAndRejectProtectedPaths(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{
		Root: root, ProtectedVirtual: []string{"/docker/kpanel", "/.kpanel-trash"},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	server.files = manager
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	response := fileRequest(server, http.MethodGet, "/v1/files?path=%2F", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"hello.txt"`) {
		t.Fatalf("list status=%d body=%s", response.Code, response.Body.String())
	}
	var listing contract.FileDirectory
	if err := json.Unmarshal(response.Body.Bytes(), &listing); err != nil {
		t.Fatal(err)
	}
	version := listing.Entries[0].ResourceVersion

	body := `{"content":"new","expectedResourceVersion":"` + version + `"}`
	response = fileRequest(server, http.MethodPut, "/v1/files/content?path=%2Fhello.txt", body)
	if response.Code != http.StatusOK {
		t.Fatalf("write status=%d body=%s", response.Code, response.Body.String())
	}
	response = fileRequest(
		server, http.MethodGet,
		"/v1/files/content?path=%2Fhello.txt&disposition=inline&mode=text", "",
	)
	if response.Code != http.StatusOK || response.Body.String() != "new" {
		t.Fatalf("text read status=%d body=%q", response.Code, response.Body.String())
	}
	head := fileRequest(
		server, http.MethodHead,
		"/v1/files/content?path=%2Fhello.txt&disposition=attachment", "",
	)
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") != "3" {
		t.Fatalf("file HEAD status=%d length=%q body=%q", head.Code, head.Header().Get("Content-Length"), head.Body.String())
	}
	if err := os.WriteFile(filepath.Join(root, "clip.mp4"), []byte("0123456789"), 0644); err != nil {
		t.Fatal(err)
	}
	ranged := fileRequestWithHeaders(
		server, http.MethodGet,
		"/v1/files/content?path=%2Fclip.mp4&disposition=inline", "",
		map[string]string{"Range": "bytes=2-5"},
	)
	if ranged.Code != http.StatusPartialContent || ranged.Body.String() != "2345" {
		t.Fatalf("range status=%d body=%q", ranged.Code, ranged.Body.String())
	}
	if ranged.Header().Get("Accept-Ranges") != "bytes" ||
		ranged.Header().Get("Content-Range") != "bytes 2-5/10" ||
		ranged.Header().Get("Content-Length") != "4" {
		t.Fatalf("range headers=%#v", ranged.Header())
	}
	if strings.Contains(ranged.Header().Get("Content-Security-Policy"), "sandbox") {
		t.Fatalf("media CSP should not sandbox native playback: %q", ranged.Header().Get("Content-Security-Policy"))
	}

	response = fileRequest(server, http.MethodPost, "/v1/files/upload?path=%2F&name=upload.txt", "uploaded")
	if response.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", response.Code, response.Body.String())
	}

	response = fileRequest(server, http.MethodGet, "/v1/files?path=%2Fdocker%2Fkpanel", "")
	if response.Code != http.StatusForbidden {
		t.Fatalf("protected status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestFileTextModeRejectsBinaryContentAndMapsBodyLimit(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	server.files = manager
	if err := os.WriteFile(filepath.Join(root, "binary.txt"), []byte{0xff, 0x00, 0xfe}, 0644); err != nil {
		t.Fatal(err)
	}
	response := fileRequest(
		server, http.MethodGet,
		"/v1/files/content?path=%2Fbinary.txt&disposition=inline&mode=text", "",
	)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("binary text status=%d body=%s", response.Code, response.Body.String())
	}

	limited := httptest.NewRecorder()
	writeFileProblem(limited, "request-id", &http.MaxBytesError{Limit: filemanager.MaxUploadBytes})
	if limited.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("body limit status=%d body=%s", limited.Code, limited.Body.String())
	}
}

func TestFileEndpointsRejectUnknownQueryAndOversizedBatch(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	server.files = manager
	response := fileRequest(server, http.MethodGet, "/v1/files?path=%2F&extra=1", "")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("query status=%d body=%s", response.Code, response.Body.String())
	}
	sources := make([]string, filemanager.MaxBatchItems+1)
	for index := range sources {
		sources[index] = "/file"
	}
	content, _ := json.Marshal(contract.FileActionRequest{Action: "trash", Sources: sources})
	response = fileRequest(server, http.MethodPost, "/v1/files/actions", string(content))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("batch status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestFileListSupportsBoundedSearchAndPagination(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	server.files = manager
	for index := range 510 {
		name := fmt.Sprintf("item-%03d.txt", index)
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "wanted.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	response := fileRequest(
		server,
		http.MethodGet,
		"/v1/files?path=%2F&limit=100&offset=500",
		"",
	)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"offset":500`) {
		t.Fatalf("page status=%d body=%s", response.Code, response.Body.String())
	}
	response = fileRequest(server, http.MethodGet, "/v1/files?path=%2F&search=wanted", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"wanted.txt"`) {
		t.Fatalf("search status=%d body=%s", response.Code, response.Body.String())
	}
	response = fileRequest(server, http.MethodGet, "/v1/files?path=%2F&offset=20000", "")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid offset status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestFileBusyMapsToTooManyRequests(t *testing.T) {
	response := httptest.NewRecorder()
	writeFileProblem(response, "request-id", filemanager.ErrBusy)
	if response.Code != http.StatusTooManyRequests ||
		!strings.Contains(response.Body.String(), `"file_transfer_busy"`) {
		t.Fatalf("busy status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestInvalidArchiveMapsToUnprocessableEntity(t *testing.T) {
	response := httptest.NewRecorder()
	writeFileProblem(response, "request-id", filemanager.ErrInvalidArchive)
	if response.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(response.Body.String(), `"file_archive_invalid"`) {
		t.Fatalf("invalid archive status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestFileActionReturnsMultiStatusForPartialResult(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	server.files = manager
	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	content, _ := json.Marshal(contract.FileActionRequest{
		Action: "trash", Sources: []string{"/keep.txt", "/missing.txt"},
	})
	response := fileRequest(server, http.MethodPost, "/v1/files/actions", string(content))
	if response.Code != http.StatusMultiStatus {
		t.Fatalf("partial action status=%d body=%s", response.Code, response.Body.String())
	}
	var result contract.FileActionResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Succeeded) != 1 || len(result.Failed) != 1 {
		t.Fatalf("partial result=%#v", result)
	}
}

func TestFileArchiveActionsRoundTripThroughAgentAPI(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	server.files = manager
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	compressBody, _ := json.Marshal(contract.FileActionRequest{
		Action: "compress", Sources: []string{"/hello.txt"}, Target: "/",
		Name: "hello.tar.gz", Format: "tar.gz",
	})
	compressResponse := fileRequest(server, http.MethodPost, "/v1/files/actions", string(compressBody))
	if compressResponse.Code != http.StatusOK {
		t.Fatalf("compress status=%d body=%s", compressResponse.Code, compressResponse.Body.String())
	}
	var compressed contract.FileActionResult
	if err := json.Unmarshal(compressResponse.Body.Bytes(), &compressed); err != nil || len(compressed.Succeeded) != 1 {
		t.Fatalf("compress result=%#v err=%v", compressed, err)
	}
	extractBody, _ := json.Marshal(contract.FileActionRequest{
		Action: "extract", Sources: []string{"/hello.tar.gz"}, Target: "/",
		Name: "restored", Format: "tar.gz",
		ExpectedResourceVersion: compressed.Succeeded[0].ResourceVersion,
	})
	extractResponse := fileRequest(server, http.MethodPost, "/v1/files/actions", string(extractBody))
	if extractResponse.Code != http.StatusOK {
		t.Fatalf("extract status=%d body=%s", extractResponse.Code, extractResponse.Body.String())
	}
	content, err := os.ReadFile(filepath.Join(root, "restored", "hello.txt"))
	if err != nil || string(content) != "hello" {
		t.Fatalf("restored content=%q err=%v", content, err)
	}
}

func TestFileTrashEndpointSupportsRestoreAndPermanentDelete(t *testing.T) {
	server := testServer(t)
	root := t.TempDir()
	manager, err := filemanager.New(filemanager.Config{Root: root, TrashVirtual: "/state/file-trash"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	server.files = manager
	for _, name := range []string{"restore.txt", "delete.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
		content, _ := json.Marshal(contract.FileActionRequest{Action: "trash", Sources: []string{"/" + name}})
		response := fileRequest(server, http.MethodPost, "/v1/files/actions", string(content))
		if response.Code != http.StatusOK {
			t.Fatalf("trash %s status=%d body=%s", name, response.Code, response.Body.String())
		}
	}
	response := fileRequest(server, http.MethodGet, "/v1/files/trash", "")
	if response.Code != http.StatusOK {
		t.Fatalf("trash list status=%d body=%s", response.Code, response.Body.String())
	}
	var trash contract.FileTrashDirectory
	if err := json.Unmarshal(response.Body.Bytes(), &trash); err != nil || trash.Total != 2 {
		t.Fatalf("trash list=%#v err=%v", trash, err)
	}
	for _, entry := range trash.Entries {
		action := "trash_delete"
		if entry.Name == "restore.txt" {
			action = "trash_restore"
		}
		content, _ := json.Marshal(contract.FileActionRequest{
			Action: action, TrashIDs: []string{entry.ID},
			ExpectedResourceVersions: map[string]string{entry.ID: entry.ResourceVersion},
		})
		response = fileRequest(server, http.MethodPost, "/v1/files/actions", string(content))
		if response.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", action, response.Code, response.Body.String())
		}
	}
	if content, err := os.ReadFile(filepath.Join(root, "restore.txt")); err != nil || string(content) != "restore.txt" {
		t.Fatalf("restored content=%q err=%v", content, err)
	}
	if _, err := os.Stat(filepath.Join(root, "delete.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("permanently deleted file remains: %v", err)
	}
}

func fileRequest(server *Server, method, target, body string) *httptest.ResponseRecorder {
	return fileRequestWithHeaders(server, method, target, body, nil)
}

func fileRequestWithHeaders(server *Server, method, target, body string, headers map[string]string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+strings.Repeat("x", 32))
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	switch target {
	case "/v1/files/upload?path=%2F&name=upload.txt":
		request.Header.Set("Content-Type", "application/octet-stream")
	default:
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
	}
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	return response
}
