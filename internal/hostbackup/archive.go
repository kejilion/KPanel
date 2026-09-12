package hostbackup

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/backup"
)

func (e *Engine) Create(ctx context.Context, directory string, modules []string, expected string) (err error) {
	i, err := e.Inventory(ctx)
	if err != nil {
		return err
	}
	if i.Revision != expected {
		return errors.New("host resources changed; scan again")
	}
	if err := validateSelection(i, modules); err != nil {
		return err
	}
	if err := backup.PrivateDir(directory); err != nil {
		return err
	}
	var expectedBytes int64
	for _, m := range i.Modules {
		if slices.Contains(modules, m.ID) {
			expectedBytes += m.Bytes
		}
	}
	if expectedBytes > backup.MaxBytes {
		return backup.ErrInvalid
	}
	if err := backup.RequireSpace(directory, expectedBytes+(128<<20)); err != nil {
		return err
	}
	selected := Payload{}
	for _, c := range i.Containers {
		if slices.Contains(modules, c.Module) {
			selected.Containers = append(selected.Containers, c)
		}
	}
	if err := orderContainers(&selected); err != nil {
		return err
	}
	running := []Container{}
	for _, c := range selected.Containers {
		if c.Running {
			running = append(running, Container{ID: c.ID, Name: c.Name})
		}
	}
	journalPath := filepath.Join(directory, "export-journal.json")
	if err := backup.WriteJSON(journalPath, running); err != nil {
		return err
	}
	// Register recovery before stopping the first writer. Cleanup gets a fresh
	// bounded context so cancellation cannot leave source services stopped.
	defer func() {
		recovery, cancel := context.WithTimeout(context.Background(), recoveryTimeout)
		defer cancel()
		var restartErr error
		for _, c := range running {
			if startErr := e.docker(recovery, "POST", "/containers/"+c.ID+"/start", nil, nil); startErr != nil {
				restartErr = errors.Join(restartErr, fmt.Errorf("source service %s could not restart", c.Name))
			}
		}
		if restartErr == nil {
			restartErr = os.Remove(journalPath)
		}
		if restartErr != nil {
			err = &backup.Failure{Code: "recovery_required", Err: errors.Join(err, restartErr)}
		}
	}()
	for n := len(selected.Containers) - 1; n >= 0; n-- {
		c := selected.Containers[n]
		if !c.Running {
			continue
		}
		if err := e.docker(ctx, "POST", "/containers/"+c.ID+"/stop", nil, nil); err != nil {
			return err
		}
	}
	for n := range i.Roots {
		if slices.Contains(modules, i.Roots[n].Module) {
			i.Roots[n].Bytes = 0
			i.Roots[n].Entries = 0
			if err := e.measure(ctx, &i.Roots[n]); err != nil {
				return err
			}
		}
	}
	for _, module := range modules {
		if module == "panel" {
			continue
		}
		payload := Payload{Version: 1, Module: module, Roots: []Root{}, Containers: []Container{}, Networks: map[string]json.RawMessage{}, Volumes: map[string]Volume{}}
		for _, root := range i.Roots {
			if root.Module == module {
				payload.Roots = append(payload.Roots, root)
			}
		}
		for _, c := range i.Containers {
			if c.Module == module {
				payload.Containers = append(payload.Containers, c)
				for name := range c.Networks {
					payload.Networks[name] = i.Networks[name]
				}
				for _, m := range c.Mounts {
					if m.Type == "volume" {
						payload.Volumes[m.Name] = i.Volumes[m.Name]
					}
				}
			}
		}
		if err := e.writePayload(ctx, filepath.Join(directory, module+".payload"), payload); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) writePayload(ctx context.Context, path string, p Payload) (err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if len(body) > 4<<20 {
		return backup.ErrInvalid
	}
	if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Size: int64(len(body)), Mode: 0600, Typeflag: tar.TypeReg}); err != nil {
		return err
	}
	if _, err := tw.Write(body); err != nil {
		return err
	}
	count := 0
	var total int64
	for _, root := range p.Roots {
		source := e.host(root.Path)
		err = filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.IsDir() && !info.Mode().IsRegular() {
				return backup.ErrInvalid
			}
			rel, err := filepath.Rel(source, path)
			if err != nil {
				return err
			}
			name := "data/" + root.ID
			if rel != "." {
				name += "/" + filepath.ToSlash(rel)
			}
			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			header.Name = name
			count++
			if count > 100000 {
				return backup.ErrInvalid
			}
			if info.Mode().IsRegular() {
				total += info.Size()
				if total > backup.MaxBytes || info.Size() > 10<<30 {
					return backup.ErrInvalid
				}
			}
			if err := tw.WriteHeader(header); err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			input, err := backup.OpenRegular(path, 10<<30)
			if err != nil {
				return err
			}
			_, err = io.CopyN(tw, input, info.Size())
			after, statErr := input.Stat()
			input.Close()
			if err != nil {
				return err
			}
			if statErr != nil || !os.SameFile(info, after) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
				return errors.New("source changed during backup")
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return f.Close()
}

func (e *Engine) ReadPayload(ctx context.Context, path, directory string) (Payload, error) {
	var p Payload
	input, err := backup.OpenRegular(path, backup.MaxBytes)
	if err != nil {
		return p, err
	}
	defer input.Close()
	compressed := bufio.NewReader(input)
	gz, err := gzip.NewReader(compressed)
	if err != nil {
		return p, err
	}
	defer gz.Close()
	gz.Multistream(false)
	bounded := &io.LimitedReader{R: contextReader{ctx, gz}, N: backup.MaxBytes + (128 << 20)}
	tr := tar.NewReader(bounded)
	h, err := tr.Next()
	if err != nil || h.Name != "manifest.json" || h.Typeflag != tar.TypeReg || h.Size > 4<<20 {
		return p, backup.ErrInvalid
	}
	body, err := io.ReadAll(tr)
	if err != nil || backup.Decode(body, &p) != nil {
		return p, backup.ErrInvalid
	}
	if err := e.validatePayload(p); err != nil {
		return p, err
	}
	if err := backup.PrivateDir(directory); err != nil {
		return p, err
	}
	var estimated int64
	for _, r := range p.Roots {
		estimated += r.Bytes
	}
	if estimated > backup.MaxBytes {
		return p, backup.ErrInvalid
	}
	if err := backup.RequireSpace(directory, estimated); err != nil {
		return p, err
	}
	roots := map[string]Root{}
	for _, r := range p.Roots {
		roots[r.ID] = r
	}
	seen := map[string]bool{}
	var total int64
	directories := map[string]*tar.Header{}
	for {
		if ctx.Err() != nil {
			return p, ctx.Err()
		}
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return p, err
		}
		parts := strings.Split(h.Name, "/")
		if len(parts) < 2 || parts[0] != "data" || roots[parts[1]].ID == "" || h.Name != filepath.ToSlash(filepath.Clean(h.Name)) || strings.Contains(h.Name, "\\") || seen[h.Name] {
			return p, backup.ErrInvalid
		}
		for _, part := range parts {
			if part == "" || part == "." || part == ".." {
				return p, backup.ErrInvalid
			}
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeDir || h.Size < 0 || h.Size > 10<<30 || h.Mode < 0 || h.Mode > 07777 || h.Uid < 0 || h.Gid < 0 || h.Uid > 1<<31-1 || h.Gid > 1<<31-1 {
			return p, backup.ErrInvalid
		}
		seen[h.Name] = true
		total += h.Size
		if len(seen) > 100000 || total > backup.MaxBytes {
			return p, backup.ErrInvalid
		}
		target := filepath.Join(directory, filepath.FromSlash(h.Name))
		if h.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(target, 0700); err != nil {
				return p, err
			}
			copy := *h
			directories[target] = &copy
		} else {
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return p, err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return p, err
			}
			_, err = io.Copy(f, tr)
			if err == nil {
				err = f.Sync()
			}
			closeErr := f.Close()
			if err != nil {
				return p, err
			}
			if closeErr != nil {
				return p, closeErr
			}
		}
		if h.Typeflag == tar.TypeDir {
			continue
		}
		if err := restoreOwnership(target, h.Uid, h.Gid); err != nil {
			return p, err
		}
		if err := os.Chmod(target, archiveMode(h.Mode)); err != nil {
			return p, err
		}
	}
	// Drain the gzip footer: a valid tar prefix is insufficient.
	tail := make([]byte, 4096)
	tailBytes := 0
	for {
		n, err := bounded.Read(tail)
		tailBytes += n
		if tailBytes > 1<<20 {
			return p, backup.ErrInvalid
		}
		for _, b := range tail[:n] {
			if b != 0 {
				return p, backup.ErrInvalid
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return p, err
		}
	}
	if bounded.N <= 0 {
		return p, backup.ErrInvalid
	}
	if _, err := compressed.Peek(1); err != io.EOF {
		return p, backup.ErrInvalid
	}
	for path, h := range directories {
		if err := restoreOwnership(path, h.Uid, h.Gid); err != nil {
			return p, err
		}
		if err := os.Chmod(path, archiveMode(h.Mode)); err != nil {
			return p, err
		}
	}
	for _, r := range p.Roots {
		info, err := os.Lstat(filepath.Join(directory, "data", r.ID))
		if err != nil || info.IsDir() != r.Directory {
			return p, backup.ErrInvalid
		}
	}
	return p, nil
}

func (e *Engine) validatePayload(p Payload) error {
	if p.Version != 1 || !slices.Contains([]string{"apps", "web", "docker"}, p.Module) || len(p.Roots) > 4096 || len(p.Containers) > 512 || len(p.Networks) > 512 {
		return backup.ErrInvalid
	}
	ids := map[string]bool{}
	paths := []string{}
	for _, r := range p.Roots {
		if !backup.ValidID(r.ID) || ids[r.ID] || r.Module != p.Module || !strings.HasPrefix(r.Path, "/") || r.Path != filepath.ToSlash(filepath.Clean(r.Path)) || e.excluded(r.Path) {
			return backup.ErrInvalid
		}
		if r.Bytes < 0 || r.Bytes > backup.MaxBytes || r.Entries < 0 || r.Entries > 100000 {
			return backup.ErrInvalid
		}
		for _, old := range paths {
			if within(old, r.Path) || within(r.Path, old) {
				return backup.ErrInvalid
			}
		}
		ids[r.ID] = true
		paths = append(paths, r.Path)
	}
	for _, c := range p.Containers {
		if !imageIDPattern.MatchString(c.ImageID) {
			return backup.ErrInvalid
		}
		if _, err := nativeMountConfig(c); err != nil {
			return err
		}
		if protected, _ := runtimeProtection(c); protected {
			return errors.New("backup cannot replace Panel or Agent runtime")
		}
		if c.Module != p.Module || !validContainerName(c.Name) || len(c.Mounts) > 512 || len(c.Networks) > 64 {
			return backup.ErrInvalid
		}
		var image string
		if json.Unmarshal(c.Config["Image"], &image) != nil || image == "" || len(image) > 512 {
			return backup.ErrInvalid
		}
		for _, mount := range c.Mounts {
			if mount.Type == "tmpfs" {
				continue
			}
			if mount.Type != "bind" && mount.Type != "volume" || !strings.HasPrefix(mount.Source, "/") || !strings.HasPrefix(mount.Destination, "/") {
				return backup.ErrInvalid
			}
			if mount.Type == "volume" {
				v, ok := p.Volumes[mount.Name]
				if !ok || v.Name != mount.Name || v.Mountpoint != mount.Source {
					return backup.ErrInvalid
				}
			}
		}
	}
	return nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
func archiveMode(mode int64) os.FileMode {
	m := os.FileMode(mode & 0777)
	if mode&04000 != 0 {
		m |= os.ModeSetuid
	}
	if mode&02000 != 0 {
		m |= os.ModeSetgid
	}
	if mode&01000 != 0 {
		m |= os.ModeSticky
	}
	return m
}
