//go:build hids

package model

import "time"

type RuntimeState struct {
	Status           string         `json:"status"`
	Message          string         `json:"message"`
	Mode             string         `json:"mode"`
	ActiveCollectors []string       `json:"active_collectors,omitempty"`
	Detail           map[string]any `json:"detail,omitempty"`
	UpdatedAt        time.Time      `json:"updated_at"`
}
