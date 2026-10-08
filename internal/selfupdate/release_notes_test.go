package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func releaseNotesRepository(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func readReleaseNotesFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > maxReleaseResponse {
		t.Fatal("rendered release notes exceed the runtime response limit")
	}
	return string(body)
}

func renderChangelogRelease(t *testing.T, root, version string) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), "notes.md")
	command := exec.Command("node", "scripts/run-repo-bash.mjs", "scripts/render-release-notes.sh",
		version, officialImageName, "sha256:"+strings.Repeat("a", 64), filepath.ToSlash(output))
	command.Dir = root
	if result, err := command.CombinedOutput(); err != nil {
		t.Fatalf("render actual Changelog v%s: %v\n%s", version, err, result)
	}
	return readReleaseNotesFile(t, output)
}

// This is also the publication gate: check the exact generated file with the
// production parser, including every category beyond the displayed item limits.
func TestRenderedReleaseNotesForPublication(t *testing.T) {
	root := releaseNotesRepository(t)
	version := os.Getenv("KPANEL_RELEASE_NOTES_VERSION")
	if version == "" {
		version = strings.TrimSpace(readReleaseNotesFile(t, filepath.Join(root, "VERSION")))
	}
	var body string
	if path := os.Getenv("KPANEL_RELEASE_NOTES_FILE"); path != "" {
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		body = readReleaseNotesFile(t, path)
	} else {
		body = renderChangelogRelease(t, root, version)
	}
	if !strings.HasPrefix(body, "## KPanel "+version+"\n") {
		t.Fatal("rendered release notes do not match the publication version")
	}
	wantNotes := make([]ReleaseNote, 0, maxReleaseNotes)
	wantUpgrade := make([]string, 0, maxUpgradeNotes)
	inUpdates, metadata := false, false
	kind := ""
	sections := 0
	for _, rawLine := range strings.Split(body, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
		if heading, ok := markdownHeading(line); ok {
			if strings.HasPrefix(line, "### ") || strings.HasPrefix(line, "## ") {
				inUpdates = isReleaseNotesHeading(heading)
				kind, metadata = "", false
				if inUpdates {
					sections++
				}
				continue
			}
			if !inUpdates {
				continue
			}
			kind, metadata = releaseNoteKind(heading), false
			if isUpgradeNotesHeading(heading) {
				kind = "upgrade"
			} else if heading == "发布边界" || heading == "测试范围" {
				metadata = true
			} else if kind == "" {
				t.Fatalf("unsupported rendered release category: %s", heading)
			}
			continue
		}
		text, ok := markdownBulletText(line)
		if !inUpdates || !ok || metadata {
			continue
		}
		text = plainReleaseNote(text)
		if text == "" || kind == "" {
			t.Fatal("release update item is empty or has no supported category")
		}
		if kind == "upgrade" {
			if len(wantUpgrade) < maxUpgradeNotes {
				wantUpgrade = append(wantUpgrade, text)
			}
		} else if len(wantNotes) < maxReleaseNotes {
			wantNotes = append(wantNotes, ReleaseNote{Kind: kind, Text: text})
		}
	}
	if sections != 1 || len(wantNotes) == 0 {
		t.Fatal("publication requires one update section with at least one readable user-visible item")
	}
	notes, upgrade := parseReleaseNotes(body)
	if len(notes) != len(wantNotes) || len(upgrade) != len(wantUpgrade) {
		t.Fatalf("runtime lost release items: notes=%d/%d upgrade=%d/%d", len(notes), len(wantNotes), len(upgrade), len(wantUpgrade))
	}
	for index, want := range wantNotes {
		if notes[index] != want {
			t.Fatalf("runtime release item %d = %#v, want %#v", index, notes[index], want)
		}
	}
	for index, want := range wantUpgrade {
		if upgrade[index] != want {
			t.Fatalf("runtime upgrade item %d = %q, want %q", index, upgrade[index], want)
		}
	}
	t.Logf("publication release notes: version=%s updates=%d upgrade=%d", version, len(notes), len(upgrade))
}

func TestPublishedChangelogFormatsStayReadable(t *testing.T) {
	root := releaseNotesRepository(t)
	data, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	changelog := string(data)
	for _, version := range []string{"1.24.0", "1.25.0-rc.11", "1.25.0-rc.12"} {
		t.Run(version, func(t *testing.T) {
			start := strings.Index(changelog, "## ["+version+"]")
			if start < 0 {
				t.Fatal("published Changelog section is missing")
			}
			section := strings.SplitN(changelog[start:], "\n", 2)[1]
			if end := strings.Index(section, "\n## ["); end >= 0 {
				section = section[:end]
			}
			// Rebuild the original published headings, before normalization was added.
			section = strings.ReplaceAll(section, "### ", "#### ")
			preview := strings.Contains(version, "-rc.")
			label := "生产"
			if preview {
				label = "预览"
			}
			body := "### 版本更新内容\n" + section + "\n### 发布产物与完整性\n" +
				fmt.Sprintf("- %s镜像：`%s@sha256:%s`\n", label, officialImageName, strings.Repeat("a", 64))
			payload, err := json.Marshal(map[string]any{
				"tag_name": "v" + version, "html_url": officialReleaseURL(version), "body": body,
				"draft": false, "prerelease": preview, "published_at": "2026-10-08T00:00:00Z",
			})
			if err != nil {
				t.Fatal(err)
			}
			source := sourceWithResponse(func(request *http.Request) (*http.Response, error) {
				response := string(payload)
				if preview {
					response = "[" + response + "]"
				}
				return releaseResponse(request, response), nil
			})
			if preview {
				source.channel = ChannelPreview
			}
			release, err := source.Latest(context.Background())
			if err != nil || len(release.Notes) < 5 {
				t.Fatalf("published source notes=%#v err=%v", release.Notes, err)
			}
			if version == "1.25.0-rc.12" && len(release.UpgradeNotes) != 3 {
				t.Fatalf("RC12 upgrade warnings were lost: %#v", release.UpgradeNotes)
			}
			if strings.Contains(release.Notes[0].Text, "scriptLinkageState") {
				t.Fatal("publication metadata leaked into the user summary")
			}
		})
	}
}
