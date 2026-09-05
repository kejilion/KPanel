package dockerx

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestComposeRedeployPreservesExternalEdits(t *testing.T) {
	for _, phase := range []string{"config", "up", "ps"} {
		for _, target := range []string{"compose", "env", "new-env"} {
			t.Run(phase+"/"+target, func(t *testing.T) {
				original := "services:\n  web:\n    image: nginx:alpine\n"
				updated := "services:\n  web:\n    image: nginx:stable\n"
				client, path := composeProjectTestClient(t, original)
				env := filepath.Join(filepath.Dir(path), ".env")
				if target != "new-env" {
					writeComposeTestFile(t, env, "APP_TAG=original\n")
				}
				project, err := client.ComposeProject(context.Background(), "demo")
				if err != nil {
					t.Fatal(err)
				}
				externalPath := path
				if target != "compose" {
					externalPath = env
				}
				upCalls := 0
				client.composeCommand = func(_ context.Context, args ...string) ([]byte, error) {
					current := "ps"
					if containsArgumentSequence(args, "config", "--services") {
						current = "config"
					}
					if containsArgumentSequence(args, "up", "--detach") {
						current = "up"
						upCalls++
					}
					if current == phase && (phase != "up" || upCalls == 1) {
						writeComposeTestFile(t, externalPath, "external edit\n")
						if phase != "config" {
							return []byte(strings.Repeat("runtime output ", 40)), errors.New("injected runtime failure")
						}
					}
					if current == "config" {
						return []byte("web\n"), nil
					}
					return []byte(strings.Repeat("a", 64)), nil
				}
				err = client.redeployComposeProject(context.Background(), MaintenanceInput{
					Action: "compose_redeploy", Name: "demo", ComposeFile: path,
					Compose: updated, ComposeEnvironment: composeEnvironmentPointer("APP_TAG=updated"),
					ExpectedResourceVersion: project.ResourceVersion,
				})
				if !errors.Is(err, ErrResourceConflict) {
					t.Errorf("expected conflict, got %v", err)
				}
				if err != nil && !strings.Contains(safeDockerJobMessage(err), "changed externally") {
					t.Errorf("conflict hidden by bounded job output: %s", safeDockerJobMessage(err))
				}
				wantUp := 1
				if phase == "config" {
					wantUp = 0
				}
				if upCalls != wantUp {
					t.Errorf("up calls = %d, want %d (no compensation)", upCalls, wantUp)
				}
				assertComposeTestFile(t, externalPath, "external edit\n")
				if target == "compose" {
					want := "APP_TAG=updated\n"
					if phase == "config" {
						want = "APP_TAG=original\n"
					}
					assertComposeTestFile(t, env, want)
				} else {
					want := updated
					if phase == "config" {
						want = original
					}
					assertComposeTestFile(t, path, want)
				}
				assertNoComposeStages(t, filepath.Dir(path))
			})
		}
	}
}

func TestComposeRedeploySecondFileFailureRestoresOwnedCompose(t *testing.T) {
	original := "services:\n  web:\n    image: nginx:alpine\n"
	client, path := composeProjectTestClient(t, original)
	env := filepath.Join(filepath.Dir(path), ".env")
	writeComposeTestFile(t, env, "APP_TAG=original\n")
	project, err := client.ComposeProject(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	client.composeCommand = func(_ context.Context, args ...string) ([]byte, error) {
		if !containsArgumentSequence(args, "config", "--services") {
			t.Fatal("runtime command after commit failure")
		}
		for i, arg := range args {
			if arg == "--env-file" {
				if err := os.Remove(args[i+1]); err != nil {
					t.Fatal(err)
				}
			}
		}
		return []byte("web\n"), nil
	}
	err = client.redeployComposeProject(context.Background(), MaintenanceInput{
		Action: "compose_redeploy", Name: "demo", ComposeFile: path,
		Compose: original + "# updated\n", ComposeEnvironment: composeEnvironmentPointer("APP_TAG=updated"),
		ExpectedResourceVersion: project.ResourceVersion,
	})
	if err == nil {
		t.Fatal("expected second file failure")
	}
	assertComposeTestFile(t, path, original)
	assertComposeTestFile(t, env, "APP_TAG=original\n")
	assertNoComposeStages(t, filepath.Dir(path))
}

func writeComposeTestFile(t *testing.T, path, source string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertComposeTestFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != want {
		t.Errorf("%s = %q, %v; want %q", filepath.Base(path), data, err, want)
	}
}

func assertNoComposeStages(t *testing.T, dir string) {
	t.Helper()
	stages, err := filepath.Glob(filepath.Join(dir, ".kpanel-*.tmp"))
	if err != nil || len(stages) != 0 {
		t.Errorf("staging residue: %v, %v", stages, err)
	}
}

func TestComposeRedeployNormalRollbackMatrix(t *testing.T) {
	for _, phase := range []string{"up", "ps", "empty-ps"} {
		for _, existed := range []bool{false, true} {
			name := phase + "/absent-env"
			if existed {
				name = phase + "/existing-env"
			}
			t.Run(name, func(t *testing.T) {
				original := "services:\n  web:\n    image: nginx:alpine\n"
				client, path := composeProjectTestClient(t, original)
				env := filepath.Join(filepath.Dir(path), ".env")
				if existed {
					writeComposeTestFile(t, env, "APP_TAG=original\n")
				}
				project, err := client.ComposeProject(context.Background(), "demo")
				if err != nil {
					t.Fatal(err)
				}
				upCalls := 0
				client.composeCommand = func(_ context.Context, args ...string) ([]byte, error) {
					if containsArgumentSequence(args, "config", "--services") {
						return []byte("web"), nil
					}
					if containsArgumentSequence(args, "up", "--detach") {
						upCalls++
						if phase == "up" && upCalls == 1 {
							return nil, errors.New("injected up failure")
						}
						return nil, nil
					}
					if phase == "empty-ps" {
						return nil, nil
					}
					return nil, errors.New("injected ps failure")
				}
				err = client.redeployComposeProject(context.Background(), MaintenanceInput{
					Action: "compose_redeploy", Name: "demo", ComposeFile: path, Compose: original + "# updated\n",
					ComposeEnvironment: composeEnvironmentPointer("APP_TAG=updated"), ExpectedResourceVersion: project.ResourceVersion,
				})
				if err == nil || !strings.Contains(err.Error(), "previous configuration restored") || upCalls != 2 {
					t.Fatalf("normal rollback: %v, up=%d", err, upCalls)
				}
				assertComposeTestFile(t, path, original)
				if existed {
					assertComposeTestFile(t, env, "APP_TAG=original\n")
				} else if _, err := os.Lstat(env); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("env must remain absent: %v", err)
				}
				assertNoComposeStages(t, filepath.Dir(path))
			})
		}
	}
}

func TestComposeRedeployRejectsInitialAndQueuedStaleVersion(t *testing.T) {
	client, path := composeProjectTestClient(t, "services:\n  web:\n    image: nginx:alpine\n")
	project, err := client.ComposeProject(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	input := MaintenanceInput{Action: "compose_redeploy", Name: "demo", ComposeFile: path,
		Compose: project.ConfigFiles[0].Source, ExpectedResourceVersion: project.ResourceVersion}
	if err := client.validateMaintenanceInput(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	writeComposeTestFile(t, path, input.Compose+"# external queued edit\n")
	client.composeCommand = func(context.Context, ...string) ([]byte, error) {
		t.Fatal("command for stale version")
		return nil, nil
	}
	if err := client.validateMaintenanceInput(context.Background(), input); !errors.Is(err, ErrResourceConflict) {
		t.Errorf("initial check: %v", err)
	}
	if err := client.redeployComposeProject(context.Background(), input); !errors.Is(err, ErrResourceConflict) {
		t.Errorf("worker check: %v", err)
	}
	assertComposeTestFile(t, path, input.Compose+"# external queued edit\n")
}

func TestComposeEditGuardPartialCommitRecovery(t *testing.T) {
	for _, external := range []string{"none", "compose", "env"} {
		t.Run(external, func(t *testing.T) {
			client, path := composeProjectTestClient(t, "services:\n  web:\n    image: nginx:alpine\n")
			env := filepath.Join(filepath.Dir(path), ".env")
			writeComposeTestFile(t, env, "original env\n")
			project, err := client.ComposeProject(context.Background(), "demo")
			if err != nil {
				t.Fatal(err)
			}
			guard, err := newComposeEditGuard(project)
			if err != nil {
				t.Fatal(err)
			}
			original, originalEnv := guard.files[path], guard.files[env]
			stageAndReplace := func(target, data string) error {
				stage, err := stageComposeProjectFile(target, []byte(data), guard.files[target].info)
				if err != nil {
					t.Fatal(err)
				}
				defer os.Remove(stage)
				known, err := readComposeEditFile(stage)
				if err != nil {
					t.Fatal(err)
				}
				return guard.replace(stage, target, known)
			}
			if err := stageAndReplace(path, "updated compose\n"); err != nil {
				t.Fatal(err)
			}
			// Inject the exact rename-success/directory-sync-failure result. The
			// real rename still runs on the Linux filesystem; no Docker is used.
			guard.replaceFile = func(stage, target string) (bool, error) {
				replaced, err := replaceComposeProjectFile(stage, target)
				if err != nil {
					return replaced, err
				}
				return replaced, errors.New("injected directory sync failure")
			}
			if err := stageAndReplace(env, "updated env\n"); err == nil {
				t.Fatal("expected sync failure")
			}
			guard.replaceFile = replaceComposeProjectFile
			if external == "compose" {
				writeComposeTestFile(t, path, "external compose\n")
			}
			if external == "env" {
				writeComposeTestFile(t, env, "external env\n")
			}
			err = rollbackComposeFiles(guard, path, original, env, originalEnv)
			if external == "none" {
				if err != nil {
					t.Fatal(err)
				}
				assertComposeTestFile(t, path, string(original.data))
				assertComposeTestFile(t, env, string(originalEnv.data))
			} else {
				if !errors.Is(err, ErrResourceConflict) {
					t.Fatalf("partial commit conflict: %v", err)
				}
				wantCompose, wantEnv := "updated compose\n", "updated env\n"
				if external == "compose" {
					wantCompose = "external compose\n"
				} else {
					wantEnv = "external env\n"
				}
				assertComposeTestFile(t, path, wantCompose)
				assertComposeTestFile(t, env, wantEnv)
			}
			assertNoComposeStages(t, filepath.Dir(path))
		})
	}
}

func TestComposeEditGuardChecksWholeConfigurationAndIdentity(t *testing.T) {
	for _, mutation := range []string{"other-compose", "same-content-inode", "symlink", "directory", "mode", "oversize"} {
		t.Run(mutation, func(t *testing.T) {
			client, path := composeProjectTestClient(t, "services:\n  web:\n    image: nginx:alpine\n")
			project, err := client.ComposeProject(context.Background(), "demo")
			if err != nil {
				t.Fatal(err)
			}
			other := filepath.Join(filepath.Dir(path), "override.yml")
			writeComposeTestFile(t, other, "services: {}\n")
			info, err := os.Lstat(other)
			if err != nil {
				t.Fatal(err)
			}
			project.ConfigFiles = append(project.ConfigFiles, ComposeProjectFile{Path: other,
				ResourceVersion: resourceHash(struct {
					Path string
					Mode os.FileMode
					Data []byte
				}{other, info.Mode().Perm(), []byte("services: {}\n")})})
			guard, err := newComposeEditGuard(project)
			if err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "other-compose":
				writeComposeTestFile(t, other, "services: {}\n# external\n")
			case "same-content-inode":
				if err := os.Rename(path, path+".old"); err != nil {
					t.Fatal(err)
				}
				writeComposeTestFile(t, path, project.ConfigFiles[0].Source)
				if err := os.Chmod(path, guard.files[path].info.Mode().Perm()); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Rename(path, path+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".old", path); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			case "directory":
				dir := filepath.Dir(path)
				if err := os.Rename(dir, dir+".old"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(dir, 0o750); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if runtime.GOOS == "windows" {
					t.Skip("Unix permission semantics")
				}
				if err := os.Chmod(path, guard.files[path].info.Mode().Perm()^0o040); err != nil {
					t.Fatal(err)
				}
			case "oversize":
				writeComposeTestFile(t, other, strings.Repeat("x", maxComposeSourceBytes+1))
			}
			if err := guard.check(); !errors.Is(err, ErrResourceConflict) {
				t.Errorf("guard accepted %s: %v", mutation, err)
			}
		})
	}
}

func TestComposeRedeployExternalEditDuringRuntimeRollback(t *testing.T) {
	for _, fails := range []bool{false, true} {
		name := "successful-command"
		if fails {
			name = "failed-command"
		}
		t.Run(name, func(t *testing.T) {
			client, path := composeProjectTestClient(t, "services:\n  web:\n    image: nginx:alpine\n")
			project, err := client.ComposeProject(context.Background(), "demo")
			if err != nil {
				t.Fatal(err)
			}
			upCalls := 0
			client.composeCommand = func(_ context.Context, args ...string) ([]byte, error) {
				if containsArgumentSequence(args, "config", "--services") {
					return []byte("web"), nil
				}
				upCalls++
				if upCalls == 1 {
					return nil, errors.New("injected first up failure")
				}
				writeComposeTestFile(t, path, "external during rollback\n")
				if fails {
					return nil, errors.New("injected rollback up failure")
				}
				return nil, nil
			}
			err = client.redeployComposeProject(context.Background(), MaintenanceInput{
				Action: "compose_redeploy", Name: "demo", ComposeFile: path,
				Compose: project.ConfigFiles[0].Source + "# updated\n", ExpectedResourceVersion: project.ResourceVersion,
			})
			if !errors.Is(err, ErrResourceConflict) || strings.Contains(err.Error(), "previous configuration restored") || upCalls != 2 {
				t.Errorf("rollback external edit: %v, up=%d", err, upCalls)
			}
			assertComposeTestFile(t, path, "external during rollback\n")
			assertNoComposeStages(t, filepath.Dir(path))
		})
	}
}
