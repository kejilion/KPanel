// Package browsercomponent manages the optional, isolated Chromium sidecar.
// Docker is the resource truth; only the last bounded task and credential persist.
package browsercomponent

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/atomicfile"
	"github.com/kejilion/kejilion-panel/internal/contract"
	"github.com/kejilion/kejilion-panel/internal/jobcontrol"
	"github.com/kejilion/kejilion-panel/internal/redact"
)

var (
	ErrInvalid     = errors.New("browser_component_invalid")
	ErrConflict    = errors.New("browser_component_changed")
	ErrBusy        = errors.New("browser_component_busy")
	ErrUnsupported = errors.New("browser_component_unsupported")
)

type Config struct {
	StateDir, RuntimeDir, Executable, DockerSocket string
	Runner                                         jobcontrol.Runner
	DockerProbe                                    func(context.Context) error
	DownloadTor                                    func(context.Context, string) error
}
type Service struct {
	mu     sync.Mutex
	config Config
}

func New(c Config) *Service {
	if c.RuntimeDir == "" {
		c.RuntimeDir = "/run/kejilion-panel/browser"
	}
	if c.DockerSocket == "" {
		c.DockerSocket = "/var/run/docker.sock"
	}
	if c.Runner == nil {
		c.Runner = commandRunner{}
	}
	if c.DownloadTor == nil {
		c.DownloadTor = downloadTor
	}
	return &Service{config: c}
}

func (s *Service) docker(ctx context.Context, args ...string) ([]byte, error) {
	return s.config.Runner.Run(ctx, "docker", append([]string{"--host", "unix://" + s.config.DockerSocket}, args...)...)
}

func (s *Service) spec(id string) jobcontrol.Spec {
	return jobcontrol.Spec{Unit: "kpanel-browser-" + id, Executable: s.config.Executable,
		Arguments: []string{"browser-component-run", "--state-dir", s.config.StateDir, "--runtime-dir", s.config.RuntimeDir, "--docker-socket", s.config.DockerSocket, "--job", id},
		StateDir:  s.config.StateDir, UMask: "0077", StopTimeout: 10 * time.Second,
		SystemdProperties: []string{"Type=exec", "RuntimeMaxSec=32min", "TimeoutStopSec=10s", "MemoryMax=256M", "TasksMax=64"}}
}

func (s *Service) readJob() (*contract.BrowserComponentJob, error) {
	data, err := s.readPrivate("task.json", 4096)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var j contract.BrowserComponentJob
	if err = json.Unmarshal(data, &j); err != nil {
		return nil, err
	}
	if len(j.ID) != 32 || !contract.ValidBrowserComponentAction(j.Action) || !contract.ValidBrowserComponentKind(j.Kind) {
		return nil, ErrInvalid
	}
	if _, err = hex.DecodeString(j.ID); err != nil {
		return nil, ErrInvalid
	}
	return &j, nil
}

func (s *Service) save(j *contract.BrowserComponentJob) error {
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	_, err = (atomicfile.Writer{}).WritePrivate(s.config.StateDir, filepath.Join(s.config.StateDir, "task.json"), ".task-*", b)
	return err
}

func (s *Service) Snapshot(ctx context.Context) (contract.BrowserComponent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot(ctx)
}

func (s *Service) snapshot(ctx context.Context) (contract.BrowserComponent, error) {
	v := contract.BrowserComponent{Image: componentImage, TorSupported: runtime.GOOS == "linux" && runtime.GOARCH == "amd64"}
	if runtime.GOOS != "linux" {
		v.Reason = "linux_required"
		return v, nil
	}
	if _, err := s.config.Runner.LookPath("docker"); err != nil {
		v.Reason = "docker_required"
		return v, nil
	}
	// Reuse the Agent's daemon/PID policy for observations. In particular, a
	// status GET must not activate a stopped Docker daemon through its socket.
	if s.config.DockerProbe != nil {
		if err := s.config.DockerProbe(ctx); err != nil {
			v.Reason = "docker_unavailable"
			return v, nil
		}
	}
	if _, err := s.docker(ctx, "compose", "version", "--short"); err != nil {
		v.Reason = "compose_required"
		return v, nil
	}
	ids, err := s.docker(ctx, "container", "ls", "-aq", "--filter", "name=^/kpanel-browser$")
	if err != nil {
		v.Reason = "docker_unavailable"
		return v, nil
	}
	identity := "missing"
	if id := strings.TrimSpace(string(ids)); id != "" {
		if len(id) > 64 || strings.ContainsAny(id, "\r\n ") {
			return v, ErrInvalid
		}
		body, err := s.docker(ctx, "inspect", id)
		if err != nil {
			return v, err
		}
		var items []struct {
			ID, Image string
			Config    struct {
				Image  string
				Labels map[string]string
			}
			State struct {
				Running bool
				Status  string
				Health  *struct{ Status string }
			}
			RestartCount int
		}
		if json.Unmarshal(body, &items) != nil || len(items) != 1 {
			return v, ErrInvalid
		}
		x := items[0]
		v.Kind = "chromium"
		if x.Config.Image == componentTorImage || x.Config.Labels["io.kpanel.browser.kind"] == "tor" {
			v.Kind, v.Image = "tor", componentTorImage
		}
		v.ContainerID = x.ID
		v.Installed = true
		v.Running = x.State.Running
		v.Healthy = x.State.Health != nil && x.State.Health.Status == "healthy"
		identity = fmt.Sprintf("%s:%s:%s:%d:%s", x.ID, x.Image, x.State.Status, x.RestartCount, v.Kind)
	}
	v.ResourceVersion = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(identity)))
	v.Supported = true
	j, err := s.readJob()
	if err != nil {
		return v, err
	}
	if j != nil && (j.State == "queued" || j.State == "running") && time.Since(j.StartedAt) > 20*time.Second {
		// A disappeared worker is recoverable through an explicit retry. Read-only
		// reconciliation does not overwrite a live worker's persisted result.
		state, e := jobcontrol.Inspect(ctx, s.config.Runner, s.spec(j.ID))
		if e == nil && !state.Running {
			j.State = "failed"
			j.Stage = "interrupted"
			j.Message = "browser_task_interrupted"
		}
	}
	v.Job = j
	return v, nil
}

func (s *Service) Start(ctx context.Context, r contract.BrowserComponentRequest) (contract.BrowserComponentJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !contract.ValidBrowserComponentAction(r.Action) || !contract.ValidBrowserComponentKind(r.Kind) || (r.Action != "install" && r.Kind != "") {
		return contract.BrowserComponentJob{}, ErrInvalid
	}
	if err := privateDirectory(s.config.StateDir); err != nil {
		return contract.BrowserComponentJob{}, err
	}
	unlock, err := lockDirectory(s.config.StateDir, "submit.lock")
	if err != nil {
		return contract.BrowserComponentJob{}, err
	}
	defer unlock()
	v, err := s.snapshot(ctx)
	if err != nil {
		return contract.BrowserComponentJob{}, err
	}
	if !v.Supported {
		return contract.BrowserComponentJob{}, ErrUnsupported
	}
	if v.Job != nil && (v.Job.State == "queued" || v.Job.State == "running") {
		return contract.BrowserComponentJob{}, ErrBusy
	}
	if r.ExpectedResourceVersion == "" || r.ExpectedResourceVersion != v.ResourceVersion {
		return contract.BrowserComponentJob{}, ErrConflict
	}
	if r.Action != "install" && !v.Installed {
		return contract.BrowserComponentJob{}, ErrConflict
	}
	if r.Kind == "" {
		r.Kind = v.Kind
	}
	if r.Kind == "" {
		r.Kind = "chromium"
	}
	if r.Action == "install" && v.Installed && r.Kind != v.Kind {
		return contract.BrowserComponentJob{}, ErrConflict
	}
	if r.Action == "install" && r.Kind == "tor" && !v.TorSupported {
		return contract.BrowserComponentJob{}, ErrUnsupported
	}
	if err = jobcontrol.Available(s.config.Runner); err != nil {
		return contract.BrowserComponentJob{}, ErrUnsupported
	}
	// Prevent a delayed previous worker from overlapping a replacement request.
	workerUnlock, err := lockDirectory(s.config.StateDir, "worker.lock")
	if err != nil {
		return contract.BrowserComponentJob{}, err
	}
	if v.Job != nil {
		if err = jobcontrol.Cleanup(s.spec(v.Job.ID)); err != nil {
			workerUnlock()
			return contract.BrowserComponentJob{}, err
		}
	}
	b := make([]byte, 16)
	if _, err = rand.Read(b); err != nil {
		workerUnlock()
		return contract.BrowserComponentJob{}, err
	}
	j := contract.BrowserComponentJob{ExpectedResourceVersion: r.ExpectedResourceVersion, ID: hex.EncodeToString(b), Action: r.Action, Kind: r.Kind, State: "queued", Stage: "queued", StartedAt: time.Now().UTC()}
	if err = s.save(&j); err != nil {
		workerUnlock()
		return j, err
	}
	workerUnlock()
	launch, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	// A failed acknowledgement may still have launched the worker. Keep the job
	// queued until the init manager can be reconciled, never launch twice.
	if err = jobcontrol.Launch(launch, s.config.Runner, s.spec(j.ID)); err != nil {
		return j, err
	}
	return j, nil
}

// PrepareRuntime republishes a separate browser credential after /run is cleared.
// The containing Agent runtime directory is accessible only to its Panel group.
func (s *Service) PrepareRuntime() error {
	if err := privateDirectory(s.config.StateDir); err != nil {
		return err
	}
	unlock, err := lockDirectory(s.config.StateDir, "token.lock")
	if err != nil {
		return err
	}
	defer unlock()
	path := filepath.Join(s.config.StateDir, "token")
	data, err := s.readPrivate("token", 64)
	if errors.Is(err, os.ErrNotExist) {
		b := make([]byte, 32)
		if _, err = rand.Read(b); err != nil {
			return err
		}
		data = []byte(hex.EncodeToString(b))
		if _, err = (atomicfile.Writer{}).WritePrivate(s.config.StateDir, path, ".token-*", data); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if len(data) != 64 {
		return errors.New("invalid browser credential")
	}
	if _, err = hex.DecodeString(string(data)); err != nil {
		return err
	}
	if err = os.MkdirAll(s.config.RuntimeDir, 0755); err != nil {
		return err
	}
	i, err := os.Lstat(s.config.RuntimeDir)
	if err != nil || !i.IsDir() || i.Mode()&os.ModeSymlink != 0 || i.Mode().Perm()&0022 != 0 || !rootOwned(i) {
		return errors.New("invalid browser runtime directory")
	}
	// Agent and worker use umask 0077. Explicitly grant traversal of this
	// credential-only directory to the unprivileged browser container.
	if err = os.Chmod(s.config.RuntimeDir, 0755); err != nil {
		return err
	}
	target := filepath.Join(s.config.RuntimeDir, "browser-token")
	if _, err = (atomicfile.Writer{}).WritePrivate(s.config.RuntimeDir, target, ".token-*", data); err != nil {
		return err
	}
	// The component rejects world-readable tokens. Its fixed GID is also the
	// Panel's primary GID; the containing host runtime remains group-restricted.
	if runtime.GOOS == "linux" {
		if err = os.Chown(target, 0, 65532); err != nil {
			return err
		}
	}
	if err = os.Chmod(target, 0440); err != nil {
		return err
	}
	marker, err := s.readPrivate("enabled", 16)
	if errors.Is(err, os.ErrNotExist) {
		if e := os.Remove(filepath.Join(s.config.RuntimeDir, "browser-enabled")); e != nil && !errors.Is(e, os.ErrNotExist) {
			return e
		}
		return nil
	}
	if err != nil || !validEnabledMarker(marker) {
		return errors.New("invalid browser enabled state")
	}
	return s.publishEnabled(string(marker))
}

func validEnabledMarker(marker []byte) bool {
	return string(marker) == "enabled" || string(marker) == "tor"
}

func (s *Service) publishEnabled(marker string) error {
	target := filepath.Join(s.config.RuntimeDir, "browser-enabled")
	if _, err := (atomicfile.Writer{}).WritePrivate(s.config.RuntimeDir, target, ".enabled-*", []byte(marker)); err != nil {
		return err
	}
	return os.Chmod(target, 0444)
}

func (s *Service) enableDefaultConnection(kind string) error {
	marker := "enabled"
	if kind == "tor" {
		marker = "tor"
	}
	if _, err := (atomicfile.Writer{}).WritePrivate(s.config.StateDir, filepath.Join(s.config.StateDir, "enabled"), ".enabled-*", []byte(marker)); err != nil {
		return err
	}
	return s.publishEnabled(marker)
}

func (s *Service) RunJob(ctx context.Context, id string) error {
	if err := privateDirectory(s.config.StateDir); err != nil {
		return err
	}
	unlock, err := lockDirectory(s.config.StateDir, "worker.lock")
	if err != nil {
		return err
	}
	defer unlock()
	j, err := s.readJob()
	if err != nil {
		return err
	}
	if j == nil || j.ID != id || j.State != "queued" {
		return ErrConflict
	}
	j.State = "running"
	j.Stage = "preparing"
	if err = s.save(j); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	err = s.execute(ctx, j)
	now := time.Now().UTC()
	j.FinishedAt = &now
	j.State = "succeeded"
	j.Stage = "complete"
	if err != nil {
		j.State = "failed"
		j.Stage = "failed"
		j.Message = redact.Text(err.Error())
		if len(j.Message) > 1024 {
			j.Message = j.Message[:1024]
		}
	}
	if e := s.save(j); e != nil {
		return errors.Join(err, e)
	}
	return err
}

func (s *Service) execute(ctx context.Context, j *contract.BrowserComponentJob) error {
	v, err := s.snapshot(ctx)
	if err != nil {
		return err
	}
	if !v.Supported || v.ResourceVersion != j.ExpectedResourceVersion {
		return ErrConflict
	}
	if j.Action == "install" && v.Installed && j.Kind != "" && j.Kind != v.Kind {
		return ErrConflict
	}
	if j.Action == "install" {
		if err := s.PrepareRuntime(); err != nil {
			return err
		}
		j.Stage = "network"
		if err := s.save(j); err != nil {
			return err
		}
		if err := s.prepareNetwork(ctx); err != nil {
			return err
		}
		runtimePath, _ := json.Marshal(s.config.RuntimeDir)
		compose := []byte(strings.ReplaceAll(string(composeTemplate), "__BROWSER_RUNTIME_DIRECTORY__", string(runtimePath)))
		assets := map[string][]byte{"compose.yml": compose, "browser-seccomp.json": seccompProfile}
		if j.Kind == "tor" {
			if !v.TorSupported {
				return ErrUnsupported
			}
			j.Stage = "downloading"
			if err := s.save(j); err != nil {
				return err
			}
			if err := s.config.DownloadTor(ctx, s.config.StateDir); err != nil {
				return err
			}
			assets = map[string][]byte{
				"compose.yml": torComposeTemplate, "tor.Dockerfile": torDockerfile, "tor-start.sh": torStart,
				"tor-browser.tar.xz.asc": torSignature, "tor-signing-key.asc": torSigningKey,
				"tor-helper-locks.tar": torHelperLocks, "tor-helpers-build.sh": torHelpersBuild,
				".dockerignore": []byte("*\n!tor.Dockerfile\n!tor-start.sh\n!tor-browser.tar.xz\n!tor-browser.tar.xz.asc\n!tor-signing-key.asc\n!tor-helper-locks.tar\n!tor-helpers-build.sh\n"),
			}
		}
		for name, data := range assets {
			if _, err := (atomicfile.Writer{}).WritePrivate(s.config.StateDir, filepath.Join(s.config.StateDir, name), ".config-*", data); err != nil {
				return err
			}
		}
		j.Stage = "installing"
		if err := s.save(j); err != nil {
			return err
		}
		prefix := []string{"compose", "--ansi", "never", "--progress", "quiet", "--project-name", "kpanel-browser", "--project-directory", s.config.StateDir, "--file", filepath.Join(s.config.StateDir, "compose.yml")}
		if j.Kind == "tor" {
			j.Stage = "building"
			if err := s.save(j); err != nil {
				return err
			}
			if _, err := s.docker(ctx, append(append([]string{}, prefix...), "build", "browser")...); err != nil {
				return err
			}
		}
		// Recheck after slow preparation before replacing any Docker resource.
		current, err := s.snapshot(ctx)
		if err != nil {
			return err
		}
		if current.ResourceVersion != j.ExpectedResourceVersion {
			return ErrConflict
		}
		args := append(append([]string{}, prefix...), "up", "--detach", "--wait", "--wait-timeout", "90")
		if j.Kind == "tor" {
			args = append(args, "--no-build", "--pull", "never")
		}
		_, err = s.docker(ctx, append(args, "browser")...)
		if err != nil {
			return err
		}
		return s.enableDefaultConnection(j.Kind)
	}
	j.Stage = j.Action
	if err := s.save(j); err != nil {
		return err
	}
	args := []string{j.Action, v.ContainerID}
	if j.Action == "uninstall" {
		args = []string{"rm", "--force", v.ContainerID}
	}
	_, err = s.docker(ctx, args...)
	if err == nil && j.Action == "uninstall" {
		for _, path := range []string{filepath.Join(s.config.StateDir, "enabled"), filepath.Join(s.config.RuntimeDir, "browser-enabled")} {
			if e := os.Remove(path); e != nil && !errors.Is(e, os.ErrNotExist) {
				return e
			}
		}
	}
	if err == nil && (j.Action == "start" || j.Action == "restart") {
		j.Stage = "checking"
		if err = s.save(j); err != nil {
			return err
		}
		ready, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		for {
			body, probeErr := s.docker(ready, "inspect", "--format", "{{json .State}}", v.ContainerID)
			var state struct {
				Running bool
				Health  *struct{ Status string }
			}
			if probeErr == nil && json.Unmarshal(body, &state) == nil && state.Running && state.Health != nil && state.Health.Status == "healthy" {
				return nil
			}
			select {
			case <-ready.Done():
				return errors.New("browser_component_health_timeout")
			case <-time.After(time.Second):
			}
		}
	}
	return err
}

func (s *Service) readPrivate(name string, limit int64) ([]byte, error) {
	path := filepath.Join(s.config.StateDir, name)
	i, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !i.Mode().IsRegular() || i.Size() > limit || i.Mode().Perm()&0077 != 0 || !rootOwned(i) {
		return nil, errors.New("invalid browser private file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(i, opened) {
		return nil, errors.New("browser private file changed")
	}
	return io.ReadAll(io.LimitReader(f, limit+1))
}

func privateDirectory(path string) error {
	if !filepath.IsAbs(path) {
		return ErrInvalid
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	i, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !i.IsDir() || i.Mode()&os.ModeSymlink != 0 || i.Mode().Perm()&0077 != 0 || !rootOwned(i) {
		return errors.New("invalid browser state directory")
	}
	return nil
}
