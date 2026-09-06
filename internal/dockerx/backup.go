package dockerx

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxDockerBackupEntries   = 100_000
	maxDockerBackupBytes     = int64(50 << 30)
	maxDockerBackupFileBytes = int64(10 << 30)
)

// Count the entries and payload sizes written to/read from tar, not filesystem
// directory sizes. Both sides must accept the same archive budget.
type dockerBackupBudget struct {
	entries int
	bytes   int64
}

func (b *dockerBackupBudget) add(size int64) error {
	if b.entries >= maxDockerBackupEntries {
		return errors.New("Docker backup contains too many entries")
	}
	if size < 0 || size > maxDockerBackupFileBytes || size > maxDockerBackupBytes-b.bytes {
		return errors.New("Docker backup exceeds the 50 GiB total or 10 GiB file safety limit")
	}
	b.entries++
	b.bytes += size
	return nil
}

func dockerBackupReservedTop(name string) bool {
	return name == ".kpanel-backups" || strings.HasPrefix(name, ".kpanel-restore-rollback-")
}

func validateDockerBackupMetadata(header *tar.Header) error {
	if header.Mode < 0 || header.Mode > int64(^uint32(0)) ||
		header.Uid < 0 || header.Gid < 0 || header.Uid > 1<<31-1 || header.Gid > 1<<31-1 {
		return errors.New("Docker backup contains invalid numeric metadata")
	}
	return nil
}

// Only controlled recovery location data bypasses the ordinary error excerpt.
// The underlying errors remain available to callers through Unwrap.
type dockerRestoreRecoveryError struct {
	root    string
	cause   error
	applied bool
}

func (e *dockerRestoreRecoveryError) Error() string {
	return fmt.Sprintf("Docker restore needs attention; previous data retained at %s; %v", e.root, e.cause)
}
func (e *dockerRestoreRecoveryError) Unwrap() error { return e.cause }

type dockerRestoreOps struct {
	removeAll func(string) error
	rename    func(string, string) error
	copyTree  func(context.Context, string, string) error
}

func defaultDockerRestoreOps() dockerRestoreOps {
	return dockerRestoreOps{os.RemoveAll, os.Rename, copyRestoredDockerTreeContext}
}

var (
	dockerBackupIDPattern  = regexp.MustCompile(`^docker-[0-9]{8}T[0-9]{6}Z-[a-f0-9]{8}\.tar\.gz$`)
	dockerBackupTopPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
	migrationHostPattern   = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)
	migrationUserPattern   = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
)

type DockerBackup struct {
	ID        string    `json:"id"`
	SizeBytes int64     `json:"sizeBytes"`
	CreatedAt time.Time `json:"createdAt"`
	Format    string    `json:"format"`
}

func (c *Client) DockerBackups() ([]DockerBackup, error) {
	root := c.dockerBackupRoot()
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return []DockerBackup{}, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("Docker backup directory is unavailable or unsafe")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	result := make([]DockerBackup, 0, len(entries))
	for _, entry := range entries {
		if !dockerBackupIDPattern.MatchString(entry.Name()) || entry.IsDir() ||
			entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		entryInfo, statErr := entry.Info()
		if statErr != nil || !entryInfo.Mode().IsRegular() ||
			entryInfo.Size() <= 0 || entryInfo.Size() > maxDockerBackupBytes {
			continue
		}
		result = append(result, DockerBackup{
			ID: entry.Name(), SizeBytes: entryInfo.Size(),
			CreatedAt: entryInfo.ModTime().UTC(), Format: "kpanel-home-docker-v1",
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}

func (c *Client) dockerBackupPath(id string) (string, error) {
	if !dockerBackupIDPattern.MatchString(id) {
		return "", ErrDockerJobNotFound
	}
	root := c.dockerBackupRoot()
	path := filepath.Join(root, id)
	if filepath.Dir(path) != root {
		return "", ErrDockerJobNotFound
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		info.Size() <= 0 || info.Size() > maxDockerBackupBytes {
		return "", ErrDockerJobNotFound
	}
	return path, nil
}

func (c *Client) dockerBackupRoot() string {
	return filepath.Join(filepath.Clean(c.appRoot), ".kpanel-backups")
}

func (c *Client) resolvedDockerAppRoot() (string, error) {
	root := filepath.Clean(c.appRoot)
	if !filepath.IsAbs(root) || root == string(filepath.Separator) {
		return "", errors.New("Docker application root is unsafe")
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", errors.New("Docker application root is unavailable or unsafe")
	}
	resolved = filepath.Clean(resolved)
	if !filepath.IsAbs(resolved) || resolved == string(filepath.Separator) {
		return "", errors.New("Docker application root resolved to an unsafe path")
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("Docker application root is unavailable or unsafe")
	}
	return resolved, nil
}

func (c *Client) restoreDockerBackup(ctx context.Context, id string) error {
	return c.restoreDockerBackupWithOps(ctx, id, defaultDockerRestoreOps())
}

func (c *Client) restoreDockerBackupWithOps(ctx context.Context, id string, ops dockerRestoreOps) error {
	archivePath, err := c.dockerBackupPath(id)
	if err != nil {
		return err
	}
	stageRoot, err := os.MkdirTemp(filepath.Clean(c.stateRoot), ".docker-restore-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stageRoot)
	if err := os.Chmod(stageRoot, 0o700); err != nil {
		return err
	}
	topLevels, err := extractDockerBackup(ctx, archivePath, stageRoot)
	if err != nil {
		return err
	}
	appRoot, err := c.resolvedDockerAppRoot()
	if err != nil {
		return err
	}
	rollbackRoot, err := os.MkdirTemp(appRoot, ".kpanel-restore-rollback-*")
	if err != nil {
		return err
	}
	if err := os.Chmod(rollbackRoot, 0o700); err != nil {
		_ = os.RemoveAll(rollbackRoot)
		return err
	}
	var replacements []dockerRestoreReplacement
	fail := func(cause error) error {
		rollbackErr := rollbackDockerRestoreWithOps(replacements, rollbackRoot, appRoot, ops)
		if rollbackErr != nil {
			return errors.Join(rollbackErr, cause)
		}
		return fmt.Errorf("Docker restore failed; previous data restored: %w", cause)
	}
	for _, name := range topLevels {
		if dockerBackupReservedTop(name) ||
			name == "." || name == ".." || !dockerBackupTopPattern.MatchString(name) {
			_ = os.RemoveAll(rollbackRoot)
			return errors.New("Docker backup contains an unsafe top-level path")
		}
	}
	for _, name := range topLevels {
		select {
		case <-ctx.Done():
			return fail(ctx.Err())
		default:
		}
		source := filepath.Join(stageRoot, "docker", name)
		target := filepath.Join(appRoot, name)
		replacement := dockerRestoreReplacement{target: target}
		if _, targetErr := os.Lstat(target); targetErr == nil {
			replacement.previous = filepath.Join(rollbackRoot, name)
			if err := ops.rename(target, replacement.previous); err != nil {
				return fail(fmt.Errorf("stage existing /home/docker/%s for rollback: %w", name, err))
			}
		} else if !errors.Is(targetErr, os.ErrNotExist) {
			return fail(fmt.Errorf("inspect existing /home/docker/%s: %w", name, targetErr))
		}
		replacements = append(replacements, replacement)
		if err := ops.copyTree(ctx, source, target); err != nil {
			return fail(err)
		}
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	if err := syncDirectoryPath(appRoot); err != nil {
		return fail(err)
	}
	if err := ops.removeAll(rollbackRoot); err != nil {
		return &dockerRestoreRecoveryError{root: rollbackRoot, cause: fmt.Errorf("restore completed but previous data cleanup failed: %w", err), applied: true}
	}
	return syncDirectoryPath(appRoot)
}

type dockerRestoreReplacement struct {
	target   string
	previous string
}

func rollbackDockerRestore(replacements []dockerRestoreReplacement, rollbackRoot, appRoot string) error {
	return rollbackDockerRestoreWithOps(replacements, rollbackRoot, appRoot, defaultDockerRestoreOps())
}

func rollbackDockerRestoreWithOps(replacements []dockerRestoreReplacement, rollbackRoot, appRoot string, ops dockerRestoreOps) error {
	var failures []error
	for index := len(replacements) - 1; index >= 0; index-- {
		replacement := replacements[index]
		if !filepath.IsAbs(replacement.target) || !pathWithin(replacement.target, appRoot) || replacement.target == appRoot ||
			(replacement.previous != "" && (!pathWithin(replacement.previous, rollbackRoot) || replacement.previous == rollbackRoot)) {
			failures = append(failures, errors.New("unsafe rollback replacement"))
			continue
		}
		// Never delete the target unless the old copy is still available. This
		// also preserves already recovered targets if recovery is retried.
		if replacement.previous != "" {
			if _, err := os.Lstat(replacement.previous); err != nil {
				failures = append(failures, fmt.Errorf("inspect previous %s: %w", filepath.Base(replacement.target), err))
				continue
			}
		}
		if err := ops.removeAll(replacement.target); err != nil {
			failures = append(failures, fmt.Errorf("remove replacement %s: %w", filepath.Base(replacement.target), err))
			continue
		}
		if replacement.previous != "" {
			if err := ops.rename(replacement.previous, replacement.target); err != nil {
				failures = append(failures, fmt.Errorf("recover previous %s: %w", filepath.Base(replacement.target), err))
			}
		}
	}
	if err := syncDirectoryPath(appRoot); err != nil {
		failures = append(failures, err)
	}
	if len(failures) == 0 {
		if err := ops.removeAll(rollbackRoot); err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) > 0 {
		return &dockerRestoreRecoveryError{root: rollbackRoot, cause: errors.Join(failures...)}
	}
	return nil
}

func extractDockerBackup(
	ctx context.Context,
	archivePath string,
	stageRoot string,
) ([]string, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, errors.New("Docker backup is not a valid gzip archive")
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	topLevels := make(map[string]bool)
	// Apply directory metadata only after children have been written. This
	// preserves read-only/zero modes and avoids process umask changing the backup.
	directories := make(map[string]*tar.Header)
	var budget dockerBackupBudget
	for {
		header, nextErr := reader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return nil, errors.New("Docker backup tar stream is invalid")
		}
		if err := budget.add(header.Size); err != nil {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(header.Name)))
		if clean == "." || clean == "docker" {
			if header.Typeflag != tar.TypeDir {
				return nil, errors.New("Docker backup root must be a directory")
			}
			continue
		}
		if strings.HasPrefix(clean, "/") || strings.HasPrefix(clean, "../") ||
			!strings.HasPrefix(clean, "docker/") {
			return nil, errors.New("Docker backup contains an unsafe path")
		}
		relative := strings.TrimPrefix(clean, "docker/")
		parts := strings.Split(relative, "/")
		if len(parts) == 0 || !dockerBackupTopPattern.MatchString(parts[0]) ||
			parts[0] == "." || parts[0] == ".." ||
			dockerBackupReservedTop(parts[0]) {
			return nil, errors.New("Docker backup contains an unsafe application path")
		}
		topLevels[parts[0]] = true
		if err := validateDockerBackupMetadata(header); err != nil {
			return nil, err
		}
		target := filepath.Join(stageRoot, filepath.FromSlash(clean))
		if !pathWithin(target, stageRoot) {
			return nil, errors.New("Docker backup path escaped the staging directory")
		}
		mode := os.FileMode(header.Mode).Perm() & 0o777
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o700); err != nil {
				return nil, err
			}
			directories[target] = header
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return nil, err
			}
			output, openErr := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
			if openErr != nil {
				return nil, openErr
			}
			written, copyErr := io.Copy(output, dockerBackupContextReader{ctx, io.LimitReader(reader, header.Size+1)})
			syncErr := output.Sync()
			closeErr := output.Close()
			if copyErr != nil || written != header.Size || syncErr != nil || closeErr != nil {
				return nil, errors.New("Docker backup entry could not be restored safely")
			}
			if err := applyNumericOwnership(target, header.Uid, header.Gid); err != nil {
				return nil, err
			}
			if err := os.Chmod(target, mode); err != nil {
				return nil, err
			}
		default:
			return nil, errors.New("Docker backup contains links or unsupported filesystem objects")
		}
	}
	// tar EOF does not by itself verify the gzip checksum/trailer.
	if n, err := io.Copy(io.Discard, dockerBackupContextReader{ctx, io.LimitReader(gzipReader, (1<<20)+1)}); err != nil || n > 1<<20 {
		return nil, errors.New("Docker backup gzip trailer is invalid or oversized")
	}
	directoryPaths := make([]string, 0, len(directories))
	for path := range directories {
		directoryPaths = append(directoryPaths, path)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(directoryPaths)))
	for _, path := range directoryPaths {
		header := directories[path]
		if err := applyNumericOwnership(path, header.Uid, header.Gid); err != nil {
			return nil, err
		}
		if err := os.Chmod(path, os.FileMode(header.Mode).Perm()); err != nil {
			return nil, err
		}
	}
	names := make([]string, 0, len(topLevels))
	for name := range topLevels {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return nil, errors.New("Docker backup does not contain application data")
	}
	return names, nil
}

func pathWithin(candidate, root string) bool {
	candidate = filepath.Clean(candidate)
	root = filepath.Clean(root)
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func copyRestoredDockerTree(source, target string) error {
	return copyRestoredDockerTreeContext(context.Background(), source, target)
}

type dockerBackupContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r dockerBackupContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func copyRestoredDockerTreeContext(ctx context.Context, source, target string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	sourceInfo, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("restore source contains a symbolic link")
	}
	if sourceInfo.IsDir() {
		if err := os.Mkdir(target, 0o700); err != nil {
			return err
		}
		uid, gid, err := fileNumericOwnership(sourceInfo)
		if err != nil {
			return err
		}
		if err := applyNumericOwnership(target, uid, gid); err != nil {
			return err
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyRestoredDockerTreeContext(ctx,
				filepath.Join(source, entry.Name()),
				filepath.Join(target, entry.Name()),
			); err != nil {
				return err
			}
		}
		if err := os.Chmod(target, sourceInfo.Mode().Perm()); err != nil {
			return err
		}
		return syncDirectoryPath(target)
	}
	if !sourceInfo.Mode().IsRegular() {
		return errors.New("restore source contains an unsupported filesystem object")
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, sourceInfo.Mode().Perm())
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(output, dockerBackupContextReader{ctx, io.LimitReader(input, sourceInfo.Size()+1)})
	syncErr := output.Sync()
	closeErr := output.Close()
	if copyErr != nil || written != sourceInfo.Size() || syncErr != nil || closeErr != nil {
		return errors.New("restore file copy failed")
	}
	uid, gid, err := fileNumericOwnership(sourceInfo)
	if err != nil {
		return err
	}
	if err := applyNumericOwnership(target, uid, gid); err != nil {
		return err
	}
	return os.Chmod(target, sourceInfo.Mode().Perm())
}

func validMigrationHost(value string) bool {
	value = strings.TrimSpace(value)
	if parsed := net.ParseIP(value); parsed != nil {
		return true
	}
	return len(value) <= 253 && migrationHostPattern.MatchString(value) &&
		!strings.Contains(value, "..")
}

func (c *Client) migrateDockerBackup(
	ctx context.Context,
	id string,
	host string,
	user string,
	port int,
) (string, error) {
	path, err := c.dockerBackupPath(id)
	if err != nil {
		return "", err
	}
	if !validMigrationHost(host) || !migrationUserPattern.MatchString(user) ||
		port < 1 || port > 65535 {
		return "", ErrInvalidDockerJob
	}
	run := c.hostCommand
	if run == nil {
		run = runFixedDockerHostCommand
	}
	destinationHost := host
	if parsed := net.ParseIP(host); parsed != nil && parsed.To4() == nil {
		destinationHost = "[" + host + "]"
	}
	destination := user + "@" + destinationHost + ":/tmp/" + id
	_, err = run(
		ctx,
		"scp",
		"-P", strconv.Itoa(port),
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "ConnectTimeout=10",
		"--",
		path,
		destination,
	)
	if err != nil {
		return "", fmt.Errorf("migrate Docker backup: %w", err)
	}
	return destination, nil
}
