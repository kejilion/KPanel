package browsercomponent

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const controlNetwork = "kejilion-panel-browser-control"

var composeProjectPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// RunNetworkReconciler restores the panel connection after a container
// replacement. It never starts Docker or recreates a browser container.
func (s *Service) RunNetworkReconciler(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	lastError := ""
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			probe, cancel := context.WithTimeout(ctx, 15*time.Second)
			err := s.reconcileNetwork(probe)
			cancel()
			if err != nil && !errors.Is(err, ErrBusy) && ctx.Err() == nil {
				if err.Error() != lastError {
					slog.Warn("browser network reconciliation failed", "error", err)
					lastError = err.Error()
				}
			} else if err == nil {
				lastError = ""
			}
			timer.Reset(30 * time.Second)
		}
	}
}

func (s *Service) reconcileNetwork(ctx context.Context) error {
	marker, err := s.readPrivate("enabled", 16)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || string(marker) != "enabled" {
		return errors.New("invalid browser enabled state")
	}
	unlock, err := lockDirectory(s.config.StateDir, "worker.lock")
	if err != nil {
		return err
	}
	defer unlock()
	// Re-read after taking the worker lock: uninstall may have removed it.
	marker, err = s.readPrivate("enabled", 16)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || string(marker) != "enabled" {
		return errors.New("invalid browser enabled state")
	}
	v, err := s.snapshot(ctx)
	if err != nil {
		return err
	}
	if !v.Supported || !v.Installed || (v.Job != nil && (v.Job.State == "queued" || v.Job.State == "running")) {
		return nil
	}
	return s.prepareNetwork(ctx)
}

// prepareNetwork connects the existing panel without recreating it. A later
// Compose update owns the same network by virtue of the standard labels.
func (s *Service) prepareNetwork(ctx context.Context) error {
	body, err := s.docker(ctx, "container", "inspect", "kejilion-panel")
	if err != nil {
		return errors.New("browser_panel_inspection_failed")
	}
	var panels []struct {
		ID     string
		Config struct{ Labels map[string]string }
		State  struct{ Running bool }
		Mounts []struct {
			Source, Destination string
			RW                  bool
		}
		NetworkSettings struct{ Networks map[string]json.RawMessage }
	}
	if json.Unmarshal(body, &panels) != nil || len(panels) != 1 || !panels[0].State.Running ||
		!composeProjectPattern.MatchString(panels[0].Config.Labels["com.docker.compose.project"]) ||
		panels[0].Config.Labels["com.docker.compose.service"] != "panel" {
		return errors.New("browser_panel_unsupported")
	}
	panel := panels[0]
	project := panel.Config.Labels["com.docker.compose.project"]
	// The existing read-only runtime mount is required to deliver the token.
	parent := filepath.Dir(s.config.RuntimeDir)
	mounted := false
	for _, m := range panel.Mounts {
		if filepath.Clean(m.Source) == parent && m.Destination == "/run/kejilion-panel" && !m.RW {
			mounted = true
		}
	}
	if !mounted {
		return errors.New("browser_panel_runtime_mount_missing")
	}
	body, err = s.docker(ctx, "network", "inspect", controlNetwork)
	if err != nil {
		if _, err = s.docker(ctx, "network", "create", "--internal", "--label", "com.docker.compose.project="+project, "--label", "com.docker.compose.network=browser-control", controlNetwork); err != nil {
			return errors.New("browser_network_create_failed")
		}
		body, err = s.docker(ctx, "network", "inspect", controlNetwork)
	}
	if err != nil {
		return errors.New("browser_network_inspection_failed")
	}
	var networks []struct {
		Internal   bool
		Driver     string
		Labels     map[string]string
		Containers map[string]struct{ Name string }
	}
	if json.Unmarshal(body, &networks) != nil || len(networks) != 1 || !networks[0].Internal || networks[0].Driver != "bridge" ||
		networks[0].Labels["com.docker.compose.project"] != project || networks[0].Labels["com.docker.compose.network"] != "browser-control" {
		return errors.New("browser_network_invalid")
	}
	for id, c := range networks[0].Containers {
		if id != panel.ID && c.Name != "kpanel-browser" {
			return errors.New("browser_network_unexpected_member")
		}
	}
	if _, ok := panel.NetworkSettings.Networks[controlNetwork]; !ok {
		if _, err = s.docker(ctx, "network", "connect", controlNetwork, panel.ID); err != nil {
			return errors.New("browser_network_connect_failed")
		}
	}
	return nil
}
