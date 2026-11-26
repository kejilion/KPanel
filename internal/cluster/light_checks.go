package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/kejilion/kejilion-panel/internal/monitoring"
)

var (
	// ErrLightChecksUnsupported means the node never advertised that it can
	// receive service checks; sending it the command would stall its relay.
	ErrLightChecksUnsupported = errors.New("light node cannot receive service checks")
	// ErrLightChecksUnavailable means the node's history relay is not connected
	// or the request failed in transit. A later attempt may succeed.
	ErrLightChecksUnavailable = errors.New("light node service checks are unavailable")
	// ErrLightChecksRejected means the node refused the list it was sent.
	ErrLightChecksRejected = errors.New("light node rejected the service checks")
)

const (
	// A node reports about once a minute. Without a fresh report the center
	// cannot tell an upgraded broker from one rolled back to an older build.
	lightChecksCapabilityTTL = 10 * time.Minute
	lightChecksTimeout       = 45 * time.Second
	maxLightChecksResponse   = 64 << 10
	maxLightReportFeatures   = 8
)

// LightChecksReadiness says whether a center may send service checks to a
// light node now. Both must hold: an unsupported node must never be sent the
// command, and a disconnected one would only time out.
type LightChecksReadiness struct {
	Supported bool
	Connected bool
}

type lightChecksCapabilities struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func (c *lightChecksCapabilities) record(id string, supported bool, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !supported {
		delete(c.seen, id)
		return
	}
	if c.seen == nil {
		c.seen = make(map[string]time.Time)
	}
	c.seen[id] = now
}

func (c *lightChecksCapabilities) supported(id string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	seen, ok := c.seen[id]
	return ok && now.Sub(seen) <= lightChecksCapabilityTTL
}

func (c *lightChecksCapabilities) forget(id string) {
	c.mu.Lock()
	delete(c.seen, id)
	c.mu.Unlock()
}

// validLightReportFeatures bounds the optional feature list a node sends with
// its report. Unknown names are allowed so a newer node can report to this
// center; only their size and characters are checked.
func validLightReportFeatures(features []string) bool {
	if len(features) > maxLightReportFeatures {
		return false
	}
	for _, feature := range features {
		if feature == "" || len(feature) > 64 {
			return false
		}
		for _, character := range feature {
			if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
				return false
			}
		}
	}
	return true
}

func hasLightReportFeature(features []string, want string) bool {
	for _, feature := range features {
		if feature == want {
			return true
		}
	}
	return false
}

func (s *Service) LightChecksReadiness(id string) LightChecksReadiness {
	record, err := s.light.Host(id)
	if err != nil || !lightNodePlatformSupported(record) {
		return LightChecksReadiness{}
	}
	return LightChecksReadiness{
		Supported: s.lightChecks.supported(id, s.now().UTC()),
		Connected: s.lightHistory != nil && s.lightHistory.available(id),
	}
}

// LightNodeChecks reads the service checks a light node currently stores.
func (s *Service) LightNodeChecks(ctx context.Context, id string) (monitoring.CheckSnapshot, error) {
	return s.lightChecksRequest(ctx, id, http.MethodGet, nil)
}

// ReplaceLightNodeChecks replaces a light node's service checks. The node
// applies its own optimistic concurrency check against input.ExpectedResourceVersion.
func (s *Service) ReplaceLightNodeChecks(ctx context.Context, id string, input monitoring.ReplaceChecksInput) (monitoring.CheckSnapshot, error) {
	body, err := json.Marshal(input)
	if err != nil || len(body) > monitoring.MaxCheckUpdateBytes {
		return monitoring.CheckSnapshot{}, ErrLightChecksRejected
	}
	return s.lightChecksRequest(ctx, id, http.MethodPut, body)
}

func (s *Service) lightChecksRequest(ctx context.Context, id, method string, body []byte) (monitoring.CheckSnapshot, error) {
	var result monitoring.CheckSnapshot
	if !validID(id) {
		return result, ErrNotFound
	}
	if _, err := s.light.Host(id); err != nil {
		return result, ErrNotFound
	}
	readiness := s.LightChecksReadiness(id)
	if !readiness.Supported {
		return result, ErrLightChecksUnsupported
	}
	if !readiness.Connected {
		return result, ErrLightChecksUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, lightChecksTimeout)
	defer cancel()
	request := LightFileRequest{Method: method, Path: ChecksPath, Body: http.NoBody}
	if body != nil {
		request.Body = bytes.NewReader(body)
		request.BodyLength = int64(len(body))
		request.Headers = map[string]string{"Content-Type": "application/json"}
	}
	response, err := s.lightHistory.Open(ctx, id, request)
	if err != nil {
		return result, ErrLightChecksUnavailable
	}
	defer response.Body.Close()
	stop := context.AfterFunc(ctx, func() { _ = response.Body.Close() })
	defer stop()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusConflict:
		return result, monitoring.ErrChecksConflict
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return result, ErrLightChecksRejected
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		return result, ErrLightChecksUnsupported
	default:
		return result, ErrLightChecksUnavailable
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, maxLightChecksResponse+1))
	if err != nil || len(content) > maxLightChecksResponse || ctx.Err() != nil {
		return result, ErrLightChecksUnavailable
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&result) != nil || !errors.Is(decoder.Decode(&extra), io.EOF) || monitoring.ValidateCheckSnapshot(result) != nil {
		return monitoring.CheckSnapshot{}, ErrLightChecksUnavailable
	}
	// Deletion during an in-flight request must not report a removed node.
	if _, err := s.light.Host(id); err != nil {
		return monitoring.CheckSnapshot{}, ErrNotFound
	}
	return result, nil
}
