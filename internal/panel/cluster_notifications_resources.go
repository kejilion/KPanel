package panel

import (
	"context"
	"encoding/json"
	"github.com/kejilion/kejilion-panel/internal/notification"
	"net/http"
)

type notificationResourceSource struct{ agent *AgentClient }

func (source notificationResourceSource) Resources(ctx context.Context) notification.ResourceSnapshot {
	var snapshot notification.ResourceSnapshot
	response, err := source.agent.Get(ctx, "/v1/notification-resources", "", newRequestID())
	if err != nil || response.StatusCode != http.StatusOK || len(response.Body) > 256<<10 || json.Unmarshal(response.Body, &snapshot) != nil {
		return notification.ResourceSnapshot{CertificateStatus: "unknown", ContainerStatus: "unknown"}
	}
	return snapshot
}
