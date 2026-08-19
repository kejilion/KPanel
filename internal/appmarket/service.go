package appmarket

import (
	"bufio"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/dockerx"
)

const maxAppMarkersBytes = 64 << 10

type Docker interface {
	Containers(context.Context) ([]contract.ContainerSummary, error)
	Lifecycle(context.Context, string, string, string) (dockerx.ActionResult, error)
	LifecycleDeclarativeApp(context.Context, dockerx.DeclarativeAppSpec, string, string) (dockerx.ActionResult, error)
	InstallDeclarativeApp(context.Context, dockerx.DeclarativeAppSpec, uint16, string) (dockerx.AppMutationResult, error)
	UpdateDeclarativeApp(context.Context, dockerx.DeclarativeAppSpec, string) (dockerx.AppMutationResult, error)
	SetDeclarativeAppAccess(context.Context, dockerx.DeclarativeAppSpec, string, string) (dockerx.AppMutationResult, error)
	UninstallDeclarativeApp(context.Context, dockerx.DeclarativeAppSpec, string) (dockerx.AppMutationResult, error)
	CheckContainerImageUpdate(context.Context, string, string) (dockerx.ImageUpdateResult, error)
}

type Capability struct {
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason,omitempty"`
}

type Runtime struct {
	Installed       bool                   `json:"installed"`
	State           string                 `json:"state"`
	Status          string                 `json:"status,omitempty"`
	ContainerID     string                 `json:"containerId,omitempty"`
	ContainerName   string                 `json:"containerName,omitempty"`
	Image           string                 `json:"image,omitempty"`
	Ports           []contract.PortBinding `json:"ports"`
	AccessMode      string                 `json:"accessMode"`
	UpdateStatus    string                 `json:"updateStatus"`
	ResourceVersion string                 `json:"resourceVersion,omitempty"`
	DetectedBy      []string               `json:"detectedBy"`
	Warning         string                 `json:"warning,omitempty"`
}

type Summary struct {
	App
	DefaultPort             int                   `json:"defaultPort,omitempty"`
	InstallPortConfigurable bool                  `json:"installPortConfigurable,omitempty"`
	Installer               string                `json:"installer"`
	Runtime                 Runtime               `json:"runtime"`
	Capabilities            map[string]Capability `json:"capabilities"`
}

type Inventory struct {
	SchemaVersion      int        `json:"schemaVersion"`
	Source             string     `json:"source"`
	ScriptSHA256       string     `json:"scriptSha256"`
	CatalogMode        string     `json:"catalogMode"`
	CatalogWarning     string     `json:"catalogWarning,omitempty"`
	CatalogRefreshedAt *time.Time `json:"catalogRefreshedAt,omitempty"`
	Categories         []Category `json:"categories"`
	Items              []Summary  `json:"items"`
	Installed          int        `json:"installed"`
	Running            int        `json:"running"`
	UpdateAvailable    int        `json:"updateAvailable"`
	CollectedAt        time.Time  `json:"collectedAt"`
}

type declarativeSpec struct {
	Token         string
	ContainerName string
	Image         string
	ContainerPort uint16
	DefaultPort   uint16
}

var declarativeSpecs = map[string]declarativeSpec{
	"speedtest": {
		Token: "speedtest", ContainerName: "speedtest",
		Image: "ghcr.io/librespeed/speedtest", ContainerPort: 8080, DefaultPort: 8028,
	},
	"it-tools": {
		Token: "it-tools", ContainerName: "it-tools",
		Image: "corentinth/it-tools:latest", ContainerPort: 80, DefaultPort: 8064,
	},
	"dosgame": {
		Token: "dosgame", ContainerName: "dosgame",
		Image: "oldiy/dosgame-web-docker:latest", ContainerPort: 262, DefaultPort: 8076,
	},
}

type Service struct {
	catalog                       Catalog
	legacy                        map[int]LegacyApp
	scriptSHA256                  string
	docker                        Docker
	appRoot                       string
	scriptAppRoot                 string
	now                           func() time.Time
	fetchCatalog                  catalogFetcher
	iconCache                     *officialIconCache
	dynamicIconSources            map[string]string
	catalogMu                     sync.Mutex
	liveCatalog                   *Catalog
	catalogExpiry                 time.Time
	catalogRefreshedAt            time.Time
	catalogWarning                string
	catalogLoading                bool
	actions                       sync.Mutex
	jobs                          *appJobRegistry
	jobExecutable                 string
	jobRunner                     jobCommandRunner
	scriptInteractiveFinder       func() (string, error)
	scriptInteractiveManageFinder func() (string, error)
	scriptManageFinder            func() (string, error)
	fileOwnerTrusted              func(os.FileInfo) bool
	listeningPorts                func(context.Context) (map[uint16][]string, error)
}

func New(docker Docker, appRoot string) (*Service, error) {
	return newService(docker, appRoot, nil)
}

func NewWithOfficialCatalog(docker Docker, appRoot string) (*Service, error) {
	return newService(docker, appRoot, newOfficialCatalogFetcher())
}

func newService(docker Docker, appRoot string, fetcher catalogFetcher) (*Service, error) {
	if docker == nil {
		return nil, errors.New("Docker client is required")
	}
	catalog, legacy, scriptSHA256, err := LoadCatalog()
	if err != nil {
		return nil, err
	}
	if appRoot == "" {
		appRoot = "/home/docker"
	}
	return &Service{
		catalog: catalog, legacy: legacy, scriptSHA256: scriptSHA256,
		docker: docker, appRoot: filepath.Clean(appRoot), now: time.Now,
		scriptAppRoot: "/root/apps", fetchCatalog: fetcher,
		dynamicIconSources:            make(map[string]string),
		scriptInteractiveFinder:       findKejilionInteractiveScript,
		scriptInteractiveManageFinder: findKejilionInteractiveManageScript,
		scriptManageFinder:            findKejilionManageScript,
		fileOwnerTrusted:              trustedFileOwner,
		listeningPorts:                systemListeningPorts,
	}, nil
}

func (s *Service) Inventory(ctx context.Context) (Inventory, error) {
	catalogState := s.currentCatalog(ctx)
	containers, err := s.docker.Containers(ctx)
	if err != nil {
		return Inventory{}, err
	}
	markers, markerWarning := s.readMarkers()
	byName := make(map[string]contract.ContainerSummary, len(containers))
	for _, container := range containers {
		byName[container.Name] = container
	}

	result := Inventory{
		SchemaVersion: 1, Source: catalogState.Catalog.Source, ScriptSHA256: s.scriptSHA256,
		CatalogMode: catalogState.Mode, CatalogWarning: catalogState.Warning,
		Categories: append([]Category(nil), catalogState.Catalog.Categories...),
		Items:      make([]Summary, 0, len(catalogState.Catalog.Apps)), CollectedAt: s.now().UTC(),
	}
	if !catalogState.RefreshedAt.IsZero() {
		refreshedAt := catalogState.RefreshedAt
		result.CatalogRefreshedAt = &refreshedAt
	}
	scriptInstallAvailable := s.scriptInstallAvailable()
	scriptManageAvailable := s.scriptManageAvailable()
	scriptMarkerRecoveryAvailable := s.scriptInteractiveManageAvailable()
	for _, app := range catalogState.Catalog.Apps {
		legacy := s.legacy[app.Num]
		item := Summary{
			App:         app,
			DefaultPort: legacy.DefaultPort,
			Installer:   installerKind(app, legacy, scriptInstallAvailable),
			Runtime: Runtime{
				State: "not_installed", AccessMode: "not_applicable",
				UpdateStatus: "not_installed", Ports: []contract.PortBinding{}, DetectedBy: []string{},
			},
			Capabilities: defaultCapabilities(app, legacy, scriptInstallAvailable),
		}
		if spec, ok := declarativeSpecs[app.Token]; ok {
			item.DefaultPort = int(spec.DefaultPort)
			item.InstallPortConfigurable = true
		} else if legacy.UsesDockerApp && legacy.DefaultPort > 0 {
			item.InstallPortConfigurable = true
		}
		marker := markers[strconv.Itoa(app.Num)] || markers[app.Token]
		containerName := legacy.Container
		storageName := legacy.Container
		scriptBacked := legacy.UsesDockerApp
		configVerified := false
		if legacy.Service != "" {
			containerName = legacy.Service
		}
		if app.Source == "thirdparty" {
			storageName = app.Token
			spec, configErr := s.readThirdPartyScriptSpec(app.Token)
			if configErr == nil {
				containerName = spec.runtimeContainer()
				storageName = spec.Container
				scriptBacked = true
				configVerified = true
				if spec.Port > 0 {
					item.DefaultPort = int(spec.Port)
					item.InstallPortConfigurable = true
				}
			} else if marker {
				scriptBacked = true
				item.Runtime.Warning = "应用配置使用动态写法；KPanel 继续复用 kejilion.sh 原生管理流程"
			}
		}
		container, hasContainer, detectedBy := resolveAppContainer(
			app,
			containerName,
			containers,
			byName,
			marker || configVerified,
		)
		if hasContainer {
			item.Runtime = runtimeFromContainer(container)
			item.Runtime.DetectedBy = append(item.Runtime.DetectedBy, "docker", detectedBy)
			if marker {
				item.Runtime.DetectedBy = append(item.Runtime.DetectedBy, "appno")
			}
			if configVerified {
				item.Runtime.DetectedBy = append(item.Runtime.DetectedBy, "app_config")
			}
			if scriptBacked {
				item.Runtime.AccessMode = "unknown"
				if mode, ok := s.readScriptAccessMode(storageName); ok {
					item.Runtime.AccessMode = mode
					item.Runtime.DetectedBy = append(item.Runtime.DetectedBy, "access_state")
				}
			}
			scriptManage := scriptBacked && scriptManageAvailable
			s.applyInstalledCapabilities(&item, container, scriptManage)
		} else if marker {
			item.Runtime.Installed = true
			item.Runtime.State = "unknown"
			item.Runtime.UpdateStatus = "unknown"
			item.Runtime.ResourceVersion = markerResourceVersion(item.App, s.scriptSHA256)
			item.Runtime.DetectedBy = []string{"appno"}
			if item.Runtime.Warning == "" {
				item.Runtime.Warning = "kejilion.sh 安装标记存在，但 Docker Engine 中未发现运行产物"
			}
			disableInstalledCapabilities(&item, "Docker Engine 中没有可执行生命周期操作的容器")
			_, markerRecoveryEligible := s.scriptSelectorFor(item)
			if markerRecoveryEligible && scriptMarkerRecoveryAvailable {
				item.Capabilities["manage"] = Capability{Enabled: true}
			} else if markerRecoveryEligible {
				item.Capabilities["manage"] = Capability{
					Reason: "请更新本机 kejilion.sh 以启用安装标记恢复协议",
				}
			}
		}
		if markerWarning != "" && item.Runtime.Warning == "" {
			item.Runtime.Warning = markerWarning
		}
		if item.Runtime.Installed {
			result.Installed++
		}
		if item.Runtime.State == "running" {
			result.Running++
		}
		if item.Runtime.UpdateStatus == "available" {
			result.UpdateAvailable++
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func resolveAppContainer(
	app App,
	preferredName string,
	containers []contract.ContainerSummary,
	byName map[string]contract.ContainerSummary,
	allowEcosystemFallback bool,
) (contract.ContainerSummary, bool, string) {
	if preferredName != "" {
		if container, ok := byName[preferredName]; ok {
			return container, true, "configured_name"
		}
	}
	if !allowEcosystemFallback {
		return contract.ContainerSummary{}, false, ""
	}

	names := make(map[string]bool, 2)
	for _, value := range []string{app.Token, app.Slug} {
		if value = strings.ToLower(strings.TrimSpace(value)); value != "" {
			names[value] = true
		}
	}
	bestScore := 0
	best := contract.ContainerSummary{}
	bestDetector := ""
	for _, container := range containers {
		score, detector := appContainerMatchScore(app, container, names)
		if score > bestScore ||
			(score == bestScore && score > 0 && strings.Compare(container.Name, best.Name) < 0) {
			bestScore = score
			best = container
			bestDetector = detector
		}
	}
	return best, bestScore > 0, bestDetector
}

func appContainerMatchScore(
	app App,
	container contract.ContainerSummary,
	names map[string]bool,
) (int, string) {
	labels := container.Labels
	if strings.EqualFold(labels["io.kejilion.panel.app"], app.Token) ||
		strings.EqualFold(labels["io.kejilion.app"], app.Token) {
		return 100, "app_label"
	}
	for _, key := range []string{"com.docker.compose.project", "com.docker.compose.service"} {
		if value := strings.ToLower(strings.TrimSpace(labels[key])); value != "" && names[value] {
			return 90, "compose_label"
		}
	}
	if value := strings.ToLower(strings.TrimSpace(container.Name)); value != "" && names[value] {
		return 80, "ecosystem_name"
	}
	workdir := filepath.ToSlash(labels["com.docker.compose.project.working_dir"])
	if value := strings.ToLower(strings.TrimSpace(filepath.Base(workdir))); value != "" && names[value] {
		return 70, "compose_workdir"
	}
	return 0, ""
}

func installerKind(app App, _ LegacyApp, scriptInstallAvailable bool) string {
	if _, ok := declarativeSpecs[app.Token]; ok {
		return "declarative"
	}
	if scriptInstallAvailable && (app.Source == "builtin" || app.Source == "thirdparty") {
		return "kejilion"
	}
	return "guided"
}

func (s *Service) currentCatalog(_ context.Context) catalogSnapshot {
	if s.fetchCatalog == nil {
		return catalogSnapshot{Catalog: s.catalog, Mode: "embedded"}
	}
	s.catalogMu.Lock()
	now := s.now().UTC()
	if s.liveCatalog != nil && now.Before(s.catalogExpiry) {
		mode := "live"
		if s.catalogWarning != "" {
			mode = "cached"
		}
		snapshot := catalogSnapshot{
			Catalog: *s.liveCatalog, Mode: mode, Warning: s.catalogWarning,
			RefreshedAt: s.catalogRefreshedAt,
		}
		s.catalogMu.Unlock()
		return snapshot
	}
	if s.liveCatalog == nil && now.Before(s.catalogExpiry) && s.catalogWarning != "" {
		snapshot := catalogSnapshot{Catalog: s.catalog, Mode: "embedded", Warning: s.catalogWarning}
		s.catalogMu.Unlock()
		return snapshot
	}
	snapshot := catalogSnapshot{Catalog: s.catalog, Mode: "embedded", Warning: s.catalogWarning}
	if s.liveCatalog != nil {
		snapshot = catalogSnapshot{
			Catalog: *s.liveCatalog, Mode: "cached", Warning: s.catalogWarning,
			RefreshedAt: s.catalogRefreshedAt,
		}
	}
	if !s.catalogLoading {
		s.catalogLoading = true
		go s.refreshCatalog()
	}
	s.catalogMu.Unlock()
	return snapshot
}

func (s *Service) refreshCatalog() {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	remote, err := s.fetchCatalog(ctx)
	cancel()
	now := s.now().UTC()
	var merged Catalog
	var dynamicSources map[string]string
	if err == nil {
		s.catalogMu.Lock()
		iconCache := s.iconCache
		var previous Catalog
		if s.liveCatalog != nil {
			previous = *s.liveCatalog
		}
		s.catalogMu.Unlock()
		dynamicSources = dynamicRemoteIconSources(s.catalog, remote)
		merged = mergeRemoteCatalogWithDynamicIcons(s.catalog, remote, iconCache != nil)
		preserveExistingAddedDates(previous, &merged)
		if iconCache != nil && len(dynamicSources) > 0 {
			iconContext, iconCancel := context.WithTimeout(context.Background(), 8*time.Second)
			iconCache.Prefetch(iconContext, dynamicSources)
			iconCancel()
		}
		if iconCache != nil {
			iconCache.Prune(dynamicSources)
		}
	}
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	s.catalogLoading = false
	if err == nil {
		s.liveCatalog = &merged
		s.dynamicIconSources = dynamicSources
		s.catalogExpiry = now.Add(remoteCatalogTTL)
		s.catalogRefreshedAt = now
		s.catalogWarning = ""
		return
	}
	slog.Warn("application catalog refresh failed", "error", err)
	s.catalogWarning = "动态目录暂不可用，已使用最近一次安全目录。"
	s.catalogExpiry = now.Add(time.Minute)
}

func runtimeFromContainer(container contract.ContainerSummary) Runtime {
	access := "unknown"
	for _, port := range container.Ports {
		if port.PublicPort == 0 {
			continue
		}
		switch port.IP {
		case "127.0.0.1", "::1":
			access = "domain_only"
		case "", "0.0.0.0", "::":
			if access != "domain_only" {
				access = "direct"
			}
		}
	}
	return Runtime{
		Installed: true, State: container.State, Status: container.Status,
		ContainerID: container.ID, ContainerName: container.Name, Image: container.Image,
		Ports: append([]contract.PortBinding{}, container.Ports...), AccessMode: access,
		UpdateStatus: "check_required", ResourceVersion: container.ResourceVersion,
	}
}

func defaultCapabilities(
	app App,
	_ LegacyApp,
	scriptInstallAvailable bool,
) map[string]Capability {
	reason := "该应用需要专属配置向导，暂不能无人值守安装"
	install := Capability{Reason: reason}
	if _, ok := declarativeSpecs[app.Token]; ok {
		install = Capability{Enabled: true}
	} else if scriptInstallAvailable &&
		(app.Source == "builtin" || app.Source == "thirdparty") {
		install = Capability{Enabled: true}
	} else if app.Source == "builtin" || app.Source == "thirdparty" {
		install = Capability{Reason: "请先更新 kejilion.sh，并在终端运行一次 k 接受许可协议"}
	}
	return map[string]Capability{
		"install":       install,
		"start":         {Reason: "应用尚未安装"},
		"stop":          {Reason: "应用尚未安装"},
		"restart":       {Reason: "应用尚未安装"},
		"check_update":  {Reason: "应用尚未安装"},
		"update":        {Reason: "应用尚未安装"},
		"uninstall":     {Reason: "应用尚未安装"},
		"add_domain":    {Reason: "应用尚未安装或没有 HTTP 端口"},
		"direct_access": {Reason: "应用尚未安装"},
		"manage":        {Reason: "应用尚未安装"},
	}
}

func (s *Service) applyInstalledCapabilities(
	item *Summary,
	container contract.ContainerSummary,
	scriptManage bool,
) {
	item.Capabilities["install"] = Capability{Reason: "应用已安装"}
	for _, action := range container.AllowedActions {
		if action == "start" || action == "stop" || action == "restart" {
			item.Capabilities[action] = Capability{Enabled: true}
		}
	}
	if !item.Capabilities["start"].Enabled {
		item.Capabilities["start"] = Capability{Reason: "当前状态不允许启动"}
	}
	if !item.Capabilities["stop"].Enabled {
		item.Capabilities["stop"] = Capability{Reason: "当前状态不允许停止"}
	}
	if !item.Capabilities["restart"].Enabled {
		item.Capabilities["restart"] = Capability{Reason: "当前状态不允许重启"}
	}
	canCheckUpdate := item.Runtime.Image != "" &&
		(!strings.HasPrefix(item.Runtime.Image, "sha256:") ||
			container.Labels["io.kejilion.panel.image"] != "")
	item.Capabilities["check_update"] = Capability{
		Enabled: canCheckUpdate,
		Reason:  reasonUnless(canCheckUpdate, "未识别可查询的镜像标签"),
	}
	if hasHTTPPort(item.Runtime.Ports) {
		item.Capabilities["add_domain"] = Capability{Enabled: true}
	}
	if spec, ok := declarativeSpecs[item.Token]; ok && container.Name == spec.ContainerName {
		switch container.State {
		case "running":
			item.Capabilities["stop"] = Capability{Enabled: true}
			item.Capabilities["restart"] = Capability{Enabled: true}
		case "created", "exited", "dead":
			item.Capabilities["start"] = Capability{Enabled: true}
		}
		item.Capabilities["uninstall"] = Capability{Enabled: true}
		if declarativeRuntimePort(container, spec) {
			item.Capabilities["update"] = Capability{Enabled: true}
			item.Capabilities["direct_access"] = Capability{Enabled: true}
		} else {
			reason := "未发现该应用约定的 TCP 端口映射"
			item.Capabilities["update"] = Capability{Reason: reason}
			item.Capabilities["direct_access"] = Capability{Reason: reason}
		}
		item.Capabilities["manage"] = Capability{Reason: "该应用使用 KPanel 统一管理架构"}
	} else if scriptManage {
		item.Capabilities["update"] = Capability{Enabled: true}
		item.Capabilities["uninstall"] = Capability{Enabled: true}
		item.Capabilities["direct_access"] = Capability{Enabled: true}
		item.Capabilities["manage"] = Capability{
			Reason: "已发现应用容器，请使用面板提供的生命周期操作",
		}
	} else {
		reason := "请更新本机 kejilion.sh 以启用应用非交互管理协议"
		item.Capabilities["update"] = Capability{Reason: reason}
		item.Capabilities["uninstall"] = Capability{Reason: reason}
		item.Capabilities["direct_access"] = Capability{Reason: reason}
		item.Capabilities["manage"] = Capability{
			Reason: "请更新本机 kejilion.sh 以启用应用交互管理协议",
		}
	}
}

func disableInstalledCapabilities(item *Summary, reason string) {
	item.Capabilities["install"] = Capability{Reason: "检测到 kejilion.sh 安装标记"}
	for _, action := range []string{"start", "stop", "restart", "check_update", "update", "uninstall", "add_domain", "direct_access", "manage"} {
		item.Capabilities[action] = Capability{Reason: reason}
	}
}

func markerResourceVersion(app App, scriptSHA256 string) string {
	sum := sha256.Sum256([]byte(app.ID + "\x00" + scriptSHA256))
	return fmt.Sprintf("marker:sha256:%x", sum)
}

func declarativeRuntimePort(container contract.ContainerSummary, spec declarativeSpec) bool {
	for _, port := range container.Ports {
		if port.PrivatePort == spec.ContainerPort && port.PublicPort > 0 && port.Type == "tcp" {
			return true
		}
	}
	return false
}

func hasHTTPPort(ports []contract.PortBinding) bool {
	for _, port := range ports {
		if port.Type == "tcp" && port.PublicPort > 0 {
			return true
		}
	}
	return false
}

func reasonUnless(ok bool, reason string) string {
	if ok {
		return ""
	}
	return reason
}

func (s *Service) readMarkers() (map[string]bool, string) {
	result := make(map[string]bool)
	path := filepath.Join(s.appRoot, "appno.txt")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return result, ""
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxAppMarkersBytes {
		return result, "无法安全读取 /home/docker/appno.txt"
	}
	file, err := os.Open(path)
	if err != nil {
		return result, "无法读取 /home/docker/appno.txt"
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), maxAppMarkersBytes)
	for scanner.Scan() {
		value := strings.TrimSpace(scanner.Text())
		if value != "" && tokenPattern.MatchString(value) {
			result[value] = true
		}
	}
	if scanner.Err() != nil {
		return result, "读取 /home/docker/appno.txt 时发生错误"
	}
	return result, ""
}

func (s *Service) Find(ctx context.Context, id string) (Summary, error) {
	inventory, err := s.Inventory(ctx)
	if err != nil {
		return Summary{}, err
	}
	for _, item := range inventory.Items {
		if item.ID == id {
			return item, nil
		}
	}
	return Summary{}, ErrNotFound
}

func (s *Service) Lifecycle(ctx context.Context, id, action, expectedVersion string) (dockerx.ActionResult, error) {
	if action != "start" && action != "stop" && action != "restart" {
		return dockerx.ActionResult{}, ErrUnsupported
	}
	item, err := s.Find(ctx, id)
	if err != nil {
		return dockerx.ActionResult{}, err
	}
	if !item.Capabilities[action].Enabled {
		return dockerx.ActionResult{}, fmt.Errorf("%w: %s", ErrForbidden, item.Capabilities[action].Reason)
	}
	if spec, ok := declarativeSpecs[item.Token]; ok {
		return s.docker.LifecycleDeclarativeApp(
			ctx,
			dockerSpec(spec),
			action,
			expectedVersion,
		)
	}
	return s.docker.Lifecycle(ctx, item.Runtime.ContainerID, action, expectedVersion)
}

type InstallInput struct {
	HostPort    uint16 `json:"hostPort"`
	AccessMode  string `json:"accessMode"`
	Interactive bool   `json:"-"`
}

type MutationInput struct {
	ResourceVersion string `json:"resourceVersion"`
	AccessMode      string `json:"accessMode,omitempty"`
}

func (s *Service) Install(ctx context.Context, id string, input InstallInput) (dockerx.AppMutationResult, error) {
	s.actions.Lock()
	defer s.actions.Unlock()
	item, err := s.Find(ctx, id)
	if err != nil {
		return dockerx.AppMutationResult{}, err
	}
	if !item.Capabilities["install"].Enabled {
		return dockerx.AppMutationResult{}, fmt.Errorf("%w: %s", ErrForbidden, item.Capabilities["install"].Reason)
	}
	spec, ok := declarativeSpecs[item.Token]
	if !ok {
		return dockerx.AppMutationResult{}, ErrUnsupported
	}
	if input.HostPort == 0 {
		input.HostPort = spec.DefaultPort
	}
	if input.AccessMode == "" {
		input.AccessMode = "direct"
	}
	portStatus, portErr := s.inspectInstallPort(ctx, input.HostPort)
	if portErr != nil {
		return dockerx.AppMutationResult{}, fmt.Errorf(
			"%w: host port validation failed: %v",
			ErrNeedsAttention,
			portErr,
		)
	}
	if !portStatus.Available {
		return dockerx.AppMutationResult{}, fmt.Errorf(
			"%w: host port %d is already bound by another listener or container",
			ErrPortConflict,
			input.HostPort,
		)
	}
	result, err := s.docker.InstallDeclarativeApp(ctx, dockerSpec(spec), input.HostPort, input.AccessMode)
	if err != nil {
		return dockerx.AppMutationResult{}, err
	}
	if err := s.addCompatibilityFiles(item, spec, input.HostPort); err != nil {
		_, rollbackErr := s.docker.UninstallDeclarativeApp(ctx, dockerSpec(spec), result.ResourceVersion)
		if rollbackErr != nil {
			return dockerx.AppMutationResult{}, fmt.Errorf("%w: compatibility write failed and container rollback failed: %v", ErrNeedsAttention, rollbackErr)
		}
		return dockerx.AppMutationResult{}, fmt.Errorf("%w: compatibility write failed; container rolled back: %v", ErrRolledBack, err)
	}
	return result, nil
}

func (s *Service) Mutate(ctx context.Context, id, action string, input MutationInput) (dockerx.AppMutationResult, error) {
	s.actions.Lock()
	defer s.actions.Unlock()
	item, err := s.Find(ctx, id)
	if err != nil {
		return dockerx.AppMutationResult{}, err
	}
	if !item.Capabilities[action].Enabled {
		return dockerx.AppMutationResult{}, fmt.Errorf("%w: %s", ErrForbidden, item.Capabilities[action].Reason)
	}
	spec, ok := declarativeSpecs[item.Token]
	if !ok {
		return dockerx.AppMutationResult{}, ErrUnsupported
	}
	switch action {
	case "update":
		return s.docker.UpdateDeclarativeApp(ctx, dockerSpec(spec), input.ResourceVersion)
	case "direct_access":
		return s.docker.SetDeclarativeAppAccess(ctx, dockerSpec(spec), input.ResourceVersion, input.AccessMode)
	case "uninstall":
		result, uninstallErr := s.docker.UninstallDeclarativeApp(ctx, dockerSpec(spec), input.ResourceVersion)
		if uninstallErr != nil {
			return dockerx.AppMutationResult{}, uninstallErr
		}
		if cleanupErr := s.removeCompatibilityFiles(item, spec); cleanupErr != nil {
			return dockerx.AppMutationResult{}, fmt.Errorf("%w: container removed but compatibility marker cleanup failed: %v", ErrNeedsAttention, cleanupErr)
		}
		return result, nil
	default:
		return dockerx.AppMutationResult{}, ErrUnsupported
	}
}

func (s *Service) CheckUpdate(
	ctx context.Context,
	id, expectedVersion string,
) (dockerx.ImageUpdateResult, error) {
	if expectedVersion == "" {
		return dockerx.ImageUpdateResult{}, dockerx.ErrVersionRequired
	}
	item, err := s.Find(ctx, id)
	if err != nil {
		return dockerx.ImageUpdateResult{}, err
	}
	if !item.Capabilities["check_update"].Enabled {
		return dockerx.ImageUpdateResult{}, fmt.Errorf(
			"%w: %s",
			ErrForbidden,
			item.Capabilities["check_update"].Reason,
		)
	}
	result, err := s.docker.CheckContainerImageUpdate(
		ctx,
		item.Runtime.ContainerID,
		item.Runtime.ResourceVersion,
	)
	if !errors.Is(err, dockerx.ErrResourceConflict) {
		return result, err
	}

	// Image inspection is read-only. A container may restart or be recreated
	// between the inventory snapshot and Docker's second inspect, so refresh
	// once instead of surfacing a harmless stale-browser conflict.
	refreshed, refreshErr := s.Find(ctx, id)
	if refreshErr != nil {
		return dockerx.ImageUpdateResult{}, refreshErr
	}
	if !refreshed.Capabilities["check_update"].Enabled {
		return dockerx.ImageUpdateResult{}, fmt.Errorf(
			"%w: %s",
			ErrForbidden,
			refreshed.Capabilities["check_update"].Reason,
		)
	}
	return s.docker.CheckContainerImageUpdate(
		ctx,
		refreshed.Runtime.ContainerID,
		refreshed.Runtime.ResourceVersion,
	)
}

func dockerSpec(spec declarativeSpec) dockerx.DeclarativeAppSpec {
	return dockerx.DeclarativeAppSpec{
		Token: spec.Token, ContainerName: spec.ContainerName,
		Image: spec.Image, ContainerPort: spec.ContainerPort,
	}
}

func (s *Service) addCompatibilityFiles(item Summary, spec declarativeSpec, hostPort uint16) error {
	portPath := filepath.Join(s.appRoot, spec.ContainerName+"_port.conf")
	portData := []byte(strconv.Itoa(int(hostPort)) + "\n")
	portBackup, portStaged, err := s.stageCompatibilityPath(portPath)
	if err != nil {
		return err
	}
	if err := s.writeCompatibilityFile(portPath, portData); err != nil {
		if rollbackErr := rollbackCompatibilityPath(portPath, portBackup, portStaged); rollbackErr != nil {
			return fmt.Errorf("%w; restore previous port artifact: %v", err, rollbackErr)
		}
		return err
	}
	if err := s.rewriteMarkers(func(values []string) []string {
		marker := strconv.Itoa(item.Num)
		for _, value := range values {
			if value == marker {
				return values
			}
		}
		return append(values, marker)
	}); err != nil {
		if rollbackErr := rollbackCompatibilityPath(portPath, portBackup, portStaged); rollbackErr != nil {
			return fmt.Errorf("%w; restore previous port artifact: %v", err, rollbackErr)
		}
		return err
	}
	discardCompatibilityBackup(portBackup, portStaged)
	return nil
}

func (s *Service) removeCompatibilityFiles(item Summary, spec declarativeSpec) error {
	path := filepath.Join(s.appRoot, spec.ContainerName+"_port.conf")
	portBackup, portStaged, err := s.stageCompatibilityPath(path)
	if err != nil {
		return err
	}
	if err := s.rewriteMarkers(func(values []string) []string {
		marker := strconv.Itoa(item.Num)
		result := make([]string, 0, len(values))
		for _, value := range values {
			if value != marker && value != item.Token {
				result = append(result, value)
			}
		}
		return result
	}); err != nil {
		if rollbackErr := rollbackCompatibilityPath(path, portBackup, portStaged); rollbackErr != nil {
			return fmt.Errorf("%w; restore previous port artifact: %v", err, rollbackErr)
		}
		return err
	}
	discardCompatibilityBackup(portBackup, portStaged)
	return nil
}

func (s *Service) stageCompatibilityPath(path string) (string, bool, error) {
	if filepath.Dir(path) != s.appRoot {
		return "", false, errors.New("invalid compatibility file path")
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if info.IsDir() {
		return "", false, errors.New("compatibility file path is a directory")
	}
	placeholder, err := os.CreateTemp(s.appRoot, ".kpanel-app-backup-*")
	if err != nil {
		return "", false, err
	}
	backupPath := placeholder.Name()
	if err := placeholder.Close(); err != nil {
		_ = os.Remove(backupPath)
		return "", false, err
	}
	if err := os.Remove(backupPath); err != nil {
		return "", false, err
	}
	if err := os.Rename(path, backupPath); err != nil {
		return "", false, err
	}
	return backupPath, true, nil
}

func rollbackCompatibilityPath(path, backupPath string, staged bool) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if !staged {
		return nil
	}
	return os.Rename(backupPath, path)
}

func discardCompatibilityBackup(path string, staged bool) {
	if !staged {
		return
	}
	if err := os.Remove(path); err != nil {
		slog.Warn("application compatibility backup cleanup failed", "path", path, "error", err)
	}
}

func (s *Service) rewriteMarkers(change func([]string) []string) error {
	if err := s.validateAppRoot(); err != nil {
		return err
	}
	path := filepath.Join(s.appRoot, "appno.txt")
	values := []string{}
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > maxAppMarkersBytes {
			return errors.New("appno.txt is not a safe regular file")
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
			value := strings.TrimSpace(line)
			if value != "" {
				values = append(values, value)
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	values = change(values)
	return s.writeCompatibilityFile(path, []byte(strings.Join(values, "\n")+"\n"))
}

func (s *Service) writeCompatibilityFile(path string, data []byte) error {
	if filepath.Dir(path) != s.appRoot || len(data) > maxAppMarkersBytes {
		return errors.New("invalid compatibility file path")
	}
	temp, err := os.CreateTemp(s.appRoot, ".kpanel-app-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	ok := false
	defer func() {
		_ = temp.Close()
		if !ok {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o644); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	ok = true
	return nil
}

func (s *Service) validateAppRoot() error {
	if !filepath.IsAbs(s.appRoot) || s.appRoot == string(filepath.Separator) {
		return errors.New("application root is unsafe")
	}
	info, err := os.Lstat(s.appRoot)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("application root is unavailable or unsafe")
	}
	return nil
}

func SortCapabilities(values map[string]Capability) []string {
	result := make([]string, 0, len(values))
	for action, capability := range values {
		if capability.Enabled {
			result = append(result, action)
		}
	}
	sort.Strings(result)
	return result
}

var (
	ErrNotFound       = errors.New("application not found")
	ErrForbidden      = errors.New("application action is not available for the current runtime state")
	ErrUnsupported    = errors.New("application action is not supported")
	ErrConflict       = errors.New("application state changed; refresh and retry")
	ErrPortConflict   = errors.New("application port is already in use")
	ErrTaskConflict   = errors.New("another application task is already running")
	ErrRolledBack     = errors.New("application action failed and was rolled back")
	ErrNeedsAttention = errors.New("application action requires manual attention")
)
