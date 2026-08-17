package dockerx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const (
	maxJSONBytes = 16 << 20
	maxLogBytes  = 1 << 20
	maxLogTail   = 1000
)

var containerIDPattern = regexp.MustCompile(`^[a-fA-F0-9]{12,64}$`)

type Client struct {
	httpClient            *http.Client
	baseURL               string
	webRoot               string
	appRoot               string
	stateRoot             string
	daemonConfigPath      string
	restartDocker         func(context.Context) error
	hostCommand           func(context.Context, string, ...string) ([]byte, error)
	composeCommand        func(context.Context, ...string) ([]byte, error)
	iptablesRulesPath     string
	now                   func() time.Time
	lifecycle             sync.Mutex
	jobStart              sync.Mutex
	jobs                  *dockerJobRegistry
	pidFile               string
	allowSocketActivation bool
	imageUpdateCountry    func(context.Context) (string, error)
}

type ImageSummary struct {
	ID              string   `json:"id"`
	RepoTags        []string `json:"repoTags"`
	RepoDigests     []string `json:"repoDigests"`
	CreatedAt       int64    `json:"createdAt"`
	SizeBytes       int64    `json:"sizeBytes"`
	Containers      int64    `json:"containers"`
	ResourceVersion string   `json:"resourceVersion"`
}

type NetworkSummary struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Driver          string    `json:"driver"`
	Scope           string    `json:"scope"`
	Internal        bool      `json:"internal"`
	Attachable      bool      `json:"attachable"`
	ContainerCount  int       `json:"containerCount"`
	CreatedAt       time.Time `json:"createdAt,omitempty"`
	ResourceVersion string    `json:"resourceVersion"`
}

type VolumeSummary struct {
	Name            string `json:"name"`
	Driver          string `json:"driver"`
	Scope           string `json:"scope"`
	Mountpoint      string `json:"mountpoint,omitempty"`
	CreatedAt       string `json:"createdAt,omitempty"`
	ResourceVersion string `json:"resourceVersion"`
}

type Logs struct {
	ContainerID string    `json:"containerId"`
	Lines       []string  `json:"lines"`
	Tail        int       `json:"tail"`
	Truncated   bool      `json:"truncated"`
	CollectedAt time.Time `json:"collectedAt"`
}

type ActionResult struct {
	ContainerID     string `json:"containerId"`
	Action          string `json:"action"`
	Status          string `json:"status"`
	ResourceVersion string `json:"resourceVersion"`
}

func New(socketPath, webRoot, stateRoot string) *Client {
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
	client := &Client{
		baseURL:           "http://docker",
		webRoot:           cleanLinuxPath(webRoot, "/home/web"),
		appRoot:           "/home/docker",
		stateRoot:         cleanLinuxPath(stateRoot, "/var/lib/kejilion-panel"),
		daemonConfigPath:  "/etc/docker/daemon.json",
		restartDocker:     restartDockerDaemon,
		hostCommand:       runFixedDockerHostCommand,
		composeCommand:    runFixedDockerComposeCommand,
		iptablesRulesPath: "/etc/iptables/rules.v4",
		now:               time.Now,
		pidFile:           "/run/docker.pid",
	}
	transport := &http.Transport{
		DisableCompression: true,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			if !client.allowSocketActivation && !daemonProcessRunning(client.pidFile) {
				return nil, ErrDockerNotRunning
			}
			return dialer.DialContext(ctx, "unix", socketPath)
		},
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 3 * time.Second,
	}
	client.httpClient = &http.Client{Transport: transport, Timeout: 15 * time.Second}
	return client
}

// ConfigureDaemonAccess keeps observation side-effect free by default. When
// socket activation is disabled, a live dockerd process matching pidFile must
// be present before a Unix Socket connection is attempted.
func (c *Client) ConfigureDaemonAccess(pidFile string, allowSocketActivation bool) {
	c.pidFile = cleanLinuxPath(pidFile, "/run/docker.pid")
	c.allowSocketActivation = allowSocketActivation
}

// ConfigureImageUpdateFallback enables the fixed Docker Hub acceleration
// fallback used by update checks. The resolver is deliberately configured by
// the Agent so update checks can reuse its cached public-network identity
// instead of creating a second country lookup or background poller.
func (c *Client) ConfigureImageUpdateFallback(
	country func(context.Context) (string, error),
) {
	c.imageUpdateCountry = country
}

func (c *Client) Ping(ctx context.Context) error {
	var value string
	if err := c.getText(ctx, "/_ping", 64, &value); err != nil {
		return err
	}
	if strings.TrimSpace(value) != "OK" {
		return fmt.Errorf("unexpected Docker ping response")
	}
	return nil
}

func (c *Client) Summary(ctx context.Context) (contract.DockerSummary, error) {
	var raw struct {
		ServerVersion     string `json:"ServerVersion"`
		Containers        int    `json:"Containers"`
		ContainersRunning int    `json:"ContainersRunning"`
		ContainersPaused  int    `json:"ContainersPaused"`
		ContainersStopped int    `json:"ContainersStopped"`
		Images            int    `json:"Images"`
	}
	if err := c.getJSON(ctx, "/info", &raw); err != nil {
		return contract.DockerSummary{Available: false, CollectedAt: c.now().UTC()}, err
	}
	return contract.DockerSummary{
		Available:     true,
		ServerVersion: raw.ServerVersion,
		Containers:    raw.Containers,
		Running:       raw.ContainersRunning,
		Paused:        raw.ContainersPaused,
		Stopped:       raw.ContainersStopped,
		Images:        raw.Images,
		CollectedAt:   c.now().UTC(),
	}, nil
}

func (c *Client) Containers(ctx context.Context) ([]contract.ContainerSummary, error) {
	var raw []containerListItem
	if err := c.getJSON(ctx, "/containers/json?all=1&size=0", &raw); err != nil {
		return nil, err
	}
	result := make([]contract.ContainerSummary, len(raw))
	workers := min(4, len(raw))
	if workers == 0 {
		return result, nil
	}
	indexes := make(chan int)
	var group sync.WaitGroup
	group.Add(workers)
	for range workers {
		go func() {
			defer group.Done()
			for index := range indexes {
				item := raw[index]
				summary := c.summaryFromList(item)
				inspect, err := c.inspect(ctx, item.ID)
				if err == nil {
					summary = c.summaryFromInspect(inspect)
				} else {
					summary.AllowedActions = []string{}
					summary.OwnershipEvidence = append(summary.OwnershipEvidence, "容器详情读取失败，暂时无法确定可用动作")
				}
				result[index] = summary
			}
		}()
	}
	for index := range raw {
		select {
		case indexes <- index:
		case <-ctx.Done():
			close(indexes)
			group.Wait()
			return nil, ctx.Err()
		}
	}
	close(indexes)
	group.Wait()
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (c *Client) Images(ctx context.Context) ([]ImageSummary, error) {
	var raw []struct {
		ID          string   `json:"Id"`
		RepoTags    []string `json:"RepoTags"`
		RepoDigests []string `json:"RepoDigests"`
		Created     int64    `json:"Created"`
		Size        int64    `json:"Size"`
		Containers  int64    `json:"Containers"`
	}
	if err := c.getJSON(ctx, "/images/json?all=0", &raw); err != nil {
		return nil, err
	}
	result := make([]ImageSummary, 0, len(raw))
	for _, item := range raw {
		version := resourceHash(item)
		result = append(result, ImageSummary{
			ID: item.ID, RepoTags: nonNil(item.RepoTags), RepoDigests: nonNil(item.RepoDigests),
			CreatedAt: item.Created, SizeBytes: item.Size, Containers: item.Containers,
			ResourceVersion: version,
		})
	}
	return result, nil
}

func (c *Client) Networks(ctx context.Context) ([]NetworkSummary, error) {
	var raw []struct {
		Name       string                 `json:"Name"`
		ID         string                 `json:"Id"`
		Created    time.Time              `json:"Created"`
		Scope      string                 `json:"Scope"`
		Driver     string                 `json:"Driver"`
		Internal   bool                   `json:"Internal"`
		Attachable bool                   `json:"Attachable"`
		Containers map[string]interface{} `json:"Containers"`
	}
	if err := c.getJSON(ctx, "/networks", &raw); err != nil {
		return nil, err
	}
	result := make([]NetworkSummary, 0, len(raw))
	for _, item := range raw {
		result = append(result, NetworkSummary{
			ID: item.ID, Name: item.Name, Driver: item.Driver, Scope: item.Scope,
			Internal: item.Internal, Attachable: item.Attachable, CreatedAt: item.Created,
			ContainerCount: len(item.Containers), ResourceVersion: resourceHash(item),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (c *Client) Volumes(ctx context.Context) ([]VolumeSummary, error) {
	var raw struct {
		Volumes []struct {
			Name       string `json:"Name"`
			Driver     string `json:"Driver"`
			Mountpoint string `json:"Mountpoint"`
			CreatedAt  string `json:"CreatedAt"`
			Scope      string `json:"Scope"`
		} `json:"Volumes"`
	}
	if err := c.getJSON(ctx, "/volumes", &raw); err != nil {
		return nil, err
	}
	result := make([]VolumeSummary, 0, len(raw.Volumes))
	for _, item := range raw.Volumes {
		result = append(result, VolumeSummary{
			Name: item.Name, Driver: item.Driver, Mountpoint: item.Mountpoint,
			CreatedAt: item.CreatedAt, Scope: item.Scope, ResourceVersion: resourceHash(item),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (c *Client) ContainerLogs(ctx context.Context, id string, tail int) (Logs, error) {
	if !containerIDPattern.MatchString(id) {
		return Logs{}, errors.New("invalid container id")
	}
	if _, err := c.inspect(ctx, id); err != nil {
		return Logs{}, err
	}
	if tail <= 0 {
		tail = 200
	}
	if tail > maxLogTail {
		tail = maxLogTail
	}
	query := url.Values{
		"stdout":     {"1"},
		"stderr":     {"1"},
		"timestamps": {"1"},
		"tail":       {strconv.Itoa(tail)},
	}
	data, truncated, err := c.getBytes(ctx, "/containers/"+id+"/logs?"+query.Encode(), maxLogBytes)
	if err != nil {
		return Logs{}, err
	}
	plain := demuxDockerStream(data)
	return Logs{
		ContainerID: id,
		Lines:       redactLines(plain, tail),
		Tail:        tail,
		Truncated:   truncated,
		CollectedAt: c.now().UTC(),
	}, nil
}

func (c *Client) Lifecycle(ctx context.Context, id, action, expectedVersion string) (ActionResult, error) {
	if !containerIDPattern.MatchString(id) {
		return ActionResult{}, errors.New("invalid container id")
	}
	if action != "start" && action != "stop" && action != "restart" &&
		action != "pause" && action != "unpause" && action != "remove" {
		return ActionResult{}, errors.New("unsupported lifecycle action")
	}
	if expectedVersion == "" {
		return ActionResult{}, ErrVersionRequired
	}
	c.lifecycle.Lock()
	defer c.lifecycle.Unlock()
	inspect, err := c.inspect(ctx, id)
	if err != nil {
		return ActionResult{}, err
	}
	summary := c.summaryFromInspect(inspect)
	if !contains(summary.AllowedActions, action) {
		return ActionResult{}, ErrActionUnsupported
	}
	if summary.ResourceVersion != expectedVersion {
		return ActionResult{}, ErrResourceConflict
	}
	endpoint := "/containers/" + id + "/" + action
	method := http.MethodPost
	if action == "remove" {
		endpoint = "/containers/" + id + "?v=0&force=1"
		method = http.MethodDelete
	}
	if action == "stop" || action == "restart" {
		endpoint += "?t=10"
	}
	var mutationErr error
	if method == http.MethodDelete {
		mutationErr = c.dockerMutation(ctx, method, endpoint, nil)
	} else {
		mutationErr = c.post(ctx, endpoint)
	}
	if mutationErr != nil {
		return ActionResult{}, mutationErr
	}
	newVersion := summary.ResourceVersion
	if action == "remove" {
		newVersion = ""
	} else if updated, inspectErr := c.inspect(ctx, id); inspectErr == nil {
		newVersion = c.summaryFromInspect(updated).ResourceVersion
	}
	return ActionResult{
		ContainerID: id, Action: action, Status: "completed", ResourceVersion: newVersion,
	}, nil
}

var (
	ErrVersionRequired   = errors.New("resourceVersion is required")
	ErrRuntimeContract   = errors.New("container does not match the required runtime contract")
	ErrActionUnsupported = errors.New("container configuration or state does not support this action")
	ErrResourceConflict  = errors.New("container resourceVersion changed")
	ErrDockerNotRunning  = errors.New("Docker daemon is not already running; socket activation is disabled")
)

type containerListItem struct {
	ID      string   `json:"Id"`
	Names   []string `json:"Names"`
	Image   string   `json:"Image"`
	ImageID string   `json:"ImageID"`
	Command string   `json:"Command"`
	Created int64    `json:"Created"`
	State   string   `json:"State"`
	Status  string   `json:"Status"`
	Ports   []struct {
		IP          string `json:"IP"`
		PrivatePort uint16 `json:"PrivatePort"`
		PublicPort  uint16 `json:"PublicPort"`
		Type        string `json:"Type"`
	} `json:"Ports"`
	Labels          map[string]string `json:"Labels"`
	Mounts          []dockerMount     `json:"Mounts"`
	NetworkSettings struct {
		Networks map[string]interface{} `json:"Networks"`
	} `json:"NetworkSettings"`
}

type containerInspect struct {
	ID           string   `json:"Id"`
	Image        string   `json:"Image"`
	Name         string   `json:"Name"`
	Created      string   `json:"Created"`
	Path         string   `json:"Path"`
	Args         []string `json:"Args"`
	RestartCount int      `json:"RestartCount"`
	Config       struct {
		Image        string                 `json:"Image"`
		Labels       map[string]string      `json:"Labels"`
		ExposedPorts map[string]interface{} `json:"ExposedPorts"`
		Env          []string               `json:"Env"`
		Tty          bool                   `json:"Tty"`
	} `json:"Config"`
	State struct {
		Status     string `json:"Status"`
		Running    bool   `json:"Running"`
		Paused     bool   `json:"Paused"`
		Restarting bool   `json:"Restarting"`
		ExitCode   int    `json:"ExitCode"`
		StartedAt  string `json:"StartedAt"`
		FinishedAt string `json:"FinishedAt"`
		Health     *struct {
			Status string `json:"Status"`
		} `json:"Health"`
	} `json:"State"`
	HostConfig struct {
		Binds   []string `json:"Binds"`
		CapAdd  []string `json:"CapAdd"`
		Devices []struct {
			PathOnHost string `json:"PathOnHost"`
		} `json:"Devices"`
		Privileged    bool     `json:"Privileged"`
		NetworkMode   string   `json:"NetworkMode"`
		PidMode       string   `json:"PidMode"`
		IpcMode       string   `json:"IpcMode"`
		UTSMode       string   `json:"UTSMode"`
		UsernsMode    string   `json:"UsernsMode"`
		SecurityOpt   []string `json:"SecurityOpt"`
		RestartPolicy struct {
			Name              string `json:"Name"`
			MaximumRetryCount int    `json:"MaximumRetryCount"`
		} `json:"RestartPolicy"`
	} `json:"HostConfig"`
	Mounts          []dockerMount `json:"Mounts"`
	NetworkSettings struct {
		Networks map[string]dockerNetworkEndpoint `json:"Networks"`
		Ports    map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		} `json:"Ports"`
	} `json:"NetworkSettings"`
}

type dockerNetworkEndpoint struct {
	IPAddress         string `json:"IPAddress"`
	GlobalIPv6Address string `json:"GlobalIPv6Address"`
}

type dockerMount struct {
	Type        string `json:"Type"`
	Name        string `json:"Name"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	RW          bool   `json:"RW"`
}

func (c *Client) summaryFromList(raw containerListItem) contract.ContainerSummary {
	name := strings.TrimPrefix(first(raw.Names), "/")
	ownership, evidence := c.ownership(raw.Labels, name)
	ports := make([]contract.PortBinding, 0, len(raw.Ports))
	for _, port := range raw.Ports {
		ports = append(ports, contract.PortBinding{
			PrivatePort: port.PrivatePort, PublicPort: port.PublicPort, IP: port.IP, Type: port.Type,
		})
	}
	var mounts []contract.Mount
	for _, mount := range raw.Mounts {
		mounts = append(mounts, contract.Mount{
			Type: mount.Type, Name: mount.Name, Source: mount.Source,
			Destination: mount.Destination, ReadOnly: !mount.RW,
		})
	}
	networks := sortedKeys(raw.NetworkSettings.Networks)
	version := resourceHash(struct {
		ID, ImageID, State, Status string
		Labels                     map[string]string
		Mounts                     []dockerMount
	}{raw.ID, raw.ImageID, raw.State, raw.Status, raw.Labels, raw.Mounts})
	return contract.ContainerSummary{
		ID: raw.ID, Name: name, Image: raw.Image, State: raw.State, Status: raw.Status,
		Ports: ports, Mounts: mounts, Networks: networks,
		ComposeProject: raw.Labels["com.docker.compose.project"],
		ComposeService: raw.Labels["com.docker.compose.service"],
		Ownership:      ownership, OwnershipEvidence: evidence, ResourceVersion: version,
		AllowedActions: []string{}, Labels: raw.Labels,
	}
}

func (c *Client) summaryFromInspect(raw containerInspect) contract.ContainerSummary {
	name := strings.TrimPrefix(raw.Name, "/")
	ownership, evidence := c.ownership(raw.Config.Labels, name)
	var mounts []contract.Mount
	for _, mount := range raw.Mounts {
		mounts = append(mounts, contract.Mount{
			Type: mount.Type, Name: mount.Name, Source: mount.Source,
			Destination: mount.Destination, ReadOnly: !mount.RW,
		})
	}
	var ports []contract.PortBinding
	for key, bindings := range raw.NetworkSettings.Ports {
		port, protocol, ok := strings.Cut(key, "/")
		if !ok {
			continue
		}
		private, _ := strconv.ParseUint(port, 10, 16)
		if len(bindings) == 0 {
			ports = append(ports, contract.PortBinding{PrivatePort: uint16(private), Type: protocol})
		}
		for _, binding := range bindings {
			public, _ := strconv.ParseUint(binding.HostPort, 10, 16)
			ports = append(ports, contract.PortBinding{
				PrivatePort: uint16(private), PublicPort: uint16(public), IP: binding.HostIP, Type: protocol,
			})
		}
	}
	health := ""
	if raw.State.Health != nil {
		health = raw.State.Health.Status
	}
	version := resourceHash(struct {
		ID, Name, Created, Image, State, StartedAt, FinishedAt string
		RestartCount                                           int
		Restarting                                             bool
		Labels                                                 map[string]string
		HostConfig                                             interface{}
		Mounts                                                 []dockerMount
	}{
		raw.ID, raw.Name, raw.Created, raw.Config.Image, raw.State.Status,
		raw.State.StartedAt, raw.State.FinishedAt, raw.RestartCount, raw.State.Restarting,
		raw.Config.Labels, raw.HostConfig, raw.Mounts,
	})
	allowed := []string{"logs"}
	switch raw.State.Status {
	case "running":
		allowed = append(allowed, "restart", "pause", "stop", "remove", "stats", "exec", "access")
	case "paused":
		allowed = append(allowed, "unpause", "restart", "stop", "remove", "stats", "access")
	case "restarting":
		allowed = append(allowed, "restart", "stop", "remove", "stats", "access")
	case "created", "exited", "dead":
		allowed = append(allowed, "start", "remove")
	}
	return contract.ContainerSummary{
		ID: raw.ID, Name: name, Image: raw.Config.Image,
		State: raw.State.Status, Status: raw.State.Status, Health: health,
		Ports: ports, Mounts: mounts, Networks: sortedKeys(raw.NetworkSettings.Networks),
		ComposeProject: raw.Config.Labels["com.docker.compose.project"],
		ComposeService: raw.Config.Labels["com.docker.compose.service"],
		Ownership:      ownership, OwnershipEvidence: evidence, ResourceVersion: version,
		AllowedActions: allowed, Labels: raw.Config.Labels,
	}
}

func (c *Client) ownership(labels map[string]string, containerName string) (string, []string) {
	if labels["io.kejilion.panel.managed"] == "true" {
		return "kejilion", []string{"io.kejilion.panel.managed=true"}
	}
	workdir := labels["com.docker.compose.project.working_dir"]
	if workdir != "" {
		if c.provenWithin(workdir, c.webRoot, true) {
			return "kejilion", []string{"Compose 工作目录为 " + c.webRoot}
		}
		if c.provenWithin(workdir, c.appRoot, false) {
			return "kejilion", []string{"Compose 工作目录位于 " + c.appRoot}
		}
	}
	if c.scriptContainerPortMarker(containerName) {
		return "kejilion", []string{"存在 kejilion.sh 容器端口标记"}
	}
	if workdir == "" {
		return "external", nil
	}
	return "external", []string{"Compose 工作目录不在 Kejilion 管理范围"}
}

func (c *Client) scriptContainerPortMarker(containerName string) bool {
	if !dockerNamePattern.MatchString(containerName) {
		return false
	}
	path := filepath.Join(c.appRoot, containerName+"_port.conf")
	if !c.provenWithin(filepath.ToSlash(path), c.appRoot, false) {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		info.Size() < 1 || info.Size() > 16 {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	port, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 16)
	return err == nil && port <= 65535
}

func (c *Client) provenWithin(candidate, root string, exact bool) bool {
	candidate = pathpkg.Clean(candidate)
	root = pathpkg.Clean(root)
	if !pathpkg.IsAbs(candidate) || !pathpkg.IsAbs(root) {
		return false
	}
	if exact && candidate != root {
		return false
	}
	if !exact && candidate != root && !strings.HasPrefix(candidate, root+"/") {
		return false
	}
	resolvedCandidate, errCandidate := filepath.EvalSymlinks(filepath.FromSlash(candidate))
	resolvedRoot, errRoot := filepath.EvalSymlinks(filepath.FromSlash(root))
	if errCandidate != nil || errRoot != nil {
		return false
	}
	relative, err := filepath.Rel(resolvedRoot, resolvedCandidate)
	return err == nil && (relative == "." || (!strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != ".."))
}

func (c *Client) inspect(ctx context.Context, id string) (containerInspect, error) {
	var raw containerInspect
	err := c.getJSON(ctx, "/containers/"+id+"/json", &raw)
	return raw, err
}

func (c *Client) getJSON(ctx context.Context, path string, target interface{}) error {
	data, _, err := c.getBytes(ctx, path, maxJSONBytes)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode Docker response: %w", err)
	}
	return nil
}

func (c *Client) getText(ctx context.Context, path string, limit int64, target *string) error {
	data, _, err := c.getBytes(ctx, path, limit)
	if err == nil {
		*target = string(data)
	}
	return err
}

func (c *Client) getBytes(ctx context.Context, path string, limit int64) ([]byte, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, false, err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, false, fmt.Errorf("Docker API unavailable: %w", err)
	}
	defer response.Body.Close()
	data, readErr := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if readErr != nil {
		return nil, false, fmt.Errorf("read Docker response: %w", readErr)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, false, dockerError(response.StatusCode, data)
	}
	truncated := int64(len(data)) > limit
	if truncated {
		data = data[:limit]
	}
	return data, truncated, nil
}

func (c *Client) post(ctx context.Context, path string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, http.NoBody)
	if err != nil {
		return err
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

func dockerError(status int, data []byte) error {
	var value struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(data, &value)
	if value.Message == "" {
		value.Message = http.StatusText(status)
	}
	return &APIError{Status: status, Message: redactText(value.Message)}
}

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Docker API %d: %s", e.Status, e.Message)
}

func isDockerStatus(err error, status int) bool {
	var apiError *APIError
	return errors.As(err, &apiError) && apiError.Status == status
}

func resourceHash(value interface{}) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func demuxDockerStream(data []byte) []byte {
	var output bytes.Buffer
	for len(data) >= 8 && (data[0] == 0 || data[0] == 1 || data[0] == 2) {
		length := int(binary.BigEndian.Uint32(data[4:8]))
		if length < 0 || length > len(data)-8 {
			return data
		}
		output.Write(data[8 : 8+length])
		data = data[8+length:]
	}
	if output.Len() == 0 {
		return data
	}
	if len(data) > 0 {
		output.Write(data)
	}
	return output.Bytes()
}

var (
	jsonSecretAssignment = regexp.MustCompile(`(?i)("(?:[^"\\]|\\.)*(?:password|passwd|pwd|token|secret|api[_-]?key|authorization|cookie)(?:[^"\\]|\\.)*"\s*:\s*)("(?:\\.|[^"\\])*"|[^,\s}\]]+)`)
	secretAssignment     = regexp.MustCompile(`(?i)(\b[A-Za-z0-9_.-]*(?:password|passwd|pwd|token|secret|api[_-]?key|authorization|cookie)[A-Za-z0-9_.-]*\b)(\s*[:=]\s*)("(?:\\.|[^"\\])*"|'[^']*'|[^\s,;]+)`)
	bearerSecret         = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	urlCredentials       = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^/\s:@]+:[^/\s@]+@`)
)

func redactLines(data []byte, limit int) []string {
	rawLines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(rawLines) > 0 && rawLines[len(rawLines)-1] == "" {
		rawLines = rawLines[:len(rawLines)-1]
	}
	if len(rawLines) > limit {
		rawLines = rawLines[len(rawLines)-limit:]
	}
	result := make([]string, 0, len(rawLines))
	inPrivateKey := false
	for _, line := range rawLines {
		if strings.Contains(line, "-----BEGIN") && strings.Contains(line, "PRIVATE KEY-----") {
			inPrivateKey = true
			result = append(result, "[REDACTED PRIVATE KEY]")
			continue
		}
		if inPrivateKey {
			if strings.Contains(line, "-----END") && strings.Contains(line, "PRIVATE KEY-----") {
				inPrivateKey = false
			}
			continue
		}
		line = redactText(line)
		if len(line) > 16<<10 {
			line = line[:16<<10] + "…"
		}
		result = append(result, line)
	}
	return result
}

func redactText(value string) string {
	value = jsonSecretAssignment.ReplaceAllString(value, `${1}"[REDACTED]"`)
	value = secretAssignment.ReplaceAllString(value, "${1}${2}[REDACTED]")
	value = bearerSecret.ReplaceAllString(value, "Bearer [REDACTED]")
	value = urlCredentials.ReplaceAllString(value, "${1}[REDACTED]@")
	return strings.Map(func(r rune) rune {
		if r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, value)
}

func cleanLinuxPath(value, fallback string) string {
	if value == "" {
		value = fallback
	}
	return pathpkg.Clean(value)
}

func sortedKeys[V any](values map[string]V) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func removeString(values []string, target string) []string {
	result := values[:0]
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
	return result
}
