//go:build hids

package model

import "time"

type Alert struct {
	RuleID     string         `json:"rule_id"`
	Severity   string         `json:"severity"`
	Title      string         `json:"title"`
	Tags       []string       `json:"tags,omitempty"`
	Detail     map[string]any `json:"detail,omitempty"`
	ObservedAt time.Time      `json:"observed_at"`
}
