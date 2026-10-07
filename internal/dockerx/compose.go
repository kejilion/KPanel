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

	"github.com/kejilion/kejilion-panel/internal/contract"
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

type composeEditFile struct {
	info os.FileInfo // nil means the path must remain absent
	data []byte
}

// This guard closes external-command windows, not the final check/rename race
// with writers that do not coordinate with KPanel. Real files remain authoritative.
type composeEditGuard struct {
	files       map[string]composeEditFile
	directories map[string]os.FileInfo
	replaceFile func(string, string) (bool, error)
}

func newComposeEditGuard(project ComposeProject) (*composeEditGuard, error) {
	guard := &composeEditGuard{
		files: make(map[string]composeEditFile), directories: make(map[string]os.FileInfo),
		replaceFile: replaceComposeProjectFile,
	}
	files := append([]ComposeProjectFile(nil), project.ConfigFiles...)
	environment := ComposeProjectFile{Path: filepath.Join(project.WorkingDirectory, ".env")}
	if project.EnvironmentFile != nil {
		environment = *project.EnvironmentFile
	}
	files = append(files, environment)
	for _, file := range files {
		dir := filepath.Dir(file.Path)
		info, err := os.Lstat(dir)
		resolved, resolveErr := filepath.EvalSymlinks(dir)
		if err != nil || resolveErr != nil || resolved != dir || !info.IsDir() {
			return nil, composeEditConflict()
		}
		guard.directories[dir] = info
		current, err := readComposeEditFile(file.Path)
		if err != nil {
			return nil, composeEditConflict()
		}
		if file.ResourceVersion == "" {
			if current.info != nil {
				return nil, composeEditConflict()
			}
		} else if current.info == nil || resourceHash(struct {
			Path string
			Mode os.FileMode
			Data []byte
		}{file.Path, current.info.Mode().Perm(), current.data}) != file.ResourceVersion {
			return nil, composeEditConflict()
		}
		guard.files[file.Path] = current
	}
	return guard, guard.check()
}

func composeEditConflict() error {
	return fmt.Errorf("Compose configuration changed externally; current files preserved and need review: %w", ErrResourceConflict)
}

func readComposeEditFile(path string) (composeEditFile, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return composeEditFile{}, nil
	}
	if err != nil {
		return composeEditFile{}, err
	}
	data, err := readBoundedRegularFile(path, maxComposeSourceBytes)
	if err != nil {
		return composeEditFile{}, err
	}
	after, err := os.Lstat(path)
	if err != nil || !sameComposeEditIdentity(info, after) {
		return composeEditFile{}, composeEditConflict()
	}
	return composeEditFile{info: info, data: data}, nil
}

func sameComposeEditIdentity(left, right os.FileInfo) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	uid, gid, err := fileNumericOwnership(left)
	otherUID, otherGID, otherErr := fileNumericOwnership(right)
	return err == nil && otherErr == nil && os.SameFile(left, right) &&
		left.Mode() == right.Mode() && uid == otherUID && gid == otherGID
}

func (guard *composeEditGuard) check() error {
	for path, expected := range guard.directories {
		info, err := os.Lstat(path)
		resolved, resolveErr := filepath.EvalSymlinks(path)
		if err != nil || resolveErr != nil || resolved != path || !sameComposeEditIdentity(expected, info) {
			return composeEditConflict()
		}
	}
	for path, expected := range guard.files {
		current, err := readComposeEditFile(path)
		if err != nil || !sameComposeEditIdentity(expected.info, current.info) || !bytes.Equal(expected.data, current.data) {
			return composeEditConflict()
		}
	}
	return nil
}

func (guard *composeEditGuard) replace(stagedPath, target string, staged composeEditFile) error {
	if err := guard.check(); err != nil {
		return err
	}
	current, err := readComposeEditFile(stagedPath)
	if err != nil || staged.info == nil || !sameComposeEditIdentity(staged.info, current.info) || !bytes.Equal(staged.data, current.data) {
		return errors.New("Compose staged configuration is unavailable or changed")
	}
	replaced, err := guard.replaceFile(stagedPath, target)
	if replaced {
		// A failed directory sync does not undo the successful rename.
		guard.files[target] = staged
	}
	return err
}

func (guard *composeEditGuard) restore(path string, original composeEditFile) error {
	if err := guard.check(); err != nil {
		return err
	}
	if sameComposeEditIdentity(guard.files[path].info, original.info) && bytes.Equal(guard.files[path].data, original.data) {
		return nil
	}
	if original.info == nil {
		if err := os.Remove(path); err != nil {
			return err
		}
		guard.files[path] = original
		return syncDirectoryPath(filepath.Dir(path))
	}
	stagedPath, err := stageComposeProjectFile(path, original.data, original.info)
	if err != nil {
		return err
	}
	defer os.Remove(stagedPath)
	staged, err := readComposeEditFile(stagedPath)
	if err != nil || !bytes.Equal(staged.data, original.data) {
		return errors.New("Compose staged configuration is unavailable or changed")
	}
	return guard.replace(stagedPath, path, staged)
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
	for _, directory := range c.managedComposeDirectories() {
		name, files, err := discoverComposeProject(directory)
		if err == nil && composeProjectPattern.MatchString(name) && len(files) > 0 {
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

// Include the roots themselves: the shared web stack commonly lives in /home/web.
func (c *Client) managedComposeDirectories() []string {
	seen := make(map[string]bool)
	var directories []string
	for _, root := range []string{c.appRoot, c.webRoot} {
		resolvedRoot, err := filepath.EvalSymlinks(filepath.Clean(root))
		if err != nil || !filepath.IsAbs(resolvedRoot) {
			continue
		}
		if !seen[resolvedRoot] {
			seen[resolvedRoot] = true
			directories = append(directories, resolvedRoot)
		}
		entries, err := os.ReadDir(resolvedRoot)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			candidate, err := filepath.EvalSymlinks(filepath.Join(resolvedRoot, entry.Name()))
			if err != nil || !pathWithin(candidate, resolvedRoot) || candidate == resolvedRoot || seen[candidate] {
				continue
			}
			seen[candidate] = true
			directories = append(directories, candidate)
		}
	}
	return directories
}

func (c *Client) resolveComposeProject(ctx context.Context, name string) (composeProjectState, error) {
	if !composeProjectPattern.MatchString(name) {
		return composeProjectState{}, ErrInvalidDockerJob
	}
	containers, err := c.ContainerListSummaries(ctx)
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
		_, paths, err = discoverComposeProject(resolvedDirectory)
		if err != nil {
			return composeProjectState{}, err
		}
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
	for _, directory := range c.managedComposeDirectories() {
		candidateName, files, err := discoverComposeProject(directory)
		if err == nil && candidateName == name && len(files) > 0 {
			candidates = append(candidates, directory)
		}
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

// Read only literal native Compose discovery variables. Other dotenv values are
// skipped, including multiline quoted values; interpolation is left to Compose.
func composeDiscoveryVariables(data []byte) (map[string]string, error) {
	values := make(map[string]string)
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil, ErrActionUnsupported
	}
	source := strings.TrimPrefix(string(data), "\ufeff")
	for source != "" {
		source = strings.TrimLeft(source, " \t\r\n")
		if source == "" {
			break
		}
		line, rest, _ := strings.Cut(source, "\n")
		if strings.HasPrefix(line, "#") {
			source = rest
			continue
		}
		index := strings.IndexAny(line, "=:")
		if index < 0 {
			source = rest
			continue
		}
		key := strings.TrimSpace(strings.TrimPrefix(line[:index], "export "))
		wanted := key == "COMPOSE_PROJECT_NAME" || key == "COMPOSE_FILE" || key == "COMPOSE_PATH_SEPARATOR"
		source = strings.TrimLeft(source[index+1:], " \t")
		var value string
		literal := false
		if source != "" && (source[0] == '\'' || source[0] == '"') {
			quote := source[0]
			literal = quote == '\''
			var builder strings.Builder
			closed := false
			for index = 1; index < len(source); index++ {
				character := source[index]
				if character == quote {
					closed = true
					break
				}
				if character == '\\' && index+1 < len(source) && (source[index+1] == quote || quote == '"') {
					if source[index+1] != quote {
						builder.WriteByte(character)
					}
					index++
					character = source[index]
				}
				builder.WriteByte(character)
			}
			if !closed {
				return nil, ErrActionUnsupported
			}
			value = builder.String()
			tail, remaining, _ := strings.Cut(source[index+1:], "\n")
			tail = strings.TrimSpace(tail)
			if tail != "" && !strings.HasPrefix(tail, "#") {
				return nil, ErrActionUnsupported
			}
			source = remaining
		} else {
			value, source, _ = strings.Cut(source, "\n")
			if comment := strings.Index(value, " #"); comment >= 0 {
				value = value[:comment]
			}
			value = strings.TrimSpace(value)
		}
		if wanted {
			// Do not guess shell expansions or double-quoted escape sequences.
			if !literal && strings.ContainsAny(value, "$\\") || strings.ContainsAny(value, "\r\n") {
				return nil, ErrActionUnsupported
			}
			values[key] = value
		}
	}
	return values, nil
}

func discoverComposeProject(directory string) (string, []string, error) {
	data, err := readBoundedRegularFile(filepath.Join(directory, ".env"), maxComposeEnvironmentBytes)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", nil, ErrActionUnsupported
	}
	variables, err := composeDiscoveryVariables(data)
	if err != nil {
		return "", nil, err
	}
	name := variables["COMPOSE_PROJECT_NAME"]
	if name == "" {
		name = filepath.Base(directory)
	}
	files := discoverDefaultComposeFiles(directory)
	if value := variables["COMPOSE_FILE"]; value != "" {
		separator := variables["COMPOSE_PATH_SEPARATOR"]
		if separator == "" {
			separator = string(filepath.ListSeparator)
		}
		// Native environment paths need not use Docker's comma-separated label format.
		files = nil
		for _, path := range strings.Split(value, separator) {
			if path == "" {
				continue
			}
			path = filepath.FromSlash(path)
			if !filepath.IsAbs(path) {
				path = filepath.Join(directory, path)
			}
			files = append(files, filepath.Clean(path))
		}
	}
	for _, path := range files {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return "", nil, ErrActionUnsupported
		}
	}
	if len(files) > maxComposeProjectFiles {
		return "", nil, ErrActionUnsupported
	}
	return name, files, nil
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
	containers, err := c.ContainerListSummaries(ctx)
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

// Move only the configuration files, never the project directory or bind-mounted data.
// Both paths remain in the same directory; the guard preserves external edits.
func (guard *composeEditGuard) move(source, target string) error {
	if err := guard.check(); err != nil {
		return err
	}
	original, ok := guard.files[source]
	if !ok || original.info == nil || guard.files[target].info != nil {
		return composeEditConflict()
	}
	if err := os.Rename(source, target); err != nil {
		return err
	}
	guard.files[source] = composeEditFile{}
	guard.files[target] = original
	return syncDirectoryPath(filepath.Dir(source))
}

func composeRemovalRecoveryEnvironment(project ComposeProject) ([]byte, bool, error) {
	name, files, err := discoverComposeProject(project.WorkingDirectory)
	if err != nil {
		return nil, false, err
	}
	matches := name == project.Name && len(files) == len(project.ConfigFiles)
	for index, file := range project.ConfigFiles {
		if index >= len(files) || files[index] != file.Path {
			matches = false
		}
	}
	if matches {
		return nil, false, nil
	}
	paths := make([]string, 0, len(project.ConfigFiles))
	for _, file := range project.ConfigFiles {
		path := filepath.ToSlash(file.Path)
		if strings.ContainsAny(path, "\r\n\\") {
			return nil, false, ErrActionUnsupported
		}
		paths = append(paths, path)
	}
	separator := ""
	for _, candidate := range []string{string(filepath.ListSeparator), "|", "^", ";", ":"} {
		if !strings.Contains(strings.Join(paths, ""), candidate) {
			separator = candidate
			break
		}
	}
	if separator == "" {
		return nil, false, ErrActionUnsupported
	}
	var source string
	if project.EnvironmentFile != nil {
		source = project.EnvironmentFile.Source
	}
	if source != "" && !strings.HasSuffix(source, "\n") {
		source += "\n"
	}
	// Native Compose variables preserve label-only identity after down, including
	// for CLI users. No Panel project registry is introduced.
	source += "# Compose project and file selection preserved for redeployment\n"
	for _, variable := range [][2]string{{"COMPOSE_PROJECT_NAME", project.Name},
		{"COMPOSE_FILE", strings.Join(paths, separator)}, {"COMPOSE_PATH_SEPARATOR", separator}} {
		source += variable[0] + "='" + strings.ReplaceAll(variable[1], "'", "\\'") + "'\n"
	}
	if len(source) > maxComposeEnvironmentBytes {
		return nil, false, ErrActionUnsupported
	}
	return []byte(source), true, nil
}

func composeRemovalSharedFiles(project ComposeProject, containers []contract.ContainerSummary, configuration, environment bool) error {
	for _, container := range containers {
		if container.ComposeProject == "" || container.ComposeProject == project.Name {
			continue
		}
		directory := container.Labels["com.docker.compose.project.working_dir"]
		resolvedDirectory, _ := filepath.EvalSymlinks(filepath.Clean(filepath.FromSlash(directory)))
		if environment && resolvedDirectory == project.WorkingDirectory {
			return fmt.Errorf("Compose environment is shared with project %s; configuration preserved: %w", container.ComposeProject, ErrResourceConflict)
		}
		if !configuration {
			continue
		}
		for _, path := range composeConfigPaths(container.Labels["com.docker.compose.project.config_files"], directory) {
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				continue
			}
			for _, file := range project.ConfigFiles {
				if resolved == file.Path {
					return fmt.Errorf("Compose configuration is shared with project %s; retain configuration to remove only this deployment: %w", container.ComposeProject, ErrResourceConflict)
				}
			}
		}
	}
	return nil
}

func (c *Client) removeComposeProject(ctx context.Context, input MaintenanceInput, archiveID string) (string, error) {
	state, err := c.resolveComposeProject(ctx, input.Name)
	if err != nil {
		return "", err
	}
	if input.ExpectedResourceVersion == "" || input.ExpectedResourceVersion != state.ResourceVersion {
		return "", ErrResourceConflict
	}
	guard, err := newComposeEditGuard(state.ComposeProject)
	if err != nil {
		return "", err
	}
	recoveryEnvironment, updateEnvironment, err := composeRemovalRecoveryEnvironment(state.ComposeProject)
	if err != nil {
		return "", err
	}
	if input.RemoveComposeFiles || updateEnvironment {
		containers, err := c.ContainerListSummaries(ctx)
		if err != nil {
			return "", err
		}
		if err := composeRemovalSharedFiles(state.ComposeProject, containers, input.RemoveComposeFiles, updateEnvironment); err != nil {
			return "", err
		}
	}
	if input.RemoveComposeFiles {
		if !dockerJobIDPattern.MatchString(archiveID) {
			return "", ErrInvalidDockerJob
		}
		for _, file := range state.ConfigFiles {
			// Missing backup targets are guarded as well, so existing files are never replaced.
			guard.files[file.Path+".kpanel-removed-"+archiveID] = composeEditFile{}
		}
	}
	if err := guard.check(); err != nil {
		return "", err
	}
	environmentPath := filepath.Join(state.WorkingDirectory, ".env")
	originalEnvironment := guard.files[environmentPath]
	if updateEnvironment {
		stagedPath, err := stageComposeEnvironmentFile(environmentPath, recoveryEnvironment, originalEnvironment.info)
		if err != nil {
			return "", err
		}
		defer os.Remove(stagedPath)
		staged, err := readComposeEditFile(stagedPath)
		if err != nil {
			return "", err
		}
		if err := guard.replace(stagedPath, environmentPath, staged); err != nil {
			return "", errors.Join(err, guard.restore(environmentPath, originalEnvironment))
		}
		// Explicitly load the newly created .env even when no environment file existed.
		state.EnvironmentFile = &ComposeProjectFile{Path: environmentPath}
	}
	arguments := append(composeProjectBase(state.ComposeProject), "down", "--remove-orphans")
	if input.RemoveVolumes {
		arguments = append(arguments, "--volumes")
	}
	if _, err := c.runCompose(ctx, arguments...); err != nil {
		if updateEnvironment {
			// down may remove all containers before failing on a network or volume.
			// Keep native recovery variables if labels are gone or cannot be queried.
			containers, listErr := c.ContainerListSummaries(ctx)
			if listErr == nil {
				for _, container := range containers {
					if container.ComposeProject == state.Name {
						err = errors.Join(err, guard.restore(environmentPath, originalEnvironment))
						break
					}
				}
			}
		}
		return "", fmt.Errorf("Compose project removal failed; configuration preserved: %w", err)
	}
	containers, err := c.ContainerListSummaries(ctx)
	if err != nil {
		return "", fmt.Errorf("Compose removal result could not be verified; configuration preserved: %w", err)
	}
	for _, container := range containers {
		if container.ComposeProject == input.Name {
			return "", errors.New("Compose project containers still exist; configuration preserved for retry")
		}
	}
	if !input.RemoveComposeFiles {
		return "", guard.check()
	}
	if err := composeRemovalSharedFiles(state.ComposeProject, containers, true, false); err != nil {
		return "", err
	}
	var moved []string
	for _, file := range state.ConfigFiles {
		backup := file.Path + ".kpanel-removed-" + archiveID
		err = ctx.Err()
		if err == nil {
			err = guard.move(file.Path, backup)
		}
		if guard.files[file.Path].info == nil {
			moved = append(moved, file.Path)
		}
		if err != nil {
			// Restore only files still owned by this task. Never overwrite external changes.
			for index := len(moved) - 1; index >= 0; index-- {
				path := moved[index]
				err = errors.Join(err, guard.move(path+".kpanel-removed-"+archiveID, path))
			}
			return state.WorkingDirectory, fmt.Errorf("Compose deployment removed but configuration archive failed; inspect original files and .kpanel-removed-%s backups: %w", archiveID, err)
		}
	}
	return state.WorkingDirectory, nil
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
	guard, err := newComposeEditGuard(state.ComposeProject)
	if err != nil {
		return err
	}
	original := guard.files[selected.Path]
	info := original.info
	environmentPath := filepath.Join(state.WorkingDirectory, ".env")
	environmentOriginal := guard.files[environmentPath]
	environmentInfo := environmentOriginal.info
	updated := []byte(input.Compose)
	if !bytes.HasSuffix(updated, []byte("\n")) {
		updated = append(updated, '\n')
	}
	stagedPath, err := stageComposeProjectFile(selected.Path, updated, info)
	if err != nil {
		return err
	}
	defer os.Remove(stagedPath)
	environmentSource := string(environmentOriginal.data)
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
	staged, err := readComposeEditFile(stagedPath)
	if err != nil || !bytes.Equal(staged.data, updated) {
		return errors.New("Compose staged configuration is unavailable or changed")
	}
	stagedEnvironment, err := readComposeEditFile(stagedEnvironmentPath)
	if err != nil || !bytes.Equal(stagedEnvironment.data, updatedEnvironment) {
		return errors.New("Compose staged environment is unavailable or changed")
	}
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
	if err := guard.replace(stagedPath, selected.Path, staged); err != nil {
		return fmt.Errorf("Compose configuration update failed: %w", errors.Join(rollbackComposeFiles(guard, selected.Path, original, environmentPath, environmentOriginal), err))
	}
	if err := guard.replace(stagedEnvironmentPath, environmentPath, stagedEnvironment); err != nil {
		return fmt.Errorf("Compose configuration update failed: %w", errors.Join(rollbackComposeFiles(guard, selected.Path, original, environmentPath, environmentOriginal), err))
	}
	if err := guard.check(); err != nil {
		return err
	}
	deployedProject := state.ComposeProject
	deployedProject.EnvironmentFile = &ComposeProjectFile{Path: environmentPath}
	base := composeProjectBase(deployedProject)
	if _, err := c.runCompose(ctx, append(base, "up", "--detach", "--remove-orphans")...); err != nil {
		return c.rollbackComposeRedeploy(
			state.ComposeProject, guard, selected.Path, original,
			environmentPath, environmentOriginal, err,
		)
	}
	if err := guard.check(); err != nil {
		return err
	}
	containerIDs, err := c.runCompose(ctx, append(base, "ps", "--all", "--quiet")...)
	if err != nil || !composeOutputHasContainer(containerIDs) {
		if err == nil {
			err = errors.New("Docker Compose did not return a created container")
		}
		return c.rollbackComposeRedeploy(
			state.ComposeProject, guard, selected.Path, original,
			environmentPath, environmentOriginal, err,
		)
	}
	return guard.check()
}

// rollbackComposeFiles never adopts state read after a failed write. The guard
// advances only to our known staged inode, including rename-success/sync-failure.
func rollbackComposeFiles(guard *composeEditGuard, path string, original composeEditFile,
	environmentPath string, environmentOriginal composeEditFile,
) error {
	if err := guard.restore(path, original); err != nil {
		return fmt.Errorf("Compose redeploy failed; configuration needs attention: %w", err)
	}
	if err := guard.restore(environmentPath, environmentOriginal); err != nil {
		return fmt.Errorf("Compose redeploy failed; environment needs attention: %w", err)
	}
	return nil
}

func (c *Client) rollbackComposeRedeploy(
	project ComposeProject,
	guard *composeEditGuard,
	path string,
	original composeEditFile,
	environmentPath string,
	environmentOriginal composeEditFile,
	cause error,
) error {
	if err := rollbackComposeFiles(guard, path, original, environmentPath, environmentOriginal); err != nil {
		return errors.Join(err, cause)
	}
	if err := guard.check(); err != nil {
		return fmt.Errorf("Compose redeploy failed; current configuration needs attention: %w", errors.Join(err, cause))
	}
	rollbackContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := c.runCompose(
		rollbackContext,
		append(composeProjectBase(project), "up", "--detach", "--remove-orphans")...,
	); err != nil {
		return fmt.Errorf("Compose redeploy failed and runtime rollback needs attention: %w", errors.Join(guard.check(), cause, err))
	}
	if err := guard.check(); err != nil {
		return fmt.Errorf("Compose redeploy failed; current configuration needs attention: %w", errors.Join(err, cause))
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

func replaceComposeProjectFile(stagedPath, target string) (bool, error) {
	if runtime.GOOS == "windows" {
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	if err := os.Rename(stagedPath, target); err != nil {
		return false, err
	}
	return true, syncDirectoryPath(filepath.Dir(target))
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
