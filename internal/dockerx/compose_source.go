package dockerx

import (
	"context"
	"path/filepath"
	"strings"
)

// IsComposeSource resolves identities only; it never reads a Compose body or
// runs Compose. Use both native labels and .env discovery, including stopped
// projects and files with arbitrary names. Errors require manual approval.
func (c *Client) IsComposeSource(ctx context.Context, raw string) (bool, error) {
	if len(raw) == 0 || len(raw) > 4096 || strings.IndexByte(raw, 0) >= 0 || !filepath.IsAbs(raw) {
		return false, ErrActionUnsupported
	}
	target, err := filepath.EvalSymlinks(filepath.Clean(raw))
	if err != nil || !c.composePathAllowed(target) {
		return false, ErrActionUnsupported
	}
	matches := func(source string) bool {
		resolved, err := filepath.EvalSymlinks(source)
		return err == nil && resolved == target
	}
	containers, err := c.ContainerListSummaries(ctx)
	if err != nil {
		return false, err
	}
	for _, container := range containers {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		label := strings.TrimSpace(container.Labels["com.docker.compose.project.config_files"])
		if label == "" {
			continue
		}
		directory := container.Labels["com.docker.compose.project.working_dir"]
		if !filepath.IsAbs(directory) {
			return false, ErrActionUnsupported
		}
		files := composeConfigPaths(label, directory)
		if len(files) > maxComposeProjectFiles {
			return false, ErrActionUnsupported
		}
		for _, file := range files {
			if matches(file) {
				return true, nil
			}
		}
	}
	directories, err := c.managedComposeDirectoriesWithLimit(ctx, 512)
	if err != nil {
		return false, err
	}
	for _, directory := range directories {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		_, files, err := discoverComposeProject(directory)
		if err != nil {
			return false, err
		}
		for _, file := range files {
			if matches(file) {
				return true, nil
			}
		}
	}
	return false, nil
}
