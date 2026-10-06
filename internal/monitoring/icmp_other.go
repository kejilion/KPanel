package monitoring

import (
	"context"
	"time"
)

func platformICMPProbe(context.Context, string) (time.Duration, error, bool) { return 0, nil, false }
