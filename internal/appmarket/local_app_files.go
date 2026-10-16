package appmarket

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/kejilion/kejilion-panel/internal/atomicfile"
	"golang.org/x/image/webp"
)

// The app market edits ~/apps-local in place, the same files kejilion.sh
// reads, so every rule here mirrors what the script and inventory accept.
const (
	maxLocalAppIconBytes  = 64 << 10
	minLocalAppIconSide   = 64
	maxLocalAppIconSide   = 512
	localAppSyntaxTimeout = 5 * time.Second
)

var (
	localAppIDPattern = regexp.MustCompile(
		`^(?:local[[:space:]]+)?app_id[[:space:]]*=[[:space:]]*(?:"([^"$` + "`" + `\\]*)"|'([^']*)'|([A-Za-z0-9_.-]+))[[:space:]]*(?:#.*)?$`,
	)
	localAppFunctionPattern = regexp.MustCompile(`(?m)^[[:space:]]*(docker_app_install|docker_app_update|docker_app_uninstall)[[:space:]]*\(\)`)
	localAppIconTypes       = map[string]string{"image/png": ".png", "image/webp": ".webp"}
	localAppIconOrder       = []string{"image/webp", "image/png"}

	// ErrLocalAppInvalid wraps a user-facing reason for rejecting a config or icon.
	ErrLocalAppInvalid = errors.New("local application is invalid")
	// ErrLocalAppExists reports a create that would replace an existing config.
	ErrLocalAppExists = errors.New("local application already exists")
)

type LocalAppIcon struct {
	ContentType string `json:"contentType"`
	Data        []byte `json:"data"`
}

// LocalAppDocument is one ~/apps-local config as the editor sees it.
type LocalAppDocument struct {
	Token           string `json:"token"`
	Content         string `json:"content"`
	ResourceVersion string `json:"resourceVersion"`
	IconURL         string `json:"iconUrl,omitempty"`
	Installed       bool   `json:"installed"`
	ContainerName   string `json:"containerName"`
}

type LocalAppWriteInput struct {
	Token           string        `json:"token"`
	Content         string        `json:"content"`
	ResourceVersion string        `json:"resourceVersion"`
	Icon            *LocalAppIcon `json:"icon,omitempty"`
	RemoveIcon      bool          `json:"removeIcon"`
}

func localAppInvalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrLocalAppInvalid, fmt.Sprintf(format, args...))
}

func (s *Service) LocalApp(ctx context.Context, token string) (LocalAppDocument, error) {
	s.actions.Lock()
	defer s.actions.Unlock()
	return s.localAppDocument(ctx, token)
}

func (s *Service) localAppDocument(ctx context.Context, token string) (LocalAppDocument, error) {
	content, err := s.readLocalAppConfig(token)
	if err != nil {
		return LocalAppDocument{}, err
	}
	icon, iconErr := s.readLocalAppIcon(token)
	if iconErr != nil && !errors.Is(iconErr, ErrAppIconNotFound) {
		return LocalAppDocument{}, iconErr
	}
	document := LocalAppDocument{
		Token: token, Content: string(content), ResourceVersion: localAppVersion(content, icon),
	}
	if icon != nil {
		document.IconURL = localAppIconURL(token, icon.Data)
	}
	if spec, specErr := parseScriptAppSpec(bytes.NewReader(content)); specErr == nil {
		document.ContainerName = spec.Container
	}
	if item, findErr := s.Find(ctx, localAppIDPrefix+token); findErr == nil {
		document.Installed = item.Runtime.Installed
	}
	return document, nil
}

// SaveLocalApp creates (empty ResourceVersion) or replaces a local config and
// optionally its icon. Identity fields stay fixed while the app is installed.
func (s *Service) SaveLocalApp(ctx context.Context, input LocalAppWriteInput) (LocalAppDocument, error) {
	s.actions.Lock()
	defer s.actions.Unlock()
	token := input.Token
	if !tokenPattern.MatchString(token) {
		return LocalAppDocument{}, localAppInvalid("名称只能使用字母、数字、下划线和短横线，且以字母或数字开头")
	}
	if err := s.checkLocalAppName(token); err != nil {
		return LocalAppDocument{}, err
	}
	if err := s.checkLocalAppIdle(token); err != nil {
		return LocalAppDocument{}, err
	}
	content := []byte(strings.ReplaceAll(input.Content, "\r\n", "\n"))
	spec, err := s.validateLocalAppContent(ctx, token, content)
	if err != nil {
		return LocalAppDocument{}, err
	}
	var icon *LocalAppIcon
	if input.Icon != nil {
		if input.RemoveIcon {
			return LocalAppDocument{}, localAppInvalid("不能同时上传和移除图标")
		}
		if err := validateLocalAppIcon(input.Icon); err != nil {
			return LocalAppDocument{}, err
		}
		icon = input.Icon
	}
	if err := s.ensureLocalAppRoot(); err != nil {
		return LocalAppDocument{}, err
	}

	current, readErr := s.readLocalAppConfig(token)
	switch {
	case input.ResourceVersion == "":
		if readErr == nil || !errors.Is(readErr, os.ErrNotExist) {
			return LocalAppDocument{}, ErrLocalAppExists
		}
	case readErr != nil:
		if errors.Is(readErr, os.ErrNotExist) {
			return LocalAppDocument{}, ErrConflict
		}
		return LocalAppDocument{}, readErr
	default:
		currentIcon, iconErr := s.readLocalAppIcon(token)
		if iconErr != nil && !errors.Is(iconErr, ErrAppIconNotFound) {
			return LocalAppDocument{}, iconErr
		}
		if localAppVersion(current, currentIcon) != input.ResourceVersion {
			return LocalAppDocument{}, ErrConflict
		}
		if item, findErr := s.Find(ctx, localAppIDPrefix+token); findErr == nil && item.Runtime.Installed {
			previous, previousErr := parseScriptAppSpec(bytes.NewReader(current))
			if previousErr == nil && (previous.Container != spec.Container || previous.Service != spec.Service) {
				return LocalAppDocument{}, localAppInvalid(
					"应用已安装，docker_name 与 docker_app_service 不能修改（当前为 %s）；如需更换请先卸载", previous.Container)
			}
		}
	}

	root := s.scriptLocalAppRoot
	if _, err := (atomicfile.Writer{}).WritePrivate(root, filepath.Join(root, token+".conf"), ".kpanel-local-*", content); err != nil {
		return LocalAppDocument{}, err
	}
	if icon != nil || input.RemoveIcon {
		for _, extension := range localAppIconTypes {
			if err := os.Remove(filepath.Join(root, token+extension)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return LocalAppDocument{}, err
			}
		}
	}
	if icon != nil {
		target := filepath.Join(root, token+localAppIconTypes[icon.ContentType])
		if _, err := (atomicfile.Writer{}).WritePrivate(root, target, ".kpanel-local-*", icon.Data); err != nil {
			return LocalAppDocument{}, err
		}
	}
	return s.localAppDocument(ctx, token)
}

// DeleteLocalApp removes a local config and icon. Installed apps must be
// uninstalled first so their containers never outlive the config managing them.
func (s *Service) DeleteLocalApp(ctx context.Context, token, resourceVersion string) error {
	s.actions.Lock()
	defer s.actions.Unlock()
	if !tokenPattern.MatchString(token) {
		return ErrNotFound
	}
	if err := s.checkLocalAppIdle(token); err != nil {
		return err
	}
	current, err := s.readLocalAppConfig(token)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return err
	}
	icon, iconErr := s.readLocalAppIcon(token)
	if iconErr != nil && !errors.Is(iconErr, ErrAppIconNotFound) {
		return iconErr
	}
	if resourceVersion == "" || localAppVersion(current, icon) != resourceVersion {
		return ErrConflict
	}
	if item, findErr := s.Find(ctx, localAppIDPrefix+token); findErr == nil && item.Runtime.Installed {
		return fmt.Errorf("%w: 应用已安装，请先在应用市场卸载后再删除配置", ErrForbidden)
	}
	root := s.scriptLocalAppRoot
	if err := os.Remove(filepath.Join(root, token+".conf")); err != nil {
		return err
	}
	for _, extension := range localAppIconTypes {
		if err := os.Remove(filepath.Join(root, token+extension)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// LocalAppIcon serves the uploaded icon of a listed local app.
func (s *Service) LocalAppIcon(token string) (LocalAppIcon, error) {
	if !tokenPattern.MatchString(token) {
		return LocalAppIcon{}, ErrAppIconNotFound
	}
	icon, err := s.readLocalAppIcon(token)
	if err != nil {
		return LocalAppIcon{}, err
	}
	if icon == nil {
		return LocalAppIcon{}, ErrAppIconNotFound
	}
	return *icon, nil
}

func (s *Service) checkLocalAppName(token string) error {
	if _, err := os.Lstat(filepath.Join(s.scriptAppRoot, token+".conf")); err == nil {
		return localAppInvalid("%s 与官方第三方应用同名，请换一个名称", token)
	}
	catalog := s.currentCatalog(context.Background()).Catalog
	for _, app := range catalog.Apps {
		if strings.EqualFold(app.Token, token) {
			return localAppInvalid("%s 与应用市场已有应用同名，请换一个名称", token)
		}
	}
	return nil
}

func (s *Service) checkLocalAppIdle(token string) error {
	if s.jobs == nil {
		return nil
	}
	s.reconcileInactiveScriptJobs()
	for _, record := range s.jobs.list() {
		if record.AppID == localAppIDPrefix+token && (record.Status == "queued" || record.Status == "running") {
			return ErrTaskConflict
		}
	}
	return nil
}

func (s *Service) validateLocalAppContent(ctx context.Context, token string, content []byte) (scriptAppSpec, error) {
	if len(bytes.TrimSpace(content)) == 0 {
		return scriptAppSpec{}, localAppInvalid("配置内容不能为空")
	}
	if len(content) > maxScriptAppConfigBytes {
		return scriptAppSpec{}, localAppInvalid("配置不能超过 %d KB", maxScriptAppConfigBytes>>10)
	}
	if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return scriptAppSpec{}, localAppInvalid("配置必须是 UTF-8 文本")
	}
	spec, err := parseScriptAppSpec(bytes.NewReader(content))
	if err != nil {
		return scriptAppSpec{}, localAppInvalid("需要静态的 local docker_name=\"容器名\"，并以 docker_app_plus 结尾（%v）", err)
	}
	if spec.Adapter != "docker_app_plus" {
		return scriptAppSpec{}, localAppInvalid("本地应用需以 docker_app_plus 注册")
	}
	appID := ""
	for _, line := range strings.Split(string(content), "\n") {
		if matches := localAppIDPattern.FindStringSubmatch(strings.TrimSpace(line)); len(matches) == 4 {
			appID = matches[1] + matches[2] + matches[3]
		}
	}
	if appID != token {
		return scriptAppSpec{}, localAppInvalid("需要 local app_id=\"%s\"，与名称保持一致，否则无法记录安装状态", token)
	}
	found := map[string]bool{}
	for _, match := range localAppFunctionPattern.FindAllStringSubmatch(string(content), -1) {
		found[match[1]] = true
	}
	for _, name := range []string{"docker_app_install", "docker_app_update", "docker_app_uninstall"} {
		if !found[name] {
			return scriptAppSpec{}, localAppInvalid("缺少 %s() 函数", name)
		}
	}
	if err := s.checkLocalAppSyntax(ctx, content); err != nil {
		return scriptAppSpec{}, err
	}
	return spec, nil
}

// checkLocalAppSyntax runs `bash -n`, which parses without executing anything.
func (s *Service) checkLocalAppSyntax(ctx context.Context, content []byte) error {
	if s.localAppSyntaxCheck != nil {
		return s.localAppSyntaxCheck(ctx, content)
	}
	return bashSyntaxCheck(ctx, content)
}

func bashSyntaxCheck(ctx context.Context, content []byte) error {
	ctx, cancel := context.WithTimeout(ctx, localAppSyntaxTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "/bin/bash", "-n")
	command.Stdin = bytes.NewReader(content)
	command.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LC_ALL=C"}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return localAppInvalid("语法检查超时")
		}
		detail := strings.TrimSpace(strings.ReplaceAll(stderr.String(), "bash: ", ""))
		if len(detail) > 400 {
			detail = detail[:400]
		}
		if detail == "" {
			detail = err.Error()
		}
		return localAppInvalid("Shell 语法错误：%s", detail)
	}
	return nil
}

func validateLocalAppIcon(icon *LocalAppIcon) error {
	if _, ok := localAppIconTypes[icon.ContentType]; !ok {
		return localAppInvalid("图标只支持 PNG 或 WebP")
	}
	if len(icon.Data) == 0 || len(icon.Data) > maxLocalAppIconBytes {
		return localAppInvalid("图标不能超过 %d KB", maxLocalAppIconBytes>>10)
	}
	var config image.Config
	var err error
	switch icon.ContentType {
	case "image/png":
		config, err = png.DecodeConfig(bytes.NewReader(icon.Data))
		if err == nil {
			_, err = png.Decode(bytes.NewReader(icon.Data))
		}
	case "image/webp":
		config, err = webp.DecodeConfig(bytes.NewReader(icon.Data))
		if err == nil {
			_, err = webp.Decode(bytes.NewReader(icon.Data))
		}
	}
	if err != nil {
		return localAppInvalid("图标文件无法解析")
	}
	if config.Width != config.Height || config.Width < minLocalAppIconSide || config.Width > maxLocalAppIconSide {
		return localAppInvalid("图标需为正方形，边长 %d–%d px", minLocalAppIconSide, maxLocalAppIconSide)
	}
	return nil
}

func (s *Service) ensureLocalAppRoot() error {
	root := s.scriptLocalAppRoot
	if root == "" {
		return errors.New("local application directory is not configured")
	}
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(root, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err = os.Lstat(root)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !s.fileOwnerTrusted(info) ||
		(runtime.GOOS == "linux" && info.Mode().Perm()&0o022 != 0) {
		return errors.New("local application directory is not a trusted directory")
	}
	return nil
}

func (s *Service) readLocalAppConfig(token string) ([]byte, error) {
	file, err := s.openScriptAppConfig(localAppSource, token)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, maxScriptAppConfigBytes+1))
}

// readLocalAppIcon returns nil without error when the app has no icon.
func (s *Service) readLocalAppIcon(token string) (*LocalAppIcon, error) {
	root := s.scriptLocalAppRoot
	if root == "" || !tokenPattern.MatchString(token) {
		return nil, ErrAppIconNotFound
	}
	for _, contentType := range localAppIconOrder {
		extension := localAppIconTypes[contentType]
		path := filepath.Join(root, token+extension)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxLocalAppIconBytes ||
			!s.fileOwnerTrusted(info) || (runtime.GOOS == "linux" && info.Mode().Perm()&0o022 != 0) {
			return nil, ErrAppIconNotFound
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		icon := &LocalAppIcon{ContentType: contentType, Data: data}
		if validateLocalAppIcon(icon) != nil {
			return nil, ErrAppIconNotFound
		}
		return icon, nil
	}
	return nil, nil
}

func localAppVersion(content []byte, icon *LocalAppIcon) string {
	digest := sha256.New()
	_, _ = digest.Write(content)
	if icon != nil {
		_, _ = digest.Write([]byte("\x00" + icon.ContentType + "\x00"))
		_, _ = digest.Write(icon.Data)
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil))
}

// localAppIconURL embeds a content hash so a replaced icon is never served stale.
func localAppIconURL(token string, data []byte) string {
	digest := sha256.Sum256(data)
	return "/api/v1/apps/local-icons/" + token + "/" + hex.EncodeToString(digest[:6])
}
