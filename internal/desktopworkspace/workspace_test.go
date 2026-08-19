package desktopworkspace

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const testShortcutID = "0123456789abcdef0123456789abcdef"

func TestOpenCreatesPrivateWorkspaceAndPersistsReplacement(t *testing.T) {
	root := filepath.Join(t.TempDir(), "desktop-workspace")
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	initial := store.Workspace()
	if !initial.Available || initial.SchemaVersion != SchemaVersion || !ValidResourceVersion(initial.ResourceVersion) {
		t.Fatalf("unexpected initial workspace: %#v", initial)
	}
	if initial.HiddenEntryKeys == nil || initial.Positions == nil || initial.Labels == nil || initial.Shortcuts == nil {
		t.Fatalf("default collections must be non-nil: %#v", initial)
	}
	if runtime.GOOS != "windows" {
		assertMode(t, root, 0o700)
		assertMode(t, filepath.Join(root, "icons"), 0o700)
		assertMode(t, filepath.Join(root, "workspace.json"), 0o600)
	}

	createdAt := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return createdAt }
	input := validReplaceInput(initial.ResourceVersion)
	saved, err := store.Replace(input)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ResourceVersion == initial.ResourceVersion || len(saved.Shortcuts) != 1 {
		t.Fatalf("replacement was not committed: %#v", saved)
	}
	if saved.Shortcuts[0].CreatedAt != createdAt || saved.Shortcuts[0].UpdatedAt != createdAt {
		t.Fatalf("unexpected shortcut timestamps: %#v", saved.Shortcuts[0])
	}

	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	restored := reopened.Workspace()
	if restored.ResourceVersion != saved.ResourceVersion || len(restored.Shortcuts) != 1 || restored.Shortcuts[0].URL != input.Shortcuts[0].URL {
		t.Fatalf("workspace did not survive restart: %#v", restored)
	}
	if _, err := reopened.Replace(input); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale resourceVersion error = %v, want conflict", err)
	}
	input.ExpectedResourceVersion = "invalid"
	if _, err := reopened.Replace(input); validationField(err) != "expectedResourceVersion" {
		t.Fatalf("invalid resourceVersion error = %v", err)
	}
}

func TestReplaceValidatesWorkspaceContract(t *testing.T) {
	store := openTestStore(t)
	base := store.Workspace()
	tests := []struct {
		name   string
		input  func() ReplaceInput
		field  string
		detail string
	}{
		{
			name: "hidden navigation entry",
			input: func() ReplaceInput {
				input := validReplaceInput(base.ResourceVersion)
				input.HiddenEntryKeys = []string{"nav:/overview"}
				return input
			},
			field: "hiddenEntryKeys",
		},
		{
			name: "negative horizontal position",
			input: func() ReplaceInput {
				input := validReplaceInput(base.ResourceVersion)
				input.Positions["nav:/overview"] = Position{X: -0.01, Y: 1}
				return input
			},
			field: "positions",
		},
		{
			name: "horizontal position exceeds normalized range",
			input: func() ReplaceInput {
				input := validReplaceInput(base.ResourceVersion)
				input.Positions["nav:/overview"] = Position{X: 1.01, Y: 1}
				return input
			},
			field: "positions",
		},
		{
			name: "negative paged vertical position",
			input: func() ReplaceInput {
				input := validReplaceInput(base.ResourceVersion)
				input.Positions["nav:/overview"] = Position{X: 0.5, Y: -0.01}
				return input
			},
			field:  "positions",
			detail: "paged y between 0 and 512",
		},
		{
			name: "paged vertical position exceeds limit",
			input: func() ReplaceInput {
				input := validReplaceInput(base.ResourceVersion)
				input.Positions["nav:/overview"] = Position{X: 0.5, Y: MaxPositions + 0.01}
				return input
			},
			field: "positions",
		},
		{
			name: "missing shortcut position",
			input: func() ReplaceInput {
				input := validReplaceInput(base.ResourceVersion)
				input.Positions["shortcut:ffffffffffffffffffffffffffffffff"] = Position{X: 0, Y: 0}
				return input
			},
			field: "positions",
		},
		{
			name: "non-site label",
			input: func() ReplaceInput {
				input := validReplaceInput(base.ResourceVersion)
				input.Labels = map[string]string{"app:builtin-1": "Name"}
				return input
			},
			field: "labels",
		},
		{
			name: "long name",
			input: func() ReplaceInput {
				input := validReplaceInput(base.ResourceVersion)
				input.Shortcuts[0].Name = strings.Repeat("界", MaxShortcutNameRunes+1)
				return input
			},
			field: "shortcuts.name",
		},
		{
			name: "long description",
			input: func() ReplaceInput {
				input := validReplaceInput(base.ResourceVersion)
				input.Shortcuts[0].Description = strings.Repeat("界", MaxDescriptionRunes+1)
				return input
			},
			field: "shortcuts.description",
		},
		{
			name: "URL credentials",
			input: func() ReplaceInput {
				input := validReplaceInput(base.ResourceVersion)
				input.Shortcuts[0].URL = "https://user:password@example.com/"
				return input
			},
			field: "shortcuts.url",
		},
		{
			name: "relative URL",
			input: func() ReplaceInput {
				input := validReplaceInput(base.ResourceVersion)
				input.Shortcuts[0].URL = "/relative"
				return input
			},
			field: "shortcuts.url",
		},
		{
			name: "unknown target type",
			input: func() ReplaceInput {
				input := validReplaceInput(base.ResourceVersion)
				input.Shortcuts[0].TargetType = "command"
				return input
			},
			field: "shortcuts.targetType",
		},
		{
			name: "relative file path",
			input: func() ReplaceInput {
				input := validReplaceInput(base.ResourceVersion)
				input.Shortcuts[0] = ShortcutInput{
					ID: testShortcutID, Name: "Config", TargetType: ShortcutTargetFile, Path: "etc/nginx.conf",
				}
				return input
			},
			field: "shortcuts.path",
		},
		{
			name: "noncanonical directory path",
			input: func() ReplaceInput {
				input := validReplaceInput(base.ResourceVersion)
				input.Shortcuts[0] = ShortcutInput{
					ID: testShortcutID, Name: "Config", TargetType: ShortcutTargetDirectory, Path: "/etc//nginx",
				}
				return input
			},
			field: "shortcuts.path",
		},
		{
			name: "file root path",
			input: func() ReplaceInput {
				input := validReplaceInput(base.ResourceVersion)
				input.Shortcuts[0] = ShortcutInput{
					ID: testShortcutID, Name: "Root", TargetType: ShortcutTargetFile, Path: "/",
				}
				return input
			},
			field: "shortcuts.path",
		},
		{
			name: "file target with URL",
			input: func() ReplaceInput {
				input := validReplaceInput(base.ResourceVersion)
				input.Shortcuts[0] = ShortcutInput{
					ID: testShortcutID, Name: "Config", TargetType: ShortcutTargetFile,
					Path: "/etc/nginx.conf", URL: "https://example.com/",
				}
				return input
			},
			field: "shortcuts.url",
		},
		{
			name: "duplicate shortcut",
			input: func() ReplaceInput {
				input := validReplaceInput(base.ResourceVersion)
				input.Shortcuts = append(input.Shortcuts, input.Shortcuts[0])
				return input
			},
			field: "shortcuts",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := store.Replace(test.input())
			if validationField(err) != test.field {
				t.Fatalf("validation field = %q from %v, want %q", validationField(err), err, test.field)
			}
			if test.detail != "" && !strings.Contains(err.Error(), test.detail) {
				t.Fatalf("validation error = %q, want detail %q", err, test.detail)
			}
		})
	}
}

func TestFileAndDirectoryShortcutTargetsPersist(t *testing.T) {
	store := openTestStore(t)
	version := store.Workspace().ResourceVersion
	input := ReplaceInput{
		ExpectedResourceVersion: version,
		Shortcuts: []ShortcutInput{
			{ID: testShortcutID, Name: "nginx.conf", TargetType: ShortcutTargetFile, Path: "/etc/nginx/nginx.conf"},
			{ID: "ffffffffffffffffffffffffffffffff", Name: "Web", TargetType: ShortcutTargetDirectory, Path: "/home/web"},
		},
	}
	saved, err := store.Replace(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Shortcuts) != 2 || saved.Shortcuts[0].TargetType != ShortcutTargetFile ||
		saved.Shortcuts[0].Path != "/etc/nginx/nginx.conf" || saved.Shortcuts[0].URL != "" ||
		saved.Shortcuts[1].TargetType != ShortcutTargetDirectory {
		t.Fatalf("unexpected file shortcuts: %#v", saved.Shortcuts)
	}
	reopened, err := Open(store.root)
	if err != nil {
		t.Fatal(err)
	}
	if restored := reopened.Workspace(); restored.ResourceVersion != saved.ResourceVersion || restored.Shortcuts[1].Path != "/home/web" {
		t.Fatalf("file shortcut targets did not survive restart: %#v", restored)
	}
}

func TestOpenMigratesVersionOneURLShortcuts(t *testing.T) {
	root := filepath.Join(t.TempDir(), "desktop-workspace")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	created := "2026-08-14T08:00:00Z"
	legacy := fmt.Sprintf(`{"schemaVersion":1,"hiddenEntryKeys":[],"positions":{},"labels":{},"shortcuts":[{"id":%q,"name":"Docs","description":"","url":"https://example.com/","createdAt":%q,"updatedAt":%q}]}`, testShortcutID, created, created)
	workspacePath := filepath.Join(root, "workspace.json")
	if err := os.WriteFile(workspacePath, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	migrated := store.Workspace()
	if !migrated.Available || migrated.SchemaVersion != SchemaVersion || len(migrated.Shortcuts) != 1 ||
		migrated.Shortcuts[0].TargetType != ShortcutTargetURL {
		t.Fatalf("legacy workspace was not migrated in memory: %#v", migrated)
	}
	input := ReplaceInput{
		ExpectedResourceVersion: migrated.ResourceVersion,
		Shortcuts: []ShortcutInput{{
			ID: testShortcutID, Name: "Docs", TargetType: ShortcutTargetURL, URL: "https://example.com/",
		}},
	}
	if _, err := store.Replace(input); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(workspacePath)
	if err != nil || !bytes.Contains(data, []byte(`"schemaVersion": 2`)) || !bytes.Contains(data, []byte(`"targetType": "url"`)) {
		t.Fatalf("migrated workspace was not persisted as v2: %s, %v", data, err)
	}
}

func TestReplaceAllowsPagedVerticalPosition(t *testing.T) {
	store := openTestStore(t)
	input := validReplaceInput(store.Workspace().ResourceVersion)
	input.Positions["nav:/overview"] = Position{X: 1, Y: MaxPositions}

	saved, err := store.Replace(input)
	if err != nil {
		t.Fatalf("paged vertical position rejected: %v", err)
	}
	if got := saved.Positions["nav:/overview"]; got != (Position{X: 1, Y: MaxPositions}) {
		t.Fatalf("paged vertical position = %#v", got)
	}
}

func TestReplaceEnforcesEntryAndMetadataLimits(t *testing.T) {
	store := openTestStore(t)
	version := store.Workspace().ResourceVersion

	tooManyShortcuts := ReplaceInput{ExpectedResourceVersion: version}
	for index := 0; index < MaxShortcuts+1; index++ {
		tooManyShortcuts.Shortcuts = append(tooManyShortcuts.Shortcuts, ShortcutInput{
			ID: fmt.Sprintf("%032x", index), Name: "Shortcut", URL: "https://example.com/",
		})
	}
	if _, err := store.Replace(tooManyShortcuts); validationField(err) != "shortcuts" {
		t.Fatalf("shortcut limit error = %v", err)
	}

	tooManyPositions := ReplaceInput{ExpectedResourceVersion: version, Positions: make(map[string]Position)}
	for index := 0; index < MaxPositions+1; index++ {
		tooManyPositions.Positions["site:"+fmt.Sprintf("%032x", index)] = Position{X: 0.5, Y: 0.5}
	}
	if _, err := store.Replace(tooManyPositions); validationField(err) != "positions" {
		t.Fatalf("position limit error = %v", err)
	}

	tooManyHidden := ReplaceInput{ExpectedResourceVersion: version}
	for index := 0; index < MaxHiddenEntryKeys+1; index++ {
		tooManyHidden.HiddenEntryKeys = append(tooManyHidden.HiddenEntryKeys, "site:"+fmt.Sprintf("%032x", index))
	}
	if _, err := store.Replace(tooManyHidden); validationField(err) != "hiddenEntryKeys" {
		t.Fatalf("hidden limit error = %v", err)
	}

	tooLarge := ReplaceInput{
		ExpectedResourceVersion: version,
		Labels:                  make(map[string]string, 1024),
	}
	for index := 0; index < 1024; index++ {
		tooLarge.Labels["site:"+fmt.Sprintf("%032x", index)] = strings.Repeat("界", MaxShortcutNameRunes)
	}
	for index := 0; index < MaxShortcuts; index++ {
		tooLarge.Shortcuts = append(tooLarge.Shortcuts, ShortcutInput{
			ID: fmt.Sprintf("%032x", index), Name: strings.Repeat("界", MaxShortcutNameRunes),
			Description: strings.Repeat("界", MaxDescriptionRunes),
			URL:         "https://example.com/" + strings.Repeat("a", MaxURLBytes-len("https://example.com/")),
		})
	}
	if _, err := store.Replace(tooLarge); validationField(err) != "workspace" {
		t.Fatalf("metadata limit error = %v", err)
	}
}

func TestCorruptWorkspaceDegradesReadsAndRejectsWritesWithoutOverwrite(t *testing.T) {
	for _, test := range []struct {
		name string
		data []byte
	}{
		{name: "malformed", data: []byte("{not-json")},
		{name: "unknown schema", data: []byte(`{"schemaVersion":99,"hiddenEntryKeys":[],"positions":{},"labels":{},"shortcuts":[]}`)},
		{name: "oversized", data: bytes.Repeat([]byte("x"), MaxWorkspaceBytes+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "desktop-workspace")
			if _, err := Open(root); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "workspace.json")
			if err := os.WriteFile(path, test.data, 0o600); err != nil {
				t.Fatal(err)
			}
			store, err := Open(root)
			if err != nil {
				t.Fatalf("corrupt metadata must not fail initialization: %v", err)
			}
			workspace := store.Workspace()
			if workspace.Available || workspace.Warning != unavailableWarning || len(workspace.Shortcuts) != 0 {
				t.Fatalf("unexpected degraded response: %#v", workspace)
			}
			before, _ := os.ReadFile(path)
			_, err = store.Replace(ReplaceInput{ExpectedResourceVersion: workspace.ResourceVersion})
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("corrupt store write error = %v, want unavailable", err)
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("unavailable store overwrote corrupt workspace")
			}
		})
	}
}

func TestPersistenceFailureLeavesInMemoryWorkspaceUnchanged(t *testing.T) {
	store := openTestStore(t)
	initial := store.Workspace()
	store.writeAtomic = func(string, string, []byte) error { return errors.New("injected write failure") }
	if _, err := store.Replace(validReplaceInput(initial.ResourceVersion)); err == nil {
		t.Fatal("expected injected persistence failure")
	}
	current := store.Workspace()
	if current.ResourceVersion != initial.ResourceVersion || len(current.Shortcuts) != 0 {
		t.Fatalf("failed write changed memory state: %#v", current)
	}
}

func TestShortcutIconLifecycleValidationAndGarbageCollection(t *testing.T) {
	store := openTestStore(t)
	workspace, err := store.Replace(validReplaceInput(store.Workspace().ResourceVersion))
	if err != nil {
		t.Fatal(err)
	}
	validPNG := encodedPNG(t, 32, 32)
	icon, err := store.PutIcon(testShortcutID, "image/png", validPNG)
	if err != nil {
		t.Fatal(err)
	}
	if len(icon.Version) != 64 || icon.ContentType != "image/png" {
		t.Fatalf("unexpected stored icon: %#v", icon)
	}
	read, err := store.Icon(testShortcutID)
	if err != nil || !bytes.Equal(read.Data, validPNG) || read.Version != icon.Version {
		t.Fatalf("read icon = %#v, %v", read, err)
	}
	if store.Workspace().Shortcuts[0].IconVersion != icon.Version {
		t.Fatal("workspace response did not expose icon version")
	}
	if _, err := store.PutIcon(testShortcutID, "image/jpeg", validPNG); !errors.Is(err, ErrIconInvalid) {
		t.Fatalf("mismatched MIME error = %v", err)
	}
	if _, err := store.PutIcon(testShortcutID, "image/svg+xml", []byte(`<svg/>`)); !errors.Is(err, ErrIconInvalid) {
		t.Fatalf("SVG error = %v", err)
	}
	validJPEG := encodedJPEG(t, 32, 32)
	truncatedImages := []struct {
		name        string
		contentType string
		format      string
		data        []byte
	}{
		{name: "PNG", contentType: "image/png", format: "png", data: validPNG},
		{name: "JPEG", contentType: "image/jpeg", format: "jpeg", data: validJPEG},
	}
	for _, test := range truncatedImages {
		t.Run("truncated "+test.name, func(t *testing.T) {
			truncated := imageConfigOnlyPrefix(t, test.data, test.format)
			if _, err := store.PutIcon(testShortcutID, test.contentType, truncated); !errors.Is(err, ErrIconInvalid) {
				t.Fatalf("truncated %s error = %v", test.name, err)
			}
		})
	}
	jpegIcon, err := store.PutIcon(testShortcutID, "image/jpeg", validJPEG)
	if err != nil {
		t.Fatalf("valid JPEG rejected: %v", err)
	}
	if jpegIcon.ContentType != "image/jpeg" || len(jpegIcon.Version) != 64 {
		t.Fatalf("unexpected JPEG result: %#v", jpegIcon)
	}
	if _, err := store.PutIcon(testShortcutID, "image/webp", webPExtendedHeaderOnly()); !errors.Is(err, ErrIconInvalid) {
		t.Fatalf("header-only WebP error = %v", err)
	}
	validWebP := encodedWebP(t)
	webPIcon, err := store.PutIcon(testShortcutID, "image/webp", validWebP)
	if err != nil {
		t.Fatalf("valid WebP rejected: %v", err)
	}
	if webPIcon.ContentType != "image/webp" || len(webPIcon.Version) != 64 {
		t.Fatalf("unexpected WebP result: %#v", webPIcon)
	}
	if _, err := store.PutIcon(testShortcutID, "image/png", encodedPNG(t, MaxIconDimension+1, 1)); !errors.Is(err, ErrIconInvalid) {
		t.Fatalf("oversized dimensions error = %v", err)
	}
	if _, err := store.PutIcon(testShortcutID, "image/png", bytes.Repeat([]byte("x"), MaxIconBytes+1)); !errors.Is(err, ErrIconInvalid) {
		t.Fatalf("oversized payload error = %v", err)
	}

	remove := ReplaceInput{
		ExpectedResourceVersion: workspace.ResourceVersion,
		HiddenEntryKeys:         workspace.HiddenEntryKeys,
		Positions:               map[string]Position{"nav:/overview": {X: 0.1, Y: 0.1}},
		Labels:                  workspace.Labels,
		Shortcuts:               []ShortcutInput{},
	}
	if _, err := store.Replace(remove); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.iconPath(testShortcutID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed shortcut icon still exists: %v", err)
	}

	orphanID := "ffffffffffffffffffffffffffffffff"
	if err := os.WriteFile(filepath.Join(store.iconsDir, orphanID+".icon"), validPNG, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(store.root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.iconsDir, orphanID+".icon")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan icon was not collected: %v", err)
	}
}

func TestConcurrentReplacementWithSameVersionHasOneWinner(t *testing.T) {
	store := openTestStore(t)
	version := store.Workspace().ResourceVersion
	inputs := []ReplaceInput{
		{ExpectedResourceVersion: version, Positions: map[string]Position{"nav:/overview": {X: 0.1, Y: 0.1}}},
		{ExpectedResourceVersion: version, Positions: map[string]Position{"nav:/overview": {X: 0.8, Y: 0.8}}},
	}
	var wait sync.WaitGroup
	errorsByInput := make([]error, len(inputs))
	for index := range inputs {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, errorsByInput[index] = store.Replace(inputs[index])
		}(index)
	}
	wait.Wait()
	successes, conflicts := 0, 0
	for _, err := range errorsByInput {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes=%d conflicts=%d", successes, conflicts)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "desktop-workspace"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func validReplaceInput(version string) ReplaceInput {
	return ReplaceInput{
		ExpectedResourceVersion: version,
		HiddenEntryKeys:         []string{"app:builtin-1", "site:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		Positions: map[string]Position{
			"nav:/overview":                         {X: 0.1, Y: 0.2},
			"app:builtin-1":                         {X: 0.2, Y: 0.3},
			"site:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": {X: 0.3, Y: 0.4},
			"shortcut:" + testShortcutID:            {X: 0.4, Y: 0.5},
		},
		Labels: map[string]string{"site:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": "示例网站"},
		Shortcuts: []ShortcutInput{{
			ID: testShortcutID, Name: "控制台", Description: "本地管理入口",
			TargetType: ShortcutTargetURL, URL: "https://example.com/admin",
		}},
	}
}

func encodedPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	value := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			value.SetNRGBA(x, y, color.NRGBA{R: 40, G: 160, B: 220, A: 255})
		}
	}
	var output bytes.Buffer
	if err := png.Encode(&output, value); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func encodedJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	value := image.NewNRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			value.SetNRGBA(x, y, color.NRGBA{R: 40, G: 160, B: 220, A: 255})
		}
	}
	var output bytes.Buffer
	if err := jpeg.Encode(&output, value, nil); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func imageConfigOnlyPrefix(t *testing.T, data []byte, expectedFormat string) []byte {
	t.Helper()
	for length := 1; length < len(data); length++ {
		_, format, configErr := image.DecodeConfig(bytes.NewReader(data[:length]))
		if configErr != nil || format != expectedFormat {
			continue
		}
		if _, decodedFormat, decodeErr := image.Decode(bytes.NewReader(data[:length])); decodeErr != nil || decodedFormat != expectedFormat {
			return append([]byte(nil), data[:length]...)
		}
	}
	t.Fatalf("could not construct truncated %s with a readable configuration", expectedFormat)
	return nil
}

func webPExtendedHeaderOnly() []byte {
	return []byte{
		'R', 'I', 'F', 'F', 22, 0, 0, 0,
		'W', 'E', 'B', 'P', 'V', 'P', '8', 'X',
		10, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	}
}

func encodedWebP(t *testing.T) []byte {
	t.Helper()
	const value = "UklGRuYBAABXRUJQVlA4INoBAACwEQCdASqAAIAAPjEYikKiIaEVDZ1EIAMEsYBpcxr/ldjJ7TTTfNtnz/oH+A9gGcA/gH96/" +
		"YzfQOAz6XDyK9WXDLQb6QvFILrj5wHXz3hKCZnqb0FI9B+oEIVtzxuGnsTfC9YoV5+CHdc99m22g0MQROjt4970E1RDO2q4j" +
		"M/TSPZ7wR7+MBq6loHdS7xvQR0SiUSiTwAA/v/YoqY+31u2qXGJyh1JBhg4zb8eGEcC/sjAEPn7/CgbEdPtO420cqGbn5RDDVr" +
		"vrW17RVnTXO9l6epXmTFmXrvAEWimiYz2VBDidhyCNz9yw2w1llKjDSDTI2tW+n8aFErnLjjPfvXEgc4IYKrfvoAAdAKtgEBvi" +
		"ofE83Kw9YRF7DnwMrChnNbGEL69zKHuN+v+Tf9QZiAkltjO0J5KnIr9q8HDv71ZZRudr/wd34qSAVOw49GY5WvOsDcFdwZGR7" +
		"ti+Nv6YZII3Ev+aZls77Nw41TO96Z5AETMPYDO5PHBPE1uyygCXjBheCDBg4KZCVxKDcLipucRbOhnjd4w3veauvC434fehjx/" +
		"GGvgnx/BCDtQc2bs45Hy9hjsJU7TbQNUzt3QJ1txwjy6Df3CoHc5BLqVj7iBXJRAAAEp/JgAAAA="
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func validationField(err error) string {
	var validation *ValidationError
	if errors.As(err, &validation) {
		return validation.Field
	}
	return ""
}

func assertMode(t *testing.T, path string, expected os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if actual := info.Mode().Perm(); actual != expected {
		t.Fatalf("%s mode = %o, want %o", path, actual, expected)
	}
}
