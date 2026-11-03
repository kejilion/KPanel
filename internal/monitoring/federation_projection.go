package monitoring

import (
	"net/url"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/contract"
)

// ProjectFederatedHistory removes configured endpoint details that are useful
// to a local administrator but may contain credentials in URL query strings.
// Local history remains unchanged; only federation responses use this view.
func ProjectFederatedHistory(value *contract.MonitoringHistory) {
	if value == nil {
		return
	}
	for index := range value.OperatorLatency {
		series := &value.OperatorLatency[index]
		target := series.Target
		if target == "" {
			target = series.Address
		}
		projected := federatedCheckTarget(target)
		series.Target = projected
		series.Address = projected
	}
}

func federatedCheckTarget(target string) string {
	if parsed, err := url.Parse(target); err == nil &&
		(parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" {
		return parsed.Host
	}
	if len(target) > 253 || strings.ContainsAny(target, "/?#@\\\"<>&") {
		return "configured-target"
	}
	return target
}
