package selfupdate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	githubLatestURL       = "https://api.github.com/repos/kejilion/KPanel/releases/latest"
	githubPreviewURL      = "https://api.github.com/repos/kejilion/KPanel/releases?per_page=20"
	maxReleaseResponse    = 1 << 20
	officialImageName     = "docker.io/kjlion/kejilion-panel"
	releaseRequestTimeout = 15 * time.Second
	maxReleaseNotes       = 8
	maxUpgradeNotes       = 3
	maxReleaseNoteRunes   = 280
)

var releaseImagePattern = regexp.MustCompile(
	`(?m)^- (生产|预览)镜像：\x60docker\.io/kjlion/kejilion-panel@(sha256:[0-9a-f]{64})\x60\r?$`,
)

var markdownLinkPattern = regexp.MustCompile(`\[([^\]]{1,200})\]\([^\r\n)]+\)`)

type githubReleasePayload struct {
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url"`
	Body        string `json:"body"`
	Draft       *bool  `json:"draft"`
	Prerelease  *bool  `json:"prerelease"`
	PublishedAt string `json:"published_at"`
}

type GitHubLatestSource struct {
	client  *http.Client
	channel Channel
}

func NewGitHubLatestSource() *GitHubLatestSource {
	return newGitHubReleaseSource(ChannelStable)
}

func NewGitHubPreviewSource() *GitHubLatestSource {
	return newGitHubReleaseSource(ChannelPreview)
}

func newGitHubReleaseSource(channel Channel) *GitHubLatestSource {
	return &GitHubLatestSource{
		channel: channel,
		client: &http.Client{
			Timeout: releaseRequestTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (s *GitHubLatestSource) Latest(ctx context.Context) (Release, error) {
	if s == nil || s.client == nil {
		return Release{}, errors.New("release client is unavailable")
	}
	channel := s.channel
	if channel == "" {
		channel = ChannelStable
	}
	if !validChannel(channel) {
		return Release{}, errors.New("release channel is invalid")
	}
	endpoint := githubLatestURL
	if channel == ChannelPreview {
		endpoint = githubPreviewURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Release{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "KPanel-Release-Update/2")
	response, err := s.client.Do(request)
	if err != nil {
		return Release{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("%s release endpoint returned HTTP %d", channel, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxReleaseResponse+1))
	if err != nil {
		return Release{}, err
	}
	if len(data) > maxReleaseResponse {
		return Release{}, fmt.Errorf("%s release metadata exceeds the size limit", channel)
	}
	if channel == ChannelPreview {
		return decodePreviewRelease(data)
	}
	return decodeStableRelease(data)
}

func decodeStableRelease(data []byte) (Release, error) {
	var payload githubReleasePayload
	if err := decodeOneJSON(data, &payload); err != nil {
		return Release{}, errors.New("stable release metadata is invalid")
	}
	if payload.Draft == nil || payload.Prerelease == nil || *payload.Draft || *payload.Prerelease ||
		strings.TrimSpace(payload.PublishedAt) == "" {
		return Release{}, errors.New("stable release endpoint did not return a published stable release")
	}
	version := normalizeStableVersion(payload.TagName)
	if err := validateReleaseIdentity(payload, version); err != nil {
		return Release{}, fmt.Errorf("stable release identity is invalid: %w", err)
	}
	return releaseFromPayload(payload, version, "生产")
}

func decodePreviewRelease(data []byte) (Release, error) {
	var payloads []githubReleasePayload
	if err := decodeOneJSON(data, &payloads); err != nil {
		return Release{}, errors.New("preview release metadata is invalid")
	}
	seen := make(map[string]bool)
	selectedVersion := ""
	var selected githubReleasePayload
	for _, payload := range payloads {
		stableVersion := normalizeStableVersion(payload.TagName)
		previewVersion := normalizePreviewVersion(payload.TagName)
		if stableVersion == "" && previewVersion == "" {
			continue
		}
		version := stableVersion
		isPreview := false
		if previewVersion != "" {
			version = previewVersion
			isPreview = true
		}
		if payload.Draft == nil || payload.Prerelease == nil {
			return Release{}, fmt.Errorf("release v%s is missing publication flags", version)
		}
		if *payload.Draft || strings.TrimSpace(payload.PublishedAt) == "" {
			continue
		}
		if *payload.Prerelease != isPreview {
			return Release{}, fmt.Errorf("release v%s has inconsistent prerelease metadata", version)
		}
		if err := validateReleaseIdentity(payload, version); err != nil {
			return Release{}, fmt.Errorf("release v%s identity is invalid: %w", version, err)
		}
		if seen[version] {
			return Release{}, fmt.Errorf("release v%s appears more than once", version)
		}
		seen[version] = true
		if selectedVersion == "" || compareVersions(version, selectedVersion) > 0 {
			selectedVersion = version
			selected = payload
		}
	}
	if selectedVersion == "" {
		return Release{}, errors.New("preview release endpoint did not return a published stable or rc release")
	}
	label := "生产"
	if normalizePreviewVersion(selectedVersion) != "" {
		label = "预览"
	}
	return releaseFromPayload(selected, selectedVersion, label)
}

func decodeOneJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("metadata contains multiple JSON values")
		}
		return err
	}
	return nil
}

func validateReleaseIdentity(payload githubReleasePayload, version string) error {
	if version == "" || payload.TagName != "v"+version {
		return errors.New("tag is not canonical")
	}
	if payload.HTMLURL != "https://github.com/kejilion/KPanel/releases/tag/v"+version {
		return errors.New("release URL is not official")
	}
	return nil
}

func releaseFromPayload(payload githubReleasePayload, version, expectedLabel string) (Release, error) {
	matches := releaseImagePattern.FindAllStringSubmatch(payload.Body, 2)
	if len(matches) != 1 || len(matches[0]) != 3 || matches[0][1] != expectedLabel {
		return Release{}, fmt.Errorf("release v%s does not contain one correctly labelled official image digest", version)
	}
	digest := normalizeImageDigest(matches[0][2])
	if digest == "" {
		return Release{}, fmt.Errorf("release v%s image digest is invalid", version)
	}
	notes, upgradeNotes := parseReleaseNotes(payload.Body)
	return Release{
		Version: version, ImageDigest: digest, ReleaseURL: payload.HTMLURL,
		PublishedAt: payload.PublishedAt, Notes: notes, UpgradeNotes: upgradeNotes,
	}, nil
}

func parseReleaseNotes(body string) ([]ReleaseNote, []string) {
	notes := make([]ReleaseNote, 0, maxReleaseNotes)
	upgradeNotes := make([]string, 0, maxUpgradeNotes)
	kind := ""
	inUpgradeNotes := false
	for _, rawLine := range strings.Split(body, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
		if heading, ok := markdownHeading(line); ok {
			if releaseKind := releaseNoteKind(heading); releaseKind != "" {
				kind = releaseKind
				inUpgradeNotes = false
				continue
			}
			if isUpgradeNotesHeading(heading) {
				kind = ""
				inUpgradeNotes = true
				continue
			}
			if !isReleaseNotesHeading(heading) {
				kind = ""
				inUpgradeNotes = false
			}
			continue
		}
		text, ok := markdownBulletText(line)
		if !ok {
			continue
		}
		text = plainReleaseNote(text)
		if text == "" {
			continue
		}
		if inUpgradeNotes && len(upgradeNotes) < maxUpgradeNotes {
			upgradeNotes = append(upgradeNotes, text)
		} else if kind != "" && len(notes) < maxReleaseNotes {
			notes = append(notes, ReleaseNote{Kind: kind, Text: text})
		}
	}
	return notes, upgradeNotes
}

func markdownHeading(line string) (string, bool) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level < 2 || level > 6 || level >= len(line) || line[level] != ' ' {
		return "", false
	}
	heading := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(line[level+1:]), "#"))
	return heading, heading != ""
}

func markdownBulletText(line string) (string, bool) {
	if len(line) < 3 || (line[0] != '-' && line[0] != '*') || line[1] != ' ' {
		return "", false
	}
	return strings.TrimSpace(line[2:]), true
}

func releaseNoteKind(heading string) string {
	switch strings.ToLower(strings.TrimSpace(heading)) {
	case "新增", "added", "new":
		return "added"
	case "变更", "changed", "改进", "improved":
		return "changed"
	case "修复", "fixed", "bug fixes":
		return "fixed"
	case "安全", "security":
		return "security"
	case "性能", "performance":
		return "performance"
	case "兼容性", "compatibility":
		return "compatibility"
	default:
		return ""
	}
}

func isReleaseNotesHeading(heading string) bool {
	switch strings.ToLower(strings.TrimSpace(heading)) {
	case "版本更新内容", "更新内容", "release notes", "what's changed":
		return true
	default:
		return false
	}
}

func isUpgradeNotesHeading(heading string) bool {
	switch strings.ToLower(strings.TrimSpace(heading)) {
	case "升级注意事项", "upgrade notes", "breaking changes":
		return true
	default:
		return false
	}
}

func plainReleaseNote(value string) string {
	value = markdownLinkPattern.ReplaceAllString(value, "$1")
	for _, marker := range []string{"**", "__", "`"} {
		value = strings.ReplaceAll(value, marker, "")
	}
	value = strings.Join(strings.Fields(value), " ")
	if utf8.RuneCountInString(value) > maxReleaseNoteRunes {
		runes := []rune(value)
		value = strings.TrimSpace(string(runes[:maxReleaseNoteRunes-1])) + "…"
	}
	return value
}
