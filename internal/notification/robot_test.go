package notification

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type robotRoundTripFunc func(*http.Request) (*http.Response, error)

func (f robotRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestDetectProviderAcceptsOnlyOfficialRobotWebhooks(t *testing.T) {
	valid := map[string]Provider{
		"https://open.feishu.cn/open-apis/bot/v2/hook/12345678-abcd":    ProviderFeishu,
		"https://oapi.dingtalk.com/robot/send?access_token=abc12345":    ProviderDingTalk,
		"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc12345": ProviderWeCom,
		testBotToken: ProviderTelegram,
	}
	for credential, expected := range valid {
		provider, ok := DetectProvider(credential)
		if !ok || provider != expected || !ValidChannelCredential(expected, credential) {
			t.Fatalf("DetectProvider(%q) = %q, %v; want %q, true", credential, provider, ok, expected)
		}
	}

	invalid := []string{
		"http://open.feishu.cn/open-apis/bot/v2/hook/12345678",
		"https://open.feishu.cn.evil.example/open-apis/bot/v2/hook/12345678",
		"https://user@open.feishu.cn/open-apis/bot/v2/hook/12345678",
		"https://open.feishu.cn:443/open-apis/bot/v2/hook/12345678",
		"https://open.feishu.cn/open-apis/bot/v2/hook/12345678?debug=1",
		"https://oapi.dingtalk.com/robot/send?access_token=abc12345&extra=1",
		"https://oapi.dingtalk.com/robot/send?access_token=abc12345#fragment",
		"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=short",
		" https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc12345",
	}
	for _, credential := range invalid {
		if provider, ok := DetectProvider(credential); ok {
			t.Fatalf("DetectProvider(%q) = %q, true", credential, provider)
		}
	}
}

func TestRobotClientSendsProviderSpecificTextPayload(t *testing.T) {
	tests := []struct {
		provider   Provider
		credential string
		response   string
		wantBody   string
	}{
		{ProviderFeishu, "https://open.feishu.cn/open-apis/bot/v2/hook/12345678", `{"code":0}`, `"msg_type":"text"`},
		{ProviderDingTalk, "https://oapi.dingtalk.com/robot/send?access_token=abc12345", `{"errcode":0}`, `"msgtype":"text"`},
		{ProviderWeCom, "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=abc12345", `{"errcode":0}`, `"msgtype":"text"`},
	}
	for _, test := range tests {
		t.Run(string(test.provider), func(t *testing.T) {
			client := newRobotClientForTest(&http.Client{Transport: robotRoundTripFunc(func(request *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatalf("read request: %v", err)
				}
				if request.Method != http.MethodPost || request.URL.String() != test.credential {
					t.Fatalf("request = %s %s", request.Method, request.URL)
				}
				if request.Header.Get("Content-Type") != "application/json" || !strings.Contains(string(body), test.wantBody) || !strings.Contains(string(body), "hello KPanel") {
					t.Fatalf("request headers/body = %#v / %s", request.Header, body)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(test.response)),
					Request:    request,
				}, nil
			})})
			if err := client.SendMessage(context.Background(), test.provider, test.credential, "hello KPanel"); err != nil {
				t.Fatalf("SendMessage() error = %v", err)
			}
		})
	}
}

func TestRobotClientBoundsResponsesAndNeverLeaksCredential(t *testing.T) {
	const credential = "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=supersecret123"
	client := newRobotClientForTest(&http.Client{Transport: robotRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", int(robotResponseBytes)+1))),
			Request:    request,
		}, nil
	})})
	err := client.SendMessage(context.Background(), ProviderWeCom, credential, "hello")
	if err == nil || !errors.Is(err, ErrChannelUnavailable) || !strings.Contains(err.Error(), "invalid_response") {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if strings.Contains(err.Error(), "supersecret123") || strings.Contains(err.Error(), credential) {
		t.Fatalf("SendMessage() leaked credential: %v", err)
	}
}

func TestRobotClientDisablesRedirects(t *testing.T) {
	client := NewRobotClient()
	request, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.client.CheckRedirect(request, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("CheckRedirect() error = %v", err)
	}
}
