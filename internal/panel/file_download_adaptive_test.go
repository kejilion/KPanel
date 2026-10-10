package panel

import (
	"context"
	"io"
	"net/http"

	"github.com/kejilion/kejilion-panel/internal/remotedownload"
)

func (s *Server) enableAdaptiveDownloadExperiment(client *remotedownload.Client) {
	budget := make(chan struct{}, 2)
	s.remoteDownloadAdaptive = func(ctx context.Context, raw string, response *http.Response) io.ReadCloser {
		return client.Adaptive(ctx, raw, response, budget)
	}
}
