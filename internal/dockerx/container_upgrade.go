package dockerx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (c *Client) standaloneUpgradeTarget(ctx context.Context, input MaintenanceInput) (containerInspect, error) {
	if !containerIDPattern.MatchString(input.Target) || input.ExpectedResourceVersion == "" {
		return containerInspect{}, ErrInvalidDockerJob
	}
	current, err := c.inspectVerifiedContainer(ctx, input.Target, input.ExpectedResourceVersion)
	if err != nil {
		return current, err
	}
	if current.Config.Labels["com.docker.compose.project"] != "" ||
		current.Config.Labels["com.docker.swarm.service.id"] != "" ||
		(current.State.Status != "running" && current.State.Status != "exited" && current.State.Status != "created") {
		return current, ErrActionUnsupported
	}
	image := standaloneUpgradeReference(input, current)
	if _, valid := normalizedImageRepository(image); !valid || imageIDPattern.MatchString(image) || len(image) > 512 {
		return current, fmt.Errorf("%w: container upgrade requires a mutable registry image tag", ErrActionUnsupported)
	}
	return current, nil
}

func standaloneUpgradeReference(input MaintenanceInput, current containerInspect) string {
	if reference := strings.TrimSpace(input.Image); reference != "" {
		return reference
	}
	return current.Config.Image
}

// Docker's inspect and create APIs share Config and HostConfig. Preserve all
// daemon-supported fields without projecting them through the panel form.
func (c *Client) standaloneUpgradePayload(ctx context.Context, current containerInspect, reference string) ([]byte, error) {
	var source struct {
		Config     map[string]json.RawMessage `json:"Config"`
		HostConfig map[string]json.RawMessage `json:"HostConfig"`
		Network    struct {
			Networks map[string]map[string]json.RawMessage `json:"Networks"`
		} `json:"NetworkSettings"`
	}
	if err := c.getJSON(ctx, "/containers/"+current.ID+"/json", &source); err != nil {
		return nil, err
	}
	if source.Config == nil || source.HostConfig == nil {
		return nil, ErrRuntimeContract
	}
	if string(source.HostConfig["AutoRemove"]) == "true" && current.State.Running {
		return nil, errors.New("automatic removal must be disabled to preserve the original running container")
	}
	if len(current.ID) >= 12 && string(source.Config["Hostname"]) == fmt.Sprintf("%q", current.ID[:12]) {
		delete(source.Config, "Hostname")
	}
	source.Config["Image"], _ = json.Marshal(reference)
	if err := preserveUpgradeVolumes(source.HostConfig, current.Mounts); err != nil {
		return nil, err
	}
	host, err := json.Marshal(source.HostConfig)
	if err != nil {
		return nil, err
	}
	source.Config["HostConfig"] = host

	mode := current.HostConfig.NetworkMode
	if mode != "host" && mode != "none" && !strings.HasPrefix(mode, "container:") && len(source.Network.Networks) != 0 {
		for name, endpoint := range source.Network.Networks {
			settings := map[string]json.RawMessage{}
			// IPAMConfig contains requested static addresses. IPAddress and
			// GlobalIPv6Address are observations of automatic allocation.
			for _, key := range []string{"IPAMConfig", "Aliases", "Links", "DriverOpts", "GwPriority"} {
				if value, ok := endpoint[key]; ok {
					settings[key] = value
				}
			}
			source.Network.Networks[name] = settings
		}
		networks, err := json.Marshal(map[string]any{"EndpointsConfig": source.Network.Networks})
		if err != nil {
			return nil, err
		}
		source.Config["NetworkingConfig"] = networks
	}
	return json.Marshal(source.Config)
}

func preserveUpgradeVolumes(host map[string]json.RawMessage, existing []dockerMount) error {
	var binds []string
	var configured []map[string]json.RawMessage
	if raw := host["Binds"]; raw != nil && json.Unmarshal(raw, &binds) != nil {
		return ErrRuntimeContract
	}
	if raw := host["Mounts"]; raw != nil && json.Unmarshal(raw, &configured) != nil {
		return ErrRuntimeContract
	}
	for _, mount := range existing {
		if mount.Type != "volume" {
			continue
		}
		if mount.Name == "" {
			return ErrRuntimeContract
		}
		covered := false
		for index, bind := range binds {
			source, rest, structured := strings.Cut(bind, ":")
			target, _, _ := strings.Cut(rest, ":")
			if structured && target == mount.Destination {
				covered = true
			} else if source == mount.Destination {
				binds[index] = mount.Name + ":" + bind
				covered = true
			}
		}
		for _, entry := range configured {
			var target string
			if json.Unmarshal(entry["Target"], &target) == nil && target == mount.Destination {
				entry["Source"], _ = json.Marshal(mount.Name)
				covered = true
			}
		}
		if !covered {
			source, _ := json.Marshal(mount.Name)
			target, _ := json.Marshal(mount.Destination)
			readOnly, _ := json.Marshal(!mount.RW)
			configured = append(configured, map[string]json.RawMessage{
				"Type": json.RawMessage(`"volume"`), "Source": source, "Target": target, "ReadOnly": readOnly,
			})
		}
	}
	var err error
	host["Binds"], err = json.Marshal(binds)
	if err != nil {
		return err
	}
	host["Mounts"], err = json.Marshal(configured)
	return err
}

func (c *Client) upgradeStandaloneContainer(ctx context.Context, input MaintenanceInput, jobID string) error {
	current, err := c.standaloneUpgradeTarget(ctx, input)
	if err != nil {
		return err
	}
	reference := standaloneUpgradeReference(input, current)
	if err := c.pullMaintenanceImage(ctx, reference); err != nil {
		return fmt.Errorf("pull replacement image: %w", err)
	}
	var image struct {
		ID string `json:"Id"`
	}
	if err := c.getJSON(ctx, "/images/"+url.PathEscape(reference)+"/json", &image); err != nil {
		return err
	}
	if !imageIDPattern.MatchString(image.ID) {
		return ErrRuntimeContract
	}
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	current, err = c.standaloneUpgradeTarget(ctx, input)
	if err != nil || (image.ID == current.Image && reference == current.Config.Image) {
		return err
	}
	payload, err := c.standaloneUpgradePayload(ctx, current, reference)
	if err != nil {
		return err
	}
	return c.switchStandaloneContainer(ctx, input, current, image.ID, jobID, payload)
}

func (c *Client) switchStandaloneContainer(ctx context.Context, input MaintenanceInput, original containerInspect, imageID, jobID string, payload []byte) (err error) {
	name := strings.TrimPrefix(original.Name, "/")
	stage := "kpanel-upgrade-" + jobID
	backup := stage + "-old"
	data, err := c.appDockerRequest(ctx, http.MethodPost, "/containers/create?name="+url.QueryEscape(stage), payload, 64<<10, c.httpClient)
	if err != nil {
		return fmt.Errorf("prepare replacement %s: %w", stage, err)
	}
	var replacement struct {
		ID string `json:"Id"`
	}
	if json.Unmarshal(data, &replacement) != nil || !containerIDPattern.MatchString(replacement.ID) || replacement.ID == original.ID {
		return fmt.Errorf("replacement identity unavailable; inspect %s", stage)
	}
	cutover := false
	applied := false
	defer func() {
		if err == nil || applied {
			return
		}
		recovery, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		removeErr := c.dockerMutation(recovery, http.MethodDelete, "/containers/"+replacement.ID+"?v=0&force=1", nil)
		var apiErr *APIError
		if errors.As(removeErr, &apiErr) && apiErr.Status == http.StatusNotFound {
			removeErr = nil
		}
		if cutover {
			removeErr = errors.Join(removeErr, c.restoreStandaloneContainer(recovery, original, name, backup))
		}
		if removeErr != nil {
			err = fmt.Errorf("upgrade failed; recovery needs attention (%s): %w", backup, errors.Join(err, removeErr))
		} else if cutover {
			err = fmt.Errorf("upgrade failed; original container restored: %w", err)
		}
	}()
	prepared, err := c.inspect(ctx, replacement.ID)
	if err != nil || prepared.Image != imageID || prepared.Name != "/"+stage || prepared.State.Status != "created" {
		return errors.Join(ErrResourceConflict, err)
	}
	current, err := c.standaloneUpgradeTarget(ctx, input)
	if err != nil {
		return err
	}
	latest, err := c.standaloneUpgradePayload(ctx, current, standaloneUpgradeReference(input, current))
	if err != nil || !bytes.Equal(latest, payload) {
		return errors.Join(ErrResourceConflict, err)
	}
	cutover = true
	if original.State.Running {
		stop, cancel := context.WithTimeout(ctx, 2*time.Minute)
		client := *c.httpClient
		client.Timeout = 0
		_, stopErr := c.appDockerRequest(stop, http.MethodPost, "/containers/"+original.ID+"/stop", nil, 64<<10, &client)
		cancel()
		if stopErr != nil {
			return fmt.Errorf("stop original container: %w", stopErr)
		}
	}
	if err := c.renameUpgradeContainer(ctx, original.ID, backup); err != nil {
		return fmt.Errorf("reserve original container: %w", err)
	}
	if err := c.renameUpgradeContainer(ctx, replacement.ID, name); err != nil {
		return fmt.Errorf("apply original container name: %w", err)
	}
	if original.State.Running {
		if err := c.post(ctx, "/containers/"+replacement.ID+"/start"); err != nil {
			return fmt.Errorf("start replacement container: %w", err)
		}
		if err := c.verifyStandaloneUpgradeStart(ctx, replacement.ID); err != nil {
			return err
		}
	}
	applied = true
	// An ambiguous cleanup result must never remove a working replacement.
	if cleanupErr := c.deleteContainer(ctx, original.ID); cleanupErr != nil {
		return fmt.Errorf("replacement applied; original container cleanup unconfirmed (%s): %w", backup, cleanupErr)
	}
	return nil
}

func (c *Client) renameUpgradeContainer(ctx context.Context, id, name string) error {
	return c.post(ctx, "/containers/"+id+"/rename?name="+url.QueryEscape(name))
}

func (c *Client) restoreStandaloneContainer(ctx context.Context, original containerInspect, name, backup string) error {
	current, err := c.inspect(ctx, original.ID)
	if err != nil {
		return err
	}
	switch strings.TrimPrefix(current.Name, "/") {
	case backup:
		if err := c.renameUpgradeContainer(ctx, original.ID, name); err != nil {
			return err
		}
	case name:
	default:
		return ErrResourceConflict
	}
	if original.State.Running && !current.State.Running {
		return c.post(ctx, "/containers/"+original.ID+"/start")
	}
	return nil
}

func (c *Client) verifyStandaloneUpgradeStart(ctx context.Context, id string) error {
	readiness, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-readiness.Done():
			return fmt.Errorf("replacement readiness: %w", readiness.Err())
		case <-ticker.C:
		}
		current, err := c.inspect(readiness, id)
		if err != nil {
			return err
		}
		if !current.State.Running || current.State.Restarting || (current.State.Health != nil && current.State.Health.Status == "unhealthy") {
			return errors.New("replacement did not stay running or failed its health check")
		}
		if current.State.Health == nil || current.State.Health.Status == "healthy" {
			return nil
		}
	}
}
