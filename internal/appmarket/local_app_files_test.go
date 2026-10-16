package appmarket

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func localAppConfig(token, container, port string) string {
	return `local app_id="` + token + `"
local app_name="笔记服务"
local docker_name="` + container + `"
local docker_port="` + port + `"

docker_app_install() {
	mkdir -p /home/docker/` + token + `
}

docker_app_update() {
	true
}

docker_app_uninstall() {
	rm -rf /home/docker/` + token + `
}

docker_app_plus
`
}

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func newLocalAppWriteService(t *testing.T) (*Service, string) {
	t.Helper()
	service, _, local := newLocalAppTestService(t)
	service.localAppSyntaxCheck = func(context.Context, []byte) error { return nil }
	return service, local
}

func markLocalAppInstalled(t *testing.T, service *Service, token string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(service.appRoot, "appno.txt"), []byte(token+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSaveLocalAppCreatesTrustedConfigAndIcon(t *testing.T) {
	service, local := newLocalAppWriteService(t)
	icon := testPNG(t, 64, 64)
	document, err := service.SaveLocalApp(context.Background(), LocalAppWriteInput{
		Token: "notes", Content: strings.ReplaceAll(localAppConfig("notes", "notes", "18080"), "\n", "\r\n"),
		Icon: &LocalAppIcon{ContentType: "image/png", Data: icon},
	})
	if err != nil {
		t.Fatal(err)
	}
	written, err := os.ReadFile(filepath.Join(local, "notes.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(written, []byte("\r\n")) || document.Content != string(written) {
		t.Fatalf("config was not normalized to LF: %q", written)
	}
	if runtime.GOOS == "linux" {
		info, statErr := os.Stat(filepath.Join(local, "notes.conf"))
		if statErr != nil || info.Mode().Perm()&0o022 != 0 {
			t.Fatalf("config mode is not trusted: %v %v", info.Mode(), statErr)
		}
	}
	if !strings.HasPrefix(document.ResourceVersion, "sha256:") || document.ContainerName != "notes" ||
		!strings.HasPrefix(document.IconURL, "/api/v1/apps/local/notes/icon/") {
		t.Fatalf("unexpected document: %#v", document)
	}
	item, err := service.Find(context.Background(), "local-notes")
	if err != nil {
		t.Fatal(err)
	}
	if item.Icon != document.IconURL || item.NameZH != "笔记服务" || item.DefaultPort != 18080 {
		t.Fatalf("inventory does not reflect the saved app: %#v", item.App)
	}
	served, err := service.LocalAppIcon("notes")
	if err != nil || served.ContentType != "image/png" || !bytes.Equal(served.Data, icon) {
		t.Fatalf("icon was not served back: %v", err)
	}
	if _, err := service.SaveLocalApp(context.Background(), LocalAppWriteInput{
		Token: "notes", Content: localAppConfig("notes", "notes", "18080"),
	}); !errors.Is(err, ErrLocalAppExists) {
		t.Fatalf("a second create replaced the config: %v", err)
	}
}

func TestSaveLocalAppRejectsConfigsTheScriptCannotManage(t *testing.T) {
	service, local := newLocalAppWriteService(t)
	if err := os.WriteFile(filepath.Join(service.scriptAppRoot, "official.conf"), []byte("docker_name=a\ndocker_app_plus\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	valid := localAppConfig("notes", "notes", "8080")
	for name, sample := range map[string]struct {
		token, content string
	}{
		"invalid name":         {"../notes", valid},
		"official name":        {"official", localAppConfig("official", "official", "8080")},
		"catalog name":         {service.catalog.Apps[0].Token, localAppConfig(service.catalog.Apps[0].Token, "x", "8080")},
		"missing app_id":       {"notes", strings.Replace(valid, `local app_id="notes"`, "", 1)},
		"mismatched app_id":    {"notes", strings.Replace(valid, `app_id="notes"`, `app_id="other"`, 1)},
		"missing uninstall":    {"notes", strings.Replace(valid, "docker_app_uninstall()", "cleanup()", 1)},
		"missing registration": {"notes", strings.Replace(valid, "docker_app_plus", "", 1)},
		"dynamic docker_name":  {"notes", strings.Replace(valid, `local docker_name="notes"`, `local docker_name="$NAME"`, 1)},
		"empty":                {"notes", "   \n"},
		"binary":               {"notes", valid + "\x00"},
		"oversized":            {"notes", valid + strings.Repeat("#", maxScriptAppConfigBytes)},
	} {
		if _, err := service.SaveLocalApp(context.Background(), LocalAppWriteInput{Token: sample.token, Content: sample.content}); !errors.Is(err, ErrLocalAppInvalid) {
			t.Errorf("%s: err = %v, want ErrLocalAppInvalid", name, err)
		}
	}
	service.localAppSyntaxCheck = func(context.Context, []byte) error { return localAppInvalid("Shell 语法错误：line 3") }
	if _, err := service.SaveLocalApp(context.Background(), LocalAppWriteInput{Token: "notes", Content: valid}); err == nil ||
		!strings.Contains(err.Error(), "语法错误") {
		t.Fatalf("syntax errors were not reported: %v", err)
	}
	if entries, _ := os.ReadDir(local); len(entries) != 0 {
		t.Fatalf("rejected configs left files behind: %v", entries)
	}
}

func TestSaveLocalAppRejectsInvalidIcons(t *testing.T) {
	service, _ := newLocalAppWriteService(t)
	for name, icon := range map[string]*LocalAppIcon{
		"not square": {ContentType: "image/png", Data: testPNG(t, 64, 80)},
		"too small":  {ContentType: "image/png", Data: testPNG(t, 32, 32)},
		"svg":        {ContentType: "image/svg+xml", Data: []byte("<svg/>")},
		"corrupt":    {ContentType: "image/webp", Data: []byte("RIFF0000WEBPVP8 garbage")},
		"too large":  {ContentType: "image/png", Data: bytes.Repeat([]byte{1}, maxLocalAppIconBytes+1)},
	} {
		_, err := service.SaveLocalApp(context.Background(), LocalAppWriteInput{
			Token: "notes", Content: localAppConfig("notes", "notes", "8080"), Icon: icon,
		})
		if !errors.Is(err, ErrLocalAppInvalid) {
			t.Errorf("%s: err = %v, want ErrLocalAppInvalid", name, err)
		}
	}
}

func TestUpdateLocalAppRequiresVersionAndKeepsInstalledIdentity(t *testing.T) {
	service, _ := newLocalAppWriteService(t)
	created, err := service.SaveLocalApp(context.Background(), LocalAppWriteInput{
		Token: "notes", Content: localAppConfig("notes", "notes", "8080"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveLocalApp(context.Background(), LocalAppWriteInput{
		Token: "notes", Content: localAppConfig("notes", "notes", "9090"), ResourceVersion: "sha256:" + strings.Repeat("0", 64),
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale version was accepted: %v", err)
	}
	markLocalAppInstalled(t, service, "notes")
	if _, err := service.SaveLocalApp(context.Background(), LocalAppWriteInput{
		Token: "notes", Content: localAppConfig("notes", "renamed", "8080"), ResourceVersion: created.ResourceVersion,
	}); !errors.Is(err, ErrLocalAppInvalid) || !strings.Contains(err.Error(), "docker_name") {
		t.Fatalf("installed container identity changed: %v", err)
	}
	updated, err := service.SaveLocalApp(context.Background(), LocalAppWriteInput{
		Token: "notes", Content: localAppConfig("notes", "notes", "9090"), ResourceVersion: created.ResourceVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Installed || updated.ResourceVersion == created.ResourceVersion || !strings.Contains(updated.Content, `"9090"`) {
		t.Fatalf("installed port edit was not saved: %#v", updated)
	}
}

func TestDeleteLocalAppRefusesInstalledAppsAndStaleVersions(t *testing.T) {
	service, local := newLocalAppWriteService(t)
	created, err := service.SaveLocalApp(context.Background(), LocalAppWriteInput{
		Token: "notes", Content: localAppConfig("notes", "notes", "8080"),
		Icon: &LocalAppIcon{ContentType: "image/png", Data: testPNG(t, 64, 64)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteLocalApp(context.Background(), "notes", "sha256:"+strings.Repeat("0", 64)); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale delete was accepted: %v", err)
	}
	markLocalAppInstalled(t, service, "notes")
	if err := service.DeleteLocalApp(context.Background(), "notes", created.ResourceVersion); !errors.Is(err, ErrForbidden) {
		t.Fatalf("installed app config was deleted: %v", err)
	}
	if err := os.Remove(filepath.Join(service.appRoot, "appno.txt")); err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteLocalApp(context.Background(), "notes", created.ResourceVersion); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(local); len(entries) != 0 {
		t.Fatalf("delete left files behind: %v", entries)
	}
	if err := service.DeleteLocalApp(context.Background(), "notes", created.ResourceVersion); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing app delete = %v, want ErrNotFound", err)
	}
}

func TestLocalAppWritesWaitForRunningJobs(t *testing.T) {
	service, _ := newLocalAppWriteService(t)
	created, err := service.SaveLocalApp(context.Background(), LocalAppWriteInput{
		Token: "notes", Content: localAppConfig("notes", "notes", "8080"),
	})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := service.configureJobs(filepath.Join(root, "jobs"), filepath.Join(root, "kejilion-agent"), &fakeJobRunner{}); err != nil {
		t.Fatal(err)
	}
	if err := service.jobs.put(appJobRecord{AppJob: AppJob{
		ID: strings.Repeat("b", 32), AppID: "local-notes", Action: "install", Status: "running", CreatedAt: time.Now(),
	}, Adapter: "kejilion"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveLocalApp(context.Background(), LocalAppWriteInput{
		Token: "notes", Content: localAppConfig("notes", "notes", "9090"), ResourceVersion: created.ResourceVersion,
	}); !errors.Is(err, ErrTaskConflict) {
		t.Fatalf("edit during a running job = %v, want ErrTaskConflict", err)
	}
	if err := service.DeleteLocalApp(context.Background(), "notes", created.ResourceVersion); !errors.Is(err, ErrTaskConflict) {
		t.Fatalf("delete during a running job = %v, want ErrTaskConflict", err)
	}
}

func TestBashSyntaxCheckParsesWithoutExecuting(t *testing.T) {
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("bash is unavailable")
	}
	marker := filepath.Join(t.TempDir(), "executed")
	if err := bashSyntaxCheck(context.Background(), []byte("touch "+marker+"\necho ok\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("syntax check executed the config: %v", err)
	}
	if err := bashSyntaxCheck(context.Background(), []byte("docker_app_install() {\n  echo\n")); !errors.Is(err, ErrLocalAppInvalid) {
		t.Fatalf("unterminated function passed the syntax check: %v", err)
	}
}
