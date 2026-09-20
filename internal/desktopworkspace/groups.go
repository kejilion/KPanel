package desktopworkspace

import (
	"maps"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

const MaxGroups = 32

// Group only owns references and presentation. It never owns host resources.
type Group struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Members   []string       `json:"members"`
	Columns   int            `json:"columns"`
	Collapsed bool           `json:"collapsed"`
	Rows      int            `json:"rows,omitempty"`
	Slots     map[string]int `json:"slots,omitempty"`
}

func cloneGroups(groups []Group) []Group {
	result := make([]Group, 0, len(groups))
	for _, group := range groups {
		group.Members = append([]string{}, group.Members...)
		if group.Slots != nil {
			slots := make(map[string]int, len(group.Slots))
			for key, slot := range group.Slots {
				slots[key] = slot
			}
			group.Slots = slots
		}
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
		if group.Rows < 0 || group.Rows > 8 {
			return invalid("reserved rows must be between 0 and 8")
		}
		groupMembers := map[string]bool{}
		for _, key := range group.Members {
			if !validPositionKey(key) || members[key] {
				return invalid("members must be unique stable entry keys across groups")
			}
			if strings.HasPrefix(key, "shortcut:") && !shortcuts[key] {
				return invalid("group shortcut members must exist")
			}
			members[key] = true
			groupMembers[key] = true
		}
		occupied := map[int]bool{}
		for key, slot := range group.Slots {
			if !groupMembers[key] || slot < 0 || slot >= MaxPositions || occupied[slot] {
				return invalid("slots require existing group members and unique cells between 0 and 511")
			}
			occupied[slot] = true
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
		pruneGroupSlots(&state.Groups[index])
	}
}

func pruneGroupSlots(group *Group) {
	members := map[string]bool{}
	for _, key := range group.Members {
		members[key] = true
	}
	for key := range group.Slots {
		if !members[key] {
			delete(group.Slots, key)
		}
	}
}

// Older cached clients send groups but do not know sparse cells/reserved rows.
// Absence of slots preserves these fields; an explicit empty object resets them.
func preserveLegacyGroupLayout(state *persistedWorkspace, current persistedWorkspace) {
	previous := cloneGroups(current.Groups)
	for index := range state.Groups {
		group := &state.Groups[index]
		for _, old := range previous {
			if old.ID == group.ID {
				if group.Slots == nil {
					group.Slots, group.Rows = old.Slots, old.Rows
					pruneGroupSlots(group)
				} else if maps.Equal(group.Slots, old.Slots) {
					// v4 JS clients spread unknown fields back unchanged. Only this
					// exact passthrough may prune removed members; changed invalid
					// cell maps must still fail validation.
					pruneGroupSlots(group)
					if !slices.Equal(group.Members, old.Members) && len(group.Members) == len(old.Members) && len(group.Slots) == len(group.Members) {
						// Legacy reordering changes only members. Preserve the gap
						// positions while applying that order to occupied cells.
						cells := make([]int, 0, len(group.Slots))
						for _, cell := range group.Slots {
							cells = append(cells, cell)
						}
						sort.Ints(cells)
						for memberIndex, key := range group.Members {
							group.Slots[key] = cells[memberIndex]
						}
					}
				}
				break
			}
		}
	}
}
