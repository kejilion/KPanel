package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/kejilion/kejilion-panel/internal/cluster"
)

const (
	panelLoginRuleKey = "panel-login"
	// A sign-in must never wait for notification storage or a channel. The
	// bounded queue only smooths bursts; overflow drops the notice, not the login.
	panelLoginQueueSize = 16
)

// Panel sign-in methods. They describe the factors that were verified, never
// the credential itself.
const (
	PanelLoginPassword     = "password"
	PanelLoginPasswordTOTP = "password_totp"
	PanelLoginPasskey      = "passkey"
	PanelLoginPasskeyTOTP  = "passkey_totp"
)

// PanelLogin is one successful sign-in to this KPanel. It deliberately carries
// no password, second-factor code, session token or user agent.
type PanelLogin struct {
	Username      string
	RemoteAddress string
	Method        string
	occurredAt    time.Time
}

func validPanelLoginMethod(value string) bool {
	switch value {
	case PanelLoginPassword, PanelLoginPasswordTOTP, PanelLoginPasskey, PanelLoginPasskeyTOTP:
		return true
	}
	return false
}

// RecordPanelLogin queues a successful sign-in for the notification center
// without blocking the login request. It reports whether the notice was
// accepted; a disabled rule is decided later, when the event is recorded.
func (s *Service) RecordPanelLogin(login PanelLogin) bool {
	if s == nil || !validPanelLoginMethod(login.Method) {
		return false
	}
	login.occurredAt = s.now()
	select {
	case s.panelLogins <- login:
		return true
	default:
		return false
	}
}

// recordPanelLogins persists queued sign-ins as local history and, when the
// active channel is ready, delivers them right away instead of waiting for the
// next evaluation. Host alert counters are not advanced here.
func (s *Service) recordPanelLogins(parent context.Context, logins []PanelLogin) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	state := s.store.stateSnapshot()
	if !state.Settings.Rules.PanelLoginEnabled || len(logins) == 0 {
		return nil
	}
	history, historyErr := s.history.snapshot()
	if historyErr != nil && s.history.loadFailed {
		return historyError(historyErr)
	}
	credential, configured, credentialErr := s.store.credential()
	canDeliver := state.Settings.Enabled && credentialErr == nil && configured && state.Telegram.HasChat && state.Telegram.TokenFingerprint == tokenFingerprint(credential)
	provider, _ := DetectProvider(credential)
	fetchCtx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	hosts := s.hosts.Hosts(fetchCtx)
	local, listed := cluster.Host{ID: cluster.LocalHostID, IsLocal: true}, false
	for _, host := range hosts.Items {
		if host.ID == cluster.LocalHostID {
			local, listed = host, true
			break
		}
	}
	hostName := safeMessageText(local.Name)
	if hostName == "" {
		hostName = local.ID
	}
	local.Name = hostName
	now := s.displayTime(parent, s.now())
	locale := normalizeNotificationLocale(state.Settings.Locale)
	for _, login := range logins {
		history.Sequence++
		event := storedEvent{Event: Event{ID: fmt.Sprint(history.Sequence), CreatedAt: now.UTC(), HostID: local.ID,
			HostName: hostName, IsLocal: true, Rule: panelLoginRuleKey, Kind: "info",
			Message: panelLoginMessage(local, login, now, locale), Delivery: "local_only"}}
		if canDeliver {
			event.Delivery = "pending"
			event.Provider = provider
			event.ChannelFingerprint = tokenFingerprint(credential)
		}
		history.Events = append(history.Events, event)
	}
	pruneHistory(&history, now)
	if err := s.history.commit(history); err != nil {
		return historyError(err)
	}
	// Delivery cancels events whose host is missing from the list. An
	// incomplete list (for example a timed-out fetch) leaves the notice pending
	// for the regular evaluation instead of cancelling it here.
	if !canDeliver || !listed {
		return nil
	}
	return s.deliverHistory(parent, state, hosts.Items, credential, canDeliver, now)
}

// drainPanelLogins collects the sign-in that woke the loop and any already
// queued behind it, so a burst becomes one history commit.
func (s *Service) drainPanelLogins(first PanelLogin) []PanelLogin {
	logins := []PanelLogin{first}
	for len(logins) < panelLoginQueueSize {
		select {
		case login := <-s.panelLogins:
			logins = append(logins, login)
		default:
			return logins
		}
	}
	return logins
}

func panelLoginMethodLabel(method, locale string) string {
	labels := map[string][3]string{
		PanelLoginPassword:     {"密码", "密碼", "Password"},
		PanelLoginPasswordTOTP: {"密码 + 两步验证", "密碼 + 兩步驟驗證", "Password + two-factor"},
		PanelLoginPasskey:      {"通行密钥", "通行金鑰", "Passkey"},
		PanelLoginPasskeyTOTP:  {"通行密钥 + 两步验证", "通行金鑰 + 兩步驟驗證", "Passkey + two-factor"},
	}
	label := labels[method]
	switch locale {
	case "en-US":
		return label[2]
	case "zh-TW":
		return label[1]
	default:
		return label[0]
	}
}

func panelLoginMessage(host cluster.Host, login PanelLogin, now time.Time, locale string) string {
	hostName := safeMessageText(host.Name)
	loginTime := formatNotificationTime(login.occurredAt.In(now.Location()))
	sentAt := formatNotificationTime(now)
	username := safeMessageText(login.Username)
	remoteAddress := safeMessageText(login.RemoteAddress)
	if remoteAddress == "" {
		remoteAddress = "-"
	}
	method := panelLoginMethodLabel(login.Method, locale)
	switch locale {
	case "en-US":
		return fmt.Sprintf("🔐 [KPanel Cluster Notice]\n\nHost: %s\nPanel login: %s\nUser: %s\nSource: %s\nMethod: %s\n\nSent: %s", hostName, loginTime, username, remoteAddress, method, sentAt)
	case "zh-TW":
		return fmt.Sprintf("🔐 [KPanel 叢集通知]\n\n主機：%s\n面板登入：%s\n使用者：%s\n來源：%s\n方式：%s\n\n傳送時間：%s", hostName, loginTime, username, remoteAddress, method, sentAt)
	default:
		return fmt.Sprintf("🔐 [KPanel 集群通知]\n\n主机：%s\n面板登录：%s\n用户：%s\n来源：%s\n方式：%s\n\n发送时间：%s", hostName, loginTime, username, remoteAddress, method, sentAt)
	}
}
