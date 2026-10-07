package dockerx

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ContainerRunCommand describes a container as an equivalent `docker run`.
// Docker keeps the resulting configuration, not the command line that created
// it, so this is a reconstruction: options the image already supplies are left
// out and anything that cannot be expressed is listed in Unsupported.
type ContainerRunCommand struct {
	ContainerID    string       `json:"containerId"`
	Name           string       `json:"name"`
	Image          string       `json:"image"`
	ComposeProject string       `json:"composeProject,omitempty"`
	ComposeService string       `json:"composeService,omitempty"`
	Options        []RunOption  `json:"options"`
	Command        []string     `json:"command"`
	Networks       []RunNetwork `json:"networks"`
	Unsupported    []string     `json:"unsupported"`
	ImageDefaults  bool         `json:"imageDefaults"`
	CollectedAt    time.Time    `json:"collectedAt"`
}

// RunOption is one `docker run` flag. A nil Value is a switch such as `-d`;
// an empty Value is a flag given an empty argument such as `--entrypoint ""`.
type RunOption struct {
	Flag  string  `json:"flag"`
	Value *string `json:"value,omitempty"`
}

// RunNetwork is an additional network that `docker run` cannot attach in the
// same command on every Engine version; it maps to `docker network connect`.
type RunNetwork struct {
	Name         string   `json:"name"`
	IP           string   `json:"ip,omitempty"`
	IPv6         string   `json:"ipv6,omitempty"`
	LinkLocalIPs []string `json:"linkLocalIps,omitempty"`
	Aliases      []string `json:"aliases,omitempty"`
}

type runInspect struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Image  string `json:"Image"`
	Config struct {
		Hostname     string                 `json:"Hostname"`
		Domainname   string                 `json:"Domainname"`
		User         string                 `json:"User"`
		Tty          bool                   `json:"Tty"`
		OpenStdin    bool                   `json:"OpenStdin"`
		Env          []string               `json:"Env"`
		Cmd          []string               `json:"Cmd"`
		Entrypoint   []string               `json:"Entrypoint"`
		Image        string                 `json:"Image"`
		WorkingDir   string                 `json:"WorkingDir"`
		Labels       map[string]string      `json:"Labels"`
		ExposedPorts map[string]interface{} `json:"ExposedPorts"`
		Volumes      map[string]interface{} `json:"Volumes"`
		StopSignal   string                 `json:"StopSignal"`
		StopTimeout  *int                   `json:"StopTimeout"`
		Healthcheck  *runHealthcheck        `json:"Healthcheck"`
		MacAddress   string                 `json:"MacAddress"`
	} `json:"Config"`
	HostConfig struct {
		Binds        []string `json:"Binds"`
		NetworkMode  string   `json:"NetworkMode"`
		PortBindings map[string][]struct {
			HostIP   string `json:"HostIp"`
			HostPort string `json:"HostPort"`
		} `json:"PortBindings"`
		RestartPolicy struct {
			Name              string `json:"Name"`
			MaximumRetryCount int    `json:"MaximumRetryCount"`
		} `json:"RestartPolicy"`
		AutoRemove      bool              `json:"AutoRemove"`
		VolumesFrom     []string          `json:"VolumesFrom"`
		CapAdd          []string          `json:"CapAdd"`
		CapDrop         []string          `json:"CapDrop"`
		DNS             []string          `json:"Dns"`
		DNSOptions      []string          `json:"DnsOptions"`
		DNSSearch       []string          `json:"DnsSearch"`
		ExtraHosts      []string          `json:"ExtraHosts"`
		GroupAdd        []string          `json:"GroupAdd"`
		IpcMode         string            `json:"IpcMode"`
		Links           []string          `json:"Links"`
		OomScoreAdj     int               `json:"OomScoreAdj"`
		PidMode         string            `json:"PidMode"`
		Privileged      bool              `json:"Privileged"`
		PublishAllPorts bool              `json:"PublishAllPorts"`
		ReadonlyRootfs  bool              `json:"ReadonlyRootfs"`
		SecurityOpt     []string          `json:"SecurityOpt"`
		StorageOpt      map[string]string `json:"StorageOpt"`
		Tmpfs           map[string]string `json:"Tmpfs"`
		UTSMode         string            `json:"UTSMode"`
		UsernsMode      string            `json:"UsernsMode"`
		CgroupnsMode    string            `json:"CgroupnsMode"`
		ShmSize         int64             `json:"ShmSize"`
		Sysctls         map[string]string `json:"Sysctls"`
		Runtime         string            `json:"Runtime"`
		LogConfig       struct {
			Type   string            `json:"Type"`
			Config map[string]string `json:"Config"`
		} `json:"LogConfig"`
		CgroupParent      string         `json:"CgroupParent"`
		BlkioWeight       uint16         `json:"BlkioWeight"`
		BlkioWeightDevice []interface{}  `json:"BlkioWeightDevice"`
		BlkioReadBps      []interface{}  `json:"BlkioDeviceReadBps"`
		BlkioWriteBps     []interface{}  `json:"BlkioDeviceWriteBps"`
		BlkioReadIOps     []interface{}  `json:"BlkioDeviceReadIOps"`
		BlkioWriteIOps    []interface{}  `json:"BlkioDeviceWriteIOps"`
		CPURealtimePeriod int64          `json:"CpuRealtimePeriod"`
		CPURealtimeRun    int64          `json:"CpuRealtimeRuntime"`
		CPUShares         int64          `json:"CpuShares"`
		CPUPeriod         int64          `json:"CpuPeriod"`
		CPUQuota          int64          `json:"CpuQuota"`
		CpusetCpus        string         `json:"CpusetCpus"`
		CpusetMems        string         `json:"CpusetMems"`
		NanoCPUs          int64          `json:"NanoCpus"`
		Memory            int64          `json:"Memory"`
		MemoryReservation int64          `json:"MemoryReservation"`
		MemorySwap        int64          `json:"MemorySwap"`
		MemorySwappiness  *int64         `json:"MemorySwappiness"`
		OomKillDisable    *bool          `json:"OomKillDisable"`
		PidsLimit         *int64         `json:"PidsLimit"`
		Devices           []runDevice    `json:"Devices"`
		DeviceCgroupRules []string       `json:"DeviceCgroupRules"`
		DeviceRequests    []runDeviceReq `json:"DeviceRequests"`
		Ulimits           []runUlimit    `json:"Ulimits"`
		Mounts            []runHostMount `json:"Mounts"`
		Init              *bool          `json:"Init"`
		Annotations       map[string]any `json:"Annotations"`
	} `json:"HostConfig"`
	NetworkSettings struct {
		Networks map[string]runEndpoint `json:"Networks"`
	} `json:"NetworkSettings"`
}

type runHealthcheck struct {
	Test          []string `json:"Test"`
	Interval      int64    `json:"Interval"`
	Timeout       int64    `json:"Timeout"`
	StartPeriod   int64    `json:"StartPeriod"`
	StartInterval int64    `json:"StartInterval"`
	Retries       int      `json:"Retries"`
}

type runDevice struct {
	PathOnHost        string `json:"PathOnHost"`
	PathInContainer   string `json:"PathInContainer"`
	CgroupPermissions string `json:"CgroupPermissions"`
}

type runDeviceReq struct {
	Driver       string            `json:"Driver"`
	Count        int               `json:"Count"`
	DeviceIDs    []string          `json:"DeviceIDs"`
	Capabilities [][]string        `json:"Capabilities"`
	Options      map[string]string `json:"Options"`
}

type runUlimit struct {
	Name string `json:"Name"`
	Soft int64  `json:"Soft"`
	Hard int64  `json:"Hard"`
}

type runHostMount struct {
	Type        string `json:"Type"`
	Source      string `json:"Source"`
	Target      string `json:"Target"`
	ReadOnly    bool   `json:"ReadOnly"`
	Consistency string `json:"Consistency"`
	BindOptions *struct {
		Propagation      string `json:"Propagation"`
		NonRecursive     bool   `json:"NonRecursive"`
		CreateMountpoint bool   `json:"CreateMountpoint"`
	} `json:"BindOptions"`
	VolumeOptions *struct {
		NoCopy       bool              `json:"NoCopy"`
		Labels       map[string]string `json:"Labels"`
		Subpath      string            `json:"Subpath"`
		DriverConfig *struct {
			Name    string            `json:"Name"`
			Options map[string]string `json:"Options"`
		} `json:"DriverConfig"`
	} `json:"VolumeOptions"`
	TmpfsOptions *struct {
		SizeBytes int64  `json:"SizeBytes"`
		Mode      uint32 `json:"Mode"`
	} `json:"TmpfsOptions"`
}

type runEndpoint struct {
	IPAMConfig *struct {
		IPv4Address  string   `json:"IPv4Address"`
		IPv6Address  string   `json:"IPv6Address"`
		LinkLocalIPs []string `json:"LinkLocalIPs"`
	} `json:"IPAMConfig"`
	Aliases    []string          `json:"Aliases"`
	NetworkID  string            `json:"NetworkID"`
	DriverOpts map[string]string `json:"DriverOpts"`
	GwPriority int               `json:"GwPriority"`
}

type runImageConfig struct {
	User         string                 `json:"User"`
	Env          []string               `json:"Env"`
	Cmd          []string               `json:"Cmd"`
	Entrypoint   []string               `json:"Entrypoint"`
	WorkingDir   string                 `json:"WorkingDir"`
	Labels       map[string]string      `json:"Labels"`
	ExposedPorts map[string]interface{} `json:"ExposedPorts"`
	Volumes      map[string]interface{} `json:"Volumes"`
	StopSignal   string                 `json:"StopSignal"`
	Healthcheck  *runHealthcheck        `json:"Healthcheck"`
}

type runDaemonDefaults struct {
	LogDriver string
	LogOpts   map[string]string
	Runtime   string
}

const defaultShmSize = 64 << 20

// ContainerRunCommand reads one container, its image and the daemon defaults
// and reconstructs the `docker run` options that produce the same container.
// It is read-only and never runs anything.
func (c *Client) ContainerRunCommand(ctx context.Context, id string) (ContainerRunCommand, error) {
	if !containerIDPattern.MatchString(id) {
		return ContainerRunCommand{}, errors.New("invalid container id")
	}
	var container runInspect
	if err := c.getJSON(ctx, "/containers/"+id+"/json", &container); err != nil {
		return ContainerRunCommand{}, err
	}
	var image struct {
		Config *runImageConfig `json:"Config"`
	}
	var imageConfig *runImageConfig
	if container.Image != "" {
		if err := c.getJSON(ctx, "/images/"+url.PathEscape(container.Image)+"/json", &image); err == nil && image.Config != nil {
			imageConfig = image.Config
		}
	}
	return buildRunCommand(container, imageConfig, c.runDaemonDefaults(ctx), c.now().UTC()), nil
}

func (c *Client) runDaemonDefaults(ctx context.Context) runDaemonDefaults {
	defaults := runDaemonDefaults{LogDriver: "json-file", Runtime: "runc", LogOpts: map[string]string{}}
	var info struct {
		LoggingDriver  string `json:"LoggingDriver"`
		DefaultRuntime string `json:"DefaultRuntime"`
	}
	if err := c.getJSON(ctx, "/info", &info); err == nil {
		if info.LoggingDriver != "" {
			defaults.LogDriver = info.LoggingDriver
		}
		if info.DefaultRuntime != "" {
			defaults.Runtime = info.DefaultRuntime
		}
	}
	// Engine merges daemon.json log-opts into every container that uses the
	// default driver; repeating them on each command would only add noise.
	if data, existed, err := readDockerDaemonConfig(c.daemonConfigPath); err == nil && existed {
		if config, err := parseDockerDaemonConfig(data); err == nil {
			if opts, ok := config["log-opts"].(map[string]any); ok {
				for key, value := range opts {
					if text, ok := value.(string); ok {
						defaults.LogOpts[key] = text
					}
				}
			}
		}
	}
	return defaults
}

func buildRunCommand(raw runInspect, image *runImageConfig, daemon runDaemonDefaults, now time.Time) ContainerRunCommand {
	name := strings.TrimPrefix(raw.Name, "/")
	imageRef := raw.Config.Image
	if imageRef == "" {
		imageRef = raw.Image
	}
	result := ContainerRunCommand{
		ContainerID: raw.ID, Name: name, Image: imageRef,
		ComposeProject: raw.Config.Labels["com.docker.compose.project"],
		ComposeService: raw.Config.Labels["com.docker.compose.service"],
		Options:        []RunOption{}, Command: []string{}, Networks: []RunNetwork{}, Unsupported: []string{},
		ImageDefaults: image != nil, CollectedAt: now,
	}
	if image == nil {
		image = &runImageConfig{}
	}
	host := raw.HostConfig
	add := func(flag string, value ...string) {
		if len(value) == 0 {
			result.Options = append(result.Options, RunOption{Flag: flag})
			return
		}
		text := value[0]
		result.Options = append(result.Options, RunOption{Flag: flag, Value: &text})
	}
	unsupported := func(flag string) {
		if !slices.Contains(result.Unsupported, flag) {
			result.Unsupported = append(result.Unsupported, flag)
		}
	}

	add("-d")
	switch {
	case raw.Config.OpenStdin && raw.Config.Tty:
		add("-it")
	case raw.Config.OpenStdin:
		add("-i")
	case raw.Config.Tty:
		add("-t")
	}
	if name != "" {
		add("--name", name)
	}
	networkMode := strings.TrimSpace(host.NetworkMode)
	sharedNetwork := networkMode == "host" || networkMode == "none" || strings.HasPrefix(networkMode, "container:")
	if raw.Config.Hostname != "" && !sharedNetwork && host.UTSMode != "host" &&
		!(len(raw.ID) >= 12 && raw.Config.Hostname == raw.ID[:12]) {
		add("--hostname", raw.Config.Hostname)
	}
	if raw.Config.Domainname != "" {
		add("--domainname", raw.Config.Domainname)
	}
	switch policy := host.RestartPolicy.Name; policy {
	case "", "no":
	case "on-failure":
		if host.RestartPolicy.MaximumRetryCount > 0 {
			add("--restart", "on-failure:"+strconv.Itoa(host.RestartPolicy.MaximumRetryCount))
		} else {
			add("--restart", policy)
		}
	default:
		add("--restart", policy)
	}
	if host.AutoRemove {
		add("--rm")
	}

	primary := networkMode
	if networkMode == "" || networkMode == "default" {
		primary = "bridge"
	}
	// NetworkMode may hold the network ID it was created with, while
	// NetworkSettings is keyed by name; show and match the name.
	if _, named := raw.NetworkSettings.Networks[primary]; !named && !sharedNetwork && len(primary) >= 12 {
		for _, network := range sortedKeys(raw.NetworkSettings.Networks) {
			if strings.HasPrefix(raw.NetworkSettings.Networks[network].NetworkID, primary) {
				primary = network
				break
			}
		}
	}
	if primary != "bridge" {
		add("--network", primary)
	}
	if !sharedNetwork {
		endpoint := raw.NetworkSettings.Networks[primary]
		if endpoint.IPAMConfig != nil {
			if endpoint.IPAMConfig.IPv4Address != "" {
				add("--ip", endpoint.IPAMConfig.IPv4Address)
			}
			if endpoint.IPAMConfig.IPv6Address != "" {
				add("--ip6", endpoint.IPAMConfig.IPv6Address)
			}
			for _, ip := range endpoint.IPAMConfig.LinkLocalIPs {
				add("--link-local-ip", ip)
			}
		}
		for _, alias := range userAliases(endpoint.Aliases, name, raw.ID) {
			add("--network-alias", alias)
		}
		for _, network := range sortedKeys(raw.NetworkSettings.Networks) {
			if network == primary {
				continue
			}
			endpoint := raw.NetworkSettings.Networks[network]
			extra := RunNetwork{Name: network, Aliases: userAliases(endpoint.Aliases, name, raw.ID)}
			if endpoint.IPAMConfig != nil {
				extra.IP, extra.IPv6 = endpoint.IPAMConfig.IPv4Address, endpoint.IPAMConfig.IPv6Address
				extra.LinkLocalIPs = append([]string(nil), endpoint.IPAMConfig.LinkLocalIPs...)
			}
			result.Networks = append(result.Networks, extra)
		}
	}
	for _, link := range host.Links {
		if value, ok := linkOption(link); ok {
			add("--link", value)
		}
	}

	for _, value := range publishOptions(host.PortBindings) {
		add("-p", value)
	}
	if host.PublishAllPorts {
		add("-P")
	}
	for _, port := range sortedPortKeys(raw.Config.ExposedPorts) {
		if _, fromImage := image.ExposedPorts[port]; fromImage {
			continue
		}
		if _, published := host.PortBindings[port]; published {
			continue
		}
		add("--expose", strings.TrimSuffix(port, "/tcp"))
	}

	mounted := map[string]bool{}
	for _, bind := range host.Binds {
		add("-v", bind)
		if parts := strings.Split(bind, ":"); len(parts) >= 2 {
			mounted[parts[1]] = true
		}
	}
	for _, mount := range host.Mounts {
		flag, value := mountOption(mount)
		add(flag, value)
		mounted[mount.Target] = true
	}
	for _, target := range sortedKeys(host.Tmpfs) {
		value := target
		if options := host.Tmpfs[target]; options != "" {
			value += ":" + options
		}
		add("--tmpfs", value)
		mounted[target] = true
	}
	for _, target := range sortedKeys(raw.Config.Volumes) {
		if _, fromImage := image.Volumes[target]; fromImage || mounted[target] {
			continue
		}
		add("-v", target)
	}
	for _, source := range host.VolumesFrom {
		add("--volumes-from", source)
	}

	imageEnv := map[string]bool{}
	for _, entry := range image.Env {
		imageEnv[entry] = true
	}
	for _, entry := range raw.Config.Env {
		if !imageEnv[entry] {
			add("-e", entry)
		}
	}
	for _, key := range sortedKeys(raw.Config.Labels) {
		value := raw.Config.Labels[key]
		if strings.HasPrefix(key, "com.docker.compose.") || strings.HasPrefix(key, "io.kejilion.panel.") {
			continue
		}
		if imageValue, fromImage := image.Labels[key]; fromImage && imageValue == value {
			continue
		}
		add("--label", key+"="+value)
	}

	if raw.Config.User != image.User && raw.Config.User != "" {
		add("--user", raw.Config.User)
	}
	// Newer Engines record "/" for images that leave the working directory empty.
	if workingDir(raw.Config.WorkingDir) != workingDir(image.WorkingDir) {
		add("--workdir", raw.Config.WorkingDir)
	}
	switch {
	case !slices.Equal(raw.Config.Entrypoint, image.Entrypoint),
		// An entrypoint identical to the image's still stops Docker from
		// inheriting the image command; keep it so the empty command survives.
		len(raw.Config.Entrypoint) > 0 && len(raw.Config.Cmd) == 0 && len(image.Cmd) > 0:
		entrypoint := ""
		if len(raw.Config.Entrypoint) > 0 {
			entrypoint = raw.Config.Entrypoint[0]
			result.Command = append(result.Command, raw.Config.Entrypoint[1:]...)
		}
		add("--entrypoint", entrypoint)
		result.Command = append(result.Command, raw.Config.Cmd...)
	case !slices.Equal(raw.Config.Cmd, image.Cmd):
		result.Command = append(result.Command, raw.Config.Cmd...)
	}

	if host.Privileged {
		add("--privileged")
	}
	// Engine stores capabilities as CAP_NET_ADMIN; the CLI form is NET_ADMIN.
	for _, capability := range host.CapAdd {
		add("--cap-add", strings.TrimPrefix(capability, "CAP_"))
	}
	for _, capability := range host.CapDrop {
		add("--cap-drop", strings.TrimPrefix(capability, "CAP_"))
	}
	for _, device := range host.Devices {
		add("--device", deviceOption(device))
	}
	for _, rule := range host.DeviceCgroupRules {
		add("--device-cgroup-rule", rule)
	}
	for _, request := range host.DeviceRequests {
		if value, ok := gpuOption(request); ok {
			add("--gpus", value)
		} else {
			unsupported("--gpus")
		}
	}
	// Engine appends label=disable by itself for privileged containers and
	// host PID/IPC namespaces, and does so again when the command is re-run.
	autoLabel := host.Privileged || host.PidMode == "host" || host.IpcMode == "host"
	for _, option := range host.SecurityOpt {
		if autoLabel && option == "label=disable" {
			continue
		}
		add("--security-opt", option)
	}
	if host.PidMode != "" {
		add("--pid", host.PidMode)
	}
	// "private" and "shareable" are daemon defaults that differ by Engine
	// version; only sharing with the host or another container is a choice.
	if host.IpcMode == "host" || strings.HasPrefix(host.IpcMode, "container:") || host.IpcMode == "none" {
		add("--ipc", host.IpcMode)
	}
	if host.UTSMode != "" {
		add("--uts", host.UTSMode)
	}
	if host.UsernsMode != "" {
		add("--userns", host.UsernsMode)
	}
	if host.CgroupnsMode != "" {
		add("--cgroupns", host.CgroupnsMode)
	}

	if host.Memory > 0 {
		add("--memory", byteSizeOption(host.Memory))
	}
	if host.MemoryReservation > 0 {
		add("--memory-reservation", byteSizeOption(host.MemoryReservation))
	}
	// Engine stores twice --memory as the swap limit when none was given.
	if host.MemorySwap == -1 || (host.MemorySwap > 0 && host.MemorySwap != host.Memory*2) {
		value := "-1"
		if host.MemorySwap > 0 {
			value = byteSizeOption(host.MemorySwap)
		}
		add("--memory-swap", value)
	}
	if host.MemorySwappiness != nil && *host.MemorySwappiness >= 0 {
		add("--memory-swappiness", strconv.FormatInt(*host.MemorySwappiness, 10))
	}
	if host.NanoCPUs > 0 {
		add("--cpus", strconv.FormatFloat(float64(host.NanoCPUs)/1e9, 'f', -1, 64))
	}
	if host.CPUShares != 0 && host.CPUShares != 1024 {
		add("--cpu-shares", strconv.FormatInt(host.CPUShares, 10))
	}
	if host.CPUPeriod > 0 {
		add("--cpu-period", strconv.FormatInt(host.CPUPeriod, 10))
	}
	if host.CPUQuota > 0 {
		add("--cpu-quota", strconv.FormatInt(host.CPUQuota, 10))
	}
	if host.CpusetCpus != "" {
		add("--cpuset-cpus", host.CpusetCpus)
	}
	if host.CpusetMems != "" {
		add("--cpuset-mems", host.CpusetMems)
	}
	if host.PidsLimit != nil && *host.PidsLimit > 0 {
		add("--pids-limit", strconv.FormatInt(*host.PidsLimit, 10))
	}
	if host.OomKillDisable != nil && *host.OomKillDisable {
		add("--oom-kill-disable")
	}
	if host.OomScoreAdj != 0 {
		add("--oom-score-adj", strconv.Itoa(host.OomScoreAdj))
	}
	if host.ShmSize > 0 && host.ShmSize != defaultShmSize {
		add("--shm-size", byteSizeOption(host.ShmSize))
	}
	for _, limit := range host.Ulimits {
		value := limit.Name + "=" + strconv.FormatInt(limit.Soft, 10)
		if limit.Hard != limit.Soft {
			value += ":" + strconv.FormatInt(limit.Hard, 10)
		}
		add("--ulimit", value)
	}
	for _, key := range sortedKeys(host.Sysctls) {
		add("--sysctl", key+"="+host.Sysctls[key])
	}

	for _, entry := range host.ExtraHosts {
		add("--add-host", entry)
	}
	for _, server := range host.DNS {
		add("--dns", server)
	}
	for _, domain := range host.DNSSearch {
		add("--dns-search", domain)
	}
	for _, option := range host.DNSOptions {
		add("--dns-option", option)
	}

	logDriver := host.LogConfig.Type
	if logDriver != "" && logDriver != daemon.LogDriver {
		add("--log-driver", logDriver)
	}
	for _, key := range sortedKeys(host.LogConfig.Config) {
		value := host.LogConfig.Config[key]
		if (logDriver == "" || logDriver == daemon.LogDriver) && daemon.LogOpts[key] == value {
			continue
		}
		add("--log-opt", key+"="+value)
	}

	healthOptions(raw.Config.Healthcheck, image.Healthcheck, add)
	if raw.Config.StopSignal != "" && raw.Config.StopSignal != image.StopSignal {
		add("--stop-signal", raw.Config.StopSignal)
	}
	if raw.Config.StopTimeout != nil {
		add("--stop-timeout", strconv.Itoa(*raw.Config.StopTimeout))
	}
	if host.Init != nil && *host.Init {
		add("--init")
	}
	if host.ReadonlyRootfs {
		add("--read-only")
	}
	for _, group := range host.GroupAdd {
		add("--group-add", group)
	}
	if host.Runtime != "" && host.Runtime != daemon.Runtime {
		add("--runtime", host.Runtime)
	}
	if host.CgroupParent != "" {
		add("--cgroup-parent", host.CgroupParent)
	}

	if raw.Config.MacAddress != "" {
		unsupported("--mac-address")
	}
	if len(host.StorageOpt) > 0 {
		unsupported("--storage-opt")
	}
	if host.BlkioWeight > 0 {
		unsupported("--blkio-weight")
	}
	if len(host.BlkioWeightDevice) > 0 {
		unsupported("--blkio-weight-device")
	}
	if len(host.BlkioReadBps) > 0 {
		unsupported("--device-read-bps")
	}
	if len(host.BlkioWriteBps) > 0 {
		unsupported("--device-write-bps")
	}
	if len(host.BlkioReadIOps) > 0 {
		unsupported("--device-read-iops")
	}
	if len(host.BlkioWriteIOps) > 0 {
		unsupported("--device-write-iops")
	}
	if host.CPURealtimePeriod > 0 {
		unsupported("--cpu-rt-period")
	}
	if host.CPURealtimeRun > 0 {
		unsupported("--cpu-rt-runtime")
	}
	if len(host.Annotations) > 0 {
		unsupported("--annotation")
	}
	for _, network := range sortedKeys(raw.NetworkSettings.Networks) {
		endpoint := raw.NetworkSettings.Networks[network]
		if len(endpoint.DriverOpts) > 0 {
			unsupported("--driver-opt")
		}
		if endpoint.GwPriority != 0 {
			unsupported("--gw-priority")
		}
	}
	return result
}

func workingDir(value string) string {
	if value == "" {
		return "/"
	}
	return value
}

func userAliases(aliases []string, name, id string) []string {
	result := []string{}
	for _, alias := range aliases {
		if alias == "" || alias == name || (len(id) >= 12 && (alias == id[:12] || alias == id)) {
			continue
		}
		if !slices.Contains(result, alias) {
			result = append(result, alias)
		}
	}
	return result
}

// linkOption turns Engine's "/db:/web/db" back into "db:db".
func linkOption(link string) (string, bool) {
	source, target, ok := strings.Cut(link, ":")
	if !ok {
		return "", false
	}
	source = strings.TrimPrefix(source, "/")
	alias := target[strings.LastIndex(target, "/")+1:]
	if source == "" || alias == "" {
		return "", false
	}
	if alias == source {
		return source, true
	}
	return source + ":" + alias, true
}

type publishedPort struct {
	hostIP, protocol string
	hostPort, port   int
	hostPortText     string
}

// publishOptions renders HostConfig.PortBindings, folding runs of
// consecutive ports (Engine stores "-p 8000-8010:8000-8010" as eleven
// bindings) back into ranges.
func publishOptions(bindings map[string][]struct {
	HostIP   string `json:"HostIp"`
	HostPort string `json:"HostPort"`
}) []string {
	var ports []publishedPort
	for key, entries := range bindings {
		portText, protocol, _ := strings.Cut(key, "/")
		if protocol == "" {
			protocol = "tcp"
		}
		port, err := strconv.Atoi(portText)
		if err != nil {
			continue
		}
		if len(entries) == 0 {
			ports = append(ports, publishedPort{protocol: protocol, port: port, hostPort: -1})
			continue
		}
		for _, entry := range entries {
			hostPort, err := strconv.Atoi(entry.HostPort)
			if err != nil {
				hostPort = -1
			}
			ports = append(ports, publishedPort{
				hostIP: entry.HostIP, protocol: protocol, port: port,
				hostPort: hostPort, hostPortText: entry.HostPort,
			})
		}
	}
	sort.Slice(ports, func(i, j int) bool {
		left, right := ports[i], ports[j]
		if left.protocol != right.protocol {
			return left.protocol < right.protocol
		}
		if left.hostIP != right.hostIP {
			return left.hostIP < right.hostIP
		}
		if left.port != right.port {
			return left.port < right.port
		}
		return left.hostPort < right.hostPort
	})
	var result []string
	for index := 0; index < len(ports); {
		start := ports[index]
		end := index
		for end+1 < len(ports) {
			next := ports[end+1]
			if start.hostPort < 0 || next.hostPort < 0 || next.protocol != start.protocol ||
				next.hostIP != start.hostIP || next.port != ports[end].port+1 || next.hostPort != ports[end].hostPort+1 {
				break
			}
			end++
		}
		last := ports[end]
		container := strconv.Itoa(start.port)
		published := start.hostPortText
		if start.hostPort >= 0 {
			published = strconv.Itoa(start.hostPort)
		}
		if end > index {
			container += "-" + strconv.Itoa(last.port)
			published += "-" + strconv.Itoa(last.hostPort)
		}
		value := container
		if published != "" {
			value = published + ":" + container
			if start.hostIP != "" {
				hostIP := start.hostIP
				if strings.Contains(hostIP, ":") {
					hostIP = "[" + hostIP + "]"
				}
				value = hostIP + ":" + value
			}
		} else if start.hostIP != "" {
			hostIP := start.hostIP
			if strings.Contains(hostIP, ":") {
				hostIP = "[" + hostIP + "]"
			}
			value = hostIP + "::" + container
		}
		if start.protocol != "tcp" {
			value += "/" + start.protocol
		}
		result = append(result, value)
		index = end + 1
	}
	return result
}

func sortedPortKeys(ports map[string]interface{}) []string {
	keys := sortedKeys(ports)
	sort.SliceStable(keys, func(i, j int) bool {
		left, _ := strconv.Atoi(strings.Split(keys[i], "/")[0])
		right, _ := strconv.Atoi(strings.Split(keys[j], "/")[0])
		return left < right
	})
	return keys
}

// mountOption prefers the familiar `-v` form whenever it means the same
// thing; KPanel's own create path stores pasted `-v` options as Mounts.
func mountOption(mount runHostMount) (string, string) {
	plainBind := mount.Type == "bind" && (mount.BindOptions == nil ||
		(mount.BindOptions.Propagation == "" && !mount.BindOptions.NonRecursive)) && mount.Consistency == ""
	plainVolume := mount.Type == "volume" && mount.Source != "" && mount.Consistency == "" && (mount.VolumeOptions == nil ||
		(!mount.VolumeOptions.NoCopy && len(mount.VolumeOptions.Labels) == 0 &&
			mount.VolumeOptions.Subpath == "" && mount.VolumeOptions.DriverConfig == nil))
	if plainBind || plainVolume {
		value := mount.Source + ":" + mount.Target
		if mount.ReadOnly {
			value += ":ro"
		}
		return "-v", value
	}
	parts := []string{"type=" + mount.Type}
	if mount.Source != "" {
		parts = append(parts, "source="+mount.Source)
	}
	parts = append(parts, "target="+mount.Target)
	if mount.ReadOnly {
		parts = append(parts, "readonly")
	}
	if mount.Consistency != "" {
		parts = append(parts, "consistency="+mount.Consistency)
	}
	if options := mount.BindOptions; options != nil {
		if options.Propagation != "" {
			parts = append(parts, "bind-propagation="+options.Propagation)
		}
		if options.NonRecursive {
			parts = append(parts, "bind-recursive=disabled")
		}
	}
	if options := mount.VolumeOptions; options != nil {
		if options.NoCopy {
			parts = append(parts, "volume-nocopy")
		}
		if options.Subpath != "" {
			parts = append(parts, "volume-subpath="+options.Subpath)
		}
		if options.DriverConfig != nil && options.DriverConfig.Name != "" {
			parts = append(parts, "volume-driver="+options.DriverConfig.Name)
			for _, key := range sortedKeys(options.DriverConfig.Options) {
				parts = append(parts, "volume-opt="+key+"="+options.DriverConfig.Options[key])
			}
		}
		for _, key := range sortedKeys(options.Labels) {
			parts = append(parts, "volume-label="+key+"="+options.Labels[key])
		}
	}
	if options := mount.TmpfsOptions; options != nil {
		if options.SizeBytes > 0 {
			parts = append(parts, "tmpfs-size="+strconv.FormatInt(options.SizeBytes, 10))
		}
		if options.Mode > 0 {
			parts = append(parts, fmt.Sprintf("tmpfs-mode=%o", options.Mode))
		}
	}
	// --mount is read as CSV, so a field such as volume-opt=o=addr=x,rw must
	// be quoted to stay one field.
	for index, part := range parts {
		if strings.ContainsAny(part, ",\"\n") {
			parts[index] = `"` + strings.ReplaceAll(part, `"`, `""`) + `"`
		}
	}
	return "--mount", strings.Join(parts, ",")
}

func deviceOption(device runDevice) string {
	value := device.PathOnHost
	if device.PathInContainer != "" && device.PathInContainer != device.PathOnHost {
		value += ":" + device.PathInContainer
	}
	if device.CgroupPermissions != "" && device.CgroupPermissions != "rwm" {
		value += ":" + device.CgroupPermissions
	}
	return value
}

func gpuOption(request runDeviceReq) (string, bool) {
	gpu := false
	for _, group := range request.Capabilities {
		if slices.Contains(group, "gpu") {
			gpu = true
		}
	}
	if !gpu || len(request.Options) > 0 || (request.Driver != "" && request.Driver != "nvidia") {
		return "", false
	}
	switch {
	case len(request.DeviceIDs) == 1:
		return "device=" + request.DeviceIDs[0], true
	case len(request.DeviceIDs) > 1:
		// --gpus is read as CSV; Docker documents '"device=0,1"' for several IDs.
		return `"device=` + strings.Join(request.DeviceIDs, ",") + `"`, true
	case request.Count < 0:
		return "all", true
	case request.Count > 0:
		return strconv.Itoa(request.Count), true
	}
	return "", false
}

func healthOptions(container, image *runHealthcheck, add func(string, ...string)) {
	if container == nil || reflect.DeepEqual(container, image) {
		return
	}
	if image == nil {
		image = &runHealthcheck{}
	}
	if len(container.Test) > 0 && !slices.Equal(container.Test, image.Test) {
		switch container.Test[0] {
		case "NONE":
			add("--no-healthcheck")
			return
		case "CMD-SHELL":
			add("--health-cmd", strings.Join(container.Test[1:], " "))
		case "CMD":
			add("--health-cmd", shellJoin(container.Test[1:]))
		}
	}
	durations := []struct {
		flag             string
		value, inherited int64
	}{
		{"--health-interval", container.Interval, image.Interval},
		{"--health-timeout", container.Timeout, image.Timeout},
		{"--health-start-period", container.StartPeriod, image.StartPeriod},
		{"--health-start-interval", container.StartInterval, image.StartInterval},
	}
	for _, duration := range durations {
		if duration.value > 0 && duration.value != duration.inherited {
			add(duration.flag, time.Duration(duration.value).String())
		}
	}
	if container.Retries > 0 && container.Retries != image.Retries {
		add("--health-retries", strconv.Itoa(container.Retries))
	}
}

// byteSizeOption prints a byte count with the largest exact unit Docker's
// size parser accepts.
func byteSizeOption(value int64) string {
	for _, unit := range []struct {
		suffix string
		size   int64
	}{{"g", 1 << 30}, {"m", 1 << 20}, {"k", 1 << 10}} {
		if value%unit.size == 0 {
			return strconv.FormatInt(value/unit.size, 10) + unit.suffix
		}
	}
	return strconv.FormatInt(value, 10) + "b"
}

func shellJoin(values []string) string {
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = shellQuote(value)
	}
	return strings.Join(quoted, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	safe := true
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' ||
			strings.ContainsRune("@%+=:,./_-", char)) {
			safe = false
			break
		}
	}
	if safe {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}
