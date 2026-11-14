package filemanager

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

var ErrFileSkipped = errors.New("已跳过同名项目")

type FileConflictPrompt func(context.Context, contract.FileConflict) (contract.FileConflictDecision, error)

// ConflictSession belongs to one operation. Callers serialize decisions, and
// durable job adapters persist an accepted policy before allowing more writes.
type ConflictSession struct {
	Policies contract.FileConflictPolicies
	Prompt   FileConflictPrompt
	Result   *contract.FileActionResult
	budget   *copyBudget
}

type conflictContextKey struct{}

func WithFileConflicts(ctx context.Context, session *ConflictSession) context.Context {
	return context.WithValue(ctx, conflictContextKey{}, session)
}

func fileConflicts(ctx context.Context) *ConflictSession {
	session, _ := ctx.Value(conflictContextKey{}).(*ConflictSession)
	return session
}

func FileConflicts(ctx context.Context) *ConflictSession { return fileConflicts(ctx) }

func ValidFileConflictPolicies(p *contract.FileConflictPolicies) bool {
	return p == nil || (validConflictPolicy(p.File, false) && validConflictPolicy(p.Directory, true))
}

func validConflictPolicy(policy string, directory bool) bool {
	return policy == "" || policy == "ask" || policy == "keep-both" || policy == "skip" || policy == "overwrite" && !directory || policy == "merge" && directory
}

// CopyName keeps compound archive extensions and never splits a UTF-8 rune.
// Directories, including names containing dots, are renamed as a whole.
func CopyName(name, kind string, attempt int) (string, error) {
	if validateName(name) != nil || attempt < 1 || attempt > 999 {
		return "", ErrInvalidPath
	}
	stem, extension := name, ""
	if kind == "file" {
		lower := strings.ToLower(name)
		for _, suffix := range []string{".tar.gz", ".tar.bz2", ".tar.xz", ".tar.zst"} {
			if strings.HasSuffix(lower, suffix) && len(name) > len(suffix) {
				extension = name[len(name)-len(suffix):]
				break
			}
		}
		if extension == "" {
			if index := strings.LastIndexByte(name, '.'); index > 0 {
				extension = name[index:]
			}
		}
		stem = strings.TrimSuffix(name, extension)
	}
	suffix := fmt.Sprintf(" (%d)%s", attempt, extension)
	for len(stem)+len(suffix) > 255 && len(stem) > 0 {
		_, size := utf8.DecodeLastRuneInString(stem)
		stem = stem[:len(stem)-size]
	}
	if stem == "" {
		return "", ErrInvalidPath
	}
	candidate := stem + suffix
	if err := validateName(candidate); err != nil {
		return "", err
	}
	return candidate, nil
}

// ResolveFileConflict is shared by Agent publication and Panel source adapters.
// Lookup must preserve permission/transport errors: only absence is nil.
func ResolveFileConflict(ctx context.Context, source contract.FileEntry, directory string, session *ConflictSession, changed bool,
	lookup func(context.Context, string) (*contract.FileEntry, error)) (contract.FileConflictResolution, error) {
	result := contract.FileConflictResolution{Name: source.Name}
	if validateName(source.Name) != nil || (source.Kind != "file" && source.Kind != "directory") {
		return result, ErrInvalidPath
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	target, err := lookup(ctx, joinVirtual(directory, source.Name))
	if err != nil || target == nil && !changed {
		return result, err
	}
	if target == nil {
		target = &contract.FileEntry{Name: source.Name, Path: joinVirtual(directory, source.Name), Kind: "missing"}
	}
	if session == nil {
		return result, ErrAlreadyExists
	}
	canReplace := target.Kind == "missing" || source.Kind == target.Kind && (source.Path == "" || source.Path != target.Path)
	policy := session.Policies.File
	if source.Kind == "directory" {
		policy = session.Policies.Directory
	}
	sameKind := target.Kind == "missing" || source.Kind == target.Kind
	if changed || !sameKind || !canReplace && policy != "skip" && policy != "keep-both" || policy == "" || policy == "ask" {
		if session.Prompt == nil {
			return result, ErrAlreadyExists
		}
		decision, err := session.Prompt(ctx, contract.FileConflict{Source: source, Target: *target, Changed: changed})
		if err != nil {
			return result, err
		}
		policy = decision.Policy
		if !validConflictPolicy(policy, source.Kind == "directory") || policy == "" || policy == "ask" || !canReplace && (policy == "overwrite" || policy == "merge") {
			return result, ErrAction
		}
		if decision.ApplyToRest && sameKind && (canReplace || policy == "skip" || policy == "keep-both") {
			if source.Kind == "directory" {
				session.Policies.Directory = policy
			} else {
				session.Policies.File = policy
			}
		}
	}
	switch policy {
	case "skip":
		result.Skipped = true
		if session.Result != nil {
			session.Result.SkippedCount++
			if len(session.Result.Skipped) < 64 {
				session.Result.Skipped = append(session.Result.Skipped, source.Path)
			}
		}
	case "keep-both":
		for attempt := 1; attempt <= 999; attempt++ {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			name, err := CopyName(source.Name, source.Kind, attempt)
			if err != nil {
				return result, err
			}
			entry, err := lookup(ctx, joinVirtual(directory, name))
			if err != nil {
				return result, err
			}
			if entry == nil {
				result.Name = name
				return result, nil
			}
		}
		return result, ErrAlreadyExists
	case "overwrite":
		result.Overwrite = target.Kind != "missing"
		result.Version = target.ResourceVersion
	case "merge":
		result.Merge = target.Kind != "missing"
		result.Version = target.ResourceVersion
	default:
		return result, ErrAction
	}
	return result, nil
}

func (m *Manager) conflictLookup(ctx context.Context, virtual string) (*contract.FileEntry, error) {
	entry, err := m.Stat(virtual)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

func (m *Manager) resolvePublication(ctx context.Context, source contract.FileEntry, directory string, changed bool) (contract.FileConflictResolution, error) {
	return ResolveFileConflict(ctx, source, directory, fileConflicts(ctx), changed, m.conflictLookup)
}

func conflictSource(name, kind string) contract.FileEntry {
	return contract.FileEntry{Name: name, Kind: kind}
}

func conflictErrorCode(err error) string {
	if errors.Is(err, ErrAlreadyExists) {
		return "already_exists"
	}
	if errors.Is(err, ErrConflict) {
		return "resource_conflict"
	}
	return ""
}

func (m *Manager) resolveAction(ctx context.Context, input contract.FileActionRequest) (contract.FileActionResult, error) {
	if input.ConflictPolicies == nil || !ValidFileConflictPolicies(input.ConflictPolicies) {
		return contract.FileActionResult{}, ErrAction
	}
	_, directory, err := m.resolveExisting(input.Target)
	if err != nil {
		return contract.FileActionResult{}, err
	}
	if err := m.mutationError(joinVirtual(directory, input.Name)); err != nil {
		return contract.FileActionResult{}, err
	}
	session := &ConflictSession{Policies: *input.ConflictPolicies}
	resolution, err := ResolveFileConflict(ctx, conflictSource(input.Name, input.SourceKind), directory, session, false, m.conflictLookup)
	return contract.FileActionResult{Action: "resolve", Succeeded: []contract.FileActionItem{}, Failed: []contract.FileActionFailure{}, Resolution: &resolution}, err
}

func (m *Manager) transferWithPolicy(ctx context.Context, source, directory, name string, move bool, budget *copyBudget, depth int) (contract.FileEntry, error) {
	if depth > 32 {
		return contract.FileEntry{}, ErrTooLarge
	}
	entry, err := m.Stat(source)
	if err != nil {
		return entry, err
	}
	if name != "" {
		entry.Name = name
	}
	if entry.Kind != "file" && entry.Kind != "directory" {
		return entry, ErrNotRegular
	}
	target := joinVirtual(directory, entry.Name)
	if move && target == source {
		if session := fileConflicts(ctx); session != nil && session.Result != nil {
			session.Result.SkippedCount++
			session.Result.Skipped = append(session.Result.Skipped, source)
		}
		return entry, ErrFileSkipped
	}
	if entry.Kind == "directory" && target != source && isWithin(target, source) {
		return entry, ErrInvalidPath
	}
	changed := false
	checkedBudget := false
	for attempt := 0; attempt < 8; attempt++ {
		choice, err := m.resolvePublication(ctx, entry, directory, changed)
		if err != nil {
			return entry, err
		}
		if choice.Skipped {
			return entry, ErrFileSkipped
		}
		if err := m.checkExpectedVersion(source, entry.ResourceVersion); err != nil {
			return entry, err
		}
		if depth == 0 && !checkedBudget {
			info, err := m.rootFS.Lstat(rootName(source))
			if err != nil {
				return entry, err
			}
			if err := m.checkPolicyTreeBudget(ctx, m.rootFS, rootName(source), info); err != nil {
				return entry, err
			}
			checkedBudget = true
		}
		if choice.Merge {
			if budget.entries++; budget.entries > budget.maxEntries {
				return entry, ErrTooLarge
			}
			parent, info, err := m.openReceiveDirectory(source, false)
			if err != nil {
				return entry, err
			}
			listing, err := parent.Open(".")
			if err != nil {
				parent.Close()
				return entry, err
			}
			children, err := listing.ReadDir(budget.maxEntries - budget.entries + 1)
			listing.Close()
			parent.Close()
			if err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, io.EOF) {
				return entry, err
			}
			if len(children)+budget.entries > budget.maxEntries {
				return entry, ErrTooLarge
			}
			sort.Slice(children, func(i, j int) bool { return children[i].Name() < children[j].Name() })
			complete := true
			for _, child := range children {
				if err := m.verifyDirectoryIdentity(source, info); err != nil {
					return entry, err
				}
				published, err := m.transferWithPolicy(ctx, joinVirtual(source, child.Name()), target, "", move, budget, depth+1)
				if errors.Is(err, ErrFileSkipped) {
					complete = false
					continue
				}
				if err != nil {
					return entry, err
				}
				_ = published
				if session := fileConflicts(ctx); session != nil && session.Result != nil {
					session.Result.CompletedCount++
				}
			}
			if move && complete {
				if err := m.verifyDirectoryIdentity(source, info); err != nil {
					return entry, err
				}
				current, err := m.Stat(source)
				if err != nil {
					return entry, err
				}
				if _, err = m.removeEmptyDirectory(source, current.ResourceVersion); err != nil {
					return entry, err
				}
			}
			if !complete {
				return entry, ErrFileSkipped
			}
			return m.Stat(target)
		}
		choiceInput := transferConflict{name: choice.Name, overwrite: choice.Overwrite, version: choice.Version}
		var published contract.FileEntry
		if move {
			published, err = m.moveOneResolved(ctx, source, directory, budget, choiceInput)
		} else {
			published, err = m.copyOneResolved(ctx, source, directory, budget, choiceInput)
		}
		if !errors.Is(err, ErrAlreadyExists) && !errors.Is(err, ErrConflict) {
			return published, err
		}
		// An already-selected keep-both policy can allocate another unoccupied
		// name. A changed overwrite must pass through the prompt again.
		changed = choice.Overwrite || choice.Merge
	}
	return entry, ErrConflict
}
