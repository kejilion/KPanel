package notification

import (
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

var channelSecretPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]{8,512}$`)

// DetectProvider validates a channel credential and returns the provider it
// belongs to. Robot webhook destinations are deliberately restricted to the
// official HTTPS hosts so this feature cannot become a generic SSRF proxy.
func DetectProvider(value string) (Provider, bool) {
	if value == "" || strings.TrimSpace(value) != value || len(value) > MaxChannelCredentialBytes ||
		!utf8.ValidString(value) || strings.ContainsAny(value, "\r\n\t\x00") {
		return "", false
	}
	if ValidBotToken(value) {
		return ProviderTelegram, true
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Fragment != "" ||
		parsed.Opaque != "" || parsed.RawPath != "" || parsed.ForceQuery || parsed.Port() != "" {
		return "", false
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "open.feishu.cn":
		const prefix = "/open-apis/bot/v2/hook/"
		secret := strings.TrimPrefix(parsed.Path, prefix)
		if parsed.RawQuery == "" && secret != parsed.Path && !strings.Contains(secret, "/") && validChannelSecret(secret) {
			return ProviderFeishu, true
		}
	case "oapi.dingtalk.com":
		if parsed.Path == "/robot/send" && validSingleQuerySecret(parsed, "access_token") {
			return ProviderDingTalk, true
		}
	case "qyapi.weixin.qq.com":
		if parsed.Path == "/cgi-bin/webhook/send" && validSingleQuerySecret(parsed, "key") {
			return ProviderWeCom, true
		}
	}
	return "", false
}

func ValidChannelCredential(provider Provider, value string) bool {
	detected, ok := DetectProvider(value)
	return ok && detected == provider
}

func validSingleQuerySecret(value *url.URL, name string) bool {
	query, err := url.ParseQuery(value.RawQuery)
	if err != nil || len(query) != 1 {
		return false
	}
	secrets, ok := query[name]
	return ok && len(secrets) == 1 && validChannelSecret(secrets[0])
}

func validChannelSecret(value string) bool {
	return channelSecretPattern.MatchString(value)
}

func providerName(provider Provider, locale string) string {
	switch provider {
	case ProviderFeishu:
		if locale == "zh-TW" {
			return "飛書"
		}
		if locale == "en-US" {
			return "Feishu"
		}
		return "飞书"
	case ProviderDingTalk:
		if locale == "zh-TW" {
			return "釘釘"
		}
		if locale != "en-US" {
			return "钉钉"
		}
		return "DingTalk"
	case ProviderWeCom:
		if locale == "en-US" {
			return "WeCom"
		}
		if locale == "zh-TW" {
			return "企業微信"
		}
		return "企业微信"
	default:
		return "Telegram"
	}
}
