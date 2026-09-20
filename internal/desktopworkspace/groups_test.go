package desktopworkspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGroupsPersistAndLegacyClientsPreserveGroups(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.Workspace().Groups) != 0 {
		t.Fatal("default desktop must be ungrouped")
	}
	input := validReplaceInput(store.Workspace().ResourceVersion)
	input.Groups = []Group{{ID: testShortcutID, Name: "运维", Columns: 3, Members: []string{"nav:/overview", "shortcut:" + testShortcutID}}}
	input.Positions["group:"+testShortcutID] = Position{X: 0.5, Y: 1.2}
	saved, err := store.Replace(input)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root)
	if err != nil || !reflect.DeepEqual(reopened.Workspace().Groups, saved.Groups) {
		t.Fatalf("reopen: %v", err)
	}
	if _, err := store.Replace(input); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale: %v", err)
	}
	legacy := validReplaceInput(saved.ResourceVersion)
	legacy.Shortcuts = nil
	delete(legacy.Positions, "shortcut:"+testShortcutID)
	saved, err = store.Replace(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.Groups) != 1 || !reflect.DeepEqual(saved.Groups[0].Members, []string{"nav:/overview"}) || saved.Positions["group:"+testShortcutID].Y != 1.2 {
		t.Fatalf("legacy lost group or retained deleted shortcut: %#v", saved)
	}
	legacy.ExpectedResourceVersion = saved.ResourceVersion
	legacy.Groups = []Group{}
	if saved, err = store.Replace(legacy); err != nil || len(saved.Groups) != 0 {
		t.Fatalf("explicit dissolve: %v", err)
	}
}

func TestGroupsRejectInvalidMembershipAndDoNotCommitFailedWrites(t *testing.T) {
	store := openTestStore(t)
	base := store.Workspace()
	for _, members := range [][]string{{"nav:/overview", "nav:/overview"}, {"group:" + testShortcutID}, {"widget:clock"}, {"shortcut:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, {"nav:/../etc"}} {
		input := validReplaceInput(base.ResourceVersion)
		input.Groups = []Group{{ID: testShortcutID, Name: "Group", Columns: 3, Members: members}}
		if _, err := store.Replace(input); validationField(err) != "groups" {
			t.Fatalf("accepted %v: %v", members, err)
		}
	}
	input := validReplaceInput(base.ResourceVersion)
	input.Groups = []Group{{ID: testShortcutID, Name: "Group", Columns: 3, Members: []string{"nav:/overview"}}, {ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Name: "Other", Columns: 2, Members: []string{"nav:/overview"}}}
	if _, err := store.Replace(input); validationField(err) != "groups" {
		t.Fatal("duplicate cross-group membership accepted")
	}
	input.Groups = input.Groups[:1]
	store.writeAtomic = func(string, string, []byte) error { return errors.New("disk full") }
	if _, err := store.Replace(input); err == nil {
		t.Fatal("write failure hidden")
	}
	if store.Workspace().ResourceVersion != base.ResourceVersion || len(store.Workspace().Groups) != 0 {
		t.Fatal("failed write committed")
	}
}

func TestV3MigratesWithoutGroupingOrRepositioning(t *testing.T) {
	root := t.TempDir()
	data := `{"schemaVersion":3,"hiddenEntryKeys":[],"hiddenWidgetKeys":[],"positions":{"nav:/overview":{"x":0.25,"y":0.5}},"widgetPositions":{},"labels":{},"shortcuts":[]}`
	if err := os.WriteFile(filepath.Join(root, "workspace.json"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	state := store.Workspace()
	if !state.Available || state.SchemaVersion != 4 || len(state.Groups) != 0 || state.Positions["nav:/overview"].X != 0.25 {
		t.Fatalf("migration changed layout: %#v", state)
	}
}

func TestGroupLimitsAreValidatedBeforeCommit(t *testing.T) {
	store := openTestStore(t)
	base := store.Workspace()
	for name, change := range map[string]func(*ReplaceInput){
		"invalid ID":   func(input *ReplaceInput) { input.Groups[0].ID = "../group" },
		"empty name":   func(input *ReplaceInput) { input.Groups[0].Name = "" },
		"long name":    func(input *ReplaceInput) { input.Groups[0].Name = strings.Repeat("组", 49) },
		"control name": func(input *ReplaceInput) { input.Groups[0].Name = "name\nvalue" },
		"columns low":  func(input *ReplaceInput) { input.Groups[0].Columns = 1 },
		"columns high": func(input *ReplaceInput) { input.Groups[0].Columns = 5 },
		"duplicate ID": func(input *ReplaceInput) { input.Groups = append(input.Groups, input.Groups[0]) },
		"group quota": func(input *ReplaceInput) {
			input.Groups = nil
			for index := 0; index <= MaxGroups; index++ {
				input.Groups = append(input.Groups, Group{ID: fmt.Sprintf("%032x", index), Name: "Group", Columns: 3})
			}
		},
		"member quota": func(input *ReplaceInput) {
			for index := 0; index <= MaxPositions; index++ {
				input.Groups[0].Members = append(input.Groups[0].Members, fmt.Sprintf("app:entry-%d", index))
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := validReplaceInput(base.ResourceVersion)
			input.Groups = []Group{{ID: testShortcutID, Name: "Group", Columns: 3}}
			change(&input)
			if _, err := store.Replace(input); validationField(err) != "groups" {
				t.Fatalf("expected group rejection: %v", err)
			}
			if store.Workspace().ResourceVersion != base.ResourceVersion {
				t.Fatal("invalid input changed confirmed state")
			}
		})
	}
}
