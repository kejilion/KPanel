package dockerx

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxDockerJobBytes = 128 << 10
	maxPullResponse   = 4 << 20
)

var (
	dockerJobIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
	dockerNamePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
	imageNamePattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]{0,254}(?::[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}|@sha256:[a-f0-9]{64})?$`)
	imageIDPattern     = regexp.MustCompile(`^(?:sha256:)?[a-f0-9]{64}$`)
)

var maintenanceActions = []string{
	"container_create", "compose_deploy", "compose_redeploy", "compose_start", "compose_stop", "compose_restart",
	"container_access", "image_pull", "image_remove",
	"network_create", "network_remove", "network_connect", "network_disconnect",
	"volume_create", "volume_remove", "prune", "container_prune", "image_prune",
	"network_prune", "volume_prune", "backup_create", "backup_restore", "backup_migrate",
	"daemon_mirror", "daemon_ipv6",
}

func MaintenanceActions() []string {
	return append([]string(nil), maintenanceActions...)
}

func IsMaintenanceAction(action string) bool {
	for _, candidate := range maintenanceActions {
		if action == candidate {
			return true
		}
	}
	return false
}

type MaintenanceInput struct {
	Action                   string                       `json:"action"`
	Image                    string                       `json:"image,omitempty"`
	Target                   string                       `json:"target,omitempty"`
	Name                     string                       `json:"name,omitempty"`
	Driver                   string                       `json:"driver,omitempty"`
	ContainerID              string                       `json:"containerId,omitempty"`
	ContainerResourceVersion string                       `json:"containerResourceVersion,omitempty"`
	ExpectedResourceVersion  string                       `json:"expectedResourceVersion,omitempty"`
	Confirmation             string                       `json:"confirmation,omitempty"`
	Preset                   string                       `json:"preset,omitempty"`
	Enabled                  bool                         `json:"enabled,omitempty"`
	IPv6CIDR                 string                       `json:"ipv6Cidr,omitempty"`
	Ports                    []ContainerCreatePort        `json:"ports,omitempty"`
	Mounts                   []ContainerCreateMount       `json:"mounts,omitempty"`
	Environment              []ContainerCreateEnvironment `json:"environment,omitempty"`
	Command                  []string                     `json:"command,omitempty"`
	Network                  string                       `json:"network,omitempty"`
	RestartPolicy            string                       `json:"restartPolicy,omitempty"`
	Compose                  string                       `json:"compose,omitempty"`
	ComposeEnvironment       *string                      `json:"composeEnvironment,omitempty"`
	ComposeFile              string                       `json:"composeFile,omitempty"`
	AllowedIP                string                       `json:"allowedIp,omitempty"`
	BackupID                 string                       `json:"backupId,omitempty"`
	MigrationHost            string                       `json:"migrationHost,omitempty"`
	MigrationUser            string                       `json:"migrationUser,omitempty"`
	MigrationPort            int                          `json:"migrationPort,omitempty"`
}

type MaintenanceJob struct {
	ID         string     `json:"id"`
	Action     string     `json:"action"`
	Target     string     `json:"target,omitempty"`
	Status     string     `json:"status"`
	Stage      string     `json:"stage"`
	Progress   int        `json:"progress"`
	Message    string     `json:"message,omitempty"`
	ResultPath string     `json:"resultPath,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

type dockerJobRecord struct {
	MaintenanceJob
	Input MaintenanceInput `json:"input"`
}

type dockerJobRegistry struct {
	mu       sync.Mutex
	stateDir string
	jobs     map[string]dockerJobRecord
}

func (c *Client) ConfigureJobs(stateDir string) error {
	stateDir = filepath.Clean(stateDir)
	if !filepath.IsAbs(stateDir) || stateDir == string(filepath.Separator) {
		return errors.New("Docker jobs require a dedicated absolute directory")
	}
	if err := ensureDockerJobDirectory(stateDir); err != nil {
		return err
	}
	registry := &dockerJobRegistry{stateDir: stateDir, jobs: make(map[string]dockerJobRecord)}
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if !dockerJobIDPattern.MatchString(id) {
			continue
		}
		record, readErr := registry.read(id)
		if readErr != nil {
			continue
		}
		if record.Status == "queued" || record.Status == "running" {
			finished := c.now().UTC()
			record.Status = "failed"
			record.Stage = "interrupted"
			record.Progress = 100
			record.Message = "Docker 后台任务被 Agent 或服务器重启中断，请刷新资源后重试"
			record.FinishedAt = &finished
			record.Input = MaintenanceInput{Action: record.Action, Target: record.Target}
			_ = registry.put(record)
		}
		registry.jobs[id] = record
	}
	c.jobs = registry
	return nil
}

func ensureDockerJobDirectory(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(path, 0o750); err != nil {
			return err
		}
		info, err = os.Lstat(path)
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Docker job state directory is unavailable or unsafe")
	}
	return nil
}

func (c *Client) StartMaintenance(ctx context.Context, input MaintenanceInput) (MaintenanceJob, error) {
	if c.jobs == nil {
		return MaintenanceJob{}, errors.New("Docker background jobs are unavailable")
	}
	input.Action = strings.TrimSpace(input.Action)
	if err := c.validateMaintenanceInput(ctx, input); err != nil {
		return MaintenanceJob{}, err
	}
	c.jobStart.Lock()
	defer c.jobStart.Unlock()
	if c.jobs.hasActive() {
		return MaintenanceJob{}, ErrDockerJobConflict
	}
	var identity [16]byte
	if _, err := rand.Read(identity[:]); err != nil {
		return MaintenanceJob{}, err
	}
	now := c.now().UTC()
	target := input.Target
	if input.Action == "container_create" {
		target = input.Name
		if target == "" {
			target = input.Image
		}
	} else if strings.HasPrefix(input.Action, "compose_") {
		target = input.Name
	} else if input.Image != "" {
		target = input.Image
	} else if input.Name != "" {
		target = input.Name
	}
	record := dockerJobRecord{
		MaintenanceJob: MaintenanceJob{
			ID: hex.EncodeToString(identity[:]), Action: input.Action, Target: target,
			Status: "queued", Stage: "queued", Progress: 0,
			Message: "Docker 任务已进入后台队列", CreatedAt: now,
		},
		Input: input,
	}
	if err := c.jobs.put(record); err != nil {
		return MaintenanceJob{}, err
	}
	go c.runMaintenance(record)
	return record.MaintenanceJob, nil
}

func (c *Client) MaintenanceJob(id string) (MaintenanceJob, error) {
	if c.jobs == nil || !dockerJobIDPattern.MatchString(id) {
		return MaintenanceJob{}, ErrDockerJobNotFound
	}
	record, ok := c.jobs.get(id)
	if !ok {
		return MaintenanceJob{}, ErrDockerJobNotFound
	}
	return record.MaintenanceJob, nil
}

func (c *Client) MaintenanceJobs() []MaintenanceJob {
	if c.jobs == nil {
		return []MaintenanceJob{}
	}
	records := c.jobs.list()
	result := make([]MaintenanceJob, 0, len(records))
	for _, record := range records {
		result = append(result, record.MaintenanceJob)
	}
	return result
}

func (c *Client) validateMaintenanceInput(ctx context.Context, input MaintenanceInput) error {
	if !IsMaintenanceAction(input.Action) {
		return ErrInvalidDockerJob
	}
	switch input.Action {
	case "container_create":
		if _, err := c.containerCreatePayload(ctx, input); err != nil {
			return err
		}
	case "compose_deploy":
		if err := c.validateComposeDeploymentInput(ctx, input); err != nil {
			return err
		}
	case "compose_redeploy", "compose_start", "compose_stop", "compose_restart":
		if err := c.validateExistingComposeProjectInput(ctx, input); err != nil {
			return err
		}
	case "container_access":
		allowedIP := net.ParseIP(input.AllowedIP)
		if !containerIDPattern.MatchString(input.Target) || input.ExpectedResourceVersion == "" ||
			(input.AllowedIP != "" && (allowedIP == nil || allowedIP.To4() == nil)) {
			return ErrInvalidDockerJob
		}
		if err := c.verifyContainerVersion(ctx, input.Target, input.ExpectedResourceVersion); err != nil {
			return err
		}
	case "image_pull":
		if !validImageReference(input.Image) {
			return ErrInvalidDockerJob
		}
	case "image_remove":
		if !validImageTarget(input.Target) || input.ExpectedResourceVersion == "" {
			return ErrInvalidDockerJob
		}
		if err := c.verifyImageVersion(ctx, input.Target, input.ExpectedResourceVersion); err != nil {
			return err
		}
	case "network_create":
		if !dockerNamePattern.MatchString(input.Name) ||
			(input.Driver != "" && !dockerNamePattern.MatchString(input.Driver)) {
			return ErrInvalidDockerJob
		}
	case "network_remove":
		if input.Target == "" || input.ExpectedResourceVersion == "" {
			return ErrInvalidDockerJob
		}
		if err := c.verifyNetworkVersion(ctx, input.Target, input.ExpectedResourceVersion); err != nil {
			return err
		}
	case "network_connect", "network_disconnect":
		if input.Target == "" || input.ExpectedResourceVersion == "" ||
			!containerIDPattern.MatchString(input.ContainerID) || input.ContainerResourceVersion == "" {
			return ErrInvalidDockerJob
		}
		if err := c.verifyNetworkMutation(ctx, input.Target, input.ExpectedResourceVersion, false); err != nil {
			return err
		}
		if err := c.verifyContainerVersion(ctx, input.ContainerID, input.ContainerResourceVersion); err != nil {
			return err
		}
	case "volume_create":
		if !dockerNamePattern.MatchString(input.Name) ||
			(input.Driver != "" && !dockerNamePattern.MatchString(input.Driver)) {
			return ErrInvalidDockerJob
		}
	case "volume_remove":
		if !dockerNamePattern.MatchString(input.Target) || input.ExpectedResourceVersion == "" {
			return ErrInvalidDockerJob
		}
		if err := c.verifyVolumeVersion(ctx, input.Target, input.ExpectedResourceVersion); err != nil {
			return err
		}
	case "prune", "container_prune", "image_prune", "network_prune", "volume_prune":
		// Authentication, CSRF protection and the typed action express intent.
		// Confirmation text is accepted for backward compatibility but is not an authorization gate.
	case "backup_create":
		// The backup source and destination are fixed by the Agent.
	case "backup_restore":
		if !dockerBackupIDPattern.MatchString(input.BackupID) {
			return ErrInvalidDockerJob
		}
		if _, err := c.dockerBackupPath(input.BackupID); err != nil {
			return err
		}
	case "backup_migrate":
		if !dockerBackupIDPattern.MatchString(input.BackupID) ||
			!validMigrationHost(input.MigrationHost) ||
			!migrationUserPattern.MatchString(input.MigrationUser) ||
			input.MigrationPort < 1 || input.MigrationPort > 65535 {
			return ErrInvalidDockerJob
		}
		if _, err := c.dockerBackupPath(input.BackupID); err != nil {
			return err
		}
	case "daemon_mirror":
		if input.Preset != "cn" && input.Preset != "official" {
			return ErrInvalidDockerJob
		}
	case "daemon_ipv6":
		if input.Enabled && !validDockerIPv6CIDR(input.IPv6CIDR) {
			return ErrInvalidDockerJob
		}
	default:
		return ErrInvalidDockerJob
	}
	return nil
}

func (c *Client) runMaintenance(record dockerJobRecord) {
	started := c.now().UTC()
	record.Status = "running"
	record.Stage = "executing"
	record.Progress = 15
	record.Message = dockerActionProgress(record.Action)
	record.StartedAt = &started
	if c.jobs.put(record) != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	var err error
	switch record.Action {
	case "container_create":
		err = c.createManagedContainer(ctx, record.Input)
	case "compose_deploy":
		err = c.deployComposeProject(ctx, record.Input)
	case "compose_redeploy":
		err = c.redeployComposeProject(ctx, record.Input)
	case "compose_start":
		err = c.runComposeProjectLifecycle(ctx, record.Input, "start")
	case "compose_stop":
		err = c.runComposeProjectLifecycle(ctx, record.Input, "stop")
	case "compose_restart":
		err = c.runComposeProjectLifecycle(ctx, record.Input, "restart")
	case "container_access":
		err = c.updateContainerAccess(
			ctx,
			record.Input.Target,
			record.Input.ExpectedResourceVersion,
			record.Input.Enabled,
			record.Input.AllowedIP,
		)
	case "image_pull":
		err = c.pullMaintenanceImage(ctx, record.Input.Image)
	case "image_remove":
		err = c.removeImage(ctx, record.Input.Target)
	case "network_create":
		err = c.createNetwork(ctx, record.Input.Name, record.Input.Driver)
	case "network_remove":
		err = c.removeNetwork(ctx, record.Input.Target)
	case "network_connect":
		err = c.connectNetwork(ctx, record.Input.Target, record.Input.ContainerID)
	case "network_disconnect":
		err = c.disconnectNetwork(ctx, record.Input.Target, record.Input.ContainerID)
	case "volume_create":
		err = c.createVolume(ctx, record.Input.Name, record.Input.Driver)
	case "volume_remove":
		err = c.removeVolume(ctx, record.Input.Target)
	case "prune":
		err = c.prune(ctx)
	case "container_prune":
		err = c.pruneResource(ctx, "containers")
	case "image_prune":
		err = c.pruneResource(ctx, "images")
	case "network_prune":
		err = c.pruneResource(ctx, "networks")
	case "volume_prune":
		err = c.pruneResource(ctx, "volumes")
	case "backup_create":
		record.ResultPath, err = c.createDockerBackup(ctx)
	case "backup_restore":
		err = c.restoreDockerBackup(ctx, record.Input.BackupID)
	case "backup_migrate":
		record.ResultPath, err = c.migrateDockerBackup(
			ctx,
			record.Input.BackupID,
			record.Input.MigrationHost,
			record.Input.MigrationUser,
			record.Input.MigrationPort,
		)
	case "daemon_mirror":
		err = c.updateDaemonMirrors(ctx, record.Input.Preset)
	case "daemon_ipv6":
		err = c.updateDaemonIPv6(ctx, record.Input.Enabled, record.Input.IPv6CIDR)
	default:
		err = ErrInvalidDockerJob
	}
	finished := c.now().UTC()
	record.Progress = 100
	record.FinishedAt = &finished
	if err != nil {
		record.Status = "failed"
		record.Stage = "failed"
		record.Message = safeDockerJobMessage(err)
	} else {
		record.Status = "succeeded"
		record.Stage = "completed"
		record.Message = dockerActionCompleted(record.Action)
	}
	record.Input = MaintenanceInput{Action: record.Action, Target: record.Target}
	_ = c.jobs.put(record)
}

func (c *Client) pullMaintenanceImage(ctx context.Context, reference string) error {
	image, tag := splitImageReference(reference)
	query := url.Values{"fromImage": {image}}
	if tag != "" {
		if strings.HasPrefix(tag, "sha256:") {
			query.Set("tag", tag)
		} else {
			query.Set("tag", tag)
		}
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+"/images/create?"+query.Encode(),
		http.NoBody,
	)
	if err != nil {
		return err
	}
	client := *c.httpClient
	client.Timeout = 0
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("Docker image pull failed: %w", err)
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(response.Body, maxPullResponse+1))
	if readErr != nil {
		return readErr
	}
	if len(data) > maxPullResponse {
		return errors.New("Docker image pull response exceeded the safety limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return dockerError(response.StatusCode, data)
	}
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		var event struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(line, &event) == nil && event.Error != "" {
			return errors.New(redactText(event.Error))
		}
	}
	return nil
}

func splitImageReference(reference string) (string, string) {
	if index := strings.LastIndex(reference, "@"); index > 0 {
		return reference, ""
	}
	lastSlash := strings.LastIndex(reference, "/")
	if index := strings.LastIndex(reference, ":"); index > lastSlash {
		return reference[:index], reference[index+1:]
	}
	return reference, "latest"
}

func validImageReference(value string) bool {
	value = strings.TrimSpace(value)
	repository := value
	if index := strings.LastIndex(repository, "@"); index > 0 {
		repository = repository[:index]
	} else if index := strings.LastIndex(repository, ":"); index > strings.LastIndex(repository, "/") {
		repository = repository[:index]
	}
	return value != "" && repository == strings.ToLower(repository) && imageNamePattern.MatchString(value) &&
		!strings.Contains(value, "..") && !strings.Contains(value, "//")
}

func validImageTarget(value string) bool {
	return imageIDPattern.MatchString(value) || validImageReference(value)
}

func (c *Client) verifyImageVersion(ctx context.Context, target, expected string) error {
	items, err := c.Images(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.ID == target || strings.TrimPrefix(item.ID, "sha256:") == strings.TrimPrefix(target, "sha256:") ||
			contains(item.RepoTags, target) || contains(item.RepoDigests, target) {
			if item.ResourceVersion != expected {
				return ErrResourceConflict
			}
			return nil
		}
	}
	return ErrDockerJobNotFound
}

func (c *Client) verifyNetworkVersion(ctx context.Context, target, expected string) error {
	return c.verifyNetworkMutation(ctx, target, expected, true)
}

func (c *Client) verifyNetworkMutation(ctx context.Context, target, expected string, _ bool) error {
	items, err := c.Networks(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.ID != target && item.Name != target {
			continue
		}
		if item.ResourceVersion != expected {
			return ErrResourceConflict
		}
		return nil
	}
	return ErrDockerJobNotFound
}

func (c *Client) verifyContainerVersion(ctx context.Context, id, expected string) error {
	inspect, err := c.inspect(ctx, id)
	if err != nil {
		return err
	}
	summary := c.summaryFromInspect(inspect)
	if summary.ResourceVersion != expected {
		return ErrResourceConflict
	}
	return nil
}

func (c *Client) verifyVolumeVersion(ctx context.Context, target, expected string) error {
	items, err := c.Volumes(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Name != target {
			continue
		}
		if item.ResourceVersion != expected {
			return ErrResourceConflict
		}
		return nil
	}
	return ErrDockerJobNotFound
}

func (c *Client) removeImage(ctx context.Context, target string) error {
	return c.dockerMutation(ctx, http.MethodDelete, "/images/"+url.PathEscape(target)+"?force=1&noprune=0", nil)
}

func (c *Client) createNetwork(ctx context.Context, name, driver string) error {
	if driver == "" {
		driver = "bridge"
	}
	return c.dockerMutation(ctx, http.MethodPost, "/networks/create", map[string]any{
		"Name": name, "Driver": driver, "CheckDuplicate": true,
		"Labels": map[string]string{"io.kejilion.panel.managed": "true"},
	})
}

func (c *Client) removeNetwork(ctx context.Context, target string) error {
	return c.dockerMutation(ctx, http.MethodDelete, "/networks/"+url.PathEscape(target), nil)
}

func (c *Client) connectNetwork(ctx context.Context, target, containerID string) error {
	return c.dockerMutation(ctx, http.MethodPost, "/networks/"+url.PathEscape(target)+"/connect", map[string]any{
		"Container": containerID,
	})
}

func (c *Client) disconnectNetwork(ctx context.Context, target, containerID string) error {
	return c.dockerMutation(ctx, http.MethodPost, "/networks/"+url.PathEscape(target)+"/disconnect", map[string]any{
		"Container": containerID,
		"Force":     false,
	})
}

func (c *Client) createVolume(ctx context.Context, name, driver string) error {
	if driver == "" {
		driver = "local"
	}
	return c.dockerMutation(ctx, http.MethodPost, "/volumes/create", map[string]any{
		"Name": name, "Driver": driver,
		"Labels": map[string]string{"io.kejilion.panel.managed": "true"},
	})
}

func (c *Client) removeVolume(ctx context.Context, target string) error {
	return c.dockerMutation(ctx, http.MethodDelete, "/volumes/"+url.PathEscape(target), nil)
}

func (c *Client) prune(ctx context.Context) error {
	steps := []string{
		"/containers/prune",
		dockerPruneEndpoint("images"),
		"/networks/prune",
		"/volumes/prune",
		"/build/prune?all=1",
	}
	for _, endpoint := range steps {
		if err := c.dockerMutation(ctx, http.MethodPost, endpoint, nil); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) pruneResource(ctx context.Context, resource string) error {
	switch resource {
	case "containers", "images", "networks", "volumes":
		return c.dockerMutation(ctx, http.MethodPost, dockerPruneEndpoint(resource), nil)
	default:
		return ErrInvalidDockerJob
	}
}

func dockerPruneEndpoint(resource string) string {
	endpoint := "/" + resource + "/prune"
	if resource == "images" {
		endpoint += "?" + url.Values{
			"filters": {`{"dangling":["false"]}`},
		}.Encode()
	}
	return endpoint
}

func (c *Client) createDockerBackup(ctx context.Context) (string, error) {
	sourceRoot, err := c.resolvedDockerAppRoot()
	if err != nil {
		return "", err
	}
	destinationRoot := c.dockerBackupRoot()
	if !filepath.IsAbs(sourceRoot) || sourceRoot == string(filepath.Separator) ||
		!filepath.IsAbs(destinationRoot) || destinationRoot == string(filepath.Separator) {
		return "", errors.New("Docker backup paths are unsafe")
	}
	if err := ensureDockerJobDirectory(destinationRoot); err != nil {
		return "", err
	}
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	filename := fmt.Sprintf(
		"docker-%s-%s.tar.gz",
		c.now().UTC().Format("20060102T150405Z"),
		hex.EncodeToString(suffix[:]),
	)
	temp, err := os.CreateTemp(destinationRoot, ".docker-backup-*.tmp")
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return "", err
	}
	gzipWriter := gzip.NewWriter(temp)
	tarWriter := tar.NewWriter(gzipWriter)
	var totalBytes int64
	walkErr := filepath.Walk(sourceRoot, func(path string, info os.FileInfo, walkError error) error {
		if walkError != nil {
			return walkError
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		relative, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		top := strings.Split(filepath.ToSlash(relative), "/")[0]
		if top == ".kpanel-backups" {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return nil
		}
		if info.Size() > 10<<30 || totalBytes+info.Size() > 50<<30 {
			return errors.New("Docker backup exceeds the 50 GiB safety limit")
		}
		totalBytes += info.Size()
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(filepath.Join("docker", relative))
		header.Uname = ""
		header.Gname = ""
		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		openedInfo, statErr := file.Stat()
		if statErr != nil || !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
			_ = file.Close()
			return errors.New("Docker backup source changed while reading")
		}
		_, copyErr := io.Copy(tarWriter, io.LimitReader(file, info.Size()+1))
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	closeTarErr := tarWriter.Close()
	closeGzipErr := gzipWriter.Close()
	syncErr := temp.Sync()
	closeErr := temp.Close()
	for _, candidate := range []error{walkErr, closeTarErr, closeGzipErr, syncErr, closeErr} {
		if candidate != nil {
			return "", candidate
		}
	}
	target := filepath.Join(destinationRoot, filename)
	if err := os.Rename(tempPath, target); err != nil {
		return "", err
	}
	if err := syncDirectoryPath(destinationRoot); err != nil {
		return "", err
	}
	return target, nil
}

var kejilionDockerMirrors = []string{
	"https://docker.1ms.run",
	"https://docker.m.ixdev.cn",
	"https://hub.rat.dev",
	"https://dockerproxy.net",
	"https://docker-registry.nmqu.com",
	"https://docker.amingg.com",
	"https://docker.hlmirror.com",
	"https://hub1.nat.tf",
	"https://hub2.nat.tf",
	"https://hub3.nat.tf",
	"https://docker.m.daocloud.io",
	"https://docker.kejilion.pro",
	"https://docker.367231.xyz",
	"https://hub.1panel.dev",
	"https://dockerproxy.cool",
	"https://docker.apiba.cn",
	"https://proxy.vvvv.ee",
}

func validDockerIPv6CIDR(value string) bool {
	ip, network, err := net.ParseCIDR(strings.TrimSpace(value))
	if err != nil || ip.To4() != nil || !ip.IsGlobalUnicast() {
		return false
	}
	ones, bits := network.Mask.Size()
	if bits != 128 || ones != 64 {
		return false
	}
	_, documentationNetwork, _ := net.ParseCIDR("2001:db8::/32")
	return documentationNetwork == nil || !documentationNetwork.Contains(ip)
}

func (c *Client) updateDaemonMirrors(ctx context.Context, preset string) error {
	return c.updateDaemonConfig(ctx, func(config map[string]any) {
		if preset == "cn" {
			config["registry-mirrors"] = append([]string(nil), kejilionDockerMirrors...)
			return
		}
		delete(config, "registry-mirrors")
	})
}

func (c *Client) updateDaemonIPv6(ctx context.Context, enabled bool, cidr string) error {
	return c.updateDaemonConfig(ctx, func(config map[string]any) {
		config["ipv6"] = enabled
		if enabled {
			config["fixed-cidr-v6"] = strings.TrimSpace(cidr)
			return
		}
		delete(config, "fixed-cidr-v6")
	})
}

func (c *Client) updateDaemonConfig(ctx context.Context, mutate func(map[string]any)) error {
	path := filepath.Clean(c.daemonConfigPath)
	parent := filepath.Dir(path)
	if !filepath.IsAbs(path) || path == string(filepath.Separator) ||
		!filepath.IsAbs(parent) || parent == string(filepath.Separator) {
		return errors.New("Docker daemon configuration path is unsafe")
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}

	original, existed, err := readDockerDaemonConfig(path)
	if err != nil {
		return err
	}
	config := make(map[string]any)
	if existed && len(bytes.TrimSpace(original)) > 0 {
		config, err = parseDockerDaemonConfig(original)
		if err != nil {
			return errors.New("Docker daemon.json is invalid; repair it before using KPanel")
		}
	}
	mutate(config)
	updated, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	updated = append(updated, '\n')
	if bytes.Equal(bytes.TrimSpace(original), bytes.TrimSpace(updated)) {
		return nil
	}
	if err := atomicWriteDockerConfig(path, updated); err != nil {
		return err
	}
	restart := c.restartDocker
	if restart == nil {
		restart = restartDockerDaemon
	}
	if err := restart(ctx); err == nil {
		return nil
	} else {
		restartErr := err
		rollbackErr := restoreDockerDaemonConfig(path, original, existed)
		if rollbackErr == nil {
			rollbackContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			rollbackErr = restart(rollbackContext)
		}
		if rollbackErr != nil {
			return fmt.Errorf("restart Docker failed and configuration rollback needs attention: %w", restartErr)
		}
		return fmt.Errorf("restart Docker failed; previous configuration restored: %w", restartErr)
	}
}

func readDockerDaemonConfig(path string) ([]byte, bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 1<<20 {
		return nil, false, errors.New("Docker daemon.json is unavailable or unsafe")
	}
	data, err := os.ReadFile(path)
	return data, true, err
}

func atomicWriteDockerConfig(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".daemon.json.kpanel-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o644); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		_ = os.Remove(path)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	return syncDirectoryPath(filepath.Dir(path))
}

func restoreDockerDaemonConfig(path string, original []byte, existed bool) error {
	if existed {
		return atomicWriteDockerConfig(path, original)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectoryPath(filepath.Dir(path))
}

func restartDockerDaemon(ctx context.Context) error {
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return exec.CommandContext(ctx, "systemctl", "restart", "docker").Run()
	}
	if _, err := exec.LookPath("service"); err == nil {
		return exec.CommandContext(ctx, "service", "docker", "restart").Run()
	}
	return errors.New("no supported Docker service manager was found")
}

func syncDirectoryPath(path string) error {
	if runtime.GOOS == "windows" {
		// Windows does not support fsync on directory handles. Linux keeps the
		// durability barrier used by production Agent transactions.
		return nil
	}
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func (c *Client) dockerMutation(ctx context.Context, method, endpoint string, body any) error {
	var payload io.Reader = http.NoBody
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, payload)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("Docker API unavailable: %w", err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return dockerError(response.StatusCode, data)
	}
	return nil
}

func dockerActionProgress(action string) string {
	switch action {
	case "container_create":
		return "正在创建并启动 Docker 容器"
	case "compose_deploy":
		return "正在校验并启动 Docker Compose 项目"
	case "compose_redeploy":
		return "正在校验配置并重新部署 Docker Compose 项目"
	case "compose_start":
		return "正在启动 Docker Compose 项目"
	case "compose_stop":
		return "正在停止 Docker Compose 项目"
	case "compose_restart":
		return "正在重启 Docker Compose 项目"
	case "container_access":
		return "正在更新容器外部访问规则"
	case "image_pull":
		return "正在拉取并校验 Docker 镜像"
	case "image_remove":
		return "正在强制删除 Docker 镜像"
	case "network_create", "network_remove", "network_connect", "network_disconnect":
		return "正在更新 Docker 网络"
	case "volume_create", "volume_remove":
		return "正在更新 Docker 存储卷"
	case "prune":
		return "正在清理未使用的 Docker 资源"
	case "container_prune":
		return "正在清理已停止的 Docker 容器"
	case "image_prune":
		return "正在清理未使用的 Docker 镜像"
	case "network_prune":
		return "正在清理未使用的 Docker 网络"
	case "volume_prune":
		return "正在清理未使用的 Docker 存储卷"
	case "backup_create":
		return "正在备份 /home/docker 应用配置与持久化数据"
	case "backup_restore":
		return "正在校验并还原 Docker 应用数据"
	case "backup_migrate":
		return "正在通过已配置的 SSH 密钥迁移 Docker 备份"
	case "daemon_mirror":
		return "正在更新 Docker 镜像源并重启 Docker Engine"
	case "daemon_ipv6":
		return "正在更新 Docker IPv6 配置并重启 Docker Engine"
	default:
		return "正在执行 Docker 后台任务"
	}
}

func dockerActionCompleted(action string) string {
	switch action {
	case "container_create":
		return "Docker 容器已创建并启动"
	case "compose_deploy":
		return "Docker Compose 项目已部署"
	case "compose_redeploy":
		return "Docker Compose 配置已更新并重新部署"
	case "compose_start":
		return "Docker Compose 项目已启动"
	case "compose_stop":
		return "Docker Compose 项目已停止"
	case "compose_restart":
		return "Docker Compose 项目已重启"
	case "container_access":
		return "容器外部访问规则已更新"
	case "image_pull":
		return "镜像拉取完成"
	case "image_remove":
		return "镜像删除完成"
	case "network_create":
		return "Docker 网络已创建"
	case "network_remove":
		return "Docker 网络已删除"
	case "network_connect":
		return "容器已连接到 Docker 网络"
	case "network_disconnect":
		return "容器已从 Docker 网络断开"
	case "volume_create":
		return "Docker 存储卷已创建"
	case "volume_remove":
		return "Docker 存储卷已删除"
	case "prune":
		return "Docker 未使用资源清理完成"
	case "container_prune":
		return "已停止的 Docker 容器清理完成"
	case "image_prune":
		return "未使用的 Docker 镜像清理完成"
	case "network_prune":
		return "未使用的 Docker 网络清理完成"
	case "volume_prune":
		return "未使用的 Docker 存储卷清理完成"
	case "backup_create":
		return "Docker 应用数据备份完成"
	case "backup_restore":
		return "Docker 应用数据已还原；现有同名目录未被覆盖"
	case "backup_migrate":
		return "Docker 备份已迁移到目标服务器 /tmp"
	case "daemon_mirror":
		return "Docker 镜像源已更新"
	case "daemon_ipv6":
		return "Docker IPv6 配置已更新"
	default:
		return "Docker 任务完成"
	}
}

func safeDockerJobMessage(err error) string {
	value := strings.TrimSpace(redactText(err.Error()))
	if len(value) > 400 {
		value = value[:400]
	}
	if value == "" {
		return "Docker 后台任务失败"
	}
	return value
}

func (registry *dockerJobRegistry) hasActive() bool {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for _, record := range registry.jobs {
		if record.Status == "queued" || record.Status == "running" {
			return true
		}
	}
	return false
}

func (registry *dockerJobRegistry) put(record dockerJobRecord) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if !dockerJobIDPattern.MatchString(record.ID) {
		return errors.New("invalid Docker job identity")
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > maxDockerJobBytes {
		return errors.New("Docker job state exceeds the safety limit")
	}
	temp, err := os.CreateTemp(registry.stateDir, "."+record.ID+".*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	target := registry.path(record.ID)
	if runtime.GOOS == "windows" {
		_ = os.Remove(target)
	}
	if err := os.Rename(tempPath, target); err != nil {
		return err
	}
	registry.jobs[record.ID] = record
	registry.pruneLocked()
	return nil
}

func (registry *dockerJobRegistry) read(id string) (dockerJobRecord, error) {
	if !dockerJobIDPattern.MatchString(id) {
		return dockerJobRecord{}, ErrDockerJobNotFound
	}
	info, err := os.Lstat(registry.path(id))
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		info.Size() <= 0 || info.Size() > maxDockerJobBytes {
		return dockerJobRecord{}, ErrDockerJobNotFound
	}
	data, err := os.ReadFile(registry.path(id))
	if err != nil {
		return dockerJobRecord{}, err
	}
	var record dockerJobRecord
	if json.Unmarshal(data, &record) != nil || record.ID != id {
		return dockerJobRecord{}, ErrDockerJobNotFound
	}
	return record, nil
}

func (registry *dockerJobRegistry) list() []dockerJobRecord {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	result := make([]dockerJobRecord, 0, len(registry.jobs))
	for _, record := range registry.jobs {
		result = append(result, record)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

func (registry *dockerJobRegistry) get(id string) (dockerJobRecord, bool) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	record, ok := registry.jobs[id]
	return record, ok
}

func (registry *dockerJobRegistry) path(id string) string {
	return filepath.Join(registry.stateDir, id+".json")
}

func (registry *dockerJobRegistry) pruneLocked() {
	records := make([]dockerJobRecord, 0, len(registry.jobs))
	for _, record := range registry.jobs {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt.After(records[j].CreatedAt)
	})
	if len(records) <= 100 {
		return
	}
	for _, record := range records[100:] {
		if record.Status == "queued" || record.Status == "running" {
			continue
		}
		delete(registry.jobs, record.ID)
		_ = os.Remove(registry.path(record.ID))
	}
}

var (
	ErrInvalidDockerJob  = errors.New("invalid Docker maintenance request")
	ErrDockerJobConflict = errors.New("another Docker maintenance job is already active")
	ErrDockerJobNotFound = errors.New("Docker maintenance job does not exist")
)
