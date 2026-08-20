package ai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/netguard"
)

type ProviderInput struct {
	Name             string           `json:"name"`
	Protocol         ProviderProtocol `json:"protocol"`
	APIMode          OpenAIAPIMode    `json:"apiMode,omitempty"`
	BaseURL          string           `json:"baseUrl"`
	EndpointScope    EndpointScope    `json:"endpointScope"`
	APIKey           *string          `json:"apiKey,omitempty"`
	Enabled          bool             `json:"enabled"`
	PrivateConfirmed bool             `json:"privateConfirmed,omitempty"`
	ExpectedVersion  int64            `json:"expectedVersion,omitempty"`
}

type ProviderService struct {
	store   *Store
	secrets *SecretBox
}

func NewProviderService(store *Store, secrets *SecretBox) (*ProviderService, error) {
	if store == nil || secrets == nil {
		return nil, errors.New("AI provider storage and secret box are required")
	}
	return &ProviderService{store: store, secrets: secrets}, nil
}

func (s *ProviderService) Save(ctx context.Context, id string, input ProviderInput) (Provider, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 80 {
		return Provider{}, errors.New("provider name must be between 1 and 80 characters")
	}
	baseURL, err := ValidateProviderURL(input.BaseURL, input.EndpointScope)
	if err != nil {
		return Provider{}, err
	}
	if input.EndpointScope == EndpointPrivate && !input.PrivateConfirmed {
		return Provider{}, errors.New("private provider endpoint requires explicit confirmation")
	}
	if input.Protocol != ProtocolOpenAICompatible && input.Protocol != ProtocolAnthropic && input.Protocol != ProtocolGemini {
		return Provider{}, errors.New("unsupported provider protocol")
	}
	if input.Protocol == ProtocolOpenAICompatible {
		if input.APIMode == "" {
			input.APIMode = OpenAIChatCompletions
		}
		if input.APIMode != OpenAIChatCompletions && input.APIMode != OpenAIResponses {
			return Provider{}, errors.New("apiMode must be chat_completions or responses")
		}
	} else {
		input.APIMode = ""
	}
	provider := Provider{ID: id, Name: input.Name, Protocol: input.Protocol, APIMode: input.APIMode, BaseURL: baseURL, EndpointScope: input.EndpointScope, Enabled: input.Enabled}
	if id != "" {
		active, err := s.store.ProviderHasActiveRun(ctx, id)
		if err != nil {
			return Provider{}, err
		}
		if active {
			return Provider{}, ErrBusy
		}
		current, err := s.store.Provider(ctx, id)
		if err != nil {
			return Provider{}, err
		}
		provider.EncryptedKey, provider.APIKeyHint, provider.CreatedAt = current.EncryptedKey, current.APIKeyHint, current.CreatedAt
	}
	if input.APIKey != nil {
		key := strings.TrimSpace(*input.APIKey)
		if len(key) > 4096 {
			return Provider{}, errors.New("provider API key is too long")
		}
		if key == "" {
			provider.EncryptedKey, provider.APIKeyHint = nil, ""
		} else {
			if provider.ID == "" {
				provider.ID = newID("prv")
			}
			provider.EncryptedKey, err = s.secrets.Seal(provider.ID, key)
			if err != nil {
				return Provider{}, err
			}
			provider.APIKeyHint = keyHint(key)
		}
	}
	return s.store.SaveProvider(ctx, provider, input.ExpectedVersion)
}

func (s *ProviderService) APIKey(provider Provider) (string, error) {
	if len(provider.EncryptedKey) == 0 {
		return "", nil
	}
	return s.secrets.Open(provider.ID, provider.EncryptedKey)
}

func ValidateProviderURL(raw string, scope EndpointScope) (string, error) {
	if len(strings.TrimSpace(raw)) > 2048 {
		return "", errors.New("provider baseUrl is too long")
	}
	if scope != EndpointPublic && scope != EndpointPrivate {
		return "", errors.New("endpointScope must be public or private")
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.Hostname() == "" {
		return "", errors.New("provider baseUrl must be an absolute HTTP(S) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" || parsed.ForceQuery {
		return "", errors.New("provider baseUrl must not contain userinfo, query, or fragment")
	}
	if parsed.Scheme != "https" && !(scope == EndpointPrivate && parsed.Scheme == "http") {
		return "", errors.New("public providers require HTTPS; HTTP is allowed only for private endpoints")
	}
	if strings.ContainsAny(parsed.Host, "\r\n\t ") {
		return "", errors.New("provider baseUrl contains invalid host characters")
	}
	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return "", errors.New("provider baseUrl port is invalid")
		}
	}
	if scope == EndpointPublic && isLiteralBlocked(parsed.Hostname()) {
		return "", errors.New("public provider resolves to a non-public address")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

func ValidateResolvedAddresses(scope EndpointScope, addresses []net.IPAddr) error {
	if scope == EndpointPrivate {
		return nil
	}
	if len(addresses) == 0 {
		return errors.New("provider hostname did not resolve")
	}
	for _, address := range addresses {
		if isBlockedIP(address.IP) {
			return fmt.Errorf("provider hostname resolved to non-public address %s", address.IP)
		}
	}
	return nil
}

func isLiteralBlocked(host string) bool {
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && isBlockedIP(ip)
}

// isBlockedIP refuses every address the IANA special-purpose registry marks
// "Globally Reachable = False", via the guard shared with the desktop
// browser's egress (internal/netguard). A provider baseUrl is operator-typed
// and then dialed by paneld, so this is the SSRF boundary for the AI
// subsystem; EndpointPrivate providers bypass it by explicit opt-in, which is
// what ValidateResolvedAddresses checks before it gets here.
func isBlockedIP(ip net.IP) bool {
	return netguard.Blocked(ip, false)
}

func keyHint(value string) string {
	runes := []rune(value)
	if len(runes) <= 4 {
		return string(runes)
	}
	return string(runes[len(runes)-4:])
}
