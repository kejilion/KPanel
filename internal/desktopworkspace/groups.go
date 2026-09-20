package desktopworkspace

import (
	"strings"
	"unicode/utf8"
)

const MaxGroups = 32

// Group only owns references and presentation. It never owns host resources.
type Group struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Members   []string `json:"members"`
	Columns   int      `json:"columns"`
	Collapsed bool     `json:"collapsed"`
}

func cloneGroups(groups []Group) []Group {
	result := make([]Group, 0, len(groups))
	for _, group := range groups {
		group.Members = append([]string{}, group.Members...)
		result = append(result, group)
	}
	return result
}

func validGroupPositionKey(key string, groups []Group) bool {
	for _, group := range groups {
		if key == "group:"+group.ID {
			return true
		}
	}
	return false
}

func validateGroups(state persistedWorkspace) error {
	invalid := func(detail string) error { return &ValidationError{Field: "groups", Detail: detail} }
	if len(state.Groups) > MaxGroups {
		return invalid("at most 32 groups are allowed")
	}
	ids, members, shortcuts := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, shortcut := range state.Shortcuts {
		shortcuts["shortcut:"+shortcut.ID] = true
	}
	for _, group := range state.Groups {
		if !ValidShortcutID(group.ID) || ids[group.ID] {
			return invalid("group IDs must be unique 32-character lowercase hexadecimal values")
		}
		ids[group.ID] = true
		if group.Name == "" || strings.TrimSpace(group.Name) != group.Name || !utf8.ValidString(group.Name) || utf8.RuneCountInString(group.Name) > MaxShortcutNameRunes || hasControl(group.Name) {
			return invalid("group names require 1 to 48 visible characters")
		}
		if group.Columns < 2 || group.Columns > 4 {
			return invalid("group columns must be between 2 and 4")
		}
		for _, key := range group.Members {
			if !validPositionKey(key) || members[key] {
				return invalid("members must be unique stable entry keys across groups")
			}
			if strings.HasPrefix(key, "shortcut:") && !shortcuts[key] {
				return invalid("group shortcut members must exist")
			}
			members[key] = true
		}
	}
	if len(members) > MaxPositions {
		return invalid("at most 512 grouped entries are allowed")
	}
	return nil
}

func pruneDeletedGroupShortcuts(state *persistedWorkspace) {
	shortcuts := map[string]bool{}
	for _, item := range state.Shortcuts {
		shortcuts["shortcut:"+item.ID] = true
	}
	for index := range state.Groups {
		members := []string{}
		for _, key := range state.Groups[index].Members {
			if !strings.HasPrefix(key, "shortcut:") || shortcuts[key] {
				members = append(members, key)
			}
		}
		state.Groups[index].Members = members
	}
}
