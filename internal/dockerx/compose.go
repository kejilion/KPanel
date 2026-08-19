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
const maxComposeEnvironmentBytes = 24 << 10
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
	EnvironmentFile  *ComposeProjectFile  `json:"environmentFile,omitempty"`
	Services         []string             `json:"services"`
	ResourceVersion  string               `json:"resourceVersion"`
}

type ComposeProjectSummary struct {
	Name string `json:"name"`
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

func (c *Client) ComposeProjects() []ComposeProjectSummary {
	names := make(map[string]struct{})
	for _, root := range []string{c.appRoot, c.webRoot} {
		resolvedRoot, err := filepath.EvalSymlinks(filepath.Clean(root))
		if err != nil || !filepath.IsAbs(resolvedRoot) {
			continue
		}
		entries, err := os.ReadDir(resolvedRoot)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			name := entry.Name()
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !composeProjectPattern.MatchString(name) {
				continue
			}
			candidate, err := filepath.EvalSymlinks(filepath.Join(resolvedRoot, name))
			if err != nil || !pathWithin(candidate, resolvedRoot) || candidate == resolvedRoot ||
				len(discoverDefaultComposeFiles(candidate)) == 0 {
				continue
			}
			names[name] = struct{}{}
		}
	}
	result := make([]ComposeProjectSummary, 0, len(names))
	for name := range names {
		result = append(result, ComposeProjectSummary{Name: name})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].Name < result[right].Name })
	return result
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
	}
	if workingDirectory == "" {
		workingDirectory, err = c.discoverManagedComposeProjectDirectory(name)
		if err != nil {
			return composeProjectState{}, err
		}
		configLabel = ""
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
		pathInfo, statErr := os.Lstat(path)
		if statErr != nil || !pathInfo.Mode().IsRegular() || pathInfo.Mode()&os.ModeSymlink != 0 {
			return composeProjectState{}, ErrActionUnsupported
		}
		path, statErr = filepath.EvalSymlinks(path)
		if statErr != nil || !filepath.IsAbs(path) || !c.composePathAllowed(path) {
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
	var environmentFile *ComposeProjectFile
	environmentPath := filepath.Join(resolvedDirectory, ".env")
	environmentInfo, environmentErr := os.Lstat(environmentPath)
	if environmentErr == nil {
		if !environmentInfo.Mode().IsRegular() || environmentInfo.Mode()&os.ModeSymlink != 0 ||
			environmentInfo.Size() > maxComposeEnvironmentBytes {
			return composeProjectState{}, ErrActionUnsupported
		}
		environmentData, readErr := os.ReadFile(environmentPath)
		if readErr != nil || !utf8.Valid(environmentData) || bytes.IndexByte(environmentData, 0) >= 0 {
			return composeProjectState{}, ErrActionUnsupported
		}
		environmentFile = &ComposeProjectFile{
			Path: environmentPath, Name: ".env", Source: string(environmentData),
			ResourceVersion: resourceHash(struct {
				Path string
				Mode os.FileMode
				Data []byte
			}{environmentPath, environmentInfo.Mode().Perm(), environmentData}),
		}
	} else if !errors.Is(environmentErr, os.ErrNotExist) {
		return composeProjectState{}, ErrActionUnsupported
	}
	serviceNames := make([]string, 0, len(services))
	for service := range services {
		serviceNames = append(serviceNames, service)
	}
	sort.Strings(serviceNames)
	project := ComposeProject{
		Name: name, WorkingDirectory: resolvedDirectory, ConfigFiles: files,
		EnvironmentFile: environmentFile, Services: serviceNames,
	}
	project.ResourceVersion = resourceHash(struct {
		Name, WorkingDirectory string
		Files                  []ComposeProjectFile
		EnvironmentFile        *ComposeProjectFile
	}{name, resolvedDirectory, files, environmentFile})
	return composeProjectState{ComposeProject: project}, nil
}

func (c *Client) discoverManagedComposeProjectDirectory(name string) (string, error) {
	var candidates []string
	for _, root := range []string{c.appRoot, c.webRoot} {
		resolvedRoot, err := filepath.EvalSymlinks(filepath.Clean(root))
		if err != nil || !filepath.IsAbs(resolvedRoot) {
			continue
		}
		candidate := filepath.Join(resolvedRoot, name)
		if !pathWithin(candidate, resolvedRoot) || candidate == resolvedRoot {
			continue
		}
		resolvedCandidate, err := filepath.EvalSymlinks(candidate)
		if err != nil || !pathWithin(resolvedCandidate, resolvedRoot) || resolvedCandidate == resolvedRoot {
			continue
		}
		info, err := os.Lstat(resolvedCandidate)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 ||
			len(discoverDefaultComposeFiles(resolvedCandidate)) == 0 {
			continue
		}
		candidates = append(candidates, resolvedCandidate)
	}
	if len(candidates) == 0 {
		return "", ErrDockerJobNotFound
	}
	if len(candidates) > 1 {
		return "", ErrResourceConflict
	}
	return candidates[0], nil
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
		strings.ContainsRune(input.Compose, 0) || !validComposeEnvironment(input.ComposeEnvironment) {
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
	if err := writeNewComposeProjectFile(composePath, []byte(source), 0o640); err != nil {
		_ = cleanupComposeProjectDirectory(root, projectDir)
		return err
	}
	environmentPath := filepath.Join(projectDir, ".env")
	environmentSource := composeEnvironmentValue(input.ComposeEnvironment)
	if environmentSource != "" && !strings.HasSuffix(environmentSource, "\n") {
		environmentSource += "\n"
	}
	if err := writeNewComposeProjectFile(environmentPath, []byte(environmentSource), 0o600); err != nil {
		_ = cleanupComposeProjectDirectory(root, projectDir)
		return err
	}
	if err := syncDirectoryPath(projectDir); err != nil {
		_ = cleanupComposeProjectDirectory(root, projectDir)
		return err
	}

	base := []string{
		"compose", "--env-file", environmentPath, "--project-directory", projectDir,
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
		!utf8.ValidString(input.Compose) || strings.ContainsRune(input.Compose, 0) ||
		!validComposeEnvironment(input.ComposeEnvironment) {
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
		!utf8.ValidString(input.Compose) || strings.ContainsRune(input.Compose, 0) ||
		!validComposeEnvironment(input.ComposeEnvironment) {
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
	environmentPath := filepath.Join(state.WorkingDirectory, ".env")
	environmentExisted := state.EnvironmentFile != nil
	var environmentInfo os.FileInfo
	var environmentOriginal []byte
	if environmentExisted {
		environmentInfo, err = os.Lstat(environmentPath)
		if err != nil || !environmentInfo.Mode().IsRegular() || environmentInfo.Mode()&os.ModeSymlink != 0 {
			return ErrResourceConflict
		}
		environmentOriginal, err = os.ReadFile(environmentPath)
		if err != nil {
			return err
		}
		environmentVersion := resourceHash(struct {
			Path string
			Mode os.FileMode
			Data []byte
		}{environmentPath, environmentInfo.Mode().Perm(), environmentOriginal})
		if environmentVersion != state.EnvironmentFile.ResourceVersion {
			return ErrResourceConflict
		}
	} else if _, statErr := os.Lstat(environmentPath); !errors.Is(statErr, os.ErrNotExist) {
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
	environmentSource := string(environmentOriginal)
	if input.ComposeEnvironment != nil {
		environmentSource = *input.ComposeEnvironment
	}
	updatedEnvironment := []byte(environmentSource)
	if len(updatedEnvironment) > 0 && !bytes.HasSuffix(updatedEnvironment, []byte("\n")) {
		updatedEnvironment = append(updatedEnvironment, '\n')
	}
	stagedEnvironmentPath, err := stageComposeEnvironmentFile(environmentPath, updatedEnvironment, environmentInfo)
	if err != nil {
		return err
	}
	defer os.Remove(stagedEnvironmentPath)
	stagedProject := state.ComposeProject
	stagedProject.ConfigFiles = append([]ComposeProjectFile(nil), state.ConfigFiles...)
	for index := range stagedProject.ConfigFiles {
		if stagedProject.ConfigFiles[index].Path == selected.Path {
			stagedProject.ConfigFiles[index].Path = stagedPath
		}
	}
	stagedProject.EnvironmentFile = &ComposeProjectFile{Path: stagedEnvironmentPath}
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
	if err := replaceComposeProjectFile(stagedEnvironmentPath, environmentPath); err != nil {
		_ = atomicWriteComposeProjectFile(selected.Path, original, info)
		return err
	}
	deployedProject := state.ComposeProject
	deployedProject.EnvironmentFile = &ComposeProjectFile{Path: environmentPath}
	base := composeProjectBase(deployedProject)
	if _, err := c.runCompose(ctx, append(base, "up", "--detach", "--remove-orphans")...); err != nil {
		return c.rollbackComposeRedeploy(
			state.ComposeProject, selected.Path, original, info,
			environmentPath, environmentOriginal, environmentInfo, environmentExisted, err,
		)
	}
	containerIDs, err := c.runCompose(ctx, append(base, "ps", "--all", "--quiet")...)
	if err != nil || !composeOutputHasContainer(containerIDs) {
		if err == nil {
			err = errors.New("Docker Compose did not return a created container")
		}
		return c.rollbackComposeRedeploy(
			state.ComposeProject, selected.Path, original, info,
			environmentPath, environmentOriginal, environmentInfo, environmentExisted, err,
		)
	}
	return nil
}

func (c *Client) rollbackComposeRedeploy(
	project ComposeProject,
	path string,
	original []byte,
	info os.FileInfo,
	environmentPath string,
	environmentOriginal []byte,
	environmentInfo os.FileInfo,
	environmentExisted bool,
	cause error,
) error {
	if err := atomicWriteComposeProjectFile(path, original, info); err != nil {
		return fmt.Errorf("Compose redeploy failed and configuration rollback needs attention: %w", cause)
	}
	if err := restoreComposeEnvironmentFile(
		environmentPath, environmentOriginal, environmentInfo, environmentExisted,
	); err != nil {
		return fmt.Errorf("Compose redeploy failed and environment rollback needs attention: %w", cause)
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
	base := []string{"compose"}
	if project.EnvironmentFile != nil {
		base = append(base, "--env-file", project.EnvironmentFile.Path)
	}
	base = append(base, "--project-directory", project.WorkingDirectory)
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

func validComposeEnvironment(source *string) bool {
	return source == nil || len(*source) <= maxComposeEnvironmentBytes && utf8.ValidString(*source) &&
		!strings.ContainsRune(*source, 0)
}

func composeEnvironmentValue(source *string) string {
	if source == nil {
		return ""
	}
	return *source
}

func writeNewComposeProjectFile(path string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	return err
}

func stageComposeEnvironmentFile(path string, data []byte, info os.FileInfo) (string, error) {
	if info != nil {
		stagedPath, err := stageComposeProjectFile(path, data, info)
		if err != nil {
			return "", err
		}
		if err := os.Chmod(stagedPath, 0o600); err != nil {
			_ = os.Remove(stagedPath)
			return "", err
		}
		return stagedPath, nil
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".kpanel-env-*.tmp")
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
	if err := temp.Chmod(0o600); err != nil {
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

func restoreComposeEnvironmentFile(path string, data []byte, info os.FileInfo, existed bool) error {
	if existed {
		return atomicWriteComposeProjectFile(path, data, info)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncDirectoryPath(filepath.Dir(path))
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
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
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
