package appmarket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	OfficialCatalogURL    = "https://app.kejilion.sh/"
	officialCatalogSource = "https://github.com/kejilion/sh"
	maxRemoteCatalogBytes = 2 << 20
	maxRemoteCatalogApps  = 500
	remoteCatalogTTL      = 5 * time.Minute
	genericThirdPartyIcon = "/app-icons/thirdparty-default.svg"
	dynamicAppIconPrefix  = "/api/v1/apps/icons/"
)

var remoteIconPattern = regexp.MustCompile(`^icons/[a-z0-9][a-z0-9_-]{0,63}[.]webp$`)

var officialCategoryKeys = map[string]bool{
	"ops": true, "ai": true, "storage": true, "media": true,
	"netsec": true, "devprod": true, "commtools": true,
}

type remoteCatalogMeta struct {
	Builtin    int    `json:"builtin"`
	ThirdParty int    `json:"thirdparty"`
	Source     string `json:"source"`
}

type remoteCatalogPayload struct {
	Meta       remoteCatalogMeta `json:"meta"`
	Categories []Category        `json:"categories"`
	Apps       []App             `json:"apps"`
}

type catalogSnapshot struct {
	Catalog     Catalog
	Mode        string
	Warning     string
	RefreshedAt time.Time
}

type catalogFetcher func(context.Context) (Catalog, error)

func newOfficialCatalogFetcher() catalogFetcher {
	client := &http.Client{
		Timeout: 6 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 2 || request.URL.Scheme != "https" ||
				request.URL.Hostname() != "app.kejilion.sh" {
				return errors.New("application catalog redirect was rejected")
			}
			return nil
		},
	}
	return func(ctx context.Context) (Catalog, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, OfficialCatalogURL, nil)
		if err != nil {
			return Catalog{}, err
		}
		request.Header.Set("Accept", "text/html")
		request.Header.Set("User-Agent", "KPanel application catalog")
		response, err := client.Do(request)
		if err != nil {
			return Catalog{}, fmt.Errorf("fetch application catalog: %w", err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return Catalog{}, fmt.Errorf("fetch application catalog: HTTP %d", response.StatusCode)
		}
		mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
		if err != nil || mediaType != "text/html" {
			return Catalog{}, errors.New("application catalog returned an invalid content type")
		}
		content, err := io.ReadAll(io.LimitReader(response.Body, maxRemoteCatalogBytes+1))
		if err != nil {
			return Catalog{}, fmt.Errorf("read application catalog: %w", err)
		}
		if len(content) > maxRemoteCatalogBytes {
			return Catalog{}, errors.New("application catalog exceeds 2 MiB")
		}
		return decodeRemoteCatalog(content)
	}
}

func decodeRemoteCatalog(content []byte) (Catalog, error) {
	const prefix = "window.__APPS__ = "
	start := strings.Index(string(content), prefix)
	if start < 0 {
		return Catalog{}, errors.New("application catalog payload was not found")
	}
	body := content[start+len(prefix):]
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	var payload remoteCatalogPayload
	if err := decoder.Decode(&payload); err != nil {
		return Catalog{}, fmt.Errorf("decode application catalog: %w", err)
	}
	remainder := strings.TrimSpace(string(body[decoder.InputOffset():]))
	if !strings.HasPrefix(remainder, ";") ||
		!strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(remainder, ";")), "</script>") {
		return Catalog{}, errors.New("application catalog payload was not terminated")
	}
	catalog := Catalog{
		SchemaVersion: 1,
		Source:        OfficialCatalogURL,
		Upstream:      payload.Meta.Source,
		Categories:    payload.Categories,
		Apps:          payload.Apps,
	}
	if err := validateRemoteCatalog(catalog, payload.Meta); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

func validateRemoteCatalog(catalog Catalog, meta remoteCatalogMeta) error {
	if meta.Source != officialCatalogSource {
		return errors.New("application catalog source changed")
	}
	if meta.Builtin < 1 || meta.Builtin > maxRemoteCatalogApps ||
		meta.ThirdParty < 0 || meta.ThirdParty > maxRemoteCatalogApps ||
		len(catalog.Apps) > maxRemoteCatalogApps ||
		meta.Builtin+meta.ThirdParty != len(catalog.Apps) {
		return errors.New("application catalog count is outside the bounded contract")
	}
	if len(catalog.Categories) != 7 {
		return errors.New("application catalog category count changed")
	}
	categories := make(map[string]bool, len(catalog.Categories))
	for _, category := range catalog.Categories {
		if !officialCategoryKeys[category.Key] || category.ZH == "" || category.EN == "" ||
			categories[category.Key] {
			return errors.New("application catalog categories are invalid")
		}
		categories[category.Key] = true
	}
	ids := make(map[string]bool, len(catalog.Apps))
	tokens := make(map[string]bool, len(catalog.Apps))
	slugs := make(map[string]bool, len(catalog.Apps))
	builtinNumbers := make(map[int]bool, meta.Builtin)
	builtin := 0
	thirdParty := 0
	for _, app := range catalog.Apps {
		if !catalogIDPattern.MatchString(app.ID) || !tokenPattern.MatchString(app.Token) ||
			!tokenPattern.MatchString(app.Slug) || !remoteIconPattern.MatchString(app.Icon) ||
			app.Icon != "icons/"+app.Slug+".webp" ||
			!categories[app.Category] || app.NameZH == "" || len(app.NameZH) > 160 ||
			len(app.NameEN) > 160 || len(app.Description) > 2000 ||
			len(app.DescriptionEN) > 2000 || !validCatalogDate(app.AddedAt) ||
			ids[app.ID] || tokens[app.Token] || slugs[app.Slug] {
			return fmt.Errorf("application catalog entry %q is invalid or duplicated", app.ID)
		}
		if app.Source != "builtin" && app.Source != "thirdparty" {
			return fmt.Errorf("application catalog entry %q has an invalid source", app.ID)
		}
		if app.Website != "" {
			website, err := url.Parse(app.Website)
			if err != nil || website.Host == "" || website.User != nil ||
				(website.Scheme != "http" && website.Scheme != "https") {
				return fmt.Errorf("application catalog entry %q has an invalid website", app.ID)
			}
		}
		if app.Source == "builtin" {
			if app.Num < 1 || app.Num > maxRemoteCatalogApps ||
				app.ID != "builtin-"+strconv.Itoa(app.Num) || builtinNumbers[app.Num] {
				return fmt.Errorf("application catalog entry %q has an inconsistent builtin number", app.ID)
			}
			builtinNumbers[app.Num] = true
			builtin++
		} else {
			if app.Num != 0 || !strings.HasPrefix(app.ID, "thirdparty-") {
				return fmt.Errorf("application catalog entry %q has an inconsistent ID", app.ID)
			}
			thirdParty++
		}
		ids[app.ID] = true
		tokens[app.Token] = true
		slugs[app.Slug] = true
	}
	if builtin != meta.Builtin || thirdParty != meta.ThirdParty {
		return errors.New("application catalog source counts are inconsistent")
	}
	return nil
}

func mergeRemoteCatalog(embedded, remote Catalog) Catalog {
	return mergeRemoteCatalogWithDynamicIcons(embedded, remote, false)
}

func mergeRemoteCatalogWithDynamicIcons(embedded, remote Catalog, enabled bool) Catalog {
	localByID := make(map[string]App, len(embedded.Apps))
	localByToken := make(map[string]App, len(embedded.Apps))
	result := Catalog{
		SchemaVersion: remote.SchemaVersion,
		Source:        remote.Source,
		Upstream:      remote.Upstream,
		Categories:    append([]Category(nil), remote.Categories...),
		Apps:          make([]App, 0, len(remote.Apps)),
	}
	localCategories := make(map[string]Category, len(embedded.Categories))
	for _, category := range embedded.Categories {
		localCategories[category.Key] = category
	}
	for index, category := range result.Categories {
		if local, ok := localCategories[category.Key]; ok && local.ZHTW != "" {
			result.Categories[index].ZHTW = local.ZHTW
		}
	}
	for _, app := range embedded.Apps {
		localByID[app.ID] = app
		localByToken[app.Token] = app
	}
	for _, app := range remote.Apps {
		local, ok := localByID[app.ID]
		if !ok {
			local, ok = localByToken[app.Token]
		}
		if ok {
			app.Icon = local.Icon
			app.IconSHA256 = local.IconSHA256
			app.NameZHTW = local.NameZHTW
			app.DescriptionZHTW = local.DescriptionZHTW
			app.AddedAt = local.AddedAt
		} else if enabled {
			app.Icon = dynamicAppIconPrefix + app.Slug + ".webp"
			app.IconSHA256 = ""
		} else {
			app.Icon = genericThirdPartyIcon
			app.IconSHA256 = ""
		}
		result.Apps = append(result.Apps, app)
	}
	return result
}

func preserveExistingAddedDates(existing Catalog, next *Catalog) {
	existingByID := make(map[string]string, len(existing.Apps))
	existingByToken := make(map[string]string, len(existing.Apps))
	for _, app := range existing.Apps {
		existingByID[app.ID] = app.AddedAt
		existingByToken[app.Token] = app.AddedAt
	}
	for index := range next.Apps {
		addedAt, ok := existingByID[next.Apps[index].ID]
		if !ok {
			addedAt, ok = existingByToken[next.Apps[index].Token]
		}
		if ok {
			next.Apps[index].AddedAt = addedAt
		}
	}
}

func dynamicRemoteIconSources(embedded, remote Catalog) map[string]string {
	localByID := make(map[string]bool, len(embedded.Apps))
	localByToken := make(map[string]bool, len(embedded.Apps))
	for _, app := range embedded.Apps {
		localByID[app.ID] = true
		localByToken[app.Token] = true
	}
	sources := make(map[string]string)
	for _, app := range remote.Apps {
		if !localByID[app.ID] && !localByToken[app.Token] {
			sources[app.Slug] = app.Icon
		}
	}
	return sources
}
