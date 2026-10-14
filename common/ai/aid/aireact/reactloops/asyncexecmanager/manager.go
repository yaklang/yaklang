package asyncexecmanager

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Manager is one owner scope. Retained execution records are local to this
// manager, including StopAll; completed records remain inspectable until the
// manager is discarded. The zero value is not usable; construct with New.
type Manager struct {
	mu             sync.Mutex
	ctx            context.Context
	cancel         context.CancelCauseFunc
	config         Config
	entries        map[string]*execution
	order          []string
	active         int
	closed         bool
	changed        chan struct{}
	stopOwnerWatch func() bool
}

func New(parent context.Context, config Config) (*Manager, error) {
	if parent == nil {
		return nil, errors.New("async execution owner context is required")
	}
	if config.MaxConcurrent < 0 || config.MaxExecutions < 0 {
		return nil, errors.New("async execution limits must not be negative")
	}
	if config.MaxConcurrent == 0 {
		config.MaxConcurrent = 4
	}
	if config.MaxExecutions == 0 {
		config.MaxExecutions = 128
	}
	ctx, cancel := context.WithCancelCause(parent)
	m := &Manager{
		ctx: ctx, cancel: cancel, config: config,
		entries: make(map[string]*execution), changed: make(chan struct{}),
	}
	m.stopOwnerWatch = context.AfterFunc(ctx, func() { m.close(context.Cause(ctx)) })
	return m, nil
}

// Start registers an execution before launching it. The receipt is lifecycle
// metadata only: Inspect explicitly obtains observations from the resource.
func (m *Manager) Start(request Request) (Summary, error) {
	return m.start(request, false)
}

// Adopt registers already-running work. Its Resource.Execute must join that
// work and cleanup even when ctx is cancelled before Execute is entered. Failed
// admission leaves ownership with the caller; it neither cancels nor restarts it.
func (m *Manager) Adopt(request Request) (Summary, error) {
	return m.start(request, true)
}

func (m *Manager) start(request Request, adopted bool) (Summary, error) {
	if isNil(request.Resource) {
		return Summary{}, ErrInvalidResource
	}
	inspector, err := resourceInspector(request.Resource)
	if err != nil {
		return Summary{}, err
	}
	id := strings.TrimSpace(request.ID)
	if id == "" {
		var bytes [16]byte
		if _, err := rand.Read(bytes[:]); err != nil {
			return Summary{}, fmt.Errorf("allocate execution id: %w", err)
		}
		id = "exec-" + hex.EncodeToString(bytes[:])
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.ctx.Err() != nil {
		return Summary{}, ErrClosed
	}
	if _, exists := m.entries[id]; exists {
		return Summary{}, ErrDuplicateID
	}
	if m.active >= m.config.MaxConcurrent || len(m.entries) >= m.config.MaxExecutions {
		return Summary{}, ErrCapacity
	}
	ctx, cancel := context.WithCancelCause(m.ctx)
	e := &execution{
		adopted: adopted,
		summary: Summary{ID: id, Metadata: cloneMetadata(request.Metadata), State: StatePending,
			CreatedAt: time.Now(), Revision: 1},
		resource: request.Resource, inspector: inspector,
		ctx: ctx, cancel: cancel, done: make(chan struct{}),
	}
	m.entries[id] = e
	m.order = append(m.order, id)
	m.active++
	m.notifyLocked()
	receipt := cloneSummary(e.summary)
	go m.run(e)
	return receipt, nil
}

func (m *Manager) run(e *execution) {
	var runErr error
	defer func() {
		if value := recover(); value != nil {
			runErr = fmt.Errorf("execution panic: %v", value)
		}
		m.finish(e, runErr)
		e.cancel(nil)
	}()
	m.mu.Lock()
	if !e.adopted && (e.ctx.Err() != nil || e.summary.State == StateStopping) {
		m.mu.Unlock()
		return
	}
	now := time.Now()
	e.summary.StartedAt = &now
	if e.summary.State != StateStopping {
		e.summary.State = StateRunning
	}
	e.summary.Revision++
	m.notifyLocked()
	m.mu.Unlock()
	// Recheck cancellation after admission, before invoking user work.
	if !e.adopted && e.ctx.Err() != nil {
		return
	}
	runErr = e.resource.Execute(e.ctx)
}

func (m *Manager) finish(e *execution, runErr error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	e.summary.EndedAt = &now
	if runErr != nil {
		e.summary.Error = runErr.Error()
	}
	cause := context.Cause(e.ctx)
	switch {
	case errors.Is(cause, context.DeadlineExceeded) || errors.Is(runErr, context.DeadlineExceeded):
		e.summary.State = StateTimedOut
	case cause != nil || e.summary.State == StateStopping || errors.Is(runErr, context.Canceled):
		e.summary.State = StateCancelled
	case runErr != nil:
		e.summary.State = StateFailed
	default:
		e.summary.State = StateCompleted
	}
	if cause != nil && e.summary.CancelReason == "" {
		e.summary.CancelReason = cause.Error()
	}
	e.summary.Revision++
	m.active--
	close(e.done)
	m.notifyLocked()
}

// List returns registration-ordered metadata snapshots. It never invokes an
// inspector, so a slow observer cannot block listing or cancellation.
func (m *Manager) List() []Summary {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]Summary, 0, len(m.order))
	for _, id := range m.order {
		items = append(items, cloneSummary(m.entries[id].summary))
	}
	return items
}

func (m *Manager) Get(id string) (Summary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[id]
	if !ok {
		return Summary{}, ErrNotFound
	}
	return cloneSummary(e.summary), nil
}

// Inspector returns the registered observer, not a manager-owned output cache.
func (m *Manager) Inspector(id string) (Inspector, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[id]
	if !ok {
		return nil, ErrNotFound
	}
	return e.inspector, nil
}

// Inspect obtains observations outside the manager lock. Lifecycle and output
// are independently sampled snapshots, not a transaction across the resource.
// On inspection failure the returned snapshot still carries lifecycle metadata.
func (m *Manager) Inspect(ctx context.Context, id string) (Snapshot, error) {
	if ctx == nil {
		return Snapshot{}, errors.New("inspection context is required")
	}
	inspector, err := m.Inspector(id)
	if err != nil {
		return Snapshot{}, err
	}
	observation, inspectErr := inspect(ctx, inspector)
	summary, err := m.Get(id)
	if err != nil {
		return Snapshot{}, err
	}
	observation.Result = append(observation.Result[:0:0], observation.Result...)
	observation.PartialResult = append(observation.PartialResult[:0:0], observation.PartialResult...)
	return Snapshot{Summary: summary, Observation: observation}, inspectErr
}

// Wait returns after Execute and its cleanup return, or ctx expires. An expired
// waiter receives the latest metadata alongside ctx.Err; it never cancels work.
func (m *Manager) Wait(ctx context.Context, id string) (Summary, error) {
	if ctx == nil {
		return Summary{}, errors.New("wait context is required")
	}
	m.mu.Lock()
	e, ok := m.entries[id]
	if !ok {
		m.mu.Unlock()
		return Summary{}, ErrNotFound
	}
	if e.summary.State.IsTerminal() {
		snapshot := cloneSummary(e.summary)
		m.mu.Unlock()
		return snapshot, nil
	}
	done := e.done
	m.mu.Unlock()
	select {
	case <-done:
		return m.Get(id)
	case <-ctx.Done():
		snapshot, _ := m.Get(id)
		return snapshot, ctx.Err()
	}
}

// Stop is idempotent and only requests cancellation. A running resource stays
// stopping until Execute returns, so cancellation cannot free capacity early.
func (m *Manager) Stop(id string) (Summary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[id]
	if !ok {
		return Summary{}, ErrNotFound
	}
	m.stopLocked(e, ErrStopped)
	return cloneSummary(e.summary), nil
}

// StopAll affects only this manager, and does not prohibit later submissions.
func (m *Manager) StopAll() []Summary {
	m.mu.Lock()
	defer m.mu.Unlock()
	items := make([]Summary, 0, len(m.order))
	for _, id := range m.order {
		e := m.entries[id]
		m.stopLocked(e, ErrStopped)
		items = append(items, cloneSummary(e.summary))
	}
	return items
}

func (m *Manager) stopLocked(e *execution, cause error) {
	if e.summary.State.IsTerminal() || e.summary.State == StateStopping {
		return
	}
	e.summary.State = StateStopping
	e.summary.CancelReason = cause.Error()
	e.summary.Revision++
	e.cancel(cause)
	m.notifyLocked()
}

// Close prohibits new work and requests cancellation; Shutdown additionally
// waits for the actual resources to return, bounded by the caller's context.
func (m *Manager) Close() { m.close(ErrClosed) }

func (m *Manager) close(cause error) {
	if cause == nil {
		cause = ErrClosed
	}
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		for _, e := range m.entries {
			m.stopLocked(e, cause)
		}
		m.notifyLocked()
	}
	m.mu.Unlock()
	m.cancel(cause)
}

func (m *Manager) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return errors.New("shutdown context is required")
	}
	m.Close()
	for {
		m.mu.Lock()
		if m.active == 0 {
			m.mu.Unlock()
			m.stopOwnerWatch()
			return nil
		}
		changed := m.changed
		m.mu.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (m *Manager) notifyLocked() {
	close(m.changed)
	m.changed = make(chan struct{})
}
