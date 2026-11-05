package agent

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/sites"
)

func TestSiteAccessPasswordReadReturnsNamesNotHashes(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"conf.d", "html", "certs"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	config := `server { server_name example.com; root /var/www/html/example.com; auth_basic "Private"; auth_basic_user_file /etc/nginx/conf.d/.users; }`
	if err := os.WriteFile(filepath.Join(root, "conf.d", "example.com.conf"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "conf.d", ".users"), []byte("alice:$apr1$privatehash\n"), 0600); err != nil {
		t.Fatal(err)
	}
	d := sites.NewDiscoverer(root)
	all, err := d.Discover()
	if err != nil || len(all) != 1 {
		t.Fatalf("site discovery: %#v, %v", all, err)
	}
	s := &Server{sitesManager: sites.NewManager(root, d, nil)}
	request := httptest.NewRequest(http.MethodGet, "/v1/sites/"+all[0].ID+"/access-passwords", nil)
	recorder := httptest.NewRecorder()
	s.siteOperation(recorder, request, "request")
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "alice") || strings.Contains(recorder.Body.String(), "privatehash") {
		t.Fatalf("unexpected response %d: %s", recorder.Code, recorder.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, request.URL.Path, nil)
	recorder = httptest.NewRecorder()
	s.siteOperation(recorder, request, "request")
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected method status: %d", recorder.Code)
	}
}
