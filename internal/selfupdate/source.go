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
)

const (
	githubLatestURL       = "https://api.github.com/repos/kejilion/KPanel/releases/latest"
	maxReleaseResponse    = 256 << 10
	officialImageName     = "docker.io/kjlion/kejilion-panel"
	releaseRequestTimeout = 15 * time.Second
)

var releaseImagePattern = regexp.MustCompile(
	`(?m)^- 生产镜像：\x60docker\.io/kjlion/kejilion-panel@(sha256:[0-9a-f]{64})\x60\r?$`,
)

type GitHubLatestSource struct {
	client *http.Client
}

func NewGitHubLatestSource() *GitHubLatestSource {
	return &GitHubLatestSource{client: &http.Client{
		Timeout: releaseRequestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

func (s *GitHubLatestSource) Latest(ctx context.Context) (Release, error) {
	if s == nil || s.client == nil {
		return Release{}, errors.New("stable release client is unavailable")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, githubLatestURL, nil)
	if err != nil {
		return Release{}, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	request.Header.Set("User-Agent", "KPanel-Automatic-Update/1")
	response, err := s.client.Do(request)
	if err != nil {
		return Release{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("stable release endpoint returned HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxReleaseResponse+1))
	if err != nil {
		return Release{}, err
	}
	if len(data) > maxReleaseResponse {
		return Release{}, errors.New("stable release metadata exceeds the size limit")
	}
	var payload struct {
		TagName     string `json:"tag_name"`
		HTMLURL     string `json:"html_url"`
		Body        string `json:"body"`
		Draft       *bool  `json:"draft"`
		Prerelease  *bool  `json:"prerelease"`
		PublishedAt string `json:"published_at"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&payload); err != nil {
		return Release{}, errors.New("stable release metadata is invalid")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Release{}, errors.New("stable release metadata contains multiple values")
	}
	if payload.Draft == nil || payload.Prerelease == nil || *payload.Draft || *payload.Prerelease ||
		strings.TrimSpace(payload.PublishedAt) == "" {
		return Release{}, errors.New("stable release endpoint did not return a published stable release")
	}
	version := normalizeStableVersion(payload.TagName)
	if version == "" || payload.TagName != "v"+version ||
		payload.HTMLURL != "https://github.com/kejilion/KPanel/releases/tag/v"+version {
		return Release{}, errors.New("stable release identity is invalid")
	}
	matches := releaseImagePattern.FindAllStringSubmatch(payload.Body, 2)
	if len(matches) != 1 || len(matches[0]) != 2 {
		return Release{}, errors.New("stable release does not contain one official image digest")
	}
	digest := normalizeImageDigest(matches[0][1])
	if digest == "" {
		return Release{}, errors.New("stable release image digest is invalid")
	}
	return Release{Version: version, ImageDigest: digest}, nil
}
