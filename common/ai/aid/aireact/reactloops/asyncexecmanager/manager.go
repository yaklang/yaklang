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
// manager, including StopAll. Completed records remain inspectable until the
// total-record limit requires eviction or the manager is discarded. The zero
// value is not usable; construct with New.
type Manager struct {
	mu             sync.Mutex
	ctx            context.Context
	cancel         context.CancelCauseFunc
	config         Config
	entries        map[string]*execution
	order          []string
	active         int
	revision       uint64
	closed         bool
	changed        chan struct{}
	stopOwnerWatch func() bool
}

func New(parent context.Context, config Config) (*Manager, error) {
	if parent == nil {
		return nil, errors.New("async execution owner context is required")
	}
	if config.MaxActiveExecutions < 0 || config.MaxRetainedExecutions < 0 {
		return nil, errors.New("async execution limits must not be negative")
	}
	if config.MaxActiveExecutions == 0 {
		config.MaxActiveExecutions = 4
	}
	if config.MaxRetainedExecutions == 0 {
		config.MaxRetainedExecutions = 128
	}
	if config.MaxRetainedExecutions < config.MaxActiveExecutions {
		return nil, fmt.Errorf(
			"async execution MaxRetainedExecutions (%d) must be >= MaxActiveExecutions (%d)",
			config.MaxRetainedExecutions, config.MaxActiveExecutions,
		)
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
	if m.active >= m.config.MaxActiveExecutions {
		return Summary{}, ErrCapacity
	}
	// MaxRetainedExecutions is a total-record limit, not an admission failure. At
	// this point active < MaxActiveExecutions <= MaxRetainedExecutions, so an
	// over-limit store must contain at least one terminal record that can be
	// evicted without rejecting the new execution.
	for len(m.entries) >= m.config.MaxRetainedExecutions {
		if !m.evictOldestTerminalLocked() {
			break
		}
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

// evictOldestTerminalLocked removes the terminal execution with the earliest
// EndedAt. The caller must hold m.mu and at least one terminal record must exist.
func (m *Manager) evictOldestTerminalLocked() bool {
	evictIndex := -1
	var oldestEnded time.Time
	oldestEndedSet := false
	for index, id := range m.order {
		entry := m.entries[id]
		if entry == nil || !entry.summary.State.IsTerminal() {
			continue
		}
		if evictIndex < 0 {
			evictIndex = index
			if entry.summary.EndedAt != nil {
				oldestEnded = *entry.summary.EndedAt
				oldestEndedSet = true
			}
			continue
		}
		if entry.summary.EndedAt == nil {
			continue
		}
		if !oldestEndedSet || entry.summary.EndedAt.Before(oldestEnded) {
			evictIndex = index
			oldestEnded = *entry.summary.EndedAt
			oldestEndedSet = true
		}
	}
	if evictIndex < 0 {
		return false
	}

	id := m.order[evictIndex]
	delete(m.entries, id)
	copy(m.order[evictIndex:], m.order[evictIndex+1:])
	m.order[len(m.order)-1] = ""
	m.order = m.order[:len(m.order)-1]
	return true
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

// Overview returns one coherent lifecycle snapshot without inspecting any
// resource. Revision advances whenever the manager's execution state changes,
// allowing clients to discard an older query response delivered after a newer
// push event.
func (m *Manager) Overview() Overview {
	m.mu.Lock()
	defer m.mu.Unlock()
	executions := make([]Summary, 0, len(m.order))
	for _, id := range m.order {
		executions = append(executions, cloneSummary(m.entries[id].summary))
	}
	return Overview{
		Revision:   m.revision,
		Executions: executions,
	}
}

// ChangeSignal returns a channel that is closed on the next lifecycle change.
// Callers should take the signal before taking an Overview, then wait on it.
// This snapshot-before-wait sequence lets missed intermediate states coalesce
// without losing the latest state.
func (m *Manager) ChangeSignal() <-chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.changed
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
	e, err := m.execution(id)
	if err != nil {
		return nil, err
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
	e, err := m.execution(id)
	if err != nil {
		return Snapshot{}, err
	}
	observation, inspectErr := inspect(ctx, e.inspector)
	summary := m.summaryOf(e)
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
		return m.summaryOf(e), nil
	case <-ctx.Done():
		return m.summaryOf(e), ctx.Err()
	}
}

func (m *Manager) execution(id string) (*execution, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[id]
	if !ok {
		return nil, ErrNotFound
	}
	return e, nil
}

func (m *Manager) summaryOf(e *execution) Summary {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneSummary(e.summary)
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
	m.revision++
	close(m.changed)
	m.changed = make(chan struct{})
}
