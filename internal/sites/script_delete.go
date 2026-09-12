package sites

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type scriptDeleteOutcome struct {
	databaseDropped bool
	siteDeleted     bool
	warnings        []string
}

type siteScriptDeleter interface {
	Delete(context.Context, string) (scriptDeleteOutcome, error)
}

type kejilionSiteScriptDeleter struct{}

func (kejilionSiteScriptDeleter) Available() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("kejilion.sh website deletion requires Linux")
	}
	if _, err := findTrustedKejilionScript(
		"web_del()",
		`web_del "$@"`,
		"KPANEL_DELETE_SITE",
	); err != nil {
		return err
	}
	_, err := findSystemdRun()
	return err
}

func (m *Manager) DeleteWritable() error {
	if m.scriptDeleter == nil {
		return fmt.Errorf("k web del adapter is unavailable")
	}
	if checker, ok := m.scriptDeleter.(interface{ Available() error }); ok {
		return checker.Available()
	}
	return nil
}

func (kejilionSiteScriptDeleter) Delete(
	ctx context.Context,
	domain string,
) (scriptDeleteOutcome, error) {
	if err := (kejilionSiteScriptDeleter{}).Available(); err != nil {
		return scriptDeleteOutcome{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	script, err := findTrustedKejilionScript(
		"web_del()",
		`web_del "$@"`,
		"KPANEL_DELETE_SITE",
	)
	if err != nil {
		return scriptDeleteOutcome{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	systemdRun, err := findSystemdRun()
	if err != nil {
		return scriptDeleteOutcome{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	deleteCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	unitID := stableID("site-delete", domain, time.Now().UTC().String())[:24]
	command := exec.CommandContext(
		deleteCtx,
		systemdRun,
		"--unit=kpanel-site-delete-"+unitID,
		"--wait",
		"--pipe",
		"--collect",
		"--quiet",
		"--property=Type=exec",
		"--property=TimeoutStartSec=600s",
		"--property=TimeoutStopSec=30s",
		"--property=User=root",
		"--property=UMask=0027",
		"--property=NoNewPrivileges=no",
		"--property=ProtectSystem=no",
		"--property=ProtectHome=no",
		"--property=PrivateTmp=no",
		"--property=PrivateDevices=no",
		"--property=RestrictNamespaces=no",
		"--property=RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK",
		"--setenv=LC_ALL=C.UTF-8",
		"--setenv=LANG=C.UTF-8",
		"--setenv=KJ_WEB_NONINTERACTIVE=1",
		"--",
		"/bin/bash",
		script,
		"web",
		"del",
		domain,
	)
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	runErr := command.Run()
	outcome := parseScriptDeleteOutcome(output.String())
	if runErr != nil {
		if errors.Is(deleteCtx.Err(), context.DeadlineExceeded) {
			return scriptDeleteOutcome{}, fmt.Errorf("%w: k web del timed out", ErrUnavailable)
		}
		if !outcome.siteDeleted {
			return scriptDeleteOutcome{}, fmt.Errorf(
				"%w: k web del failed: %s",
				ErrUnavailable,
				safeScriptDeleteOutput(output.String()),
			)
		}
	}
	return outcome, nil
}

func (m *Manager) deleteWithScript(
	ctx context.Context,
	id string,
	primaryDomain string,
) (DeleteResult, error) {
	normalized, err := normalizeScriptDomain(primaryDomain)
	if err != nil || normalized != primaryDomain {
		return DeleteResult{}, fmt.Errorf("%w: primaryDomain must be a valid normalized domain", ErrInvalidInput)
	}
	siteWriteMutex.Lock()
	defer siteWriteMutex.Unlock()
	current, err := m.findActionableByID(id, "delete")
	if err != nil {
		return DeleteResult{}, err
	}
	if current.PrimaryDomain != normalized {
		return DeleteResult{}, fmt.Errorf("%w: site identity and primaryDomain do not match", ErrConflict)
	}
	if m.scriptDeleter == nil {
		return DeleteResult{}, fmt.Errorf("%w: k web del adapter is unavailable", ErrUnavailable)
	}

	candidates := []struct {
		kind string
		path string
	}{
		{"nginx_config", filepath.Join(m.webRoot, "conf.d", normalized+".conf")},
		{"document_root", filepath.Join(m.webRoot, "html", normalized)},
		{"certificate", filepath.Join(m.webRoot, "certs", normalized+"_cert.pem")},
		{"certificate_key", filepath.Join(m.webRoot, "certs", normalized+"_key.pem")},
		{"certificate_policy", filepath.Join(m.webRoot, "certs", normalized+".custom")},
		{"certificate_renewal", filepath.Join(m.webRoot, "certs", normalized+".auto-renewal")},
	}
	removed := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if _, statErr := os.Lstat(candidate.path); statErr == nil {
			removed = append(removed, candidate.kind)
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return DeleteResult{}, fmt.Errorf("%w: inspect %s before k web del: %v", ErrUnavailable, candidate.kind, statErr)
		}
	}

	outcome, err := m.scriptDeleter.Delete(ctx, normalized)
	if err != nil {
		return DeleteResult{}, err
	}
	for _, candidate := range candidates {
		if _, statErr := os.Lstat(candidate.path); !errors.Is(statErr, os.ErrNotExist) {
			if statErr == nil {
				return DeleteResult{}, fmt.Errorf(
					"%w: k web del left %s at %s",
					ErrNeedsAttention,
					candidate.kind,
					candidate.path,
				)
			}
			return DeleteResult{}, fmt.Errorf(
				"%w: verify %s after k web del: %v",
				ErrNeedsAttention,
				candidate.kind,
				statErr,
			)
		}
	}
	return DeleteResult{
		ID: current.ID, PrimaryDomain: current.PrimaryDomain,
		Status: "deleted", Mode: "full", ResourceVersion: current.ResourceVersion,
		Removed: removed, DatabaseDropped: outcome.databaseDropped, Warnings: outcome.warnings,
	}, nil
}

func parseScriptDeleteOutcome(output string) scriptDeleteOutcome {
	result := scriptDeleteOutcome{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "KPANEL_DELETE_SITE deleted "):
			result.siteDeleted = true
		case strings.HasPrefix(line, "KPANEL_DELETE_DATABASE dropped "):
			result.databaseDropped = true
		case strings.HasPrefix(line, "KPANEL_DELETE_DATABASE failed "):
			result.warnings = append(
				result.warnings,
				"站点已删除，但同名数据库删除失败；请在数据库中核对并手动清理残留",
			)
		case strings.HasPrefix(line, "KPANEL_DELETE_DATABASE skipped "):
			result.warnings = append(result.warnings, "未检测到 MySQL 运行环境，数据库清理已跳过")
		case line == "KPANEL_DELETE_WARNING nginx_unavailable":
			result.warnings = append(result.warnings, "未检测到 Nginx 容器，站点产物已删除但无需执行重载")
		case line == "Nginx 配置验证或重载失败":
			result.warnings = append(result.warnings, "站点产物已删除，但 Nginx 配置验证或重载失败；请检查 Nginx 当前状态")
		}
	}
	return result
}

func safeScriptDeleteOutput(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "script returned no diagnostic output"
	}
	if len(value) > 2000 {
		value = value[len(value)-2000:]
	}
	return value
}
