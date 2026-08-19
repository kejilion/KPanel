package appmarket

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"time"
)

//go:embed catalog.json
var catalogJSON []byte

//go:embed legacy-apps.json
var legacyAppsJSON []byte

var (
	catalogIDPattern = regexp.MustCompile(`^(?:builtin|thirdparty)-[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
	tokenPattern     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
)

type Category struct {
	Key  string `json:"key"`
	ZH   string `json:"zh"`
	ZHTW string `json:"zh_tw,omitempty"`
	EN   string `json:"en"`
}

type App struct {
	ID              string `json:"id"`
	Num             int    `json:"num,omitempty"`
	Source          string `json:"source"`
	Token           string `json:"token"`
	NameZH          string `json:"name_zh"`
	NameZHTW        string `json:"name_zh_tw,omitempty"`
	NameEN          string `json:"name_en"`
	Description     string `json:"desc_zh"`
	DescriptionZHTW string `json:"desc_zh_tw,omitempty"`
	DescriptionEN   string `json:"desc_en"`
	Category        string `json:"cat"`
	Website         string `json:"url,omitempty"`
	Icon            string `json:"icon"`
	IconSHA256      string `json:"iconSha256"`
	Slug            string `json:"slug"`
	AddedAt         string `json:"addedAt,omitempty"`
}

type Catalog struct {
	SchemaVersion int        `json:"schemaVersion"`
	Source        string     `json:"source"`
	Upstream      string     `json:"upstream"`
	Categories    []Category `json:"categories"`
	Apps          []App      `json:"apps"`
}

type LegacyApp struct {
	Num                int    `json:"num"`
	Container          string `json:"container"`
	Service            string `json:"service,omitempty"`
	Image              string `json:"image"`
	DefaultPort        int    `json:"defaultPort"`
	UsesDockerApp      bool   `json:"usesDockerApp"`
	UsesPanelInstaller bool   `json:"usesPanelInstaller"`
}

type legacySnapshot struct {
	SchemaVersion int         `json:"schemaVersion"`
	ScriptSHA256  string      `json:"scriptSha256"`
	Apps          []LegacyApp `json:"apps"`
}

func LoadCatalog() (Catalog, map[int]LegacyApp, string, error) {
	var catalog Catalog
	if err := json.Unmarshal(catalogJSON, &catalog); err != nil {
		return Catalog{}, nil, "", fmt.Errorf("decode embedded application catalog: %w", err)
	}
	var legacy legacySnapshot
	if err := json.Unmarshal(legacyAppsJSON, &legacy); err != nil {
		return Catalog{}, nil, "", fmt.Errorf("decode embedded legacy application map: %w", err)
	}
	if catalog.SchemaVersion != 1 || legacy.SchemaVersion != 1 {
		return Catalog{}, nil, "", errors.New("unsupported embedded application catalog schema")
	}
	if len(catalog.Apps) < 115 || len(legacy.Apps) != 115 {
		return Catalog{}, nil, "", errors.New("embedded application catalog is incomplete")
	}
	categories := make(map[string]bool, len(catalog.Categories))
	for _, category := range catalog.Categories {
		if category.Key == "" || category.ZH == "" || categories[category.Key] {
			return Catalog{}, nil, "", errors.New("embedded application categories are invalid")
		}
		categories[category.Key] = true
	}
	ids := make(map[string]bool, len(catalog.Apps))
	tokens := make(map[string]bool, len(catalog.Apps))
	for _, app := range catalog.Apps {
		if !catalogIDPattern.MatchString(app.ID) || !tokenPattern.MatchString(app.Token) ||
			app.NameZH == "" || app.Icon == "" || !categories[app.Category] ||
			!validCatalogDate(app.AddedAt) || ids[app.ID] || tokens[app.Token] {
			return Catalog{}, nil, "", fmt.Errorf("embedded application %q is invalid or duplicated", app.ID)
		}
		ids[app.ID] = true
		tokens[app.Token] = true
	}
	legacyByNumber := make(map[int]LegacyApp, len(legacy.Apps))
	for _, item := range legacy.Apps {
		if item.Num < 1 || item.Num > 115 ||
			item.DefaultPort < 0 || item.DefaultPort > 65535 ||
			legacyByNumber[item.Num].Num != 0 {
			return Catalog{}, nil, "", errors.New("embedded legacy application map is invalid")
		}
		legacyByNumber[item.Num] = item
	}
	sort.Slice(catalog.Apps, func(i, j int) bool {
		if catalog.Apps[i].Num == 0 {
			return false
		}
		if catalog.Apps[j].Num == 0 {
			return true
		}
		return catalog.Apps[i].Num < catalog.Apps[j].Num
	})
	return catalog, legacyByNumber, legacy.ScriptSHA256, nil
}

func validCatalogDate(value string) bool {
	if value == "" {
		return true
	}
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}
