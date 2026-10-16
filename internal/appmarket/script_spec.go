package appmarket

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

const maxScriptAppConfigBytes = 256 << 10

var (
	dockerObjectNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
	scriptAssignmentPattern = regexp.MustCompile(
		`^(?:local[[:space:]]+)?(docker_name|docker_app_service)[[:space:]]*=[[:space:]]*(?:"([A-Za-z0-9][A-Za-z0-9_.-]{0,127})"|'([A-Za-z0-9][A-Za-z0-9_.-]{0,127})'|([A-Za-z0-9][A-Za-z0-9_.-]{0,127}))[[:space:]]*(?:#.*)?$`,
	)
	scriptAssignmentPrefixPattern = regexp.MustCompile(
		`^(?:local[[:space:]]+)?(docker_name|docker_app_service)[[:space:]]*=`,
	)
	scriptPortAssignmentPattern = regexp.MustCompile(
		`^(?:local[[:space:]]+)?docker_port[[:space:]]*=[[:space:]]*(?:"(?:[$][{]docker_port:-)?([0-9]{1,5})(?:[}])?"|'(?:[$][{]docker_port:-)?([0-9]{1,5})(?:[}])?'|(?:[$][{]docker_port:-)?([0-9]{1,5})(?:[}])?)[[:space:]]*(?:#.*)?$`,
	)
)

type scriptAppSpec struct {
	Container string
	Service   string
	Adapter   string
	Port      uint16
}

func (spec scriptAppSpec) runtimeContainer() string {
	if spec.Service != "" {
		return spec.Service
	}
	return spec.Container
}

// scriptAppConfigRoot returns where kejilion.sh reads the .conf for a source.
func (s *Service) scriptAppConfigRoot(source string) (string, bool) {
	switch source {
	case "thirdparty":
		return s.scriptAppRoot, true
	case localAppSource:
		return s.scriptLocalAppRoot, true
	}
	return "", false
}

func (s *Service) openScriptAppConfig(source, token string) (*os.File, error) {
	root, ok := s.scriptAppConfigRoot(source)
	if !ok || root == "" {
		return nil, errors.New("application source has no script configuration")
	}
	if !tokenPattern.MatchString(token) {
		return nil, errors.New("invalid application token")
	}
	path := filepath.Join(root, token+".conf")
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		info.Size() <= 0 || info.Size() > maxScriptAppConfigBytes ||
		!s.fileOwnerTrusted(info) ||
		(runtime.GOOS == "linux" && info.Mode().Perm()&0o022 != 0) {
		return nil, errors.New("application configuration is not a trusted regular file")
	}
	return os.Open(path)
}

func (s *Service) readThirdPartyScriptSpec(token string) (scriptAppSpec, error) {
	return s.readScriptAppSpec("thirdparty", token)
}

func (s *Service) readScriptAppSpec(source, token string) (scriptAppSpec, error) {
	file, err := s.openScriptAppConfig(source, token)
	if err != nil {
		return scriptAppSpec{}, err
	}
	defer file.Close()
	return parseScriptAppSpec(file)
}

// parseScriptAppSpec reads the static container metadata kejilion.sh relies on;
// it also validates local app configs before they are written.
func parseScriptAppSpec(source io.Reader) (scriptAppSpec, error) {
	spec := scriptAppSpec{}
	assignments := make(map[string]bool, 2)
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 4<<10), maxScriptAppConfigBytes)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch line {
		case "docker_app", "docker_app_plus":
			if spec.Adapter != "" {
				return scriptAppSpec{}, errors.New("application adapter is duplicated")
			}
			spec.Adapter = line
			continue
		}
		if spec.Port == 0 {
			portMatches := scriptPortAssignmentPattern.FindStringSubmatch(line)
			if len(portMatches) == 4 {
				rawPort := portMatches[1] + portMatches[2] + portMatches[3]
				port, parseErr := strconv.ParseUint(rawPort, 10, 16)
				if parseErr == nil && port > 0 {
					spec.Port = uint16(port)
				}
				continue
			}
		}
		matches := scriptAssignmentPattern.FindStringSubmatch(line)
		if len(matches) == 5 {
			key := matches[1]
			if assignments[key] {
				return scriptAppSpec{}, fmt.Errorf("%s is duplicated", key)
			}
			assignments[key] = true
			value := matches[2] + matches[3] + matches[4]
			if !dockerObjectNamePattern.MatchString(value) {
				return scriptAppSpec{}, fmt.Errorf("%s is invalid", key)
			}
			if key == "docker_name" {
				spec.Container = value
			} else {
				spec.Service = value
			}
			continue
		}
		if scriptAssignmentPrefixPattern.MatchString(line) {
			return scriptAppSpec{}, errors.New("dynamic container assignments are not accepted")
		}
	}
	if err := scanner.Err(); err != nil {
		return scriptAppSpec{}, err
	}
	if spec.Container == "" || spec.Adapter == "" {
		return scriptAppSpec{}, errors.New("application container metadata is incomplete")
	}
	return spec, nil
}

func (s *Service) readScriptAccessMode(containerName string) (string, bool) {
	if !dockerObjectNamePattern.MatchString(containerName) {
		return "", false
	}
	path := filepath.Join(s.appRoot, containerName+"_access.conf")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 ||
		info.Size() <= 0 || info.Size() > 64 ||
		!s.fileOwnerTrusted(info) ||
		(runtime.GOOS == "linux" && info.Mode().Perm()&0o022 != 0) {
		return "", false
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	mode := strings.TrimSpace(string(value))
	if mode != "direct" && mode != "domain_only" {
		return "", false
	}
	return mode, true
}
