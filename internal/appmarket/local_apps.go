package appmarket

import (
	"bufio"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Local applications are user-authored kejilion.sh configs in ~/apps-local.
// kejilion.sh resolves `app <name>` from ~/apps first, so an official config
// with the same name always wins; KPanel mirrors that order.
const (
	localAppSource   = "local"
	localAppIDPrefix = "local-"
	localAppCategory = "local"
	maxLocalApps     = 200
)

var localAppMetadataPattern = regexp.MustCompile(
	`^(?:local[[:space:]]+)?(app_name|app_text|app_url)[[:space:]]*=[[:space:]]*(?:"([^"$` + "`" + `\\]*)"|'([^']*)')[[:space:]]*(?:#.*)?$`,
)

var localAppCategoryEntry = Category{
	Key: localAppCategory, ZH: "本地应用", ZHTW: "本地應用", EN: "Local apps",
}

// Older kejilion.sh only resolves ~/apps; without this marker `app <name>`
// cannot find a local config, so script actions stay disabled.
var localAppProtocol = regexp.MustCompile(`(?m)^KPANEL_LOCAL_APPS_PROTOCOL_VERSION="1"\r?$`)

const localAppScriptOutdated = "请先更新 kejilion.sh 以支持 ~/apps-local 本地应用"

func usesScriptAppConfig(source string) bool {
	return source == "thirdparty" || source == localAppSource
}

func (s *Service) localAppScriptSupported() bool {
	if s.scriptInteractiveFinder == nil {
		return false
	}
	path, err := s.scriptInteractiveFinder()
	return err == nil && s.scriptHasProtocol(path, localAppProtocol)
}

func disableLocalAppScriptActions(item *Summary) {
	for _, action := range []string{"install", "update", "uninstall", "direct_access", "manage"} {
		if item.Capabilities[action].Enabled {
			item.Capabilities[action] = Capability{Reason: localAppScriptOutdated}
		}
	}
}

// withLocalApps returns catalog plus the trusted local configs. It never
// mutates the shared catalog slices.
func (s *Service) withLocalApps(catalog Catalog) Catalog {
	if s.scriptLocalAppRoot == "" {
		return catalog
	}
	entries, err := os.ReadDir(s.scriptLocalAppRoot)
	if err != nil {
		return catalog
	}
	taken := make(map[string]bool, len(catalog.Apps))
	for _, app := range catalog.Apps {
		taken[strings.ToLower(app.Token)] = true
	}
	local := make([]App, 0)
	for _, entry := range entries {
		if len(local) >= maxLocalApps {
			break
		}
		name := entry.Name()
		token, ok := strings.CutSuffix(name, ".conf")
		if !ok || !entry.Type().IsRegular() || !tokenPattern.MatchString(token) ||
			taken[strings.ToLower(token)] {
			continue
		}
		if _, err := os.Lstat(filepath.Join(s.scriptAppRoot, name)); err == nil {
			continue
		}
		app, ok := s.readLocalApp(token)
		if !ok {
			continue
		}
		taken[strings.ToLower(token)] = true
		local = append(local, app)
	}
	if len(local) == 0 {
		return catalog
	}
	sort.Slice(local, func(i, j int) bool { return local[i].Token < local[j].Token })
	result := catalog
	result.Categories = append(append([]Category(nil), catalog.Categories...), localAppCategoryEntry)
	result.Apps = append(append(make([]App, 0, len(catalog.Apps)+len(local)), catalog.Apps...), local...)
	return result
}

func (s *Service) readLocalApp(token string) (App, bool) {
	file, err := s.openScriptAppConfig(localAppSource, token)
	if err != nil {
		return App{}, false
	}
	defer file.Close()
	app := App{
		ID: localAppIDPrefix + token, Source: localAppSource, Token: token, Slug: token,
		NameZH: token, NameEN: token, Category: localAppCategory, Icon: genericThirdPartyIcon,
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4<<10), maxScriptAppConfigBytes)
	for scanner.Scan() {
		matches := localAppMetadataPattern.FindStringSubmatch(strings.TrimSpace(scanner.Text()))
		if len(matches) != 4 {
			continue
		}
		value := strings.TrimSpace(matches[2] + matches[3])
		switch matches[1] {
		case "app_name":
			if value != "" && len(value) <= 160 {
				app.NameZH, app.NameEN = value, value
			}
		case "app_text":
			if len(value) <= 2000 {
				app.Description, app.DescriptionEN = value, value
			}
		case "app_url":
			if website, err := url.Parse(value); err == nil && website.Host != "" &&
				website.User == nil && (website.Scheme == "http" || website.Scheme == "https") {
				app.Website = value
			}
		}
	}
	if scanner.Err() != nil {
		return App{}, false
	}
	return app, true
}
