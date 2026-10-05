//go:build windows

package filemanager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"golang.org/x/sys/windows"
)

func TestWindowsOfficeSavePreservesPrivateAccess(t *testing.T) {
	m, root, _ := windowsVolumeManager(t)
	setTestDACL(t, root, testAccessSDDL(t, true, true))
	name := filepath.Join(root, "private.docx")
	if err := os.WriteFile(name, officeFixtures(t)["sample.docx"], 0o600); err != nil {
		t.Fatal(err)
	}
	setTestDACL(t, name, testAccessSDDL(t, false, false))
	want := testDACL(t, name)
	doc, err := m.ReadOffice(context.Background(), "/C/private.docx")
	if err != nil {
		t.Fatal(err)
	}
	input := contract.FileWriteRequest{
		ExpectedResourceVersion: doc.Entry.ResourceVersion,
		ExpectedContentVersion:  doc.ContentVersion,
		OfficeEdits:             []contract.OfficeEdit{{ID: doc.Sections[0].Items[0].ID, Text: "Private update"}},
	}
	if _, err := m.WriteOffice(context.Background(), doc.Entry.Path, input); err != nil {
		t.Fatal(err)
	}
	if got := testDACL(t, name); got != want {
		t.Fatalf("Office save widened access: %s, want %s", got, want)
	}
	file, err := openAsRestrictedReader(testWorldRestrictedReader(t), name)
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		if file != nil {
			file.Close()
		}
		t.Fatalf("private Office document became readable: %v", err)
	}
	updated, err := m.ReadOffice(context.Background(), doc.Entry.Path)
	if err != nil || updated.Sections[0].Items[0].Text != "Private update" {
		t.Fatalf("saved content did not round trip: %+v %v", updated, err)
	}
}
