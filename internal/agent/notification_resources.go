package agent

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/kejilion/kejilion-panel/internal/dockerx"
	"github.com/kejilion/kejilion-panel/internal/sites"
)

var notificationResourceGate = make(chan struct{}, 1)

func (s *Server) notificationResources(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDFrom(w)
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		writeProblem(w, requestID, http.StatusBadRequest, "invalid_notification_resources", "通知资源请求无效", "")
		return
	}
	select {
	case notificationResourceGate <- struct{}{}:
		defer func() { <-notificationResourceGate }()
	default:
		writeProblem(w, requestID, http.StatusTooManyRequests, "notification_resources_busy", "通知资源读取繁忙", "")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	result := struct {
		Certificates      []sites.NotificationCertificate `json:"certificates"`
		Containers        []dockerx.NotificationContainer `json:"containers"`
		CertificateStatus string                          `json:"certificateStatus"`
		ContainerStatus   string                          `json:"containerStatus"`
		ObservedAt        time.Time                       `json:"observedAt"`
	}{Certificates: []sites.NotificationCertificate{}, Containers: []dockerx.NotificationContainer{}, CertificateStatus: "unknown", ContainerStatus: "unknown"}
	if s.sites != nil {
		items, err := s.sites.NotificationResources(ctx)
		if err == nil {
			result.Certificates = items
			result.CertificateStatus = "ready"
		} else if errors.Is(err, sites.ErrNotificationResourceLimit) {
			result.CertificateStatus = "limited"
		}
	}
	if s.docker != nil {
		items, err := s.docker.NotificationResources(ctx)
		if err == nil {
			result.Containers = items
			result.ContainerStatus = "ready"
		} else if errors.Is(err, dockerx.ErrNotificationResourceLimit) {
			result.ContainerStatus = "limited"
		}
	}
	result.ObservedAt = s.now().UTC()
	writeJSON(w, http.StatusOK, result)
}
