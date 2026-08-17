package dockerx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const maxComposeSourceBytes = 24 << 10
const maxComposeProjectFiles = 8

var composeProjectPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

type ComposeProjectFile struct {
	Path            string `json:"path"`
	Name            string `json:"name"`
	Source          string `json:"source"`
	ResourceVersion string `json:"resourceVersion"`
}

type ComposeProject struct {
	Name             string               `json:"name"`
	WorkingDirectory string               `json:"workingDirectory"`
	ConfigFiles      []ComposeProjectFile `json:"configFiles"`
	Services         []string             `json:"services"`
	ResourceVersion  string               `json:"resourceVersion"`
}

type composeProjectState struct {
	ComposeProject
}

func (c *Client) ComposeProject(ctx context.Context, name string) (ComposeProject, error) {
	state, err := c.resolveComposeProject(ctx, name)
	if err != nil {
		return ComposeProject{}, err
	}
	return state.ComposeProject, nil
}

func (c *Client) resolveComposeProject(ctx context.Context, name string) (composeProjectState, error) {
	if !composeProjectPattern.MatchString(name) {
		return composeProjectState{}, ErrInvalidDockerJob
	}
	containers, err := c.Containers(ctx)
	if err != nil {
		return composeProjectState{}, err
	}
	workingDirectory := ""
	configLabel := ""
	services := make(map[string]struct{})
	var containerVersions []string
	for _, container := range containers {
		if container.ComposeProject != name {
			continue
		}
		labels := container.Labels
		candidateDirectory := strings.TrimSpace(labels["com.docker.compose.project.working_dir"])
		candidateFiles := strings.TrimSpace(labels["com.docker.compose.project.config_files"])
		if workingDirectory == "" {
			workingDirectory = candidateDirectory
		} else if candidateDirectory != "" && workingDirectory != candidateDirectory {
			return composeProjectState{}, ErrResourceConflict
		}
		if configLabel == "" {
			configLabel = candidateFiles
		} else if candidateFiles != "" && configLabel != candidateFiles {
			return composeProjectState{}, ErrResourceConflict
		}
		if container.ComposeService != "" {
			services[container.ComposeService] = struct{}{}
		}
		containerVersions = append(containerVersions, container.ResourceVersion)
	}
	if len(containerVersions) == 0 {
		return composeProjectState{}, ErrDockerJobNotFound
	}
	workingDirectory = filepath.Clean(filepath.FromSlash(workingDirectory))
	if !filepath.IsAbs(workingDirectory) || !c.composePathAllowed(workingDirectory) {
		return composeProjectState{}, ErrActionUnsupported
	}
	resolvedDirectory, err := filepath.EvalSymlinks(workingDirectory)
	if err != nil {
		return composeProjectState{}, ErrActionUnsupported
	}
	directoryInfo, err := os.Lstat(resolvedDirectory)
	if err != nil || !directoryInfo.IsDir() || directoryInfo.Mode()&os.ModeSymlink != 0 {
		return composeProjectState{}, ErrActionUnsupported
	}

	paths := composeConfigPaths(configLabel, resolvedDirectory)
	if len(paths) == 0 {
		paths = discoverDefaultComposeFiles(resolvedDirectory)
	}
	if len(paths) == 0 || len(paths) > maxComposeProjectFiles {
		return composeProjectState{}, ErrActionUnsupported
	}
	files := make([]ComposeProjectFile, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(path)
		if !filepath.IsAbs(path) || !c.composePathAllowed(path) {
			return composeProjectState{}, ErrActionUnsupported
		}
		info, statErr := os.Lstat(path)
		if statErr != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
			info.Size() <= 0 || info.Size() > maxComposeSourceBytes {
			return composeProjectState{}, ErrActionUnsupported
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil || !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
			return composeProjectState{}, ErrActionUnsupported
		}
		files = append(files, ComposeProjectFile{
			Path: path, Name: filepath.Base(path), Source: string(data),
			ResourceVersion: resourceHash(struct {
				Path string
				Mode os.FileMode
				Data []byte
			}{path, info.Mode().Perm(), data}),
		})
	}
	serviceNames := make([]string, 0, len(services))
	for service := range services {
		serviceNames = append(serviceNames, service)
	}
	sort.Strings(serviceNames)
	sort.Strings(containerVersions)
	project := ComposeProject{
		Name: name, WorkingDirectory: resolvedDirectory, ConfigFiles: files, Services: serviceNames,
	}
	project.ResourceVersion = resourceHash(struct {
		Name, WorkingDirectory string
		Files                  []ComposeProjectFile
		Services               []string
		Containers             []string
	}{name, resolvedDirectory, files, serviceNames, containerVersions})
	return composeProjectState{ComposeProject: project}, nil
}

func (c *Client) composePathAllowed(path string) bool {
	resolvedPath, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil || !filepath.IsAbs(resolvedPath) {
		return false
	}
	for _, root := range []string{c.appRoot, c.webRoot} {
		resolvedRoot, rootErr := filepath.EvalSymlinks(filepath.Clean(root))
		if rootErr != nil || !filepath.IsAbs(resolvedRoot) {
			continue
		}
		relative, relErr := filepath.Rel(resolvedRoot, resolvedPath)
		if relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func composeConfigPaths(label, workingDirectory string) []string {
	if label == "" {
		return nil
	}
	seen := make(map[string]struct{})
	var result []string
	for _, value := range strings.Split(label, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		path := filepath.FromSlash(value)
		if !filepath.IsAbs(path) {
			path = filepath.Join(workingDirectory, path)
		}
		path = filepath.Clean(path)
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	return result
}

func discoverDefaultComposeFiles(workingDirectory string) []string {
	for _, name := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
		path := filepath.Join(workingDirectory, name)
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return []string{path}
		}
	}
	return nil
}

func (c *Client) validateComposeDeploymentInput(ctx context.Context, input MaintenanceInput) error {
	project := strings.TrimSpace(input.Name)
	source := strings.TrimSpace(input.Compose)
	if project != input.Name || !composeProjectPattern.MatchString(project) || source == "" ||
		len(input.Compose) > maxComposeSourceBytes || !utf8.ValidString(input.Compose) ||
		strings.ContainsRune(input.Compose, 0) {
		return ErrInvalidDockerJob
	}
	root, err := c.resolvedDockerAppRoot()
	if err != nil {
		return err
	}
	target := filepath.Join(root, project)
	if !pathWithin(target, root) || target == root {
		return ErrInvalidDockerJob
	}
	if _, statErr := os.Lstat(target); statErr == nil {
		return ErrResourceConflict
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	containers, err := c.Containers(ctx)
	if err != nil {
		return err
	}
	for _, container := range containers {
		if container.ComposeProject == project {
			return ErrResourceConflict
		}
	}
	return nil
}

func (c *Client) deployComposeProject(ctx context.Context, input MaintenanceInput) error {
	if err := c.validateComposeDeploymentInput(ctx, input); err != nil {
		return err
	}
	root, err := c.resolvedDockerAppRoot()
	if err != nil {
		return err
	}
	projectDir := filepath.Join(root, input.Name)
	if err := os.Mkdir(projectDir, 0o750); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrResourceConflict
		}
		return err
	}
	composePath := filepath.Join(projectDir, "docker-compose.yml")
	source := input.Compose
	if !strings.HasSuffix(source, "\n") {
		source += "\n"
	}
	file, err := os.OpenFile(composePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		_ = cleanupComposeProjectDirectory(root, projectDir)
		return err
	}
	if _, err = file.WriteString(source); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = cleanupComposeProjectDirectory(root, projectDir)
		return err
	}
	if err := syncDirectoryPath(projectDir); err != nil {
		_ = cleanupComposeProjectDirectory(root, projectDir)
		return err
	}

	base := []string{
		"compose", "--project-directory", projectDir,
		"--file", composePath, "--project-name", input.Name,
	}
	services, err := c.runCompose(ctx, append(base, "config", "--services")...)
	if err != nil {
		_ = cleanupComposeProjectDirectory(root, projectDir)
		return fmt.Errorf("Compose configuration is invalid: %w", err)
	}
	if len(strings.Fields(string(services))) == 0 {
		_ = cleanupComposeProjectDirectory(root, projectDir)
		return errors.New("Compose configuration does not define an active service")
	}
	if _, err := c.runCompose(ctx, append(base, "up", "--detach")...); err != nil {
		return c.rollbackComposeDeployment(root, projectDir, base, "start Compose project", err)
	}
	containerIDs, err := c.runCompose(ctx, append(base, "ps", "--all", "--quiet")...)
	if err != nil {
		return c.rollbackComposeDeployment(root, projectDir, base, "verify Compose project", err)
	}
	validContainer := false
	for _, value := range strings.Fields(string(containerIDs)) {
		if containerIDPattern.MatchString(value) {
			validContainer = true
			break
		}
	}
	if !validContainer {
		return c.rollbackComposeDeployment(
			root, projectDir, base, "verify Compose project",
			errors.New("Docker Compose did not return a created container"),
		)
	}
	if err := syncDirectoryPath(root); err != nil {
		return fmt.Errorf("Compose project started but directory durability needs attention: %w", err)
	}
	return nil
}

func (c *Client) validateExistingComposeProjectInput(ctx context.Context, input MaintenanceInput) error {
	state, err := c.resolveComposeProject(ctx, strings.TrimSpace(input.Name))
	if err != nil {
		return err
	}
	if input.Name != strings.TrimSpace(input.Name) || input.ExpectedResourceVersion == "" ||
		input.ExpectedResourceVersion != state.ResourceVersion {
		return ErrResourceConflict
	}
	if input.Action != "compose_redeploy" {
		return nil
	}
	if strings.TrimSpace(input.Compose) == "" || len(input.Compose) > maxComposeSourceBytes ||
		!utf8.ValidString(input.Compose) || strings.ContainsRune(input.Compose, 0) {
		return ErrInvalidDockerJob
	}
	for _, file := range state.ConfigFiles {
		if file.Path == input.ComposeFile {
			return nil
		}
	}
	return ErrInvalidDockerJob
}

func (c *Client) runComposeProjectLifecycle(ctx context.Context, input MaintenanceInput, operation string) error {
	state, err := c.resolveComposeProject(ctx, input.Name)
	if err != nil {
		return err
	}
	if input.ExpectedResourceVersion == "" || input.ExpectedResourceVersion != state.ResourceVersion {
		return ErrResourceConflict
	}
	if operation != "start" && operation != "stop" && operation != "restart" {
		return ErrInvalidDockerJob
	}
	_, err = c.runCompose(ctx, append(composeProjectBase(state.ComposeProject), operation)...)
	if err != nil {
		return fmt.Errorf("Compose project %s failed: %w", operation, err)
	}
	return nil
}

func (c *Client) redeployComposeProject(ctx context.Context, input MaintenanceInput) error {
	state, err := c.resolveComposeProject(ctx, input.Name)
	if err != nil {
		return err
	}
	if input.ExpectedResourceVersion == "" || input.ExpectedResourceVersion != state.ResourceVersion {
		return ErrResourceConflict
	}
	var selected ComposeProjectFile
	found := false
	for _, file := range state.ConfigFiles {
		if file.Path == input.ComposeFile {
			selected = file
			found = true
			break
		}
	}
	if !found || strings.TrimSpace(input.Compose) == "" || len(input.Compose) > maxComposeSourceBytes ||
		!utf8.ValidString(input.Compose) || strings.ContainsRune(input.Compose, 0) {
		return ErrInvalidDockerJob
	}
	info, err := os.Lstat(selected.Path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return ErrResourceConflict
	}
	original, err := os.ReadFile(selected.Path)
	if err != nil {
		return err
	}
	currentVersion := resourceHash(struct {
		Path string
		Mode os.FileMode
		Data []byte
	}{selected.Path, info.Mode().Perm(), original})
	if currentVersion != selected.ResourceVersion {
		return ErrResourceConflict
	}
	updated := []byte(input.Compose)
	if !bytes.HasSuffix(updated, []byte("\n")) {
		updated = append(updated, '\n')
	}
	stagedPath, err := stageComposeProjectFile(selected.Path, updated, info)
	if err != nil {
		return err
	}
	defer os.Remove(stagedPath)
	stagedProject := state.ComposeProject
	stagedProject.ConfigFiles = append([]ComposeProjectFile(nil), state.ConfigFiles...)
	for index := range stagedProject.ConfigFiles {
		if stagedProject.ConfigFiles[index].Path == selected.Path {
			stagedProject.ConfigFiles[index].Path = stagedPath
		}
	}
	services, err := c.runCompose(ctx, append(composeProjectBase(stagedProject), "config", "--services")...)
	if err != nil {
		return fmt.Errorf("Compose configuration is invalid: %w", err)
	}
	if len(strings.Fields(string(services))) == 0 {
		return errors.New("Compose configuration does not define an active service")
	}
	if err := replaceComposeProjectFile(stagedPath, selected.Path); err != nil {
		return err
	}
	base := composeProjectBase(state.ComposeProject)
	if _, err := c.runCompose(ctx, append(base, "up", "--detach", "--remove-orphans")...); err != nil {
		return c.rollbackComposeRedeploy(state.ComposeProject, selected.Path, original, info, err)
	}
	containerIDs, err := c.runCompose(ctx, append(base, "ps", "--all", "--quiet")...)
	if err != nil || !composeOutputHasContainer(containerIDs) {
		if err == nil {
			err = errors.New("Docker Compose did not return a created container")
		}
		return c.rollbackComposeRedeploy(state.ComposeProject, selected.Path, original, info, err)
	}
	return nil
}

func (c *Client) rollbackComposeRedeploy(
	project ComposeProject,
	path string,
	original []byte,
	info os.FileInfo,
	cause error,
) error {
	if err := atomicWriteComposeProjectFile(path, original, info); err != nil {
		return fmt.Errorf("Compose redeploy failed and configuration rollback needs attention: %w", cause)
	}
	rollbackContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := c.runCompose(
		rollbackContext,
		append(composeProjectBase(project), "up", "--detach", "--remove-orphans")...,
	); err != nil {
		return fmt.Errorf("Compose redeploy failed and runtime rollback needs attention: %w", cause)
	}
	return fmt.Errorf("Compose redeploy failed; previous configuration restored: %w", cause)
}

func composeProjectBase(project ComposeProject) []string {
	base := []string{"compose", "--project-directory", project.WorkingDirectory}
	for _, file := range project.ConfigFiles {
		base = append(base, "--file", file.Path)
	}
	return append(base, "--project-name", project.Name)
}

func composeOutputHasContainer(output []byte) bool {
	for _, value := range strings.Fields(string(output)) {
		if containerIDPattern.MatchString(value) {
			return true
		}
	}
	return false
}

func stageComposeProjectFile(path string, data []byte, info os.FileInfo) (string, error) {
	temp, err := os.CreateTemp(filepath.Dir(path), ".kpanel-compose-*.tmp")
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	failed := true
	defer func() {
		if failed {
			_ = temp.Close()
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(info.Mode().Perm()); err != nil {
		return "", err
	}
	uid, gid, err := fileNumericOwnership(info)
	if err != nil {
		return "", err
	}
	if err := applyNumericOwnership(tempPath, uid, gid); err != nil {
		return "", err
	}
	if _, err := temp.Write(data); err != nil {
		return "", err
	}
	if err := temp.Sync(); err != nil {
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	failed = false
	return tempPath, nil
}

func replaceComposeProjectFile(stagedPath, target string) error {
	if runtime.GOOS == "windows" {
		if err := os.Remove(target); err != nil {
			return err
		}
	}
	if err := os.Rename(stagedPath, target); err != nil {
		return err
	}
	return syncDirectoryPath(filepath.Dir(target))
}

func atomicWriteComposeProjectFile(path string, data []byte, info os.FileInfo) error {
	stagedPath, err := stageComposeProjectFile(path, data, info)
	if err != nil {
		return err
	}
	defer os.Remove(stagedPath)
	return replaceComposeProjectFile(stagedPath, path)
}

func (c *Client) rollbackComposeDeployment(
	root string,
	projectDir string,
	base []string,
	step string,
	cause error,
) error {
	rollbackContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, rollbackErr := c.runCompose(rollbackContext, append(base, "down", "--remove-orphans")...)
	if rollbackErr != nil {
		return fmt.Errorf("%s failed and rollback needs attention: %w", step, cause)
	}
	if cleanupErr := cleanupComposeProjectDirectory(root, projectDir); cleanupErr != nil {
		return fmt.Errorf("%s failed; containers rolled back but project cleanup needs attention: %w", step, cause)
	}
	return fmt.Errorf("%s failed; Compose project rolled back: %w", step, cause)
}

func (c *Client) runCompose(ctx context.Context, arguments ...string) ([]byte, error) {
	run := c.composeCommand
	if run == nil {
		run = runFixedDockerComposeCommand
	}
	output, err := run(ctx, arguments...)
	if err == nil {
		return output, nil
	}
	detail := strings.TrimSpace(redactText(string(output)))
	if len(detail) > 400 {
		detail = detail[:400]
	}
	if detail == "" {
		return output, err
	}
	return output, fmt.Errorf("%w: %s", err, detail)
}

func cleanupComposeProjectDirectory(root, target string) error {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if target == root || !pathWithin(target, root) {
		return errors.New("Compose cleanup target is unsafe")
	}
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("Compose cleanup target is unavailable or unsafe")
	}
	if err := os.RemoveAll(target); err != nil {
		return err
	}
	return syncDirectoryPath(root)
}
