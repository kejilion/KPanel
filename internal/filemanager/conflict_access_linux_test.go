package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

func TestConflictCopyInSameDirectoryAppliesKeepBothToRest(t *testing.T) {
	m, root := newTestManager(t)
	mustMkdirAll(t, filepath.Join(root, "same"))
	for _, name := range []string{"a.txt", "b.txt"} {
		mustWrite(t, filepath.Join(root, "same", name), "original")
	}
	prompts := 0
	session := &ConflictSession{Prompt: func(context.Context, contract.FileConflict) (contract.FileConflictDecision, error) {
		prompts++
		return contract.FileConflictDecision{Policy: "keep-both", ApplyToRest: true}, nil
	}}
	result, err := m.Action(WithFileConflicts(context.Background(), session), contract.FileActionRequest{Action: "copy", Sources: []string{"/same/a.txt", "/same/b.txt"}, Target: "/same"})
	if err != nil || len(result.Failed) > 0 || len(result.Succeeded) != 2 || prompts != 1 {
		t.Fatalf("result %+v error %v prompts %d", result, err, prompts)
	}
	for _, name := range []string{"a.txt", "a (1).txt", "b.txt", "b (1).txt"} {
		body, err := os.ReadFile(filepath.Join(root, "same", name))
		if err != nil || string(body) != "original" {
			t.Fatalf("%s=%q %v", name, body, err)
		}
	}
}

func TestConflictOverwritePreservesTargetAccessForCopyAndMove(t *testing.T) {
	for _, action := range []string{"copy", "move"} {
		t.Run(action, func(t *testing.T) {
			m, root := newTestManager(t)
			mustMkdirAll(t, filepath.Join(root, "from"))
			mustMkdirAll(t, filepath.Join(root, "to"))
			mustWrite(t, filepath.Join(root, "from", "a.txt"), "new")
			mustWrite(t, filepath.Join(root, "to", "a.txt"), "old")
			os.Chmod(filepath.Join(root, "to", "a.txt"), 0640)
			result, err := m.Action(context.Background(), contract.FileActionRequest{Action: action, Sources: []string{"/from/a.txt"}, Target: "/to", ConflictPolicies: &contract.FileConflictPolicies{File: "overwrite"}})
			if err != nil || len(result.Failed) > 0 {
				t.Fatalf("%+v %v", result, err)
			}
			info, err := os.Stat(filepath.Join(root, "to", "a.txt"))
			if err != nil || info.Mode().Perm() != 0640 {
				t.Fatalf("target access %+v %v", info, err)
			}
		})
	}
}
