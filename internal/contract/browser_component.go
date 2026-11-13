package contract

import "time"

type BrowserComponentRequest struct {
	Action                  string `json:"action"`
	Kind                    string `json:"kind,omitempty"`
	ExpectedResourceVersion string `json:"expectedResourceVersion"`
}

func ValidBrowserComponentKind(kind string) bool {
	return kind == "" || kind == "chromium" || kind == "tor"
}

func ValidBrowserComponentAction(action string) bool {
	switch action {
	case "install", "start", "stop", "restart", "uninstall":
		return true
	}
	return false
}

type BrowserComponentJob struct {
	ExpectedResourceVersion string     `json:"expectedResourceVersion"`
	ID                      string     `json:"id"`
	Action                  string     `json:"action"`
	Kind                    string     `json:"kind,omitempty"`
	State                   string     `json:"state"`
	Stage                   string     `json:"stage"`
	Message                 string     `json:"message,omitempty"`
	StartedAt               time.Time  `json:"startedAt"`
	FinishedAt              *time.Time `json:"finishedAt,omitempty"`
}

type BrowserComponent struct {
	Kind            string               `json:"kind,omitempty"`
	TorSupported    bool                 `json:"torSupported"`
	ContainerID     string               `json:"containerID,omitempty"`
	Supported       bool                 `json:"supported"`
	Reason          string               `json:"reason,omitempty"`
	Installed       bool                 `json:"installed"`
	Running         bool                 `json:"running"`
	Healthy         bool                 `json:"healthy"`
	Image           string               `json:"image"`
	ResourceVersion string               `json:"resourceVersion"`
	Job             *BrowserComponentJob `json:"job,omitempty"`
}
