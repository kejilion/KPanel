package sites

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

type NginxController interface {
	NginxReady(context.Context) error
	NginxTest(context.Context) error
	NginxReload(context.Context) error
}

type SiteDataRuntime interface {
	DropSiteDatabase(context.Context, string) (bool, error)
}

type Manager struct {
	webRoot             string
	discoverer          *Discoverer
	nginx               NginxController
	siteDataRuntime     SiteDataRuntime
	scriptDeleter       siteScriptDeleter
	certificateReplacer siteCertificateReplacer
	recipeJobs          *recipeJobRegistry
	jobExecutable       string
	jobRunner           recipeJobCommandRunner
	testHook            func(stage, path string)
}

var siteWriteMutex sync.Mutex

func NewManager(webRoot string, discoverer *Discoverer, nginx NginxController) *Manager {
	if discoverer == nil {
		discoverer = NewDiscoverer(webRoot)
	}
	manager := &Manager{
		webRoot: filepath.Clean(webRoot), discoverer: discoverer, nginx: nginx,
		scriptDeleter:       kejilionSiteScriptDeleter{},
		certificateReplacer: scriptCertificateReplacer{},
	}
	if runtime, ok := nginx.(SiteDataRuntime); ok {
		manager.siteDataRuntime = runtime
	}
	manager.recipeJobs = newRecipeJobRegistry("")
	return manager
}

func (m *Manager) Writable(ctx context.Context) error {
	if err := m.writePrerequisites(ctx); err != nil {
		return err
	}
	if err := m.nginx.NginxTest(ctx); err != nil {
		return fmt.Errorf("%w: existing Nginx configuration is invalid: %v", ErrUnavailable, err)
	}
	return nil
}

func (m *Manager) writePrerequisites(ctx context.Context) error {
	if !atomicSiteWritesSupported() {
		return fmt.Errorf("%w: atomic site transactions require Linux", ErrUnavailable)
	}
	if !filepath.IsAbs(m.webRoot) || m.webRoot == string(filepath.Separator) {
		return fmt.Errorf("%w: managed Web root must be a dedicated absolute directory", ErrUnavailable)
	}
	if m.nginx == nil {
		return fmt.Errorf("%w: Nginx controller is unavailable", ErrUnavailable)
	}
	for _, path := range []string{
		m.webRoot,
		filepath.Join(m.webRoot, "conf.d"),
		filepath.Join(m.webRoot, "html"),
	} {
		info, err := os.Lstat(path)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: managed directory %s is unavailable or unsafe", ErrUnavailable, path)
		}
	}
	if err := m.nginx.NginxReady(ctx); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return nil
}

func (m *Manager) Create(ctx context.Context, input SiteInput) (contract.SiteSummary, error) {
	spec, err := normalizeSiteInput(input)
	if err != nil {
		return contract.SiteSummary{}, err
	}
	if input.ExpectedResourceVersion != "" {
		return contract.SiteSummary{}, fmt.Errorf("%w: expectedResourceVersion is not valid for create", ErrInvalidInput)
	}

	siteWriteMutex.Lock()
	defer siteWriteMutex.Unlock()
	if err := m.Writable(ctx); err != nil {
		return contract.SiteSummary{}, err
	}
	if err := m.checkCollisions(spec, ""); err != nil {
		return contract.SiteSummary{}, err
	}

	config := renderManagedConfig(spec)
	configPath := filepath.Join(m.webRoot, "conf.d", spec.Primary+".conf")
	configTemp, err := writeTemporaryFile(filepath.Dir(configPath), "."+spec.Primary+".kp-*.tmp", config, 0o640)
	if err != nil {
		return contract.SiteSummary{}, fmt.Errorf("%w: stage Nginx configuration: %v", ErrUnavailable, err)
	}
	defer os.Remove(configTemp)

	var staticPath, staticTemp string
	var staticInfo os.FileInfo
	if siteNeedsDocumentRoot(spec.Kind) {
		staticPath = filepath.Join(m.webRoot, "html", spec.Primary)
		indexName, indexBody := managedDefaultDocument(spec)
		staticTemp, err = stageStaticDirectory(
			filepath.Dir(staticPath),
			spec.Primary,
			indexName,
			indexBody,
		)
		if err != nil {
			return contract.SiteSummary{}, fmt.Errorf("%w: stage document root: %v", ErrUnavailable, err)
		}
		defer removeStagedStaticDirectory(staticTemp, indexName)
		if err := atomicNoReplace(staticTemp, staticPath); err != nil {
			if pathExists(staticPath) {
				return contract.SiteSummary{}, fmt.Errorf("%w: document root already exists", ErrConflict)
			}
			return contract.SiteSummary{}, fmt.Errorf("%w: publish document root: %v", ErrUnavailable, err)
		}
		staticTemp = ""
		staticInfo, _ = os.Lstat(staticPath)
		if err := syncDirectory(filepath.Dir(staticPath)); err != nil {
			if rollbackErr := m.removeCreatedStatic(staticPath, staticInfo, spec); rollbackErr != nil {
				return contract.SiteSummary{}, rollbackErr
			}
			return contract.SiteSummary{}, fmt.Errorf("%w: sync document root publication; candidate rolled back: %v", ErrUnavailable, err)
		}
	}

	m.callHook("before_config_publish", configPath)
	if err := atomicNoReplace(configTemp, configPath); err != nil {
		rollbackErr := m.removeCreatedStatic(staticPath, staticInfo, spec)
		if rollbackErr != nil {
			return contract.SiteSummary{}, rollbackErr
		}
		if pathExists(configPath) {
			return contract.SiteSummary{}, fmt.Errorf("%w: Nginx configuration already exists", ErrConflict)
		}
		return contract.SiteSummary{}, fmt.Errorf("%w: publish Nginx configuration: %v", ErrUnavailable, err)
	}
	configTemp = ""
	configInfo, _ := os.Lstat(configPath)
	if err := syncDirectory(filepath.Dir(configPath)); err != nil {
		if rollbackErr := m.rollbackCreate(configPath, configInfo, config, staticPath, staticInfo, spec); rollbackErr != nil {
			return contract.SiteSummary{}, rollbackErr
		}
		return contract.SiteSummary{}, fmt.Errorf("%w: sync configuration publication; candidate rolled back: %v", ErrUnavailable, err)
	}
	m.callHook("candidate_published", configPath)

	if !fileMatches(configPath, configInfo, config) {
		return contract.SiteSummary{}, fmt.Errorf("%w: candidate changed before validation", ErrNeedsAttention)
	}
	if err := m.nginx.NginxTest(ctx); err != nil {
		if rollbackErr := m.rollbackCreate(configPath, configInfo, config, staticPath, staticInfo, spec); rollbackErr != nil {
			return contract.SiteSummary{}, rollbackErr
		}
		return contract.SiteSummary{}, fmt.Errorf("%w: candidate failed nginx -t: %v", ErrUnprocessable, err)
	}
	if !fileMatches(configPath, configInfo, config) {
		return contract.SiteSummary{}, fmt.Errorf("%w: candidate changed during validation", ErrNeedsAttention)
	}
	if err := m.nginx.NginxReload(ctx); err != nil {
		if rollbackErr := m.rollbackCreate(configPath, configInfo, config, staticPath, staticInfo, spec); rollbackErr != nil {
			return contract.SiteSummary{}, rollbackErr
		}
		if recoveryErr := m.validateAndReloadPrevious(ctx); recoveryErr != nil {
			return contract.SiteSummary{}, recoveryErr
		}
		return contract.SiteSummary{}, fmt.Errorf("%w: Nginx reload failed and the candidate was rolled back: %v", ErrUnavailable, err)
	}
	if !fileMatches(configPath, configInfo, config) {
		return contract.SiteSummary{}, fmt.Errorf("%w: candidate changed while reloading", ErrNeedsAttention)
	}
	return m.discoverManaged(spec.Primary)
}

func (m *Manager) Update(ctx context.Context, id string, input SiteInput) (contract.SiteSummary, error) {
	spec, err := normalizeEditableSiteInput(input)
	if err != nil {
		return contract.SiteSummary{}, err
	}
	if input.ExpectedResourceVersion == "" {
		return contract.SiteSummary{}, fmt.Errorf("%w: expectedResourceVersion is required", ErrInvalidInput)
	}

	siteWriteMutex.Lock()
	defer siteWriteMutex.Unlock()
	if err := m.Writable(ctx); err != nil {
		return contract.SiteSummary{}, err
	}
	current, err := m.findManagedByID(id)
	if err != nil {
		return contract.SiteSummary{}, err
	}
	if current.PrimaryDomain != spec.Primary || current.Kind != spec.Kind {
		return contract.SiteSummary{}, fmt.Errorf("%w: primary domain and site type are immutable", ErrForbidden)
	}
	if current.ResourceVersion != input.ExpectedResourceVersion {
		return contract.SiteSummary{}, fmt.Errorf("%w: resource version changed", ErrConflict)
	}
	if err := m.checkCollisions(spec, current.ID); err != nil {
		return contract.SiteSummary{}, err
	}
	if current.Origin == contract.OriginCLI || current.Consistency != contract.ConsistencyInSync {
		return m.updateCLIConfig(ctx, current, spec, input.ExpectedResourceVersion)
	}
	if siteNeedsDocumentRoot(spec.Kind) {
		rootInfo, rootErr := os.Lstat(filepath.Join(m.webRoot, "html", spec.Primary))
		if rootErr != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
			return contract.SiteSummary{}, fmt.Errorf("%w: document root is unavailable or unsafe", ErrForbidden)
		}
	}

	oldSpec, err := managedSpecFromSummary(current)
	if err != nil {
		return contract.SiteSummary{}, fmt.Errorf("%w: current site is no longer canonical", ErrForbidden)
	}
	oldConfig := renderManagedConfig(oldSpec)
	newConfig := renderManagedConfig(spec)
	configPath := filepath.Join(m.webRoot, "conf.d", spec.Primary+".conf")
	oldInfo, err := os.Lstat(configPath)
	if err == nil && !fileMatches(configPath, oldInfo, oldConfig) &&
		(oldSpec.Kind == contract.SiteStatic || oldSpec.Kind == contract.SiteReverseProxy) {
		legacyConfig := renderManagedConfigV1(oldSpec)
		if fileMatches(configPath, oldInfo, legacyConfig) {
			oldConfig = legacyConfig
		}
	}
	if err != nil || !fileMatches(configPath, oldInfo, oldConfig) {
		return contract.SiteSummary{}, fmt.Errorf("%w: current configuration is no longer canonical", ErrForbidden)
	}
	if bytes.Equal(oldConfig, newConfig) {
		return current, nil
	}

	candidatePath, err := writeTemporaryFile(filepath.Dir(configPath), "."+spec.Primary+".kp-*.tmp", newConfig, 0o640)
	if err != nil {
		return contract.SiteSummary{}, fmt.Errorf("%w: stage Nginx configuration: %v", ErrUnavailable, err)
	}
	keepBackup := false
	defer func() {
		if !keepBackup {
			_ = os.Remove(candidatePath)
		}
	}()

	latest, err := m.findManagedByID(id)
	if err != nil || latest.ResourceVersion != input.ExpectedResourceVersion ||
		!fileMatches(configPath, oldInfo, oldConfig) {
		return contract.SiteSummary{}, fmt.Errorf("%w: resource changed before commit", ErrConflict)
	}
	m.callHook("before_exchange", configPath)
	if err := atomicExchange(candidatePath, configPath); err != nil {
		return contract.SiteSummary{}, fmt.Errorf("%w: atomically replace configuration: %v", ErrUnavailable, err)
	}
	candidateInfo, candidateInfoErr := os.Lstat(configPath)
	if err := syncDirectory(filepath.Dir(configPath)); err != nil {
		if candidateInfoErr != nil {
			keepBackup = true
			return contract.SiteSummary{}, fmt.Errorf("%w: inspect unsynced candidate; backup retained at %s: %v", ErrNeedsAttention, candidatePath, candidateInfoErr)
		}
		if restoreErr := restoreExchange(configPath, candidateInfo, newConfig, candidatePath); restoreErr != nil {
			keepBackup = true
			return contract.SiteSummary{}, restoreErr
		}
		return contract.SiteSummary{}, fmt.Errorf("%w: sync candidate publication; previous configuration restored: %v", ErrUnavailable, err)
	}
	if candidateInfoErr != nil {
		keepBackup = true
		return contract.SiteSummary{}, fmt.Errorf("%w: inspect published candidate; backup retained at %s: %v", ErrNeedsAttention, candidatePath, candidateInfoErr)
	}
	backupInfo, _ := os.Lstat(candidatePath)
	if !fileMatches(candidatePath, backupInfo, oldConfig) {
		if restoreErr := restoreExchange(configPath, candidateInfo, newConfig, candidatePath); restoreErr != nil {
			keepBackup = true
			return contract.SiteSummary{}, restoreErr
		}
		return contract.SiteSummary{}, fmt.Errorf("%w: external change won the update race", ErrConflict)
	}
	m.callHook("candidate_published", configPath)

	if !fileMatches(configPath, candidateInfo, newConfig) {
		keepBackup = true
		return contract.SiteSummary{}, fmt.Errorf("%w: candidate changed before validation; backup retained at %s", ErrNeedsAttention, candidatePath)
	}
	if err := m.nginx.NginxTest(ctx); err != nil {
		if restoreErr := restoreExchange(configPath, candidateInfo, newConfig, candidatePath); restoreErr != nil {
			keepBackup = true
			return contract.SiteSummary{}, restoreErr
		}
		return contract.SiteSummary{}, fmt.Errorf("%w: candidate failed nginx -t: %v", ErrUnprocessable, err)
	}
	if !fileMatches(configPath, candidateInfo, newConfig) {
		keepBackup = true
		return contract.SiteSummary{}, fmt.Errorf("%w: candidate changed during validation; backup retained at %s", ErrNeedsAttention, candidatePath)
	}
	if err := m.nginx.NginxReload(ctx); err != nil {
		if restoreErr := restoreExchange(configPath, candidateInfo, newConfig, candidatePath); restoreErr != nil {
			keepBackup = true
			return contract.SiteSummary{}, restoreErr
		}
		if recoveryErr := m.validateAndReloadPrevious(ctx); recoveryErr != nil {
			return contract.SiteSummary{}, recoveryErr
		}
		return contract.SiteSummary{}, fmt.Errorf("%w: Nginx reload failed and the previous configuration was restored: %v", ErrUnavailable, err)
	}
	if !fileMatches(configPath, candidateInfo, newConfig) {
		keepBackup = true
		return contract.SiteSummary{}, fmt.Errorf("%w: candidate changed while reloading; backup retained at %s", ErrNeedsAttention, candidatePath)
	}
	if err := os.Remove(candidatePath); err != nil {
		keepBackup = true
		return contract.SiteSummary{}, fmt.Errorf("%w: update succeeded but backup cleanup failed at %s: %v", ErrNeedsAttention, candidatePath, err)
	}
	if err := syncDirectory(filepath.Dir(configPath)); err != nil {
		return contract.SiteSummary{}, fmt.Errorf("%w: update succeeded but configuration directory cleanup could not be synced: %v", ErrNeedsAttention, err)
	}
	return m.discoverManaged(spec.Primary)
}

type DeleteResult struct {
	ID              string   `json:"id"`
	PrimaryDomain   string   `json:"primaryDomain"`
	Status          string   `json:"status"`
	Mode            string   `json:"mode"`
	ResourceVersion string   `json:"resourceVersion"`
	Removed         []string `json:"removed"`
	DatabaseDropped bool     `json:"databaseDropped"`
	Warnings        []string `json:"warnings,omitempty"`
}

type DeleteInput struct {
	PrimaryDomain string `json:"primaryDomain"`
}

type stagedDeleteArtifact struct {
	kind      string
	path      string
	backup    string
	info      os.FileInfo
	directory bool
}

func (m *Manager) DeleteWithOptions(
	ctx context.Context,
	id string,
	input DeleteInput,
) (DeleteResult, error) {
	return m.deleteWithScript(ctx, id, input.PrimaryDomain)
}

func (m *Manager) verifiedDeleteConfig(
	site contract.SiteSummary,
) (string, os.FileInfo, []byte, error) {
	confRoot := filepath.Join(m.webRoot, "conf.d")
	for _, artifact := range site.Artifacts {
		if artifact.Kind != "nginx_config" {
			continue
		}
		path := filepath.Clean(artifact.Path)
		relative, err := filepath.Rel(confRoot, path)
		if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) ||
			filepath.IsAbs(relative) || filepath.Ext(path) != ".conf" {
			break
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			break
		}
		if artifact.Hash == "" {
			return path, info, nil, nil
		}
		data, err := readRegularFile(path, maxConfigBytes)
		if err == nil && hashBytes(data) == artifact.Hash {
			return path, info, data, nil
		}
		break
	}
	return "", nil, nil, fmt.Errorf(
		"%w: current Nginx configuration is no longer the discovered artifact",
		ErrForbidden,
	)
}

func (m *Manager) detachedDeleteArtifacts(
	site contract.SiteSummary,
	mode string,
) ([]stagedDeleteArtifact, error) {
	roots := map[string]string{
		"orphan_html":        filepath.Join(m.webRoot, "html"),
		"orphan_certificate": filepath.Join(m.webRoot, "certs"),
	}
	var result []stagedDeleteArtifact
	seen := make(map[string]bool)
	appendArtifact := func(kind, candidate, root string) error {
		candidate = filepath.Clean(candidate)
		root = filepath.Clean(root)
		if candidate == root || !pathWithin(candidate, root) || seen[candidate] {
			return nil
		}
		info, err := os.Lstat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		seen[candidate] = true
		result = append(result, stagedDeleteArtifact{
			kind: kind, path: candidate, info: info, directory: info.IsDir(),
		})
		return nil
	}
	for _, artifact := range site.Artifacts {
		root, ok := roots[artifact.Kind]
		if !ok {
			continue
		}
		if err := appendArtifact(artifact.Kind, artifact.Path, root); err != nil {
			return nil, err
		}
		if mode == "full" && artifact.Kind == "orphan_certificate" &&
			strings.HasSuffix(artifact.Path, "_cert.pem") {
			keyPath := strings.TrimSuffix(artifact.Path, "_cert.pem") + "_key.pem"
			if err := appendArtifact("certificate_key", keyPath, root); err != nil {
				return nil, err
			}
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("%w: no removable site artifact was found", ErrForbidden)
	}
	return result, nil
}

func (m *Manager) fullDeleteArtifacts(
	site contract.SiteSummary,
	configData []byte,
	artifacts []stagedDeleteArtifact,
) ([]stagedDeleteArtifact, error) {
	seen := make(map[string]bool)
	for _, artifact := range artifacts {
		seen[filepath.Clean(artifact.path)] = true
	}
	appendCandidate := func(kind, candidate, root string) error {
		candidate = filepath.Clean(candidate)
		root = filepath.Clean(root)
		if candidate == root || !pathWithin(candidate, root) || seen[candidate] {
			return nil
		}
		info, err := os.Lstat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		seen[candidate] = true
		artifacts = append(artifacts, stagedDeleteArtifact{
			kind: kind, path: candidate, info: info, directory: info.IsDir(),
		})
		return nil
	}

	htmlRoot := filepath.Join(m.webRoot, "html")
	documentRoot := filepath.Clean(site.DocumentRoot)
	if site.Kind == contract.SiteWordPress && filepath.Base(documentRoot) == "wordpress" {
		documentRoot = filepath.Dir(documentRoot)
	}
	if site.DocumentRoot == "" {
		documentRoot = filepath.Join(htmlRoot, site.PrimaryDomain)
	}
	if err := appendCandidate("document_root", documentRoot, htmlRoot); err != nil {
		return nil, err
	}

	directives := parseDirectives(stripComments(string(configData)))
	certRoot := filepath.Join(m.webRoot, "certs")
	for _, raw := range uniqueStrings(directives["ssl_certificate"]) {
		if candidate, ok := m.discoverer.certificateHostPath(raw); ok {
			if err := appendCandidate("certificate", candidate, certRoot); err != nil {
				return nil, err
			}
		}
	}
	for _, raw := range uniqueStrings(directives["ssl_certificate_key"]) {
		if candidate, ok := m.discoverer.certificateHostPath(raw); ok {
			if err := appendCandidate("certificate_key", candidate, certRoot); err != nil {
				return nil, err
			}
		}
	}
	for _, candidate := range []struct {
		kind string
		path string
	}{
		{"certificate", filepath.Join(certRoot, site.PrimaryDomain+"_cert.pem")},
		{"certificate_key", filepath.Join(certRoot, site.PrimaryDomain+"_key.pem")},
	} {
		if err := appendCandidate(candidate.kind, candidate.path, certRoot); err != nil {
			return nil, err
		}
	}
	return artifacts, nil
}

func reserveDeleteBackup(path string) (string, error) {
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".kp-delete-*.tmp")
	if err != nil {
		return "", err
	}
	name := temp.Name()
	if err := temp.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	if err := os.Remove(name); err != nil {
		return "", err
	}
	return name, nil
}

func restoreDeleteArtifacts(staged []stagedDeleteArtifact) error {
	for index := len(staged) - 1; index >= 0; index-- {
		artifact := staged[index]
		if _, err := os.Lstat(artifact.path); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf(
				"%w: restore target is occupied; backup retained at %s",
				ErrNeedsAttention,
				artifact.backup,
			)
		}
		backupInfo, err := os.Lstat(artifact.backup)
		if err != nil || artifact.info == nil || !os.SameFile(artifact.info, backupInfo) {
			return fmt.Errorf(
				"%w: delete backup changed; retained at %s",
				ErrNeedsAttention,
				artifact.backup,
			)
		}
		if err := atomicNoReplace(artifact.backup, artifact.path); err != nil {
			return fmt.Errorf(
				"%w: restore %s from %s: %v",
				ErrNeedsAttention,
				artifact.kind,
				artifact.backup,
				err,
			)
		}
	}
	return syncDeleteDirectories(staged)
}

func syncDeleteDirectories(artifacts []stagedDeleteArtifact) error {
	seen := make(map[string]bool)
	for _, artifact := range artifacts {
		directory := filepath.Dir(artifact.path)
		if seen[directory] {
			continue
		}
		seen[directory] = true
		if err := syncDirectory(directory); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) restoreDeletedSite(
	ctx context.Context,
	staged []stagedDeleteArtifact,
) error {
	if err := restoreDeleteArtifacts(staged); err != nil {
		return err
	}
	return m.validateAndReloadPrevious(ctx)
}

func (m *Manager) checkCollisions(spec managedSpec, excludeID string) error {
	items, err := m.discoverer.Discover()
	if err != nil {
		return fmt.Errorf("%w: discover existing sites: %v", ErrUnavailable, err)
	}
	requested := make(map[string]bool, len(spec.Aliases)+1)
	requested[spec.Primary] = true
	for _, alias := range spec.Aliases {
		requested[alias] = true
	}
	for _, site := range items {
		if site.ID == excludeID {
			continue
		}
		if requested[site.PrimaryDomain] {
			return fmt.Errorf("%w: domain %s is already configured", ErrConflict, site.PrimaryDomain)
		}
		for _, domain := range site.Domains {
			if requested[domain] {
				return fmt.Errorf("%w: domain %s is already configured", ErrConflict, domain)
			}
		}
	}
	for domain := range requested {
		conflictPaths := []string{
			filepath.Join(m.webRoot, "conf.d", domain+".conf"),
			filepath.Join(m.webRoot, "html", domain),
		}
		for _, path := range conflictPaths {
			if excludeID != "" && domain == spec.Primary &&
				(path == filepath.Join(m.webRoot, "conf.d", spec.Primary+".conf") ||
					(siteNeedsDocumentRoot(spec.Kind) && path == filepath.Join(m.webRoot, "html", spec.Primary))) {
				continue
			}
			if _, statErr := os.Lstat(path); statErr == nil {
				return fmt.Errorf("%w: managed path already exists for %s", ErrConflict, domain)
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return fmt.Errorf("%w: inspect collision path: %v", ErrUnavailable, statErr)
			}
		}
	}
	return nil
}

func (m *Manager) findManagedByID(id string) (contract.SiteSummary, error) {
	return m.findActionableByID(id, "update")
}

func (m *Manager) findActionableByID(id, action string) (contract.SiteSummary, error) {
	items, err := m.discoverer.Discover()
	if err != nil {
		return contract.SiteSummary{}, fmt.Errorf("%w: discover sites: %v", ErrUnavailable, err)
	}
	for _, site := range items {
		if site.ID == id {
			if !containsString(site.AllowedActions, action) {
				return contract.SiteSummary{}, fmt.Errorf(
					"%w: the discovered artifact does not support %s",
					ErrForbidden,
					action,
				)
			}
			return site, nil
		}
	}
	return contract.SiteSummary{}, fmt.Errorf("%w: site does not exist", ErrConflict)
}

func (m *Manager) discoverManaged(primary string) (contract.SiteSummary, error) {
	items, err := m.discoverer.Discover()
	if err != nil {
		return contract.SiteSummary{}, fmt.Errorf("%w: rediscover committed site: %v", ErrNeedsAttention, err)
	}
	for _, site := range items {
		if site.PrimaryDomain == primary && site.Origin == contract.OriginWeb {
			return site, nil
		}
	}
	return contract.SiteSummary{}, fmt.Errorf("%w: committed site could not be rediscovered", ErrNeedsAttention)
}

func (m *Manager) rollbackCreate(
	configPath string,
	configInfo os.FileInfo,
	config []byte,
	staticPath string,
	staticInfo os.FileInfo,
	spec managedSpec,
) error {
	if !fileMatches(configPath, configInfo, config) {
		return fmt.Errorf("%w: candidate changed; refusing rollback", ErrNeedsAttention)
	}
	if err := os.Remove(configPath); err != nil {
		return fmt.Errorf("%w: remove failed candidate: %v", ErrNeedsAttention, err)
	}
	if err := syncDirectory(filepath.Dir(configPath)); err != nil {
		return fmt.Errorf("%w: sync rollback: %v", ErrNeedsAttention, err)
	}
	return m.removeCreatedStatic(staticPath, staticInfo, spec)
}

func (m *Manager) removeCreatedStatic(path string, expected os.FileInfo, spec managedSpec) error {
	if path == "" {
		return nil
	}
	current, err := os.Lstat(path)
	if err != nil || expected == nil || !os.SameFile(expected, current) || !current.IsDir() {
		return fmt.Errorf("%w: document root changed; refusing rollback", ErrNeedsAttention)
	}
	indexName, indexBody := managedDefaultDocument(spec)
	entries, err := os.ReadDir(path)
	if err != nil || len(entries) != 1 || entries[0].Name() != indexName ||
		entries[0].Type()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: document root contains external changes; refusing rollback", ErrNeedsAttention)
	}
	indexPath := filepath.Join(path, indexName)
	indexInfo, err := os.Lstat(indexPath)
	if err != nil || !fileMatches(indexPath, indexInfo, indexBody) {
		return fmt.Errorf("%w: default index changed; refusing rollback", ErrNeedsAttention)
	}
	if err := os.Remove(indexPath); err != nil {
		return fmt.Errorf("%w: remove staged index: %v", ErrNeedsAttention, err)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("%w: remove staged document root: %v", ErrNeedsAttention, err)
	}
	return syncDirectory(filepath.Dir(path))
}

func (m *Manager) validateAndReloadPrevious(ctx context.Context) error {
	if err := m.nginx.NginxTest(ctx); err != nil {
		return fmt.Errorf("%w: rollback completed but previous Nginx configuration is invalid: %v", ErrNeedsAttention, err)
	}
	if err := m.nginx.NginxReload(ctx); err != nil {
		return fmt.Errorf("%w: rollback completed but previous Nginx configuration could not be reloaded: %v", ErrNeedsAttention, err)
	}
	return nil
}

func (m *Manager) callHook(stage, path string) {
	if m.testHook != nil {
		m.testHook(stage, path)
	}
}

func restoreExchange(configPath string, candidateInfo os.FileInfo, candidate []byte, backupPath string) error {
	if !fileMatches(configPath, candidateInfo, candidate) {
		return fmt.Errorf("%w: candidate changed; previous configuration retained at %s", ErrNeedsAttention, backupPath)
	}
	if err := atomicExchange(configPath, backupPath); err != nil {
		return fmt.Errorf("%w: restore previous configuration from %s: %v", ErrNeedsAttention, backupPath, err)
	}
	if err := syncDirectory(filepath.Dir(configPath)); err != nil {
		return fmt.Errorf("%w: sync restored configuration: %v", ErrNeedsAttention, err)
	}
	return nil
}

func writeTemporaryFile(directory, pattern string, data []byte, mode os.FileMode) (string, error) {
	file, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return "", err
	}
	path := file.Name()
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(mode); err != nil {
		return "", err
	}
	if _, err := file.Write(data); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := syncDirectory(directory); err != nil {
		return "", err
	}
	ok = true
	return path, nil
}

func stageStaticDirectory(parent, primary, indexName string, index []byte) (string, error) {
	path, err := os.MkdirTemp(parent, "."+primary+".kp-*")
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			_ = removeStagedStaticDirectory(path, indexName)
		}
	}()
	if err := os.Chmod(path, 0o755); err != nil {
		return "", err
	}
	indexPath := filepath.Join(path, indexName)
	file, err := os.OpenFile(indexPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	if _, err := file.Write(index); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := syncDirectory(path); err != nil {
		return "", err
	}
	if err := syncDirectory(parent); err != nil {
		return "", err
	}
	ok = true
	return path, nil
}

func removeStagedStaticDirectory(path, indexName string) error {
	if path == "" {
		return nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if len(entries) != 1 || entries[0].Name() != indexName || entries[0].Type()&os.ModeSymlink != 0 {
		return errors.New("unexpected staged document root contents")
	}
	if err := os.Remove(filepath.Join(path, indexName)); err != nil {
		return err
	}
	return os.Remove(path)
}

func fileMatches(path string, expectedInfo os.FileInfo, expected []byte) bool {
	if expectedInfo == nil {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		!os.SameFile(expectedInfo, info) {
		return false
	}
	data, err := readRegularFile(path, int64(len(expected)))
	return err == nil && bytes.Equal(data, expected)
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
