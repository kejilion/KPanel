package agent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/appmarket"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/diagnostics"
	"github.com/kejilion/kejilion-panel/internal/dockerx"
	"github.com/kejilion/kejilion-panel/internal/filemanager"
	"github.com/kejilion/kejilion-panel/internal/monitoring"
	"github.com/kejilion/kejilion-panel/internal/sites"
	"github.com/kejilion/kejilion-panel/internal/systeminfo"
	"github.com/kejilion/kejilion-panel/internal/systemmanage"
	"github.com/kejilion/kejilion-panel/internal/terminal"
	"github.com/kejilion/kejilion-panel/internal/webenv"
)

const maxAgentRequestBytes = 64 << 10

type Config struct {
	Token           []byte
	Version         string
	ProtocolVersion string
	WebRoot         string
	StateDir        string
	System          *systeminfo.Collector
	SystemManager   *systemmanage.Manager
	Sites           *sites.Discoverer
	SitesManager    *sites.Manager
	Docker          *dockerx.Client
	AppMarket       *appmarket.Service
	Diagnostics     *diagnostics.Service
	WebEnvironment  *webenv.Service
	Files           *filemanager.Manager
	SiteIcons       siteIconProvider
	Monitoring      monitoringHistoryProvider
	Terminals       *terminal.Manager
	Now             func() time.Time
}

type siteIconProvider interface {
	Get(context.Context, string) (sites.SiteIcon, error)
	Appearance(context.Context, string) (sites.SiteAppearance, error)
}

type monitoringHistoryProvider interface {
	History(context.Context, string) (contract.MonitoringHistory, error)
	HistoryBetween(context.Context, string, time.Time, time.Time) (contract.MonitoringHistory, error)
}

type Server struct {
	tokenHash        [32]byte
	version          string
	protocolVersion  string
	webRoot          string
	system           *systeminfo.Collector
	systemManager    *systemmanage.Manager
	sites            *sites.Discoverer
	sitesManager     *sites.Manager
	docker           *dockerx.Client
	appMarket        *appmarket.Service
	diagnostics      *diagnostics.Service
	webEnvironment   *webenv.Service
	files            *filemanager.Manager
	siteIcons        siteIconProvider
	monitoring       monitoringHistoryProvider
	terminals        *terminal.Manager
	thumbnailGate    chan struct{}
	storageUsageGate chan struct{}
	processesGate    chan struct{}
	now              func() time.Time
}

func (s *Server) Close() {
	if s.terminals != nil {
		s.terminals.CloseAll()
	}
}

func NewServer(config Config) (*Server, error) {
	if len(config.Token) < 32 {
		return nil, errors.New("Agent token must contain at least 32 bytes")
	}
	if config.Version == "" || config.ProtocolVersion == "" {
		return nil, errors.New("Agent version and protocol version are required")
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	if config.System == nil {
		config.System = systeminfo.NewCollector()
	}
	if config.SystemManager == nil {
		config.SystemManager = systemmanage.NewManager(systemmanage.Config{Enabled: false})
	}
	if config.Sites == nil {
		config.Sites = sites.NewDiscoverer(config.WebRoot)
	}
	if config.Docker == nil {
		return nil, errors.New("Docker client is required")
	}
	if config.StateDir != "" {
		if err := config.Docker.ConfigureJobs(filepath.Join(config.StateDir, "docker-jobs")); err != nil {
			return nil, fmt.Errorf("initialize Docker job state: %w", err)
		}
	}
	if config.SitesManager == nil {
		config.SitesManager = sites.NewManager(config.WebRoot, config.Sites, config.Docker)
		if config.StateDir != "" {
			executable, executableErr := os.Executable()
			if executableErr != nil {
				return nil, fmt.Errorf("resolve Agent executable: %w", executableErr)
			}
			if err := config.SitesManager.ConfigureRecipeJobState(
				filepath.Join(config.StateDir, "site-recipe-jobs"),
				executable,
			); err != nil {
				return nil, fmt.Errorf("initialize site recipe job state: %w", err)
			}
		}
	}
	if config.AppMarket == nil {
		var err error
		config.AppMarket, err = appmarket.New(config.Docker, "/home/docker")
		if err != nil {
			return nil, fmt.Errorf("initialize application market: %w", err)
		}
	}
	if config.WebEnvironment == nil && config.StateDir != "" {
		var environmentErr error
		config.WebEnvironment, environmentErr = webenv.New(filepath.Join(config.StateDir, "environment-jobs"))
		if environmentErr != nil {
			return nil, fmt.Errorf("initialize web environment service: %w", environmentErr)
		}
	}
	if config.SiteIcons == nil && config.StateDir != "" {
		var iconErr error
		config.SiteIcons, iconErr = sites.NewIconCache(
			filepath.Join(config.StateDir, "site-icons"),
			config.Sites.Discover,
		)
		if iconErr != nil {
			return nil, fmt.Errorf("initialize site icon cache: %w", iconErr)
		}
	}
	if config.Files == nil {
		var fileErr error
		config.Files, fileErr = filemanager.New(defaultFileManagerConfig(config.StateDir))
		if fileErr != nil {
			return nil, fmt.Errorf("initialize file manager: %w", fileErr)
		}
	}
	return &Server{
		tokenHash:        sha256.Sum256(config.Token),
		version:          config.Version,
		protocolVersion:  config.ProtocolVersion,
		webRoot:          config.WebRoot,
		system:           config.System,
		systemManager:    config.SystemManager,
		sites:            config.Sites,
		sitesManager:     config.SitesManager,
		docker:           config.Docker,
		appMarket:        config.AppMarket,
		diagnostics:      config.Diagnostics,
		webEnvironment:   config.WebEnvironment,
		files:            config.Files,
		siteIcons:        config.SiteIcons,
		monitoring:       config.Monitoring,
		terminals:        config.Terminals,
		thumbnailGate:    make(chan struct{}, 2),
		storageUsageGate: make(chan struct{}, 1),
		processesGate:    make(chan struct{}, 1),
		now:              config.Now,
	}, nil
}

func defaultFileManagerConfig(stateDirectory string) filemanager.Config {
	trashDirectory := "/var/lib/kejilion-panel/file-trash"
	protectedDirectories := []string{
		"/var/lib/kejilion-panel",
		"/run/kejilion-panel",
		"/etc/kejilion-panel",
		"/home/docker/kpanel/secrets",
		"/home/docker/kpanel/data/panel",
		"/home/docker/kpanel/data/agent",
		"/home/docker/kpanel/run",
		"/home/.kpanel-trash",
	}
	if stateDirectory = strings.TrimSpace(stateDirectory); stateDirectory != "" && path.IsAbs(stateDirectory) {
		stateDirectory = path.Clean(stateDirectory)
		trashDirectory = path.Join(stateDirectory, "file-trash")
		protectedDirectories = append(protectedDirectories, stateDirectory)
	}
	return filemanager.Config{
		Root: "/", TrashVirtual: trashDirectory,
		ProtectedVirtual: protectedDirectories,
		ReadOnlyVirtual:  []string{"/proc", "/sys", "/dev"},
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := requestID()
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	if !s.authorized(r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="kejilion-agent"`)
		writeProblem(w, requestID, http.StatusUnauthorized, "agent_unauthorized", "认证失败", "")
		return
	}

	switch {
	case r.URL.Path == "/v1/health":
		s.requireMethod(w, r, requestID, http.MethodGet, s.health)
	case r.URL.Path == "/v1/capabilities":
		s.requireMethod(w, r, requestID, http.MethodGet, s.capabilities)
	case r.URL.Path == "/v1/system/summary":
		s.requireMethod(w, r, requestID, http.MethodGet, s.systemSummary)
	case r.URL.Path == "/v1/system/telemetry":
		s.requireMethod(w, r, requestID, http.MethodGet, s.systemTelemetry)
	case r.URL.Path == "/v1/system/public-network":
		s.requireMethod(w, r, requestID, http.MethodGet, s.publicNetwork)
	case r.URL.Path == "/v1/system/processes":
		s.requireMethod(w, r, requestID, http.MethodGet, s.systemProcesses)
	case r.URL.Path == "/v1/system/storage-usage":
		s.requireMethod(w, r, requestID, http.MethodGet, s.systemStorageUsage)
	case r.URL.Path == "/v1/system/hosts":
		s.requireMethod(w, r, requestID, http.MethodGet, s.systemHosts)
	case r.URL.Path == "/v1/system/cron":
		s.requireMethod(w, r, requestID, http.MethodGet, s.systemCron)
	case r.URL.Path == "/v1/system/network-interfaces":
		s.requireMethod(w, r, requestID, http.MethodGet, s.systemNetworkInterfaces)
	case r.URL.Path == "/v1/system/firewall":
		s.requireMethod(w, r, requestID, http.MethodGet, s.systemFirewall)
	case r.URL.Path == "/v1/system/port-usage":
		s.requireMethod(w, r, requestID, http.MethodGet, s.systemPortUsage)
	case r.URL.Path == "/v1/system/traffic-shutdown":
		s.requireMethod(w, r, requestID, http.MethodGet, s.systemTrafficShutdown)
	case r.URL.Path == "/v1/system/accounts":
		s.requireMethod(w, r, requestID, http.MethodGet, s.systemAccounts)
	case r.URL.Path == "/v1/system/ssh-defense":
		s.requireMethod(w, r, requestID, http.MethodGet, s.systemSSHDefense)
	case r.URL.Path == "/v1/system/system-tuning":
		s.requireMethod(w, r, requestID, http.MethodGet, s.systemTuning)
	case r.URL.Path == "/v1/monitoring/history":
		s.requireMethod(w, r, requestID, http.MethodGet, s.monitoringHistory)
	case r.URL.Path == "/v1/system/actions":
		s.requireMethod(w, r, requestID, http.MethodPost, s.systemAction)
	case r.URL.Path == "/v1/system/resource-actions":
		s.requireMethod(w, r, requestID, http.MethodPost, s.systemResourceAction)
	case r.URL.Path == "/v1/system/traffic-shutdown/actions":
		s.requireMethod(w, r, requestID, http.MethodPost, s.systemTrafficShutdownAction)
	case r.URL.Path == "/v1/system/account-actions":
		s.requireMethod(w, r, requestID, http.MethodPost, s.systemAccountAction)
	case r.URL.Path == "/v1/system/ssh-defense/actions":
		s.requireMethod(w, r, requestID, http.MethodPost, s.systemSSHDefenseAction)
	case r.URL.Path == "/v1/system/system-tuning/actions":
		s.requireMethod(w, r, requestID, http.MethodPost, s.systemTuningAction)
	case r.URL.Path == "/v1/terminals":
		s.requireMethod(w, r, requestID, http.MethodPost, s.terminalOpen)
	case strings.HasPrefix(r.URL.Path, "/v1/terminals/"):
		s.terminalOperation(w, r, requestID)
	case r.URL.Path == "/v1/sites":
		s.siteCollection(w, r, requestID)
	case r.URL.Path == "/v1/site-installations":
		s.requireMethod(w, r, requestID, http.MethodGet, s.siteInstallationList)
	case strings.HasPrefix(r.URL.Path, "/v1/site-installations/"):
		s.siteInstallation(w, r, requestID)
	case strings.HasPrefix(r.URL.Path, "/v1/sites/"):
		s.siteOperation(w, r, requestID)
	case r.URL.Path == "/v1/web-environment":
		s.requireMethod(w, r, requestID, http.MethodGet, s.webEnvironmentSummary)
	case r.URL.Path == "/v1/web-environment/catalog":
		s.requireMethod(w, r, requestID, http.MethodGet, s.webEnvironmentCatalog)
	case r.URL.Path == "/v1/web-environment/backups":
		s.requireMethod(w, r, requestID, http.MethodGet, s.webEnvironmentBackups)
	case strings.HasPrefix(r.URL.Path, "/v1/web-environment/backups/"):
		s.requireMethod(w, r, requestID, http.MethodGet, s.webEnvironmentBackupDownload)
	case r.URL.Path == "/v1/web-environment/jobs":
		s.webEnvironmentJobs(w, r, requestID)
	case strings.HasPrefix(r.URL.Path, "/v1/web-environment/jobs/"):
		s.webEnvironmentJob(w, r, requestID)
	case r.URL.Path == "/v1/apps":
		s.requireMethod(w, r, requestID, http.MethodGet, s.appList)
	case strings.HasPrefix(r.URL.Path, "/v1/apps/icons/"):
		s.requireMethod(w, r, requestID, http.MethodGet, s.appIcon)
	case r.URL.Path == "/v1/app-jobs":
		s.requireMethod(w, r, requestID, http.MethodGet, s.appJobList)
	case strings.HasPrefix(r.URL.Path, "/v1/app-jobs/"):
		s.appJobOperation(w, r, requestID)
	case strings.HasPrefix(r.URL.Path, "/v1/apps/"):
		s.appOperation(w, r, requestID)
	case r.URL.Path == "/v1/diagnostics":
		s.requireMethod(w, r, requestID, http.MethodGet, s.diagnosticCatalog)
	case r.URL.Path == "/v1/diagnostic-jobs":
		s.diagnosticJobCollection(w, r, requestID)
	case strings.HasPrefix(r.URL.Path, "/v1/diagnostic-jobs/"):
		s.diagnosticJob(w, r)
	case r.URL.Path == "/v1/files":
		s.requireMethod(w, r, requestID, http.MethodGet, s.fileList)
	case r.URL.Path == "/v1/files/entry":
		s.requireMethod(w, r, requestID, http.MethodGet, s.fileEntry)
	case r.URL.Path == "/v1/files/entries":
		s.requireMethod(w, r, requestID, http.MethodPost, s.fileEntries)
	case r.URL.Path == "/v1/files/trash":
		s.requireMethod(w, r, requestID, http.MethodGet, s.fileTrashList)
	case r.URL.Path == "/v1/files/content":
		s.fileContent(w, r, requestID)
	case r.URL.Path == "/v1/files/text":
		s.requireMethod(w, r, requestID, http.MethodGet, s.fileText)
	case r.URL.Path == "/v1/files/tail":
		s.requireMethod(w, r, requestID, http.MethodGet, s.fileTail)
	case r.URL.Path == "/v1/files/upload":
		s.requireMethod(w, r, requestID, http.MethodPost, s.fileUpload)
	case r.URL.Path == "/v1/files/transfer/export":
		s.requireMethod(w, r, requestID, http.MethodGet, s.fileTransferExport)
	case r.URL.Path == "/v1/files/transfer/import":
		s.requireMethod(w, r, requestID, http.MethodPost, s.fileTransferImport)
	case r.URL.Path == "/v1/files/actions":
		s.requireMethod(w, r, requestID, http.MethodPost, s.fileAction)
	case r.URL.Path == "/v1/docker/summary":
		s.requireMethod(w, r, requestID, http.MethodGet, s.dockerSummary)
	case r.URL.Path == "/v1/docker/environment":
		s.requireMethod(w, r, requestID, http.MethodGet, s.dockerEnvironment)
	case r.URL.Path == "/v1/docker/containers":
		s.requireMethod(w, r, requestID, http.MethodGet, s.containerList)
	case r.URL.Path == "/v1/docker/compose-projects":
		s.requireMethod(w, r, requestID, http.MethodGet, s.composeProjectList)
	case strings.HasPrefix(r.URL.Path, "/v1/docker/compose-projects/"):
		s.requireMethod(w, r, requestID, http.MethodGet, s.composeProject)
	case r.URL.Path == "/v1/docker/container-stats":
		s.requireMethod(w, r, requestID, http.MethodGet, s.containerStats)
	case r.URL.Path == "/v1/nginx/test":
		s.requireMethod(w, r, requestID, http.MethodGet, s.nginxTest)
	case r.URL.Path == "/v1/nginx/reload":
		s.requireMethod(w, r, requestID, http.MethodPost, s.nginxReload)
	case r.URL.Path == "/v1/docker/images":
		s.requireMethod(w, r, requestID, http.MethodGet, s.imageList)
	case r.URL.Path == "/v1/docker/networks":
		s.requireMethod(w, r, requestID, http.MethodGet, s.networkList)
	case r.URL.Path == "/v1/docker/volumes":
		s.requireMethod(w, r, requestID, http.MethodGet, s.volumeList)
	case r.URL.Path == "/v1/docker/backups":
		s.requireMethod(w, r, requestID, http.MethodGet, s.dockerBackupList)
	case r.URL.Path == "/v1/docker/jobs":
		s.requireMethod(w, r, requestID, http.MethodGet, s.dockerJobList)
	case strings.HasPrefix(r.URL.Path, "/v1/docker/jobs/"):
		s.requireMethod(w, r, requestID, http.MethodGet, s.dockerJob)
	case r.URL.Path == "/v1/docker/tasks":
		s.requireMethod(w, r, requestID, http.MethodPost, s.dockerTask)
	case strings.HasPrefix(r.URL.Path, "/v1/docker/containers/"):
		s.containerOperation(w, r, requestID)
	default:
		writeProblem(w, requestID, http.StatusNotFound, "not_found", "资源不存在", "")
	}
}

func (s *Server) authorized(r *http.Request) bool {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") || strings.Contains(header, ",") {
		return false
	}
	token := strings.TrimPrefix(header, "Bearer ")
	if token == "" || strings.TrimSpace(token) != token {
		return false
	}
	candidate := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(candidate[:], s.tokenHash[:]) == 1
}

func (s *Server) requireMethod(w http.ResponseWriter, r *http.Request, requestID, method string, next http.HandlerFunc) {
	if r.Method != method {
		w.Header().Set("Allow", method)
		writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
		return
	}
	next(w, r)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 800*time.Millisecond)
	defer cancel()
	var reasons []string
	if err := availableWebRoot(s.webRoot); err != nil {
		reasons = append(reasons, "web_root_unavailable")
	}
	if err := s.docker.Ping(ctx); err != nil {
		reasons = append(reasons, "docker_unavailable")
	}
	status := "ok"
	if len(reasons) > 0 {
		status = "degraded"
	}
	writeJSON(w, http.StatusOK, contract.AgentHealth{
		Status: status, Version: s.version, ProtocolVersion: s.protocolVersion,
		ReadOnly: false, Reasons: reasons, CheckedAt: s.now().UTC(),
	})
}

func (s *Server) capabilities(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var (
		dockerAvailable    bool
		siteErr            error
		siteWriteErr       error
		wordPressWriteErr  error
		proxyWriteErr      error
		recipeWriteErr     error
		templateWriteErr   error
		diagnosticErr      = errors.New("体检服务未配置")
		environmentReadErr = errors.New("LDNMP 环境读取服务未配置")
		environmentErr     = errors.New("LDNMP 环境服务未配置")
		fileErr            = errors.New("文件管理服务未配置")
	)
	var checks sync.WaitGroup
	checks.Add(10)
	go func() {
		defer checks.Done()
		pingContext, pingCancel := context.WithTimeout(ctx, 800*time.Millisecond)
		defer pingCancel()
		dockerAvailable = s.docker.Ping(pingContext) == nil
	}()
	go func() {
		defer checks.Done()
		siteErr = availableWebRoot(s.webRoot)
	}()
	go func() {
		defer checks.Done()
		siteWriteErr = s.sitesManager.Writable(ctx)
	}()
	go func() {
		defer checks.Done()
		wordPressWriteErr = s.sitesManager.WordPressWritable(ctx)
	}()
	go func() {
		defer checks.Done()
		proxyWriteErr = s.sitesManager.ProxyWritable()
	}()
	go func() {
		defer checks.Done()
		recipeWriteErr = s.sitesManager.RecipeWritable()
	}()
	go func() {
		defer checks.Done()
		templateWriteErr = s.sitesManager.TemplateWritable()
	}()
	go func() {
		defer checks.Done()
		if s.diagnostics != nil {
			diagnosticErr = s.diagnostics.Available()
		}
	}()
	go func() {
		defer checks.Done()
		if s.webEnvironment != nil {
			environmentReadErr = s.webEnvironment.Readable()
			environmentErr = s.webEnvironment.Available()
		}
	}()
	go func() {
		defer checks.Done()
		if s.files != nil {
			fileErr = s.files.Available()
		}
	}()
	checks.Wait()
	items := []contract.Capability{
		{ID: "system.read", Enabled: true, Methods: []string{"GET"}},
		{ID: "system.processes.read", Enabled: true, Methods: []string{"GET"}},
		{ID: "system.storage.read", Enabled: true, Methods: []string{"GET"}},
		{ID: "monitoring.history.read", Enabled: s.monitoring != nil, Reason: reasonUnless(s.monitoring != nil, "历史监控服务未配置"), Methods: []string{"GET"}},
		{ID: "apps.read", Enabled: dockerAvailable, Reason: reasonUnless(dockerAvailable, "Docker Engine 不可用"), Methods: []string{"GET"}},
		{ID: "apps.lifecycle", Enabled: dockerAvailable, Reason: reasonUnless(dockerAvailable, "Docker Engine 不可用"), Methods: []string{"POST"}},
		{ID: "apps.install", Enabled: dockerAvailable, Reason: reasonUnless(dockerAvailable, "Docker Engine 不可用"), Methods: []string{"POST"}},
		{ID: "sites.read", Enabled: siteErr == nil, Reason: reasonIf(siteErr, "Kejilion Web 根目录不可用"), Methods: []string{"GET"}},
		{ID: "docker.read", Enabled: dockerAvailable, Reason: reasonUnless(dockerAvailable, "Docker Engine 不可用"), Methods: []string{"GET"}},
		{ID: "docker.logs", Enabled: dockerAvailable, Reason: reasonUnless(dockerAvailable, "Docker Engine 不可用"), Methods: []string{"GET"}},
		{ID: "docker.lifecycle", Enabled: dockerAvailable, Reason: reasonUnless(dockerAvailable, "Docker Engine 不可用"), Methods: []string{"POST"}},
		{ID: "docker.exec", Enabled: dockerAvailable, Reason: reasonUnless(dockerAvailable, "Docker Engine 不可用"), Methods: []string{"POST"}},
		{ID: "docker.maintenance", Enabled: dockerAvailable, Reason: reasonUnless(dockerAvailable, "Docker Engine 不可用"), Methods: []string{"GET", "POST"}},
		{ID: "nginx.validate", Enabled: dockerAvailable, Reason: reasonUnless(dockerAvailable, "Docker Engine 不可用"), Methods: []string{"GET"}},
		{ID: "nginx.reload", Enabled: dockerAvailable, Reason: reasonUnless(dockerAvailable, "Docker Engine 不可用"), Methods: []string{"POST"}},
		{ID: "sites.write", Enabled: siteWriteErr == nil, Reason: reasonIf(siteWriteErr, "网站写入依赖未就绪"), Methods: []string{"POST", "PATCH"}},
		{ID: "sites.wordpress.install", Enabled: wordPressWriteErr == nil, Reason: reasonIf(wordPressWriteErr, "WordPress 一键搭建条件不满足"), Methods: []string{"POST"}},
		{ID: "sites.proxy.install", Enabled: proxyWriteErr == nil, Reason: reasonIf(proxyWriteErr, "kejilion.sh IP+端口反向代理命令不可用"), Methods: []string{"POST"}},
		{ID: "sites.recipes.install", Enabled: recipeWriteErr == nil, Reason: reasonIf(recipeWriteErr, "kejilion.sh 一键建站协议不可用"), Methods: []string{"POST"}},
		{ID: "sites.templates.install", Enabled: templateWriteErr == nil, Reason: reasonIf(templateWriteErr, "kejilion.sh 交互建站模板不可用"), Methods: []string{"POST"}},
		{ID: "diagnostics.run", Enabled: diagnosticErr == nil, Reason: reasonIf(diagnosticErr, "请更新本机 kejilion.sh 以启用体检协议"), Methods: []string{"GET", "POST"}},
		{ID: "files.read", Enabled: fileErr == nil, Reason: reasonIf(fileErr, "宿主机文件根目录不可用"), Methods: []string{"GET"}},
		{ID: "files.write", Enabled: fileErr == nil, Reason: reasonIf(fileErr, "宿主机文件根目录不可用"), Methods: []string{"POST", "PUT"}},
		{ID: "web.environment.read", Enabled: environmentReadErr == nil, Reason: reasonIf(environmentReadErr, "请更新本机 kejilion.sh 以启用 LDNMP 环境协议"), Methods: []string{"GET"}},
		{ID: "web.environment.install", Enabled: environmentErr == nil, Reason: reasonIf(environmentErr, "LDNMP 安装协议不可用"), Methods: []string{"POST"}},
		{ID: "web.environment.protection.write", Enabled: environmentErr == nil, Reason: reasonIf(environmentErr, "LDNMP 防护协议不可用"), Methods: []string{"POST"}},
		{ID: "web.environment.optimization.write", Enabled: environmentErr == nil, Reason: reasonIf(environmentErr, "LDNMP 优化协议不可用"), Methods: []string{"POST"}},
		{ID: "web.environment.update", Enabled: environmentErr == nil, Reason: reasonIf(environmentErr, "LDNMP 更新协议不可用"), Methods: []string{"POST"}},
		{ID: "web.environment.backup", Enabled: environmentErr == nil, Reason: reasonIf(environmentErr, "LDNMP 备份协议不可用"), Methods: []string{"GET", "POST"}},
		{ID: "web.environment.restore", Enabled: environmentErr == nil, Reason: reasonIf(environmentErr, "LDNMP 还原协议不可用"), Methods: []string{"POST"}},
		{ID: "web.environment.uninstall", Enabled: environmentErr == nil, Reason: reasonIf(environmentErr, "LDNMP 卸载协议不可用"), Methods: []string{"POST"}},
	}
	items = append(items, s.systemManager.Capabilities()...)
	writeJSON(w, http.StatusOK, contract.PageResult[contract.Capability]{Items: items})
}

func availableWebRoot(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("web root is not a directory")
	}
	return nil
}

func (s *Server) systemSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := s.system.Collect(r.Context())
	if err != nil && summary.Hostname == "" {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "system_unavailable", "系统状态不可用", "")
		return
	}
	summary.Management.SSH.Defense = s.systemManager.SSHDefenseStatus(r.Context())
	summary.Management.BBRv3 = s.systemManager.BBRv3Status(r.Context())
	summary.Management.Maintenance = s.systemManager.MaintenanceStatus()
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) systemTelemetry(w http.ResponseWriter, r *http.Request) {
	summary, err := s.system.Collect(r.Context())
	if err != nil && summary.Hostname == "" {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "system_unavailable", "系统状态不可用", "")
		return
	}
	disk := contract.DiskCapacitySummary{}
	if len(summary.Disks) > 0 {
		selected := summary.Disks[0]
		for _, candidate := range summary.Disks {
			if candidate.MountPoint == "/" ||
				(selected.MountPoint != "/" && candidate.TotalBytes > selected.TotalBytes) {
				selected = candidate
			}
		}
		disk = contract.DiskCapacitySummary{
			TotalBytes: selected.TotalBytes, UsedBytes: selected.UsedBytes,
			UsagePercent: selected.UsagePercent,
		}
	}
	writeJSON(w, http.StatusOK, contract.HostTelemetry{
		AgentVersion: s.version, AgentProtocolVersion: s.protocolVersion,
		Hostname: summary.Hostname, OS: summary.OS, OSID: summary.OSID,
		OSLike: summary.OSLike, Kernel: summary.Kernel, Architecture: summary.Architecture,
		UptimeSeconds: summary.UptimeSeconds, Load: summary.Load, CPU: summary.CPU,
		Memory: summary.Memory, Disk: disk, Network: summary.Network,
		PublicNetwork: summary.PublicNetwork, CollectedAt: summary.CollectedAt,
	})
}

func (s *Server) publicNetwork(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3500*time.Millisecond)
	defer cancel()
	writeJSON(w, http.StatusOK, s.system.PublicNetwork(ctx))
}

func (s *Server) systemProcesses(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" {
		writeProblem(w, requestIDFrom(w), http.StatusBadRequest, "invalid_query", "Process query is invalid", "")
		return
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(values) > 4 {
		writeProblem(w, requestIDFrom(w), http.StatusBadRequest, "invalid_query", "Process query is invalid", "")
		return
	}
	for key, entries := range values {
		if len(entries) != 1 || (key != "q" && key != "sort" && key != "order" && key != "limit") {
			writeProblem(w, requestIDFrom(w), http.StatusBadRequest, "invalid_query", "Process query is invalid", "")
			return
		}
	}
	select {
	case s.processesGate <- struct{}{}:
		defer func() { <-s.processesGate }()
	default:
		writeProblem(w, requestIDFrom(w), http.StatusTooManyRequests, "process_metrics_busy", "Another process sample is already running", "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	var result systeminfo.ProcessSnapshot
	if r.URL.RawQuery == "" {
		result, err = s.system.Processes(ctx)
	} else {
		limit := 0
		if value := values.Get("limit"); value != "" {
			limit, err = strconv.Atoi(value)
			if err != nil {
				writeProblem(w, requestIDFrom(w), http.StatusBadRequest, "invalid_query", "Process query is invalid", "")
				return
			}
		}
		query, queryErr := systeminfo.NormalizeProcessQuery(systeminfo.ProcessQuery{
			Search: values.Get("q"), Sort: values.Get("sort"), Order: values.Get("order"), Limit: limit,
		})
		if queryErr != nil {
			writeProblem(w, requestIDFrom(w), http.StatusUnprocessableEntity, "invalid_process_query", "Process query is invalid", "")
			return
		}
		result, err = s.system.QueryProcesses(ctx, query)
	}
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "process_metrics_unavailable", "Process metrics are unavailable", "")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) systemStorageUsage(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || !strictQuery(r.URL.Query(), "path") || r.URL.Query().Get("path") == "" {
		writeProblem(w, requestIDFrom(w), http.StatusBadRequest, "invalid_query", "Storage usage query is invalid", "")
		return
	}
	select {
	case s.storageUsageGate <- struct{}{}:
		defer func() { <-s.storageUsageGate }()
	default:
		writeProblem(w, requestIDFrom(w), http.StatusTooManyRequests, "storage_usage_busy", "Another storage usage analysis is already running", "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	result, err := s.system.StorageUsage(ctx, r.URL.Query().Get("path"))
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusUnprocessableEntity, "storage_usage_unavailable", "Storage usage analysis is unavailable", "")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) monitoringHistory(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" {
		writeProblem(w, requestID, http.StatusUnprocessableEntity, "invalid_monitoring_query", "监控查询参数无效", "")
		return
	}
	values := r.URL.Query()
	if len(values) > 3 || len(values["range"]) > 1 || len(values["start"]) > 1 || len(values["end"]) > 1 {
		writeProblem(w, requestID, http.StatusUnprocessableEntity, "invalid_monitoring_query", "监控查询参数无效", "")
		return
	}
	for key := range values {
		if key != "range" && key != "start" && key != "end" {
			writeProblem(w, requestID, http.StatusUnprocessableEntity, "invalid_monitoring_query", "监控查询参数无效", "")
			return
		}
	}
	rangeValue := values.Get("range")
	switch rangeValue {
	case "", "1h", "6h", "24h", "7d", "30d", "3m", "6m", "12m":
	default:
		writeProblem(w, requestID, http.StatusUnprocessableEntity, "invalid_monitoring_range", "监控时间范围无效", "")
		return
	}
	if s.monitoring == nil {
		writeProblem(w, requestID, http.StatusServiceUnavailable, "monitoring_unavailable", "历史监控尚未就绪", "")
		return
	}
	startValue, startPresent := values["start"]
	endValue, endPresent := values["end"]
	if startPresent != endPresent || (startPresent && (startValue[0] == "" || endValue[0] == "")) {
		writeProblem(w, requestID, http.StatusUnprocessableEntity, "invalid_monitoring_window", "监控时间区间无效", "")
		return
	}
	var result contract.MonitoringHistory
	var err error
	if startPresent {
		start, startErr := time.Parse(time.RFC3339Nano, startValue[0])
		end, endErr := time.Parse(time.RFC3339Nano, endValue[0])
		if startErr != nil || endErr != nil {
			writeProblem(w, requestID, http.StatusUnprocessableEntity, "invalid_monitoring_window", "监控时间区间无效", "")
			return
		}
		result, err = s.monitoring.HistoryBetween(r.Context(), rangeValue, start, end)
	} else {
		result, err = s.monitoring.History(r.Context(), rangeValue)
	}
	if err != nil {
		switch {
		case errors.Is(err, monitoring.ErrInvalidRange):
			writeProblem(w, requestID, http.StatusUnprocessableEntity, "invalid_monitoring_range", "监控时间范围无效", "")
		case errors.Is(err, monitoring.ErrInvalidWindow):
			writeProblem(w, requestID, http.StatusUnprocessableEntity, "invalid_monitoring_window", "监控时间区间无效", "")
		case errors.Is(err, monitoring.ErrBusy):
			writeProblem(w, requestID, http.StatusTooManyRequests, "monitoring_busy", "历史监控查询繁忙", "")
		default:
			writeProblem(w, requestID, http.StatusServiceUnavailable, "monitoring_unavailable", "历史监控暂时不可用", "")
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) systemAction(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_system_action", "系统操作 URL 无效", "")
		return
	}
	var input contract.SystemActionRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "请求格式无效", "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	result, err := s.systemManager.Execute(ctx, input)
	if err != nil {
		status, code, title := http.StatusServiceUnavailable, "system_action_failed", "系统操作失败"
		switch {
		case errors.Is(err, systemmanage.ErrInvalidInput):
			status, code, title = http.StatusUnprocessableEntity, "invalid_system_action", "系统操作参数无效"
		case errors.Is(err, systemmanage.ErrDisabled), errors.Is(err, systemmanage.ErrUnsupported):
			status, code, title = http.StatusForbidden, "system_action_unavailable", "系统操作不可用"
		case errors.Is(err, systemmanage.ErrConflict):
			status, code, title = http.StatusConflict, "system_action_conflict", "系统配置发生冲突"
		case errors.Is(err, systemmanage.ErrRolledBack):
			status, code, title = http.StatusUnprocessableEntity, "system_action_rolled_back", "系统操作失败并已回滚"
		case errors.Is(err, systemmanage.ErrNeedsAttention):
			status, code, title = http.StatusServiceUnavailable, "system_action_needs_attention", "系统操作需要人工检查"
		}
		writeProblem(w, requestID, status, code, title, safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) siteList(w http.ResponseWriter, _ *http.Request) {
	items, err := s.sites.Discover()
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "sites_unavailable", "网站状态不可用", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, contract.PageResult[contract.SiteSummary]{Items: items})
}

func (s *Server) siteCollection(w http.ResponseWriter, r *http.Request, requestID string) {
	switch r.Method {
	case http.MethodGet:
		s.siteList(w, r)
	case http.MethodPost:
		if r.URL.RawPath != "" || r.URL.RawQuery != "" {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_site_request", "网站写入 URL 无效", "")
			return
		}
		var input sites.SiteInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "请求格式无效", "")
			return
		}
		if input.Type == "wordpress" {
			job, err := s.sitesManager.StartWordPress(r.Context(), input)
			if err != nil {
				s.writeSiteError(w, requestID, err)
				return
			}
			writeJSON(w, http.StatusAccepted, job)
			return
		}
		if input.Type == "recipe" {
			job, err := s.sitesManager.StartRecipe(r.Context(), input)
			if err != nil {
				s.writeSiteError(w, requestID, err)
				return
			}
			writeJSON(w, http.StatusAccepted, job)
			return
		}
		if input.Type == "proxy" {
			job, err := s.sitesManager.StartProxy(input)
			if err != nil {
				s.writeSiteError(w, requestID, err)
				return
			}
			writeJSON(w, http.StatusAccepted, job)
			return
		}
		if input.Type == "static" || input.Type == "php" || input.Type == "proxy_domain" ||
			input.Type == "load_balance" || input.Type == "redirect" {
			job, err := s.sitesManager.StartTemplate(input)
			if err != nil {
				s.writeSiteError(w, requestID, err)
				return
			}
			writeJSON(w, http.StatusAccepted, job)
			return
		}
		result, err := s.sitesManager.Create(r.Context(), input)
		if err != nil {
			s.writeSiteError(w, requestID, err)
			return
		}
		writeJSON(w, http.StatusCreated, result)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
		writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
	}
}

func (s *Server) siteInstallationList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(
		w,
		http.StatusOK,
		contract.PageResult[sites.RecipeJob]{Items: s.sitesManager.InstallationJobs()},
	)
}

func (s *Server) siteInstallation(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.URL.RawPath != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_site_installation_request", "安装任务 URL 无效", "")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/site-installations/")
	parts := strings.Split(rest, "/")
	id := parts[0]
	if !validSiteID(id) || len(parts) > 2 {
		writeProblem(w, requestID, http.StatusNotFound, "not_found", "安装任务不存在", "")
		return
	}
	if len(parts) == 2 && parts[1] == "terminal" {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
			return
		}
		query, err := parseTerminalReadQuery(r.URL.Query(), false)
		if err != nil {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_terminal_offset", "终端偏移量无效", "")
			return
		}
		chunk, err := waitForTerminalChunk(
			r.Context(),
			query.Wait,
			func() (sites.SiteTerminalChunk, error) {
				return s.sitesManager.InstallationTerminal(id, query.Offset)
			},
			func(chunk sites.SiteTerminalChunk) bool {
				return chunk.DataBase64 != "" || chunk.Finished ||
					(query.HasInputState && chunk.InputOpen != query.InputOpen)
			},
		)
		if errors.Is(err, context.Canceled) {
			return
		}
		if err != nil {
			writeProblem(w, requestID, http.StatusNotFound, "site_terminal_not_found", "建站终端不存在", "")
			return
		}
		writeJSON(w, http.StatusOK, chunk)
		return
	}
	if len(parts) == 2 && parts[1] == "input" {
		if r.Method != http.MethodPost || r.URL.RawQuery != "" {
			w.Header().Set("Allow", http.MethodPost)
			writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
			return
		}
		var input struct {
			Data string `json:"data"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "请求格式无效", "")
			return
		}
		if err := s.sitesManager.WriteInstallationInput(id, input.Data); err != nil {
			status, code, title := http.StatusConflict, "site_terminal_closed", "建站终端输入已关闭"
			if errors.Is(err, sites.ErrInvalidInput) {
				status, code, title = http.StatusUnprocessableEntity, "invalid_terminal_input", "终端输入无效"
			}
			writeProblem(w, requestID, status, code, title, safeDetail(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if len(parts) != 1 || r.Method != http.MethodGet || r.URL.RawQuery != "" {
		w.Header().Set("Allow", http.MethodGet)
		writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
		return
	}
	job, err := s.sitesManager.InstallationJob(id)
	if err != nil {
		writeProblem(w, requestID, http.StatusNotFound, "not_found", "安装任务不存在", "")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) siteOperation(w http.ResponseWriter, r *http.Request, requestID string) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/sites/")
	parts := strings.Split(rest, "/")
	if len(parts) == 2 && parts[1] == "appearance" {
		if r.URL.RawPath != "" || r.URL.RawQuery != "" {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_site_appearance_request", "网站外观信息 URL 无效", "")
			return
		}
		if !validSiteID(parts[0]) {
			writeProblem(w, requestID, http.StatusNotFound, "not_found", "网站外观信息不存在", "")
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
			return
		}
		s.siteAppearance(w, r, requestID, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "icon" {
		if r.URL.RawPath != "" || r.URL.RawQuery != "" {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_site_icon_request", "网站图标 URL 无效", "")
			return
		}
		if !validSiteID(parts[0]) {
			writeProblem(w, requestID, http.StatusNotFound, "not_found", "网站图标不存在", "")
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
			return
		}
		s.siteIcon(w, r, requestID, parts[0])
		return
	}
	if (r.Method == http.MethodPatch || r.Method == http.MethodDelete) &&
		(r.URL.RawPath != "" || r.URL.RawQuery != "") {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_site_request", "网站写入 URL 无效", "")
		return
	}
	id := rest
	if !validSiteID(id) {
		writeProblem(w, requestID, http.StatusNotFound, "not_found", "资源不存在", "")
		return
	}
	if r.Method != http.MethodPatch && r.Method != http.MethodDelete {
		w.Header().Set("Allow", http.MethodPatch+", "+http.MethodDelete)
		writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
		return
	}
	if r.Method == http.MethodDelete {
		var input sites.DeleteInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "请求格式无效", "")
			return
		}
		result, err := s.sitesManager.DeleteWithOptions(r.Context(), id, input)
		if err != nil {
			s.writeSiteError(w, requestID, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	var input sites.SiteInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "请求格式无效", "")
		return
	}
	result, err := s.sitesManager.Update(r.Context(), id, input)
	if err != nil {
		s.writeSiteError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) siteAppearance(w http.ResponseWriter, r *http.Request, requestID, id string) {
	if s.siteIcons == nil {
		writeProblem(w, requestID, http.StatusServiceUnavailable, "site_appearance_unavailable", "网站名称暂不可用", "")
		return
	}
	appearance, err := s.siteIcons.Appearance(r.Context(), id)
	if errors.Is(err, context.Canceled) {
		return
	}
	if err != nil {
		if errors.Is(err, sites.ErrSiteIconNotFound) {
			writeProblem(w, requestID, http.StatusNotFound, "site_appearance_not_found", "网站外观信息不存在", "")
			return
		}
		writeProblem(w, requestID, http.StatusServiceUnavailable, "site_appearance_unavailable", "网站名称暂不可用", "")
		return
	}
	writeJSON(w, http.StatusOK, appearance)
}

func (s *Server) siteIcon(w http.ResponseWriter, r *http.Request, requestID, id string) {
	if s.siteIcons == nil {
		writeProblem(w, requestID, http.StatusServiceUnavailable, "site_icon_unavailable", "网站图标暂不可用", "")
		return
	}
	icon, err := s.siteIcons.Get(r.Context(), id)
	if errors.Is(err, context.Canceled) {
		return
	}
	if err != nil {
		if errors.Is(err, sites.ErrSiteIconNotFound) {
			writeProblem(w, requestID, http.StatusNotFound, "site_icon_not_found", "网站未提供可用图标", "")
			return
		}
		writeProblem(w, requestID, http.StatusServiceUnavailable, "site_icon_unavailable", "网站图标暂不可用", "")
		return
	}
	if len(icon.Data) == 0 || len(icon.Data) > 256<<10 || !validSiteIconContentType(icon.ContentType) {
		writeProblem(w, requestID, http.StatusBadGateway, "invalid_site_icon", "网站图标响应无效", "")
		return
	}
	w.Header().Set("Content-Type", icon.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(icon.Data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(icon.Data)
}

func validSiteIconContentType(value string) bool {
	switch value {
	case "image/png", "image/jpeg", "image/gif", "image/vnd.microsoft.icon", "image/webp":
		return true
	default:
		return false
	}
}

func (s *Server) writeSiteError(w http.ResponseWriter, requestID string, err error) {
	status, code, title := http.StatusServiceUnavailable, "sites_unavailable", "网站写入暂不可用"
	switch {
	case errors.Is(err, sites.ErrInvalidInput):
		status, code, title = http.StatusBadRequest, "invalid_site_request", "网站请求无效"
	case errors.Is(err, sites.ErrForbidden):
		status, code, title = http.StatusUnprocessableEntity, "site_action_unsupported", "当前网站结构没有对应操作适配器"
	case errors.Is(err, sites.ErrConflict):
		status, code, title = http.StatusConflict, "resource_conflict", "网站资源发生冲突"
	case errors.Is(err, sites.ErrUnprocessable):
		status, code, title = http.StatusUnprocessableEntity, "site_validation_failed", "网站配置验证失败"
	case errors.Is(err, sites.ErrNeedsAttention):
		status, code, title = http.StatusServiceUnavailable, "site_needs_attention", "网站操作需要人工检查"
	case errors.Is(err, sites.ErrUnavailable):
		status, code, title = http.StatusServiceUnavailable, "sites_unavailable", "网站写入暂不可用"
	}
	writeProblem(w, requestID, status, code, title, safeDetail(err))
}

func validSiteID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func (s *Server) appList(w http.ResponseWriter, r *http.Request) {
	inventory, err := s.appMarket.Inventory(r.Context())
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "apps_unavailable", "应用市场状态不可用", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, inventory)
}

func (s *Server) appIcon(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	const prefix = "/v1/apps/icons/"
	const suffix = ".webp"
	rest := strings.TrimPrefix(r.URL.Path, prefix)
	if r.URL.RawPath != "" || r.URL.RawQuery != "" || !strings.HasSuffix(rest, suffix) {
		writeProblem(w, requestID, http.StatusNotFound, "app_icon_not_found", "应用图标不存在", "")
		return
	}
	slug := strings.TrimSuffix(rest, suffix)
	if slug == "" || strings.Contains(slug, "/") {
		writeProblem(w, requestID, http.StatusNotFound, "app_icon_not_found", "应用图标不存在", "")
		return
	}
	icon, err := s.appMarket.Icon(r.Context(), slug)
	if errors.Is(err, context.Canceled) {
		return
	}
	if err != nil {
		if errors.Is(err, appmarket.ErrAppIconNotFound) {
			writeProblem(w, requestID, http.StatusNotFound, "app_icon_not_found", "应用图标不存在", "")
			return
		}
		writeProblem(w, requestID, http.StatusServiceUnavailable, "app_icon_unavailable", "应用图标暂不可用", "")
		return
	}
	if icon.ContentType != "image/webp" || len(icon.Data) == 0 ||
		len(icon.Data) > 128<<10 {
		writeProblem(w, requestID, http.StatusBadGateway, "invalid_app_icon", "应用图标响应无效", "")
		return
	}
	w.Header().Set("Content-Type", icon.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(icon.Data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(icon.Data)
}

func (s *Server) appJobList(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestIDFrom(w), http.StatusBadRequest, "invalid_app_job_request", "应用任务 URL 无效", "")
		return
	}
	writeJSON(w, http.StatusOK, contract.PageResult[appmarket.AppJob]{
		Items: s.appMarket.AppJobs(),
	})
}

func (s *Server) appJobOperation(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.URL.RawPath != "" {
		writeProblem(w, requestIDFrom(w), http.StatusBadRequest, "invalid_app_job_request", "应用任务 URL 无效", "")
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/app-jobs/"), "/")
	if len(parts) < 1 || len(parts) > 2 || !validSiteID(parts[0]) {
		writeProblem(w, requestIDFrom(w), http.StatusNotFound, "app_job_not_found", "应用任务不存在", "")
		return
	}
	id := parts[0]
	if len(parts) == 1 {
		if r.Method != http.MethodGet || r.URL.RawQuery != "" {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_app_job_request", "应用任务请求无效", "")
			return
		}
		job, err := s.appMarket.AppJob(id)
		if err != nil {
			writeProblem(w, requestID, http.StatusNotFound, "app_job_not_found", "应用任务不存在", "")
			return
		}
		writeJSON(w, http.StatusOK, job)
		return
	}
	switch parts[1] {
	case "terminal":
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
			return
		}
		query, err := parseTerminalReadQuery(r.URL.Query(), true)
		if err != nil {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_terminal_offset", "终端偏移量无效", "")
			return
		}
		chunk, err := waitForTerminalChunk(
			r.Context(),
			query.Wait,
			func() (appmarket.TerminalChunk, error) {
				return s.appMarket.AppJobTerminal(id, query.Offset)
			},
			func(chunk appmarket.TerminalChunk) bool {
				return chunk.DataBase64 != "" || chunk.Finished ||
					(query.HasInputState && chunk.InputOpen != query.InputOpen)
			},
		)
		if errors.Is(err, context.Canceled) {
			return
		}
		if err != nil {
			writeProblem(w, requestID, http.StatusNotFound, "app_terminal_not_found", "交互终端不存在", "")
			return
		}
		writeJSON(w, http.StatusOK, chunk)
	case "input":
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
			return
		}
		if r.URL.RawQuery != "" {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_app_job_request", "应用任务请求无效", "")
			return
		}
		var input struct {
			Data string `json:"data"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "请求格式无效", "")
			return
		}
		if err := s.appMarket.WriteAppJobInput(id, input.Data); err != nil {
			status, code, title := http.StatusConflict, "app_terminal_closed", "交互终端输入已关闭"
			if errors.Is(err, appmarket.ErrNotFound) {
				status, code, title = http.StatusNotFound, "app_terminal_not_found", "交互终端不存在"
			} else if errors.Is(err, appmarket.ErrForbidden) {
				status, code, title = http.StatusUnprocessableEntity, "invalid_terminal_input", "终端输入无效"
			}
			writeProblem(w, requestID, status, code, title, safeDetail(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case "cancel":
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
			return
		}
		if r.URL.RawQuery != "" {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_app_job_request", "应用任务请求无效", "")
			return
		}
		job, err := s.appMarket.CancelAppJob(id)
		if err != nil {
			status, code, title := http.StatusServiceUnavailable, "app_job_cancel_failed", "交互任务无法结束"
			switch {
			case errors.Is(err, appmarket.ErrNotFound):
				status, code, title = http.StatusNotFound, "app_job_not_found", "应用任务不存在"
			case errors.Is(err, appmarket.ErrForbidden):
				status, code, title = http.StatusUnprocessableEntity, "app_job_not_cancellable", "该任务不允许手动结束"
			case errors.Is(err, appmarket.ErrConflict):
				status, code, title = http.StatusConflict, "app_job_not_active", "应用任务已结束"
			}
			writeProblem(w, requestID, status, code, title, safeDetail(err))
			return
		}
		writeJSON(w, http.StatusAccepted, job)
	default:
		writeProblem(w, requestID, http.StatusNotFound, "not_found", "资源不存在", "")
	}
}

func (s *Server) appOperation(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.Method == http.MethodGet {
		s.appInstallPortStatus(w, r, requestID)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
		return
	}
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_app_request", "应用操作 URL 无效", "")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/apps/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeProblem(w, requestID, http.StatusNotFound, "not_found", "资源不存在", "")
		return
	}
	id, action := parts[0], parts[1]
	timeout := 2 * time.Minute
	if action == "install" || action == "update" {
		timeout = 12 * time.Minute
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	if action == "install" {
		var input appmarket.InstallInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "请求格式无效", "")
			return
		}
		result, err := s.appMarket.StartInstall(ctx, id, input)
		if err != nil {
			s.writeAppError(w, requestID, err)
			return
		}
		writeJSON(w, http.StatusAccepted, result)
		return
	}

	var input appmarket.MutationInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "请求格式无效", "")
		return
	}
	if action == "start" || action == "stop" || action == "restart" {
		result, err := s.appMarket.Lifecycle(ctx, id, action, input.ResourceVersion)
		if err != nil {
			s.writeAppError(w, requestID, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	if action == "check_update" {
		result, err := s.appMarket.CheckUpdate(ctx, id, input.ResourceVersion)
		if err != nil {
			s.writeAppError(w, requestID, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	if action != "update" && action != "uninstall" &&
		action != "direct_access" && action != "manage" {
		writeProblem(w, requestID, http.StatusNotFound, "not_found", "资源不存在", "")
		return
	}
	job, scriptBacked, err := s.appMarket.StartScriptMutation(ctx, id, action, input)
	if scriptBacked {
		if err != nil {
			s.writeAppError(w, requestID, err)
			return
		}
		writeJSON(w, http.StatusAccepted, job)
		return
	}
	result, err := s.appMarket.Mutate(ctx, id, action, input)
	if err != nil {
		s.writeAppError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) appInstallPortStatus(
	w http.ResponseWriter,
	r *http.Request,
	requestID string,
) {
	if r.URL.RawPath != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_app_request", "应用端口检查 URL 无效", "")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/apps/")
	parts := strings.Split(rest, "/")
	values := r.URL.Query()
	if len(parts) != 2 || parts[0] == "" || parts[1] != "install-port" ||
		len(values) != 1 || len(values["port"]) != 1 {
		writeProblem(w, requestID, http.StatusNotFound, "not_found", "资源不存在", "")
		return
	}
	port, err := strconv.ParseUint(values.Get("port"), 10, 16)
	if err != nil || port == 0 {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_app_port", "应用端口无效", "端口必须在 1-65535 之间")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	status, err := s.appMarket.CheckInstallPort(ctx, parts[0], uint16(port))
	if err != nil {
		s.writeAppError(w, requestID, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) writeAppError(w http.ResponseWriter, requestID string, err error) {
	status, code, title := http.StatusServiceUnavailable, "app_action_failed", "应用操作失败"
	switch {
	case errors.Is(err, appmarket.ErrNotFound):
		status, code, title = http.StatusNotFound, "app_not_found", "应用不存在"
	case errors.Is(err, appmarket.ErrForbidden), errors.Is(err, appmarket.ErrUnsupported),
		errors.Is(err, dockerx.ErrRuntimeContract), errors.Is(err, dockerx.ErrActionUnsupported):
		status, code, title = http.StatusUnprocessableEntity, "app_action_unsupported", "当前应用状态或适配器不支持此操作"
	case errors.Is(err, dockerx.ErrVersionRequired):
		status, code, title = http.StatusBadRequest, "resource_version_required", "必须提供资源版本"
	case errors.Is(err, appmarket.ErrTaskConflict):
		status, code, title = http.StatusConflict, "app_task_conflict", "已有应用任务正在运行"
		err = errors.New("请先完成或关闭当前任务；若后台进程已经结束，刷新后会自动释放任务锁")
	case errors.Is(err, appmarket.ErrPortConflict):
		status, code, title = http.StatusConflict, "app_port_conflict", "应用安装端口已被占用"
	case errors.Is(err, appmarket.ErrConflict),
		errors.Is(err, dockerx.ErrResourceConflict), errors.Is(err, dockerx.ErrAppConflict):
		status, code, title = http.StatusConflict, "resource_conflict", "应用资源已发生变化"
	case errors.Is(err, appmarket.ErrRolledBack), errors.Is(err, dockerx.ErrAppRolledBack):
		status, code, title = http.StatusUnprocessableEntity, "app_action_rolled_back", "应用操作失败并已回滚"
	case errors.Is(err, appmarket.ErrNeedsAttention), errors.Is(err, dockerx.ErrAppNeedsAttention):
		status, code, title = http.StatusServiceUnavailable, "app_needs_attention", "应用操作需要人工检查"
	}
	writeProblem(w, requestID, status, code, title, safeDetail(err))
}

func (s *Server) diagnosticCatalog(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestIDFrom(w), http.StatusBadRequest, "invalid_diagnostic_request", "体检请求无效", "")
		return
	}
	if s.diagnostics == nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "diagnostics_unavailable", "体检服务不可用", "")
		return
	}
	catalog, err := s.diagnostics.Catalog(r.Context())
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "diagnostics_unavailable", "体检服务不可用", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, catalog)
}

func (s *Server) diagnosticJobCollection(w http.ResponseWriter, r *http.Request, requestID string) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_diagnostic_request", "体检请求无效", "")
		return
	}
	if s.diagnostics == nil {
		writeProblem(w, requestID, http.StatusServiceUnavailable, "diagnostics_unavailable", "体检服务不可用", "")
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, contract.PageResult[diagnostics.Job]{Items: s.diagnostics.Jobs()})
	case http.MethodPost:
		var input struct {
			CheckID string `json:"checkId"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "请求格式无效", "")
			return
		}
		job, err := s.diagnostics.Start(r.Context(), input.CheckID)
		if err != nil {
			status, code, title := http.StatusUnprocessableEntity, "diagnostic_request_invalid", "体检任务请求无效"
			switch {
			case errors.Is(err, diagnostics.ErrConflict):
				status, code, title = http.StatusConflict, "diagnostic_job_conflict", "已有体检任务正在运行"
			case errors.Is(err, diagnostics.ErrUnsupported):
				status, code, title = http.StatusServiceUnavailable, "diagnostics_unavailable", "体检服务不可用"
			}
			writeProblem(w, requestID, status, code, title, safeDetail(err))
			return
		}
		writeJSON(w, http.StatusAccepted, job)
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodPost)
		writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
	}
}

func (s *Server) diagnosticJob(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" {
		writeProblem(w, requestIDFrom(w), http.StatusBadRequest, "invalid_diagnostic_request", "体检请求无效", "")
		return
	}
	if s.diagnostics == nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "diagnostics_unavailable", "体检服务不可用", "")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/v1/diagnostic-jobs/")
	parts := strings.Split(rest, "/")
	id := parts[0]
	if !validSiteID(id) || len(parts) > 2 {
		writeProblem(w, requestIDFrom(w), http.StatusNotFound, "diagnostic_job_not_found", "体检任务不存在", "")
		return
	}
	if len(parts) == 2 && parts[1] == "terminal" {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeProblem(w, requestIDFrom(w), http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
			return
		}
		query, err := parseTerminalReadQuery(r.URL.Query(), false)
		if err != nil {
			writeProblem(w, requestIDFrom(w), http.StatusBadRequest, "invalid_terminal_offset", "终端偏移量无效", "")
			return
		}
		chunk, err := waitForTerminalChunk(
			r.Context(),
			query.Wait,
			func() (diagnostics.TerminalChunk, error) {
				return s.diagnostics.Terminal(id, query.Offset)
			},
			func(chunk diagnostics.TerminalChunk) bool {
				return chunk.DataBase64 != "" || chunk.Finished ||
					(query.HasInputState && chunk.InputOpen != query.InputOpen)
			},
		)
		if errors.Is(err, context.Canceled) {
			return
		}
		if err != nil {
			writeProblem(w, requestIDFrom(w), http.StatusNotFound, "diagnostic_terminal_not_found", "体检终端不存在", "")
			return
		}
		writeJSON(w, http.StatusOK, chunk)
		return
	}
	if len(parts) == 2 && parts[1] == "input" {
		if r.Method != http.MethodPost || r.URL.RawQuery != "" {
			w.Header().Set("Allow", http.MethodPost)
			writeProblem(w, requestIDFrom(w), http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
			return
		}
		var input struct {
			Data string `json:"data"`
		}
		if err := decodeJSON(w, r, &input); err != nil {
			writeProblem(w, requestIDFrom(w), http.StatusBadRequest, "invalid_request", "请求格式无效", "")
			return
		}
		if err := s.diagnostics.WriteInput(id, input.Data); err != nil {
			status, code, title := http.StatusConflict, "diagnostic_terminal_closed", "体检终端输入已关闭"
			if errors.Is(err, diagnostics.ErrInvalidInput) {
				status, code, title = http.StatusUnprocessableEntity, "invalid_terminal_input", "终端输入无效"
			}
			writeProblem(w, requestIDFrom(w), status, code, title, safeDetail(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	if len(parts) != 1 || r.Method != http.MethodGet || r.URL.RawQuery != "" {
		w.Header().Set("Allow", http.MethodGet)
		writeProblem(w, requestIDFrom(w), http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
		return
	}
	job, err := s.diagnostics.Job(id)
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusNotFound, "diagnostic_job_not_found", "体检任务不存在", "")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) dockerSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := s.docker.Summary(r.Context())
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "docker_unavailable", "Docker Engine 不可用", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (s *Server) dockerEnvironment(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.docker.Environment(r.Context()))
}

func (s *Server) containerList(w http.ResponseWriter, r *http.Request) {
	items, err := s.docker.Containers(r.Context())
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "docker_unavailable", "容器列表不可用", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, contract.PageResult[contract.ContainerSummary]{Items: items})
}

func (s *Server) composeProjectList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, contract.PageResult[dockerx.ComposeProjectSummary]{Items: s.docker.ComposeProjects()})
}

func (s *Server) composeProject(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestIDFrom(w), http.StatusBadRequest, "invalid_query", "Compose project query is invalid", "")
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/v1/docker/compose-projects/")
	project, err := s.docker.ComposeProject(r.Context(), name)
	if err != nil {
		status, code, title := http.StatusUnprocessableEntity, "compose_project_unavailable", "Compose 项目配置不可管理"
		switch {
		case errors.Is(err, dockerx.ErrDockerJobNotFound):
			status, code, title = http.StatusNotFound, "compose_project_not_found", "Compose 项目不存在"
		case errors.Is(err, dockerx.ErrResourceConflict):
			status, code, title = http.StatusConflict, "resource_conflict", "Compose 项目状态不一致"
		case errors.Is(err, dockerx.ErrInvalidDockerJob):
			status, code, title = http.StatusBadRequest, "compose_project_invalid", "Compose 项目名称无效"
		}
		writeProblem(w, requestIDFrom(w), status, code, title, safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (s *Server) containerStats(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestIDFrom(w), http.StatusBadRequest, "invalid_query", "Docker container stats query is invalid", "")
		return
	}
	result, err := s.docker.RunningContainerStats(r.Context(), 64, 4)
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "docker_unavailable", "Docker container stats are unavailable", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) nginxTest(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestIDFrom(w), http.StatusBadRequest, "invalid_query", "Nginx test query is invalid", "")
		return
	}
	err := s.docker.NginxTest(r.Context())
	if err == nil {
		writeJSON(w, http.StatusOK, map[string]any{"valid": true, "checkedAt": s.now().UTC()})
		return
	}
	var commandError *dockerx.NginxExecError
	if errors.As(err, &commandError) {
		writeJSON(w, http.StatusOK, map[string]any{
			"valid": false, "error": safeDetail(commandError), "checkedAt": s.now().UTC(),
		})
		return
	}
	writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "nginx_unavailable", "Managed Nginx is unavailable", safeDetail(err))
}

func (s *Server) nginxReload(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestIDFrom(w), http.StatusBadRequest, "invalid_query", "Nginx reload query is invalid", "")
		return
	}
	if err := s.docker.NginxTest(r.Context()); err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusUnprocessableEntity, "nginx_config_invalid", "Nginx configuration validation failed", safeDetail(err))
		return
	}
	if err := s.docker.NginxReload(r.Context()); err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "nginx_reload_failed", "Nginx reload failed", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": true, "reloaded": true, "reloadedAt": s.now().UTC()})
}

func (s *Server) imageList(w http.ResponseWriter, r *http.Request) {
	items, err := s.docker.Images(r.Context())
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "docker_unavailable", "镜像列表不可用", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, contract.PageResult[dockerx.ImageSummary]{Items: items})
}

func (s *Server) networkList(w http.ResponseWriter, r *http.Request) {
	items, err := s.docker.Networks(r.Context())
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "docker_unavailable", "网络列表不可用", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, contract.PageResult[dockerx.NetworkSummary]{Items: items})
}

func (s *Server) volumeList(w http.ResponseWriter, r *http.Request) {
	items, err := s.docker.Volumes(r.Context())
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "docker_unavailable", "存储卷列表不可用", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, contract.PageResult[dockerx.VolumeSummary]{Items: items})
}

func (s *Server) dockerBackupList(w http.ResponseWriter, _ *http.Request) {
	items, err := s.docker.DockerBackups()
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusServiceUnavailable, "docker_backups_unavailable", "Docker 备份列表不可用", safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, contract.PageResult[dockerx.DockerBackup]{Items: items})
}

func (s *Server) dockerJobList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, contract.PageResult[dockerx.MaintenanceJob]{
		Items: s.docker.MaintenanceJobs(),
	})
}

func (s *Server) dockerJob(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/docker/jobs/")
	job, err := s.docker.MaintenanceJob(id)
	if err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusNotFound, "docker_job_not_found", "Docker 任务不存在", "")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) dockerTask(w http.ResponseWriter, r *http.Request) {
	var input dockerx.MaintenanceInput
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, requestIDFrom(w), http.StatusBadRequest, "invalid_request", "请求格式无效", "")
		return
	}
	job, err := s.docker.StartMaintenance(r.Context(), input)
	if err != nil {
		status, code, title := http.StatusUnprocessableEntity, "docker_task_invalid", "Docker 任务请求无效"
		switch {
		case errors.Is(err, dockerx.ErrDockerJobConflict):
			status, code, title = http.StatusConflict, "docker_task_conflict", "已有 Docker 后台任务正在运行"
		case errors.Is(err, dockerx.ErrResourceConflict):
			status, code, title = http.StatusConflict, "resource_conflict", "Docker 资源已发生变化"
		case errors.Is(err, dockerx.ErrDockerJobNotFound):
			status, code, title = http.StatusNotFound, "docker_resource_not_found", "Docker 资源不存在"
		}
		writeProblem(w, requestIDFrom(w), status, code, title, safeDetail(err))
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) containerOperation(w http.ResponseWriter, r *http.Request, requestID string) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/docker/containers/")
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		writeProblem(w, requestID, http.StatusNotFound, "not_found", "资源不存在", "")
		return
	}
	id, action := parts[0], parts[1]
	if action == "logs" {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
			return
		}
		tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
		logs, err := s.docker.ContainerLogs(r.Context(), id, tail)
		if err != nil {
			if errors.Is(err, dockerx.ErrRuntimeContract) {
				writeProblem(w, requestID, http.StatusUnprocessableEntity, "container_logs_unsupported", "当前容器运行契约无法读取日志", "")
				return
			}
			writeProblem(w, requestID, http.StatusBadGateway, "docker_logs_failed", "容器日志不可用", safeDetail(err))
			return
		}
		writeJSON(w, http.StatusOK, logs)
		return
	}
	if action == "stats" {
		if r.Method != http.MethodGet || r.URL.RawPath != "" || r.URL.RawQuery != "" {
			w.Header().Set("Allow", http.MethodGet)
			writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
			return
		}
		stats, err := s.docker.ContainerStats(r.Context(), id)
		if err != nil {
			writeProblem(w, requestID, http.StatusBadGateway, "docker_stats_failed", "容器性能数据不可用", safeDetail(err))
			return
		}
		writeJSON(w, http.StatusOK, stats)
		return
	}
	if action == "exec" {
		if r.Method != http.MethodPost || r.URL.RawPath != "" || r.URL.RawQuery != "" {
			w.Header().Set("Allow", http.MethodPost)
			writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
			return
		}
		var input dockerx.ContainerExecInput
		if err := decodeJSON(w, r, &input); err != nil {
			writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "请求格式无效", "")
			return
		}
		result, err := s.docker.ContainerExec(r.Context(), id, input)
		if err != nil {
			status, code, title := http.StatusUnprocessableEntity, "container_exec_rejected", "容器控制台操作被拒绝"
			switch {
			case errors.Is(err, dockerx.ErrResourceConflict):
				status, code, title = http.StatusConflict, "resource_conflict", "资源已被其他操作修改"
			case errors.Is(err, dockerx.ErrRuntimeContract), errors.Is(err, dockerx.ErrActionUnsupported):
				status, code, title = http.StatusUnprocessableEntity, "container_exec_unsupported", "当前容器状态不支持控制台"
			case errors.Is(err, dockerx.ErrVersionRequired):
				status, code, title = http.StatusBadRequest, "resource_version_required", "必须提供资源版本"
			}
			writeProblem(w, requestID, status, code, title, safeDetail(err))
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeProblem(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不允许", "")
		return
	}
	var input struct {
		ResourceVersion string `json:"resourceVersion"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_request", "请求格式无效", "")
		return
	}
	result, err := s.docker.Lifecycle(r.Context(), id, action, input.ResourceVersion)
	if err != nil {
		status, code, title := http.StatusBadRequest, "docker_action_rejected", "容器操作被拒绝"
		switch {
		case errors.Is(err, dockerx.ErrResourceConflict):
			status, code, title = http.StatusConflict, "resource_conflict", "资源已被其他操作修改"
		case errors.Is(err, dockerx.ErrRuntimeContract), errors.Is(err, dockerx.ErrActionUnsupported):
			status, code, title = http.StatusUnprocessableEntity, "container_action_unsupported", "当前容器状态或运行契约不支持此操作"
		case errors.Is(err, dockerx.ErrVersionRequired):
			status, code, title = http.StatusBadRequest, "resource_version_required", "必须提供资源版本"
		}
		writeProblem(w, requestID, status, code, title, safeDetail(err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target interface{}) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxAgentRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra interface{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, requestID string, status int, code, title, detail string) {
	writeProblemWithRetryable(w, requestID, status, code, title, detail, status >= 500)
}

func writeProblemWithRetryable(
	w http.ResponseWriter,
	requestID string,
	status int,
	code string,
	title string,
	detail string,
	retryable bool,
) {
	writeJSON(w, status, contract.Problem{
		Type: "about:blank", Title: title, Status: status, Code: code,
		Detail: detail, RequestID: requestID, Retryable: retryable,
	})
}

func requestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(value[:])
}

func requestIDFrom(w http.ResponseWriter) string {
	return w.Header().Get("X-Request-ID")
}

func safeDetail(err error) string {
	if err == nil {
		return ""
	}
	value := err.Error()
	if len(value) > 500 {
		value = value[:500]
	}
	return value
}

func reasonIf(err error, reason string) string {
	if err != nil {
		return reason
	}
	return ""
}

func reasonUnless(value bool, reason string) string {
	if !value {
		return reason
	}
	if strings.Contains(reason, "仅") {
		return reason
	}
	return ""
}
