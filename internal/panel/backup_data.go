package panel

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/ai"
	"github.com/kejilion/kejilion-panel/internal/auth"
	"github.com/kejilion/kejilion-panel/internal/backup"
	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/store"
)

type panelBackupData struct {
	Version  int               `json:"version"`
	Origin   string            `json:"origin,omitempty"`
	Identity json.RawMessage   `json:"identity"`
	TOTPKey  []byte            `json:"totpKey,omitempty"`
	Files    map[string][]byte `json:"files"`
	AI       *ai.AccessBackup  `json:"ai,omitempty"`
}

var panelBackupRoots = []string{"cluster-state.json", "cluster-state-v2.json", "cluster-light-state.json", "cluster-file-peers-v2.json", "cluster-secrets", "cluster-secrets-v2", "cluster-light-secrets", "cluster-light-terminal-keys", "desktop-workspace", "notifications"}
var backupLeaf = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,160}$`)

func panelBackupPath(name string) bool {
	parts := strings.Split(name, "/")
	if len(parts) > 3 {
		return false
	}
	for _, p := range parts {
		if !backupLeaf.MatchString(p) || p == "." || p == ".." {
			return false
		}
	}
	if len(parts) == 1 {
		return name == "cluster-state.json" || name == "cluster-state-v2.json" || name == "cluster-light-state.json" || name == "cluster-file-peers-v2.json"
	}
	switch parts[0] {
	case "cluster-secrets", "cluster-secrets-v2", "cluster-light-secrets", "cluster-light-terminal-keys":
		return len(parts) == 2 && !strings.HasSuffix(parts[1], ".previous") && !strings.HasPrefix(parts[1], ".")
	case "desktop-workspace":
		return name == "desktop-workspace/workspace.json" || len(parts) == 3 && parts[1] == "icons" && strings.HasSuffix(parts[2], ".icon")
	case "notifications":
		return name == "notifications/notification-state.json" || name == "notifications/telegram-bot-token"
	}
	return false
}

func readPanelBackupFiles(root string) (map[string][]byte, error) {
	files := map[string][]byte{}
	var total int64
	for _, name := range panelBackupRoots {
		path := filepath.Join(root, name)
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			continue
		}
		err := filepath.WalkDir(path, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return backup.ErrInvalid
			}
			if entry.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			if !panelBackupPath(relative) {
				return nil
			}
			if len(files) >= 4096 {
				return backup.ErrInvalid
			}
			f, err := backup.OpenRegular(path, 8<<20)
			if err != nil {
				return err
			}
			defer f.Close()
			data, err := io.ReadAll(io.LimitReader(f, (8<<20)+1))
			if err != nil {
				return err
			}
			total += int64(len(data))
			if len(data) > 8<<20 || total > backup.MaxPanelBytes {
				return backup.ErrInvalid
			}
			files[relative] = data
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}

func backupFilesVersion(files map[string][]byte) string {
	keys := make([]string, 0, len(files))
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	hash := sha256.New()
	for _, k := range keys {
		hash.Write([]byte(k))
		hash.Write([]byte{0})
		hash.Write(files[k])
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// Comparison excludes heartbeat timestamps and consumed authentication counters;
// those change without an administrator changing the restored configuration.
func panelBackupRevision(data []byte) (string, error) {
	var value panelBackupData
	if err := json.Unmarshal(data, &value); err != nil {
		return "", err
	}
	scrub := func(raw []byte) ([]byte, error) {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		var walk func(any)
		walk = func(v any) {
			switch x := v.(type) {
			case map[string]any:
				for _, k := range []string{"updatedAt", "lastSeenAt", "lastSnapshot", "lastAttemptAt", "lastSuccessAt", "lastError", "lastErrorCode", "consecutiveFailures", "totpLastUsedStep", "resourceVersion"} {
					delete(x, k)
				}
				for _, child := range x {
					walk(child)
				}
			case []any:
				for _, child := range x {
					walk(child)
				}
			}
		}
		walk(v)
		return json.Marshal(v)
	}
	var err error
	value.Identity, err = scrub(value.Identity)
	if err != nil {
		return "", err
	}
	for name, raw := range value.Files {
		if strings.HasSuffix(name, ".json") {
			value.Files[name], err = scrub(raw)
			if err != nil {
				return "", err
			}
		}
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:]), nil
}

func sanitizeBackupFile(name string, data []byte) ([]byte, error) {
	if !strings.HasSuffix(name, ".json") {
		return data, nil
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	if strings.HasPrefix(name, "cluster-") {
		// Durable active/revoked relationships and identity keys travel together.
		// One-time pairing codes and unfinished enrollment transactions never do.
		for _, key := range []string{"pairingCodes", "enrollments"} {
			if _, ok := value[key]; ok {
				value[key] = json.RawMessage(`[]`)
			}
		}
		if name == "cluster-state-v2.json" {
			var controllers []map[string]json.RawMessage
			if err := json.Unmarshal(value["controllers"], &controllers); err != nil {
				return nil, err
			}
			kept := []map[string]json.RawMessage{}
			for _, controller := range controllers {
				if bytes.Equal(controller["state"], []byte(`"active"`)) || bytes.Equal(controller["state"], []byte(`"revoked"`)) {
					kept = append(kept, controller)
				}
			}
			value["controllers"], _ = json.Marshal(kept)
		}
		var hosts []map[string]json.RawMessage
		if raw, ok := value["hosts"]; ok {
			if err := json.Unmarshal(raw, &hosts); err != nil {
				return nil, err
			}
			filtered := []map[string]json.RawMessage{}
			for _, h := range hosts {
				if raw, ok := h["state"]; ok && !bytes.Equal(raw, []byte(`"active"`)) {
					continue
				}
				for _, key := range []string{"lastSnapshot", "lastAttemptAt", "lastSuccessAt", "lastError", "lastErrorCode", "pairingCredentialFile"} {
					delete(h, key)
				}
				filtered = append(filtered, h)
			}
			value["hosts"], _ = json.Marshal(filtered)
		}
	}
	if name == "notifications/notification-state.json" {
		delete(value, "alertStates")
		var settings map[string]json.RawMessage
		if err := json.Unmarshal(value["settings"], &settings); err != nil {
			return nil, err
		}
		settings["enabled"] = json.RawMessage(`false`)
		value["settings"], _ = json.Marshal(settings)
	}
	return json.Marshal(value)
}

func (s *Server) exportPanelBackup(ctx context.Context) ([]byte, error) {
	// Two complete reads detect concurrent key/config replacement rather than
	// publishing a mixed snapshot. Continual activity fails with a retryable error.
	for attempt := 0; attempt < 3; attempt++ {
		files, err := readPanelBackupFiles(s.config.DataDir)
		if err != nil {
			return nil, err
		}
		identity, err := s.store.ExportIdentity()
		if err != nil {
			return nil, err
		}
		value := panelBackupData{Version: 1, Origin: s.config.PublicURL, Identity: identity, Files: files}
		if f, err := backup.OpenRegular(s.config.TOTPKeyPath, 32); err == nil {
			value.TOTPKey, err = io.ReadAll(f)
			f.Close()
			if err != nil {
				return nil, err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if s.ai != nil {
			access, err := s.ai.Providers.ExportAccess(ctx)
			if err != nil {
				return nil, err
			}
			value.AI = &access
		} else if _, err := os.Stat(filepath.Join(s.config.DataDir, "ai.db")); err == nil {
			return nil, errors.New("AI access storage is unavailable")
		}
		after, err := readPanelBackupFiles(s.config.DataDir)
		if err != nil {
			return nil, err
		}
		identityAfter, err := s.store.ExportIdentity()
		if err != nil {
			return nil, err
		}
		if backupFilesVersion(files) != backupFilesVersion(after) || !bytes.Equal(identity, identityAfter) {
			continue
		}
		for name, data := range files {
			clean, err := sanitizeBackupFile(name, data)
			if err != nil {
				return nil, err
			}
			files[name] = clean
		}
		if err := cluster.PruneBackupSecrets(value.Files); err != nil {
			return nil, err
		}
		data, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		if int64(len(data)) > backup.MaxPanelBytes {
			return nil, backup.ErrInvalid
		}
		return data, nil
	}
	return nil, errors.New("panel configuration changed during backup; retry when settings are idle")
}

func decodePanelBackup(data []byte) (panelBackupData, error) {
	var value panelBackupData
	if int64(len(data)) > backup.MaxPanelBytes || backup.Decode(data, &value) != nil || value.Version != 1 || len(value.Files) > 4096 {
		return value, backup.ErrInvalid
	}
	if err := store.ValidateIdentityBackup(value.Identity); err != nil {
		return value, err
	}
	var identity struct {
		Users []store.User `json:"users"`
	}
	if err := json.Unmarshal(value.Identity, &identity); err != nil {
		return value, err
	}
	if err := auth.ValidateBackupIdentity(identity.Users[0], value.TOTPKey); err != nil {
		return value, err
	}
	if len(value.TOTPKey) != 0 && len(value.TOTPKey) != 32 || identity.Users[0].TOTPSecret != "" && len(value.TOTPKey) != 32 {
		return value, backup.ErrInvalid
	}
	for name, data := range value.Files {
		if !panelBackupPath(name) || len(data) > 8<<20 {
			return value, backup.ErrInvalid
		}
		clean, err := sanitizeBackupFile(name, data)
		if err != nil {
			return value, err
		}
		value.Files[name] = clean
	}
	if err := cluster.PruneBackupSecrets(value.Files); err != nil {
		return value, err
	}
	if value.AI != nil {
		if err := ai.ValidateAccessBackup(*value.AI); err != nil {
			return value, err
		}
	}
	return value, nil
}
