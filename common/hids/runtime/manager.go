//go:build hids && linux

package runtime

import (
	"context"
	"sync"

	"github.com/yaklang/yaklang/common/hids/model"
)

type ApplyResult struct {
	State model.RuntimeState
}

const runtimeObservationBufferSize = 4096

type Manager struct {
	mu           sync.Mutex
	instance     *Instance
	health       health
	alerts       chan model.Alert
	observations chan model.Event
}

func NewManager() *Manager {
	return &Manager{
		health:       newHealth("stopped", "hids runtime is stopped"),
		alerts:       make(chan model.Alert, 64),
		observations: make(chan model.Event, runtimeObservationBufferSize),
	}
}

func (m *Manager) Apply(parent context.Context, spec model.DesiredSpec) (ApplyResult, error) {
	if parent == nil {
		parent = context.Background()
	}

	instance, err := newInstance(spec)
	if err != nil {
		return ApplyResult{}, err
	}
	if err := instance.start(parent); err != nil {
		return ApplyResult{}, err
	}
	state := instance.runtimeState()

	m.mu.Lock()
	oldInstance := m.instance
	m.instance = instance
	m.health = newHealth(state.Status, state.Message)
	m.mu.Unlock()

	if oldInstance != nil {
		_ = oldInstance.close()
	}
	go m.forwardAlerts(instance.alerts())
	go m.forwardObservations(instance.observations())

	return ApplyResult{State: state}, nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	instance := m.instance
	m.instance = nil
	m.health = newHealth("stopped", "hids runtime is stopped")
	m.mu.Unlock()

	if instance == nil {
		return nil
	}
	return instance.close()
}

func (m *Manager) State() model.RuntimeState {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := model.RuntimeState{
		Status:    m.health.status,
		Message:   m.health.message,
		UpdatedAt: m.health.updatedAt,
	}
	if m.instance != nil {
		state = m.instance.runtimeState()
	}
	return state
}

func (m *Manager) Alerts() <-chan model.Alert {
	if m == nil {
		return nil
	}
	return m.alerts
}

func (m *Manager) Observations() <-chan model.Event {
	if m == nil {
		return nil
	}
	return m.observations
}

func (m *Manager) ReplayInventory(ctx context.Context) error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	instance := m.instance
	m.mu.Unlock()
	if instance == nil {
		return nil
	}
	return instance.replayInventory(ctx)
}

func (m *Manager) forwardAlerts(alerts <-chan model.Alert) {
	if m == nil || alerts == nil {
		return
	}
	for alert := range alerts {
		select {
		case m.alerts <- alert:
		default:
		}
	}
}

func (m *Manager) forwardObservations(observations <-chan model.Event) {
	if m == nil || observations == nil {
		return
	}
	for observation := range observations {
		select {
		case m.observations <- observation:
		default:
		}
	}
}
