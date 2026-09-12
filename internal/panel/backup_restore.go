package panel

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/kejilion/kejilion-panel/internal/ai"
	"github.com/kejilion/kejilion-panel/internal/backup"
	"github.com/kejilion/kejilion-panel/internal/cluster"
	"github.com/kejilion/kejilion-panel/internal/desktopworkspace"
	"github.com/kejilion/kejilion-panel/internal/notification"
	"github.com/kejilion/kejilion-panel/internal/store"
	"github.com/kejilion/kejilion-panel/internal/version"
)

var ErrBackupRestart = errors.New("panel restart requested by restore")

type panelRestoreJournal struct {
	ID       string            `json:"id"`
	Phase    string            `json:"phase"`
	Previous map[string][]byte `json:"previous"`
	Names    []string          `json:"names"`
	Identity []byte            `json:"identity"`
	TOTPKey  []byte            `json:"totpKey"`
	AI       *ai.AccessBackup  `json:"ai,omitempty"`
}

func panelRestorePath(c Config) string {
	return filepath.Join(c.DataDir, "backups", "panel-restore.json")
}

func PanelRestorePending(c Config) bool          { _, err := os.Lstat(panelRestorePath(c)); return err == nil }
func (s *Server) BackupRestart() <-chan struct{} { return s.backupRestart }

// validatePanelRestore opens copies through their owning stores. No background
// service is started, so validation cannot contact peers or send notifications.
func (s *Server) validatePanelRestore(value panelBackupData, directory string) error {
	return validatePanelRestoreConfig(s.config, value, directory)
}

func validatePanelRestoreConfig(c Config, value panelBackupData, directory string) error {
	if err := backup.PrivateDir(directory); err != nil {
		return err
	}
	defer os.RemoveAll(directory)
	for name, data := range value.Files {
		if err := backup.AtomicFile(filepath.Join(directory, filepath.FromSlash(name)), data); err != nil {
			return err
		}
	}
	// A missing identity must not be silently generated during a migration.
	if _, ok := value.Files["cluster-state.json"]; !ok {
		return backup.ErrInvalid
	}
	if _, ok := value.Files["cluster-secrets-v2/node-identity.v2key"]; !ok {
		return backup.ErrInvalid
	}
	cs, err := cluster.NewService(cluster.ServiceConfig{DataDir: directory, PanelVersion: version.Version, PublicURL: c.PublicURL, PrivateCIDRs: c.ClusterPrivateCIDRs, Telemetry: clusterTelemetrySource{}})
	if err != nil {
		return err
	}
	defer cs.Close()
	if err := cs.ValidateBackupReferences(); err != nil {
		return err
	}
	if _, err := desktopworkspace.Open(filepath.Join(directory, "desktop-workspace")); err != nil {
		return err
	}
	ns, err := notification.NewService(notification.Config{DataDir: directory, Hosts: cs})
	if err != nil {
		return err
	}
	return ns.Close()
}

func stagePanelRestore(c Config, id string, data []byte) error {
	if !backup.ValidID(id) {
		return backup.ErrInvalid
	}
	if _, err := decodePanelBackup(data); err != nil {
		return err
	}
	if _, err := os.Lstat(panelRestorePath(c)); !errors.Is(err, os.ErrNotExist) {
		return backup.ErrBusy
	}
	if err := backup.AtomicFile(filepath.Join(c.DataDir, "backups", id, "panel-restore.payload"), data); err != nil {
		return err
	}
	return backup.WriteJSON(panelRestorePath(c), panelRestoreJournal{ID: id, Phase: "pending"})
}

// ApplyPanelRestore runs only after all HTTP handlers, stores and background
// workers have closed. Durable rollback precedes the first live file change.
func ApplyPanelRestore(c Config) (err error) {
	body, err := backup.ReadFile(panelRestorePath(c), 96<<20)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var j panelRestoreJournal
	if backup.Decode(body, &j) != nil || !backup.ValidID(j.ID) {
		return backup.ErrInvalid
	}
	if j.Phase != "pending" {
		return recoverPanelRestore(c, j)
	}
	// A failed preflight has not touched live data. Cancel this intent so a
	// corrupt upload or changed destination cannot trap startup in a retry loop.
	defer func() {
		if err == nil || j.Phase != "pending" {
			return
		}
		if cleanupErr := setPanelRestoreResult(c, j.ID, "failed", "operation_failed"); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
			return
		}
		if cleanupErr := os.Remove(panelRestorePath(c)); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
			return
		}
		err = errors.Join(err, backup.SyncDir(filepath.Dir(panelRestorePath(c))))
	}()
	state, err := backup.ReadFile(filepath.Join(c.DataDir, "backups", j.ID, "record.json"), 32<<10)
	if err != nil {
		return err
	}
	var record backup.Record
	if backup.Decode(state, &record) != nil || record.Status != "restarting" {
		return errors.New("restore intent was not committed")
	}
	data, err := backup.ReadFile(filepath.Join(c.DataDir, "backups", j.ID, "panel-restore.payload"), backup.MaxPanelBytes)
	if err != nil {
		return err
	}
	value, err := decodePanelBackup(data)
	if err != nil {
		return err
	}
	if err := validatePanelRestoreConfig(c, value, filepath.Join(c.DataDir, "backups", j.ID, "apply-check")); err != nil {
		return err
	}
	j.Previous, err = readPanelBackupFiles(c.DataDir)
	if err != nil {
		return err
	}
	j.Identity, err = backup.ReadFile(c.StorePath, 32<<20)
	if err != nil {
		return err
	}
	j.TOTPKey, err = backup.ReadFile(c.TOTPKeyPath, 32)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if value.AI != nil {
		err = ai.WithBackupAccess(c.DataDir, func(p *ai.ProviderService) error { v, e := p.ExportAccess(context.Background()); j.AI = &v; return e })
		if err != nil {
			return err
		}
	}
	all := map[string]bool{}
	for name := range j.Previous {
		all[name] = true
	}
	for name := range value.Files {
		all[name] = true
	}
	for name := range all {
		j.Names = append(j.Names, name)
	}
	sort.Strings(j.Names)
	// Retain revocations already present on the destination for this identity.
	if data, ok := value.Files["cluster-state-v2.json"]; ok {
		value.Files["cluster-state-v2.json"], err = mergeBackupRevocations(data, j.Previous["cluster-state-v2.json"])
		if err != nil {
			return err
		}
	}
	j.Phase = "applying"
	if err = backup.WriteJSON(panelRestorePath(c), j); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, recoverPanelRestore(c, j))
		}
	}()
	for _, name := range j.Names {
		path := filepath.Join(c.DataDir, filepath.FromSlash(name))
		if data, ok := value.Files[name]; ok {
			err = backup.AtomicFile(path, data)
		} else {
			err = os.Remove(path)
			if errors.Is(err, os.ErrNotExist) {
				err = nil
			}
		}
		if err != nil {
			return err
		}
	}
	if len(value.TOTPKey) > 0 {
		err = backup.AtomicFile(c.TOTPKeyPath, value.TOTPKey)
	} else {
		err = os.Remove(c.TOTPKeyPath)
		if errors.Is(err, os.ErrNotExist) {
			err = nil
		}
	}
	if err != nil {
		return err
	}
	st, err := store.Open(c.StorePath)
	if err != nil {
		return err
	}
	err = st.RestoreIdentity(value.Identity)
	err = errors.Join(err, st.Close())
	if err != nil {
		return err
	}
	if value.AI != nil {
		err = ai.WithBackupAccess(c.DataDir, func(p *ai.ProviderService) error { return p.RestoreAccess(context.Background(), *value.AI) })
		if err != nil {
			return err
		}
	}
	j.Phase = "applied"
	return backup.WriteJSON(panelRestorePath(c), j)
}

func mergeBackupRevocations(incoming, current []byte) ([]byte, error) {
	if len(current) == 0 {
		return incoming, nil
	}
	var a, b map[string]json.RawMessage
	if json.Unmarshal(incoming, &a) != nil || json.Unmarshal(current, &b) != nil {
		return nil, backup.ErrInvalid
	}
	if string(a["nodeId"]) != string(b["nodeId"]) {
		return incoming, nil
	}
	var old, next []map[string]json.RawMessage
	if json.Unmarshal(b["controllers"], &old) != nil || json.Unmarshal(a["controllers"], &next) != nil {
		return nil, backup.ErrInvalid
	}
	for _, r := range old {
		if string(r["state"]) != `"revoked"` {
			continue
		}
		found := false
		for n, v := range next {
			if string(r["id"]) == string(v["id"]) {
				next[n] = r
				found = true
			}
		}
		if !found {
			next = append(next, r)
		}
	}
	a["controllers"], _ = json.Marshal(next)
	return json.Marshal(a)
}

func recoverPanelRestore(c Config, j panelRestoreJournal) error {
	if j.Phase == "committed" {
		return finishCommittedPanelRestore(c, j)
	}
	if j.Phase == "pending" {
		return nil
	}
	if j.Phase != "applying" && j.Phase != "applied" {
		return backup.ErrInvalid
	}
	if len(j.Names) > 4096 || len(j.Identity) == 0 {
		return backup.ErrInvalid
	}
	for _, name := range j.Names {
		if !panelBackupPath(name) {
			return backup.ErrInvalid
		}
		target := filepath.Join(c.DataDir, filepath.FromSlash(name))
		if err := backup.NoLinkParents(target); err != nil {
			return err
		}
		if data, ok := j.Previous[name]; ok {
			if err := backup.AtomicFile(target, data); err != nil {
				return err
			}
		} else if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := backup.AtomicFile(c.StorePath, j.Identity); err != nil {
		return err
	}
	if len(j.TOTPKey) > 0 {
		if err := backup.AtomicFile(c.TOTPKeyPath, j.TOTPKey); err != nil {
			return err
		}
	} else if err := os.Remove(c.TOTPKeyPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if j.AI != nil {
		if err := ai.WithBackupAccess(c.DataDir, func(p *ai.ProviderService) error { return p.RestoreAccess(context.Background(), *j.AI) }); err != nil {
			return err
		}
	}
	if err := setPanelRestoreResult(c, j.ID, "failed", "rolled_back"); err != nil {
		return err
	}
	return os.Remove(panelRestorePath(c))
}

// RecoverPanelRestore is called before startup. An interrupted switch is rolled
// back; a pending, explicitly requested restore can be applied once.
func RecoverPanelRestore(c Config) error {
	data, err := backup.ReadFile(panelRestorePath(c), 96<<20)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var j panelRestoreJournal
	if backup.Decode(data, &j) != nil || !backup.ValidID(j.ID) {
		return backup.ErrInvalid
	}
	if j.Phase == "pending" {
		err := ApplyPanelRestore(c)
		if err != nil && !PanelRestorePending(c) {
			return nil
		} // Failed preflight already recorded; start the unchanged panel.
		return err
	}
	return recoverPanelRestore(c, j)
}

func (s *Server) FinalizePanelRestore() error {
	data, err := backup.ReadFile(panelRestorePath(s.config), 96<<20)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var j panelRestoreJournal
	if backup.Decode(data, &j) != nil || j.Phase != "applied" || !backup.ValidID(j.ID) {
		return backup.ErrInvalid
	}
	if j.AI != nil && s.ai == nil {
		return errors.New("restored AI access could not be opened")
	}
	if !s.auth.IsInitialized() {
		return errors.New("restored account could not be opened")
	}
	// The durable commit point precedes the public result. Startup may finish
	// cleanup after this point, but must never roll back a committed restore.
	j.Phase = "committed"
	if err := backup.WriteJSON(panelRestorePath(s.config), j); err != nil {
		return err
	}
	if err := s.backups.Update(j.ID, func(r *backup.Record) {
		r.Status = "completed"
		r.Stage = "completed"
		r.ErrorCode = ""
		if !slices.Contains(r.CompletedModules, "panel") {
			r.CompletedModules = append(r.CompletedModules, "panel")
		}
	}); err != nil {
		return err
	}
	return finishCommittedPanelRestore(s.config, j)
}

func finishCommittedPanelRestore(c Config, j panelRestoreJournal) error {
	if err := setPanelRestoreResult(c, j.ID, "completed", "completed"); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(c.DataDir, "backups", j.ID, "panel-restore.payload")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(panelRestorePath(c)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return backup.SyncDir(filepath.Join(c.DataDir, "backups"))
}

func setPanelRestoreResult(c Config, id, status, stage string) error {
	path := filepath.Join(c.DataDir, "backups", id, "record.json")
	data, err := backup.ReadFile(path, 32<<10)
	if err != nil {
		return err
	}
	var r backup.Record
	if backup.Decode(data, &r) != nil || r.ID != id {
		return backup.ErrInvalid
	}
	r.Status = status
	r.Stage = stage
	r.ErrorCode = stage
	if status == "completed" {
		r.ErrorCode = ""
		if !slices.Contains(r.CompletedModules, "panel") {
			r.CompletedModules = append(r.CompletedModules, "panel")
		}
	} else if len(r.CompletedModules) > 0 {
		r.ErrorCode = "partially_restored"
	}
	return backup.WriteJSON(path, r)
}
