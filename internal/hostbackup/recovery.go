package hostbackup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/backup"
)

// Recover only undoes an interrupted transaction (or finishes committed cleanup).
// It never restarts a requested restore from the beginning.
func (e *Engine) Recover(ctx context.Context, id, directory string) error {
	if !backup.ValidID(id) || filepath.Base(directory) != id {
		return backup.ErrInvalid
	}
	exportPath := filepath.Join(directory, "export-journal.json")
	if data, err := backup.ReadFile(exportPath, 4<<20); err == nil {
		var running []Container
		if backup.Decode(data, &running) != nil || len(running) > 512 {
			return backup.ErrInvalid
		}
		var restartErr error
		for _, c := range running {
			if len(c.ID) != 64 || strings.Trim(c.ID, "0123456789abcdef") != "" {
				return backup.ErrInvalid
			}
			restartErr = errors.Join(restartErr, e.docker(ctx, "POST", "/containers/"+c.ID+"/start", nil, nil))
		}
		if restartErr != nil {
			return restartErr
		}
		if err := os.Remove(exportPath); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	path := filepath.Join(directory, "restore-journal.json")
	data, err := backup.ReadFile(path, 4<<20)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var j restoreJournal
	if backup.Decode(data, &j) != nil || j.ID != id || len(j.Roots) > 4096 || len(j.Containers) > 512 || len(j.Networks) > 512 {
		return backup.ErrInvalid
	}
	if j.Phase == "completed" || j.Phase == "rolled_back" {
		return nil
	}
	switch j.Phase {
	case "prepared", "applying", "needs_attention", "cleanup_pending":
	default:
		return backup.ErrInvalid
	}
	for _, r := range j.Roots {
		relative, err := filepath.Rel(e.Root, r.Target)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) || e.excluded("/"+filepath.ToSlash(relative)) || r.Previous != r.Target+".kpanel-restore-"+id || r.Next != r.Target+".kpanel-next-"+id {
			return backup.ErrInvalid
		}
		if err := backup.NoLinkParents(r.Target); err != nil {
			return err
		}
		if err := backup.NoLinkParents(r.Previous); err != nil {
			return err
		}
	}
	for _, r := range j.Containers {
		if !validContainerName(r.Name) || r.Previous != "kpanel-previous-"+id+"-"+r.Name {
			return backup.ErrInvalid
		}
		for _, cid := range []string{r.OldID, r.NewID} {
			if cid != "" && (len(cid) != 64 || strings.Trim(cid, "0123456789abcdef") != "") {
				return backup.ErrInvalid
			}
		}
	}
	for _, name := range j.Networks {
		if !validContainerName(name) {
			return backup.ErrInvalid
		}
	}
	if j.Phase == "cleanup_pending" {
		if err := e.cleanupRecovery(ctx, &j); err != nil {
			return err
		}
		j.Phase = "completed"
	} else {
		if err := e.rollback(ctx, &j); err != nil {
			j.Phase = "needs_attention"
			return errors.Join(err, backup.WriteJSON(path, j))
		}
		j.Phase = "rolled_back"
	}
	return backup.WriteJSON(path, j)
}

func recoveryPending(directory string) bool {
	if _, err := os.Stat(filepath.Join(directory, "export-journal.json")); err == nil {
		return true
	}
	if data, err := backup.ReadFile(filepath.Join(directory, "restore-journal.json"), 4<<20); err == nil {
		var j restoreJournal
		if backup.Decode(data, &j) != nil {
			return true
		}
		return j.Phase != "completed" && j.Phase != "rolled_back"
	}
	return false
}
