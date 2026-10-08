package sites

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

var (
	serverNameLinePattern = regexp.MustCompile(`(?m)^([ \t]*server_name[ \t]+)[^;\r\n]+;`)
	proxyPassLinePattern  = regexp.MustCompile(`(?m)^([ \t]*proxy_pass[ \t]+)https?://[^; \t\r\n]+;`)
	upstreamServerPattern = regexp.MustCompile(`(?m)^[ \t]*server[ \t]+[^;\r\n]+;[ \t]*\r?$`)
	returnLinePattern     = regexp.MustCompile(`(?m)^([ \t]*return[ \t]+)(301|302|307|308)[ \t]+[^;\r\n]+;`)
)

// updateCLIConfig patches only the small set of directives that KPanel
// understands in a discovered kejilion.sh site. It deliberately preserves
// certificates, security rules, product-specific roots and every unknown
// directive, so the CLI and Web remain on the same physical artifact.
func (m *Manager) updateCLIConfig(
	ctx context.Context,
	current contract.SiteSummary,
	spec managedSpec,
	expectedVersion string,
) (contract.SiteSummary, error) {
	if current.ResourceVersion != expectedVersion {
		return contract.SiteSummary{}, fmt.Errorf("%w: discovered site changed", ErrConflict)
	}
	configPath, oldInfo, oldConfig, err := m.verifiedDeleteConfig(current)
	if err != nil {
		return contract.SiteSummary{}, fmt.Errorf("%w: read kejilion.sh site configuration: %v", ErrUnavailable, err)
	}
	newConfig, err := patchCLIConfig(oldConfig, current, spec)
	if err != nil {
		return contract.SiteSummary{}, err
	}
	if bytes.Equal(oldConfig, newConfig) {
		return current, nil
	}
	candidatePath, err := writeTemporaryFile(
		filepath.Dir(configPath),
		"."+current.PrimaryDomain+".cli-*.tmp",
		newConfig,
		0o640,
	)
	if err != nil {
		return contract.SiteSummary{}, fmt.Errorf("%w: stage kejilion.sh configuration: %v", ErrUnavailable, err)
	}
	keepBackup := false
	defer func() {
		if !keepBackup {
			_ = os.Remove(candidatePath)
		}
	}()
	// Capture the staged inode before publication. Inspecting the live path
	// afterwards could accidentally identify an external writer's replacement.
	candidateInfo, err := os.Lstat(candidatePath)
	if err != nil {
		return contract.SiteSummary{}, fmt.Errorf("%w: inspect staged configuration: %v", ErrUnavailable, err)
	}

	latest, err := m.findManagedByID(current.ID)
	if err != nil || latest.ResourceVersion != expectedVersion {
		return contract.SiteSummary{}, fmt.Errorf("%w: discovered site changed before commit", ErrConflict)
	}
	if !fileMatches(configPath, oldInfo, oldConfig) {
		return contract.SiteSummary{}, fmt.Errorf("%w: discovered configuration changed before commit", ErrConflict)
	}
	m.callHook("before_exchange", configPath)
	if err := atomicExchange(candidatePath, configPath); err != nil {
		return contract.SiteSummary{}, fmt.Errorf("%w: replace kejilion.sh configuration: %v", ErrUnavailable, err)
	}
	rollback := func(cause error) (contract.SiteSummary, error) {
		if restoreErr := restoreExchange(configPath, candidateInfo, newConfig, candidatePath); restoreErr != nil {
			keepBackup = true
			return contract.SiteSummary{}, restoreErr
		}
		if reloadErr := m.validateAndReloadPrevious(ctx); reloadErr != nil {
			return contract.SiteSummary{}, reloadErr
		}
		return contract.SiteSummary{}, cause
	}
	if err := syncDirectory(filepath.Dir(configPath)); err != nil {
		return rollback(fmt.Errorf("%w: sync candidate publication: %v", ErrUnavailable, err))
	}
	if !fileMatches(candidatePath, oldInfo, oldConfig) {
		return rollback(fmt.Errorf("%w: external change won the update race", ErrConflict))
	}
	m.callHook("candidate_published", configPath)
	if !fileMatches(configPath, candidateInfo, newConfig) {
		keepBackup = true
		return contract.SiteSummary{}, fmt.Errorf("%w: candidate changed before validation; backup retained at %s", ErrNeedsAttention, candidatePath)
	}
	if err := m.nginx.NginxTest(ctx); err != nil {
		return rollback(fmt.Errorf("%w: candidate failed nginx -t: %v", ErrUnprocessable, err))
	}
	if !fileMatches(configPath, candidateInfo, newConfig) {
		keepBackup = true
		return contract.SiteSummary{}, fmt.Errorf("%w: candidate changed during validation; backup retained at %s", ErrNeedsAttention, candidatePath)
	}
	if err := m.nginx.NginxReload(ctx); err != nil {
		return rollback(fmt.Errorf("%w: Nginx reload failed: %v", ErrUnavailable, err))
	}
	if !fileMatches(configPath, candidateInfo, newConfig) {
		keepBackup = true
		return contract.SiteSummary{}, fmt.Errorf("%w: candidate changed while reloading; backup retained at %s", ErrNeedsAttention, candidatePath)
	}
	if err := os.Remove(candidatePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		keepBackup = true
		return contract.SiteSummary{}, fmt.Errorf(
			"%w: site is active but old configuration cleanup failed: %v",
			ErrNeedsAttention,
			err,
		)
	}
	if err := syncDirectory(filepath.Dir(configPath)); err != nil {
		return contract.SiteSummary{}, fmt.Errorf("%w: sync updated configuration: %v", ErrNeedsAttention, err)
	}
	items, err := m.discoverer.Discover()
	if err != nil {
		return contract.SiteSummary{}, fmt.Errorf("%w: rediscover updated site: %v", ErrNeedsAttention, err)
	}
	for _, site := range items {
		if site.ID == current.ID {
			return site, nil
		}
	}
	return contract.SiteSummary{}, fmt.Errorf("%w: updated site could not be rediscovered", ErrNeedsAttention)
}

func patchCLIConfig(old []byte, current contract.SiteSummary, spec managedSpec) ([]byte, error) {
	text := string(old)
	serverNames := append([]string{spec.Primary}, spec.Aliases...)
	if matches := serverNameLinePattern.FindAllStringIndex(text, -1); len(matches) == 0 || len(matches) > 4 {
		return nil, fmt.Errorf("%w: server_name structure is ambiguous", ErrForbidden)
	}
	text = serverNameLinePattern.ReplaceAllString(text, "${1}"+strings.Join(serverNames, " ")+";")

	switch spec.Kind {
	case contract.SiteReverseProxy, contract.SiteDomainProxy:
		targets := []string{spec.Upstream}
		patched, err := patchCLIUpstreams(text, targets)
		if err != nil {
			return nil, err
		}
		text = patched
	case contract.SiteLoadBalance:
		patched, err := patchCLIUpstreams(text, spec.Upstreams)
		if err != nil {
			return nil, err
		}
		text = patched
	case contract.SiteRedirect:
		if len(returnLinePattern.FindAllStringIndex(text, -1)) != 1 {
			return nil, fmt.Errorf("%w: redirect structure is ambiguous", ErrForbidden)
		}
		replacement := fmt.Sprintf("${1}%d %s$$request_uri;", spec.RedirectCode, spec.RedirectTarget)
		text = returnLinePattern.ReplaceAllString(text, replacement)
	case contract.SitePHP:
		oldSocket, newSocket := "/run/php/php-fpm.sock", "/run/php/php-fpm.sock"
		if current.Target == "php74" {
			oldSocket = "/run/php74/php-fpm.sock"
		}
		if spec.PHPVersion == "7.4" {
			newSocket = "/run/php74/php-fpm.sock"
		}
		if oldSocket != newSocket {
			if strings.Contains(text, oldSocket) {
				text = strings.ReplaceAll(text, oldSocket, newSocket)
			} else {
				oldRuntime, newRuntime := "php:9000", "php:9000"
				if current.Target == "php74" {
					oldRuntime = "php74:9000"
				}
				if spec.PHPVersion == "7.4" {
					newRuntime = "php74:9000"
				}
				if !strings.Contains(text, oldRuntime) {
					return nil, fmt.Errorf("%w: PHP runtime directive is ambiguous", ErrForbidden)
				}
				text = strings.ReplaceAll(text, oldRuntime, newRuntime)
			}
		}
	case contract.SiteStatic:
		// Alias-only update; product files and site rules remain untouched.
	default:
		return nil, fmt.Errorf("%w: the current site structure has no update adapter", ErrForbidden)
	}
	return []byte(text), nil
}

func patchCLIUpstreams(text string, targets []string) (string, error) {
	indices := upstreamPattern.FindStringSubmatchIndex(text)
	if len(indices) >= 6 {
		if len(upstreamPattern.FindAllStringIndex(text, -1)) != 1 {
			return "", fmt.Errorf("%w: multiple upstream blocks are ambiguous", ErrForbidden)
		}
		bodyStart, bodyEnd := indices[4], indices[5]
		body := text[bodyStart:bodyEnd]
		if len(upstreamServerPattern.FindAllStringIndex(body, -1)) == 0 {
			return "", fmt.Errorf("%w: upstream block has no server directives", ErrForbidden)
		}
		var servers []string
		for _, target := range targets {
			parsed, err := url.Parse(target)
			if err != nil || parsed.Host == "" {
				return "", fmt.Errorf("%w: invalid upstream target", ErrUnprocessable)
			}
			servers = append(servers, "    server "+parsed.Host+";")
		}
		body = upstreamServerPattern.ReplaceAllString(body, "")
		body = strings.TrimRight(body, " \t\r\n") + "\n" + strings.Join(servers, "\n") + "\n"
		return text[:bodyStart] + body + text[bodyEnd:], nil
	}
	if len(targets) != 1 || len(proxyPassLinePattern.FindAllStringIndex(text, -1)) != 1 {
		return "", fmt.Errorf("%w: proxy structure is ambiguous", ErrForbidden)
	}
	return proxyPassLinePattern.ReplaceAllString(text, "${1}"+targets[0]+";"), nil
}
