//go:build !windows

package systeminfo

import (
	"context"
	"github.com/kejilion/kejilion-panel/internal/contract"
)

func (*Collector) collectPlatformRuntime(context.Context) (contract.SystemSummary, error, bool) {
	return contract.SystemSummary{}, nil, false
}
