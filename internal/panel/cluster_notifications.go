package panel

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kejilion/kejilion-panel/internal/notification"
)

const clusterNotificationsPath = "/api/v1/cluster/notifications"

func (s *Server) handleClusterNotifications(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawPath != "" || r.URL.RawQuery != "" {
		s.writeProblem(w, r, http.StatusBadRequest, "invalid_cluster_notification_request", "Invalid cluster notification request", "")
		return
	}
	switch {
	case r.URL.Path == clusterNotificationsPath && r.Method == http.MethodGet:
		if _, _, ok := s.requireSession(w, r); !ok {
			return
		}
		if s.notifications == nil {
			s.writeProblem(w, r, http.StatusServiceUnavailable, "cluster_notifications_unavailable", "Cluster notifications unavailable", "")
			return
		}
		s.writeJSON(w, http.StatusOK, s.notifications.Snapshot())
	case r.URL.Path == clusterNotificationsPath && r.Method == http.MethodPut:
		s.handleClusterNotificationsUpdate(w, r)
	case r.URL.Path == clusterNotificationsPath+"/discover" && r.Method == http.MethodPost:
		s.handleClusterNotificationsDiscover(w, r)
	case r.URL.Path == clusterNotificationsPath+"/test" && r.Method == http.MethodPost:
		s.handleClusterNotificationsTest(w, r)
	default:
		s.writeProblem(w, r, http.StatusNotFound, "route_not_found", "Route not found", "")
	}
}

func (s *Server) handleClusterNotificationsUpdate(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireClusterMutation(w, r)
	if !ok {
		return
	}
	var input notification.UpdateInput
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	change := map[string]any{
		"enabled": input.Enabled, "rules": input.Rules, "provider": auditNotificationProvider(input.Provider),
		"credentialProvided": strings.TrimSpace(input.ChannelCredential) != "" || strings.TrimSpace(input.TelegramBotToken) != "",
	}
	target := auditNotificationProvider(input.Provider)
	if err := s.audit(r, session.User.ID, "cluster.notifications.update", "cluster-notifications", target, "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	if s.notifications == nil {
		_ = s.audit(r, session.User.ID, "cluster.notifications.update", "cluster-notifications", target, "failure", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "cluster_notifications_unavailable", "Cluster notifications unavailable", "")
		return
	}
	snapshot, err := s.notifications.Configure(r.Context(), input)
	if err != nil {
		_ = s.audit(r, session.User.ID, "cluster.notifications.update", "cluster-notifications", target, "failure", change)
		s.writeNotificationError(w, r, err)
		return
	}
	_ = s.audit(r, session.User.ID, "cluster.notifications.update", "cluster-notifications", target, "success", change)
	s.writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleClusterNotificationsDiscover(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireClusterMutation(w, r)
	if !ok {
		return
	}
	var input struct {
		ExpectedResourceVersion string `json:"expectedResourceVersion"`
	}
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	change := map[string]any{"expectedResourceVersion": input.ExpectedResourceVersion}
	if err := s.audit(r, session.User.ID, "cluster.notifications.discover", "cluster-notifications", "telegram", "intent", change); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	if s.notifications == nil {
		_ = s.audit(r, session.User.ID, "cluster.notifications.discover", "cluster-notifications", "telegram", "failure", change)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "cluster_notifications_unavailable", "Cluster notifications unavailable", "")
		return
	}
	snapshot, err := s.notifications.Discover(r.Context(), input.ExpectedResourceVersion)
	if err != nil {
		_ = s.audit(r, session.User.ID, "cluster.notifications.discover", "cluster-notifications", "telegram", "failure", change)
		s.writeNotificationError(w, r, err)
		return
	}
	_ = s.audit(r, session.User.ID, "cluster.notifications.discover", "cluster-notifications", "telegram", "success", change)
	s.writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) handleClusterNotificationsTest(w http.ResponseWriter, r *http.Request) {
	session, ok := s.requireClusterMutation(w, r)
	if !ok {
		return
	}
	var input struct{}
	if err := s.decodeJSON(w, r, &input); err != nil {
		return
	}
	if err := s.audit(r, session.User.ID, "cluster.notifications.test", "cluster-notifications", "channel", "intent", nil); err != nil {
		s.writeProblem(w, r, http.StatusServiceUnavailable, "audit_unavailable", "Audit storage unavailable", "")
		return
	}
	if s.notifications == nil {
		_ = s.audit(r, session.User.ID, "cluster.notifications.test", "cluster-notifications", "channel", "failure", nil)
		s.writeProblem(w, r, http.StatusServiceUnavailable, "cluster_notifications_unavailable", "Cluster notifications unavailable", "")
		return
	}
	snapshot, err := s.notifications.Test(r.Context())
	if err != nil {
		_ = s.audit(r, session.User.ID, "cluster.notifications.test", "cluster-notifications", "channel", "failure", nil)
		s.writeNotificationError(w, r, err)
		return
	}
	_ = s.audit(r, session.User.ID, "cluster.notifications.test", "cluster-notifications", "channel", "success", nil)
	s.writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) writeNotificationError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusUnprocessableEntity
	code := "cluster_notifications_failed"
	title := "Cluster notifications failed"
	var validation *notification.ValidationError
	if errors.As(err, &validation) {
		s.writeValidationProblem(w, r, validation.Field, validation.Message)
		return
	}
	switch {
	case errors.Is(err, notification.ErrConflict):
		status, code, title = http.StatusConflict, "cluster_notifications_changed", "Cluster notification settings changed"
	case errors.Is(err, notification.ErrTokenRequired):
		code, title = "cluster_notifications_token_required", "Telegram bot token is required"
	case errors.Is(err, notification.ErrCredentialRequired):
		code, title = "cluster_notifications_credential_required", "Notification channel credential is required"
	case errors.Is(err, notification.ErrNotConfigured):
		code, title = "cluster_notifications_not_configured", "Telegram notifications are not configured"
	case errors.Is(err, notification.ErrChannelNotConfigured):
		code, title = "cluster_notifications_channel_not_configured", "Notification channel is not configured"
	case errors.Is(err, notification.ErrNotReady):
		code, title = "cluster_notifications_not_ready", "Telegram notifications are not ready"
	case errors.Is(err, notification.ErrChannelNotReady):
		code, title = "cluster_notifications_channel_not_ready", "Notification channel is not ready"
	case errors.Is(err, notification.ErrProviderMismatch):
		code, title = "cluster_notifications_provider_mismatch", "Notification credential does not match the selected channel"
	case errors.Is(err, notification.ErrUnsupportedProvider):
		code, title = "cluster_notifications_unsupported_provider", "Notification channel is unsupported"
	case errors.Is(err, notification.ErrDiscoverUnsupported):
		code, title = "cluster_notifications_discover_unsupported", "Notification channel does not support chat discovery"
	case errors.Is(err, notification.ErrInvalidCredential):
		code, title = "cluster_notifications_invalid_credential", "Notification channel credential is invalid"
	case errors.Is(err, notification.ErrChatNotFound):
		code, title = "cluster_notifications_chat_not_found", "Telegram private chat was not found"
	case errors.Is(err, notification.ErrTelegramInvalidToken):
		code, title = "cluster_notifications_invalid_token", "Telegram bot token is invalid"
	case errors.Is(err, notification.ErrTelegramWebhookActive):
		code, title = "cluster_notifications_webhook_active", "Telegram webhook is active"
	case errors.Is(err, notification.ErrTelegramUnavailable):
		status, code, title = http.StatusBadGateway, "cluster_notifications_telegram_unavailable", "Telegram Bot API is unavailable"
	case errors.Is(err, notification.ErrChannelUnavailable):
		status, code, title = http.StatusBadGateway, "cluster_notifications_channel_unavailable", "Notification channel is unavailable"
	default:
		var typed *notification.Error
		if errors.As(err, &typed) {
			switch typed.Code {
			case "credential_store_unavailable_after_delivery", "credential_rollback_failed_after_delivery",
				"state_store_unavailable_after_delivery":
				status, code, title = http.StatusServiceUnavailable, "cluster_notifications_save_failed_after_delivery", "Validation message delivered but notification settings were not saved"
			case "credential_store_unavailable", "credential_rollback_failed", "state_store_unavailable", "credential_file_unavailable",
				"token_store_unavailable", "token_file_unavailable":
				status, code, title = http.StatusServiceUnavailable, "cluster_notifications_unavailable", "Cluster notifications unavailable"
			case "invalid_response", "api_error", "rate_limited", "unavailable":
				status, code, title = http.StatusBadGateway, "cluster_notifications_channel_unavailable", "Notification channel is unavailable"
			}
		}
	}
	s.writeProblem(w, r, status, code, title, "")
}

func auditNotificationProvider(provider notification.Provider) string {
	switch provider {
	case notification.ProviderTelegram, notification.ProviderFeishu, notification.ProviderDingTalk, notification.ProviderWeCom:
		return string(provider)
	case "":
		return "unchanged"
	default:
		return "unknown"
	}
}
