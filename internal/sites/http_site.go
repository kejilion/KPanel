package sites

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

const httpSiteProtocol = `KPANEL_WEB_HTTP_PROTOCOL_VERSION="1"`

func normalizeScriptSiteAddress(input ScriptSiteInput) (ScriptSiteInput, int, error) {
	host, port, err := contract.ParseSiteAddress(input.PrimaryDomain)
	if err != nil {
		return input, 0, fmt.Errorf("%w: invalid domain or IPv4:port", ErrInvalidInput)
	}
	if port != 0 && (input.Certificate != "" || input.PrivateKey != "") {
		return input, 0, fmt.Errorf("%w: HTTP sites do not use certificates", ErrInvalidInput)
	}
	if port != 0 {
		if _, err := findTrustedKejilionScript(httpSiteProtocol); err != nil {
			return input, 0, fmt.Errorf("%w: HTTP port adapter requires an updated kejilion.sh: %v", ErrUnavailable, err)
		}
	}
	input.PrimaryDomain = host
	return input, port, nil
}

func normalizeScriptDomain(raw string) (string, error) {
	if ip := net.ParseIP(raw); ip != nil && ip.To4() != nil {
		return ip.String(), nil
	}
	return normalizeFQDN(raw)
}

// The legacy template/validator is frozen while script generation is migrated.
// Reuse all of its field and alias checks with an unused domain, then restore
// the already validated IPv4 identity before any collision check or write.
func normalizeEditableSiteInput(input SiteInput) (managedSpec, error) {
	ip := net.ParseIP(input.PrimaryDomain)
	if ip == nil || ip.To4() == nil {
		return normalizeSiteInput(input)
	}
	aliases := make(map[string]bool, len(input.Aliases))
	for _, alias := range input.Aliases {
		aliases[strings.ToLower(alias)] = true
	}
	for index := 0; ; index++ {
		input.PrimaryDomain = "ipv4-" + strconv.Itoa(index) + ".invalid"
		if !aliases[input.PrimaryDomain] {
			break
		}
	}
	spec, err := normalizeSiteInput(input)
	if err != nil {
		return managedSpec{}, err
	}
	spec.Primary = ip.String()
	return spec, nil
}

func withHTTPPort(invocation scriptSiteInvocation, port int) scriptSiteInvocation {
	if port != 0 {
		invocation.arguments = append([]string{"http-site", strconv.Itoa(port)}, invocation.arguments...)
		invocation.required = append(invocation.required, httpSiteProtocol)
	}
	return invocation
}
