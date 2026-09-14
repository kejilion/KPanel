package selfupdate

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func releaseResponse(request *http.Request, body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func sourceWithResponse(handler roundTripFunc) *GitHubLatestSource {
	return &GitHubLatestSource{client: &http.Client{
		Transport: handler,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}
}

func previewSourceWithResponse(handler roundTripFunc) *GitHubLatestSource {
	source := sourceWithResponse(handler)
	source.channel = ChannelPreview
	return source
}

func validReleaseJSON(version, digest string) string {
	return `{"tag_name":"v` + version + `","html_url":"https://github.com/kejilion/KPanel/releases/tag/v` + version +
		`","body":"### 发布产物与完整性\n\n- 生产镜像：\u0060docker.io/kjlion/kejilion-panel@` + digest +
		`\u0060\n","draft":false,"prerelease":false,"published_at":"2026-09-14T00:00:00Z"}`
}

func TestGitHubLatestSourceAcceptsOnePublishedStableDigest(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	var requestSeen *http.Request
	source := sourceWithResponse(func(request *http.Request) (*http.Response, error) {
		requestSeen = request
		return releaseResponse(request, validReleaseJSON("12.34.56", digest)), nil
	})

	release, err := source.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "12.34.56" || release.ImageDigest != digest {
		t.Fatalf("release = %#v", release)
	}
	if release.ReleaseURL != "https://github.com/kejilion/KPanel/releases/tag/v12.34.56" ||
		release.PublishedAt != "2026-09-14T00:00:00Z" {
		t.Fatalf("release identity = %#v", release)
	}
	if requestSeen == nil || requestSeen.Method != http.MethodGet || requestSeen.URL.String() != githubLatestURL ||
		requestSeen.UserAgent() != "KPanel-Release-Update/2" ||
		requestSeen.Header.Get("X-GitHub-Api-Version") != "2022-11-28" {
		t.Fatalf("unexpected request: %#v", requestSeen)
	}
}

func TestReleaseNotesAreBoundedPlainTextAndKeepUpgradeWarningsSeparate(t *testing.T) {
	body := `### 版本更新内容

#### 新增
- 增加 **KPanel 专用确认框**，详情见 [发布页](https://example.invalid/release)。
#### 变更
- 设置页与应用市场复用 **同一结构**。
#### 修复
- 修复更新内容不可见的问题。
#### 升级注意事项
- 更新期间服务会短暂重启。
- 请先确认关键业务已有备份。

### 发布产物与完整性
- 生产镜像：忽略`
	notes, upgradeNotes := parseReleaseNotes(body)
	if len(notes) != 3 || notes[0] != (ReleaseNote{Kind: "added", Text: "增加 KPanel 专用确认框，详情见 发布页。"}) ||
		notes[1] != (ReleaseNote{Kind: "changed", Text: "设置页与应用市场复用 同一结构。"}) ||
		notes[2].Kind != "fixed" {
		t.Fatalf("notes = %#v", notes)
	}
	if len(upgradeNotes) != 2 || upgradeNotes[0] != "更新期间服务会短暂重启。" {
		t.Fatalf("upgrade notes = %#v", upgradeNotes)
	}
}

func TestGitHubPreviewSourceSelectsHighestStableOrRCRelease(t *testing.T) {
	stableDigest := "sha256:" + strings.Repeat("a", 64)
	previewDigest := "sha256:" + strings.Repeat("b", 64)
	payload := `[` +
		validReleaseJSON("1.8.0", stableDigest) + `,` +
		strings.ReplaceAll(
			strings.ReplaceAll(validReleaseJSON("1.9.0-rc.2", previewDigest), `"prerelease":false`, `"prerelease":true`),
			`生产镜像`, `预览镜像`,
		) + `,` +
		strings.ReplaceAll(validReleaseJSON("9.0.0-beta.1", previewDigest), `"prerelease":false`, `"prerelease":true`) +
		`]`
	var requestSeen *http.Request
	source := previewSourceWithResponse(func(request *http.Request) (*http.Response, error) {
		requestSeen = request
		return releaseResponse(request, payload), nil
	})

	release, err := source.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "1.9.0-rc.2" || release.ImageDigest != previewDigest {
		t.Fatalf("release = %#v", release)
	}
	if requestSeen == nil || requestSeen.URL.String() != githubPreviewURL {
		t.Fatalf("unexpected request: %#v", requestSeen)
	}
}

func TestGitHubPreviewSourceAllowsFinalStableToSupersedeRC(t *testing.T) {
	previewDigest := "sha256:" + strings.Repeat("a", 64)
	stableDigest := "sha256:" + strings.Repeat("b", 64)
	preview := strings.ReplaceAll(
		strings.ReplaceAll(validReleaseJSON("2.0.0-rc.9", previewDigest), `"prerelease":false`, `"prerelease":true`),
		`生产镜像`, `预览镜像`,
	)
	payload := `[` + preview + `,` + validReleaseJSON("2.0.0", stableDigest) + `]`
	source := previewSourceWithResponse(func(request *http.Request) (*http.Response, error) {
		return releaseResponse(request, payload), nil
	})

	release, err := source.Latest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "2.0.0" || release.ImageDigest != stableDigest {
		t.Fatalf("release = %#v", release)
	}
}

func TestGitHubPreviewSourceRejectsMismatchedOrAmbiguousMetadata(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	rc := strings.ReplaceAll(validReleaseJSON("1.2.0-rc.1", digest), `生产镜像`, `预览镜像`)
	tests := []string{
		`[` + rc + `]`,
		`[` + strings.ReplaceAll(validReleaseJSON("1.2.0", digest), `"prerelease":false`, `"prerelease":true`) + `]`,
		`[` + validReleaseJSON("1.2.0", digest) + `,` + validReleaseJSON("1.2.0", digest) + `]`,
	}
	for index, payload := range tests {
		source := previewSourceWithResponse(func(request *http.Request) (*http.Response, error) {
			return releaseResponse(request, payload), nil
		})
		if _, err := source.Latest(context.Background()); err == nil {
			t.Fatalf("unsafe preview metadata %d was accepted", index)
		}
	}
}

func TestGitHubLatestSourceRejectsUntrustedOrMutableMetadata(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	valid := validReleaseJSON("1.2.3", digest)
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "redirect", status: http.StatusFound, body: valid},
		{name: "prerelease", status: http.StatusOK, body: strings.Replace(valid, `"prerelease":false`, `"prerelease":true`, 1)},
		{name: "draft", status: http.StatusOK, body: strings.Replace(valid, `"draft":false`, `"draft":true`, 1)},
		{name: "missing stable flags", status: http.StatusOK, body: `{"tag_name":"v1.2.3"}`},
		{name: "prerelease tag", status: http.StatusOK, body: strings.Replace(valid, "v1.2.3", "v1.2.3-rc.1", 2)},
		{name: "wrong repository", status: http.StatusOK, body: strings.Replace(valid, "github.com/kejilion/KPanel", "github.com/other/KPanel", 1)},
		{name: "tag image", status: http.StatusOK, body: strings.Replace(valid, "@"+digest, ":1.2.3", 1)},
		{name: "duplicate digest", status: http.StatusOK, body: strings.Replace(valid, `\n","draft"`, `\n- 生产镜像：\u0060docker.io/kjlion/kejilion-panel@`+digest+`\u0060\n","draft"`, 1)},
		{name: "trailing value", status: http.StatusOK, body: valid + `{}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := sourceWithResponse(func(request *http.Request) (*http.Response, error) {
				response := releaseResponse(request, test.body)
				response.StatusCode = test.status
				return response, nil
			})
			if _, err := source.Latest(context.Background()); err == nil {
				t.Fatal("unsafe release metadata was accepted")
			}
		})
	}
}

func TestGitHubLatestSourceEnforcesResponseLimitAndTransportFailure(t *testing.T) {
	oversized := sourceWithResponse(func(request *http.Request) (*http.Response, error) {
		return releaseResponse(request, strings.Repeat("x", maxReleaseResponse+1)), nil
	})
	if _, err := oversized.Latest(context.Background()); err == nil {
		t.Fatal("oversized response was accepted")
	}

	want := errors.New("offline")
	offline := sourceWithResponse(func(*http.Request) (*http.Response, error) { return nil, want })
	if _, err := offline.Latest(context.Background()); !errors.Is(err, want) {
		t.Fatalf("error = %v, want transport failure", err)
	}
}
