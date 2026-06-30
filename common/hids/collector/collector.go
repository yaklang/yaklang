//go:build hids && linux

package collector

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
)

type Instance interface {
	Name() string
	Start(ctx context.Context, sink chan<- model.Event) error
	Close() error
}

type HealthSnapshot struct {
	Name      string         `json:"name"`
	Backend   string         `json:"backend,omitempty"`
	Status    string         `json:"status"`
	Message   string         `json:"message,omitempty"`
	UpdatedAt time.Time      `json:"updated_at"`
	Detail    map[string]any `json:"detail,omitempty"`
}

type HealthReporter interface {
	HealthSnapshot() HealthSnapshot
}
