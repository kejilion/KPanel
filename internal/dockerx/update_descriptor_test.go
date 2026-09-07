package dockerx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestImageUpdateContainerdDescriptorAfterTagMoves(t *testing.T) {
	const index = "application/vnd.oci.image.index.v1+json"
	const manifest = "application/vnd.oci.image.manifest.v1+json"
	old := "sha256:" + strings.Repeat("b", 64)
	newDigest := "sha256:" + strings.Repeat("c", 64)
	for _, tc := range []struct {
		name, kind, remoteKind, want                             string
		same, wrongID, wrongRepo, malformed, refuseOld, withRepo bool
	}{
		{name: "untagged index current", kind: index, remoteKind: index, same: true, want: "current"},
		{name: "untagged index changed", kind: index, remoteKind: index, want: "available"},
		{name: "untagged manifest changed", kind: manifest, remoteKind: manifest, want: "available"},
		{name: "inspect identity mismatch", kind: index, remoteKind: index, wrongID: true},
		{name: "different repository is not overridden", kind: index, remoteKind: index, wrongRepo: true},
		{name: "config is not a manifest", kind: "application/vnd.oci.image.config.v1+json", remoteKind: index},
		{name: "missing kind", remoteKind: index},
		{name: "invalid digest", kind: index, remoteKind: index, malformed: true},
		{name: "manifest cannot compare with index", kind: manifest, remoteKind: index},
		{name: "same digest inconsistent kind", kind: manifest, remoteKind: index, same: true},
		{name: "old index removed uses local identity", kind: index, remoteKind: index, refuseOld: true, want: "available"},
		{name: "tagged old index removed uses local identity", kind: index, remoteKind: index, refuseOld: true, withRepo: true, want: "available"},
		{name: "tagged wrong ID cannot bypass old lookup", kind: index, remoteKind: index, refuseOld: true, withRepo: true, wrongID: true},
		{name: "old manifest removed still needs platform identity", kind: manifest, remoteKind: manifest, refuseOld: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := strings.Repeat("a", 64)
			raw := managedInspect(id, "2026-09-06T00:00:00Z", 0)
			raw.Image, raw.Config.Image, raw.Config.Labels = old, "example:latest", nil
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("unexpected write: %s", r.Method)
				}
				switch {
				case strings.HasPrefix(r.URL.Path, "/containers/"):
					_ = json.NewEncoder(w).Encode(raw)
				case strings.HasPrefix(r.URL.Path, "/images/"):
					if !strings.Contains(r.URL.Path, raw.Image) {
						t.Errorf("inspected mutable tag: %s", r.URL.Path)
					}
					imageID, digest := old, old
					if tc.wrongID {
						imageID = newDigest
					}
					if tc.malformed {
						digest = "sha256:not-a-digest"
					}
					repos := []string{}
					if tc.withRepo {
						repos = []string{"example@" + old}
					}
					if tc.wrongRepo {
						repos = []string{"other/image@" + old}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"Id": imageID, "RepoDigests": repos, "Descriptor": updateDescriptor{Digest: digest, MediaType: tc.kind}})
				case strings.HasPrefix(r.URL.Path, "/distribution/"):
					digest, kind := newDigest, tc.remoteKind
					if tc.same {
						digest = old
					}
					if strings.Contains(r.URL.Path, "@") {
						if tc.kind == index && !tc.wrongID && !tc.wrongRepo && !tc.malformed {
							t.Error("trusted local index should not require an old registry manifest")
						}
						if tc.refuseOld {
							http.Error(w, "not found", http.StatusNotFound)
							return
						}
						digest, kind = old, tc.kind
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"Descriptor": updateDescriptor{Digest: digest, MediaType: kind}, "Platforms": []map[string]string{{"os": "linux", "architecture": "amd64"}}})
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client := testHTTPClient(server)
			result, err := client.CheckContainerImageUpdate(context.Background(), id, client.summaryFromInspect(raw).ResourceVersion)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("uncertain identity succeeded: %+v", result)
				}
				return
			}
			if err != nil || result.Status != tc.want || result.LocalDigest != old {
				t.Fatalf("result=%+v error=%v want=%s", result, err, tc.want)
			}
		})
	}
}
