//go:build hids && linux

package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
)

var ErrCurrentStateUnavailable = errors.New("hids current-state runtime is not running")

// CollectCurrentStateWithSink collects a complete inventory and synchronously
// delivers each observation to sink. The returned count includes only successful
// sink calls. Unlike CollectCurrentState's asynchronous runtime pipeline, this
// path lets a caller acknowledge completion only after durable publication.
// The collection's deadline also applies to every sink call.
func (m *Manager) CollectCurrentStateWithSink(
	ctx context.Context,
	collectType string,
	sink func(context.Context, model.Event) error,
) (int, error) {
	instance, err := m.currentStateInstance()
	if err != nil {
		return 0, err
	}
	count, err := instance.collectCurrentStateWithSink(ctx, collectType, sink)
	if err != nil {
		return count, fmt.Errorf("collect hids current state: %w", err)
	}
	return count, nil
}

func (m *Manager) currentStateInstance() (*Instance, error) {
	if m == nil {
		return nil, ErrCurrentStateUnavailable
	}
	m.mu.Lock()
	instance := m.instance
	m.mu.Unlock()
	if instance == nil {
		return nil, ErrCurrentStateUnavailable
	}
	return instance, nil
}

func (i *Instance) collectCurrentStateWithSink(
	ctx context.Context,
	collectType string,
	sink func(context.Context, model.Event) error,
) (int, error) {
	if i == nil || i.baselineProvider == nil || sink == nil {
		return 0, ErrCurrentStateUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	collectCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := collectCtx.Err(); err != nil {
		return 0, err
	}
	var events []model.Event
	var err error
	switch strings.TrimSpace(collectType) {
	case "processes":
		events, err = i.baselineProvider.ListProcessEvents(collectCtx)
	case "connections":
		events, err = i.baselineProvider.ListNetworkEvents(collectCtx)
	default:
		return 0, fmt.Errorf("unsupported collect_type: %s", collectType)
	}
	if err != nil {
		return 0, fmt.Errorf("list %s inventory: %w", collectType, err)
	}
	if err := collectCtx.Err(); err != nil {
		return 0, err
	}
	for index, event := range events {
		if err := collectCtx.Err(); err != nil {
			return index, err
		}
		if err := sink(collectCtx, event); err != nil {
			return index, fmt.Errorf("publish %s inventory: %w", collectType, err)
		}
	}
	return len(events), collectCtx.Err()
}
