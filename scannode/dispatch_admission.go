package scannode

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const dispatchReservationRetention = 24 * time.Hour

type dispatchAdmissionState uint8

const (
	dispatchAdmissionPending dispatchAdmissionState = iota
	dispatchAdmissionClaimed
	dispatchAdmissionRunning
	dispatchAdmissionTerminal
)

type dispatchReservation struct {
	ref       jobExecutionRef
	identity  string
	sessionID string
	deadline  time.Time

	task             *Task
	release          func()
	state            dispatchAdmissionState
	preparing        bool
	claimedPublished bool
	capacityDeferred bool
	finishedAt       time.Time
	cancelReason     string
	cancelPublished  bool
	stale            atomic.Bool
}

type pendingDispatchCancel struct {
	jobID     string
	subtaskID string
	reason    string
	createdAt time.Time
}

type dispatchReserveResult uint8

const (
	dispatchReserved dispatchReserveResult = iota
	dispatchDuplicate
	dispatchConflict
	dispatchStaleSession
	dispatchCancelled
)

type dispatchAdmissionRegistry struct {
	mu sync.Mutex

	sessionID     string
	shuttingDown  bool
	byAttempt     map[string]*dispatchReservation
	byCommand     map[string]string
	bySubtask     map[string]map[string]*dispatchReservation
	pendingCancel map[string]pendingDispatchCancel
}

func newDispatchAdmissionRegistry() *dispatchAdmissionRegistry {
	return &dispatchAdmissionRegistry{
		byAttempt:     make(map[string]*dispatchReservation),
		byCommand:     make(map[string]string),
		bySubtask:     make(map[string]map[string]*dispatchReservation),
		pendingCancel: make(map[string]pendingDispatchCancel),
	}
}

func (r *dispatchAdmissionRegistry) SwitchSession(sessionID string) []*dispatchReservation {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessionID == sessionID {
		return nil
	}
	stale := make([]*dispatchReservation, 0, len(r.byAttempt))
	for _, reservation := range r.byAttempt {
		reservation.stale.Store(true)
		stale = append(stale, reservation)
	}
	r.sessionID = sessionID
	r.byAttempt = make(map[string]*dispatchReservation)
	r.byCommand = make(map[string]string)
	r.bySubtask = make(map[string]map[string]*dispatchReservation)
	r.pendingCancel = make(map[string]pendingDispatchCancel)
	return stale
}

func (r *dispatchAdmissionRegistry) Reserve(
	sessionID string,
	ref jobExecutionRef,
	identity string,
	deadline time.Time,
) (*dispatchReservation, dispatchReserveResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked(time.Now().UTC())
	if r.shuttingDown || (r.sessionID != "" && sessionID != r.sessionID) {
		return nil, dispatchStaleSession
	}
	if r.sessionID == "" {
		r.sessionID = sessionID
	}
	if attemptID, exists := r.byCommand[ref.CommandID]; exists && attemptID != ref.AttemptID {
		return nil, dispatchConflict
	}
	if existing := r.byAttempt[ref.AttemptID]; existing != nil {
		if existing.ref.CommandID != ref.CommandID || existing.identity != identity {
			return existing, dispatchConflict
		}
		return existing, dispatchDuplicate
	}
	if cancelled, exists := r.pendingCancel[ref.AttemptID]; exists {
		if cancelled.jobID != ref.JobID || cancelled.subtaskID != ref.SubtaskID {
			return nil, dispatchConflict
		}
		reservation := r.addReservationLocked(sessionID, ref, identity, deadline)
		reservation.cancelReason = cancelled.reason
		delete(r.pendingCancel, ref.AttemptID)
		return reservation, dispatchCancelled
	}
	return r.addReservationLocked(sessionID, ref, identity, deadline), dispatchReserved
}

func (r *dispatchAdmissionRegistry) addReservationLocked(
	sessionID string,
	ref jobExecutionRef,
	identity string,
	deadline time.Time,
) *dispatchReservation {
	reservation := &dispatchReservation{
		ref:       ref,
		identity:  identity,
		sessionID: sessionID,
		deadline:  deadline,
		state:     dispatchAdmissionPending,
	}
	r.byAttempt[ref.AttemptID] = reservation
	r.byCommand[ref.CommandID] = ref.AttemptID
	items := r.bySubtask[ref.SubtaskID]
	if items == nil {
		items = make(map[string]*dispatchReservation)
		r.bySubtask[ref.SubtaskID] = items
	}
	items[ref.AttemptID] = reservation
	return reservation
}

func (r *dispatchAdmissionRegistry) Snapshot(
	reservation *dispatchReservation,
) (state dispatchAdmissionState, preparing bool, claimedPublished bool, stale bool) {
	if reservation == nil {
		return dispatchAdmissionTerminal, false, false, true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return reservation.state, reservation.preparing, reservation.claimedPublished, reservation.stale.Load()
}

func (r *dispatchAdmissionRegistry) BeginPrepare(reservation *dispatchReservation) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reservation == nil || reservation.stale.Load() ||
		reservation.state != dispatchAdmissionPending || reservation.preparing {
		return false
	}
	reservation.preparing = true
	return true
}

func (r *dispatchAdmissionRegistry) PrepareFailed(reservation *dispatchReservation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reservation != nil && reservation.state == dispatchAdmissionPending {
		reservation.preparing = false
		reservation.capacityDeferred = true
	}
}

func (r *dispatchAdmissionRegistry) AttachClaim(
	reservation *dispatchReservation,
	task *Task,
	release func(),
) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reservation == nil || reservation.stale.Load() ||
		reservation.state != dispatchAdmissionPending {
		return false
	}
	reservation.task = task
	reservation.release = release
	reservation.preparing = false
	reservation.capacityDeferred = false
	return true
}

func (r *dispatchAdmissionRegistry) Prepared(reservation *dispatchReservation) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return reservation != nil && reservation.state == dispatchAdmissionPending &&
		reservation.task != nil && reservation.release != nil
}

func (r *dispatchAdmissionRegistry) DetachPrepared(
	reservation *dispatchReservation,
) (*Task, func(), bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reservation == nil || reservation.state != dispatchAdmissionPending ||
		reservation.task == nil {
		return nil, nil, false
	}
	task := reservation.task
	release := reservation.release
	reservation.task = nil
	reservation.release = nil
	reservation.preparing = false
	return task, release, true
}

func (r *dispatchAdmissionRegistry) MarkClaimedPublished(reservation *dispatchReservation) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reservation == nil || reservation.stale.Load() ||
		reservation.state != dispatchAdmissionPending {
		return false
	}
	reservation.claimedPublished = true
	reservation.state = dispatchAdmissionClaimed
	return true
}

func (r *dispatchAdmissionRegistry) MarkRunning(reservation *dispatchReservation) (*Task, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reservation == nil || reservation.stale.Load() ||
		reservation.state != dispatchAdmissionClaimed {
		return nil, false
	}
	reservation.state = dispatchAdmissionRunning
	return reservation.task, true
}

func (r *dispatchAdmissionRegistry) MarkExpired(reservation *dispatchReservation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reservation != nil && reservation.state == dispatchAdmissionPending {
		reservation.preparing = false
		reservation.state = dispatchAdmissionTerminal
		reservation.finishedAt = time.Now().UTC()
	}
}

func (r *dispatchAdmissionRegistry) MarkTerminal(reservation *dispatchReservation) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reservation == nil || reservation.stale.Load() ||
		reservation.state == dispatchAdmissionTerminal {
		return false
	}
	reservation.state = dispatchAdmissionTerminal
	reservation.preparing = false
	reservation.finishedAt = time.Now().UTC()
	return true
}

func (r *dispatchAdmissionRegistry) CancelJob(
	sessionID string,
	ref jobExecutionRef,
) (beforeStart, running []*dispatchReservation, matched bool, staleSession bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessionID != "" && sessionID != "" && sessionID != r.sessionID {
		return nil, nil, false, true
	}
	reservations := make([]*dispatchReservation, 0, 1)
	if strings.TrimSpace(ref.AttemptID) != "" {
		if reservation := r.byAttempt[ref.AttemptID]; reservation != nil &&
			reservation.ref.JobID == ref.JobID && reservation.ref.SubtaskID == ref.SubtaskID {
			reservations = append(reservations, reservation)
		}
	} else {
		for _, reservation := range r.bySubtask[ref.SubtaskID] {
			if reservation != nil && (ref.JobID == "" || reservation.ref.JobID == ref.JobID) {
				reservations = append(reservations, reservation)
			}
		}
	}
	for _, reservation := range reservations {
		if reservation == nil || reservation.stale.Load() {
			continue
		}
		matched = true
		switch reservation.state {
		case dispatchAdmissionPending, dispatchAdmissionClaimed:
			reservation.state = dispatchAdmissionTerminal
			reservation.preparing = false
			reservation.finishedAt = time.Now().UTC()
			beforeStart = append(beforeStart, reservation)
		case dispatchAdmissionRunning:
			running = append(running, reservation)
		}
	}
	return beforeStart, running, matched, false
}

func (r *dispatchAdmissionRegistry) BeginShutdown() (beforeStart, running []*dispatchReservation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shuttingDown = true
	for _, reservation := range r.byAttempt {
		if reservation == nil || reservation.stale.Load() {
			continue
		}
		switch reservation.state {
		case dispatchAdmissionPending, dispatchAdmissionClaimed:
			reservation.state = dispatchAdmissionTerminal
			reservation.preparing = false
			reservation.finishedAt = time.Now().UTC()
			beforeStart = append(beforeStart, reservation)
		case dispatchAdmissionRunning:
			running = append(running, reservation)
		}
	}
	return beforeStart, running
}

func (r *dispatchAdmissionRegistry) Deadline(reservation *dispatchReservation) time.Time {
	if reservation == nil {
		return time.Time{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return reservation.deadline
}

func (r *dispatchAdmissionRegistry) TaskAndRelease(
	reservation *dispatchReservation,
) (*Task, func()) {
	if reservation == nil {
		return nil, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return reservation.task, reservation.release
}

func (r *dispatchAdmissionRegistry) RollbackClaimedAck(
	reservation *dispatchReservation,
) (*Task, func(), bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reservation == nil || reservation.stale.Load() || reservation.state != dispatchAdmissionClaimed {
		return nil, nil, false
	}
	task := reservation.task
	release := reservation.release
	reservation.task = nil
	reservation.release = nil
	reservation.state = dispatchAdmissionPending
	reservation.preparing = false
	reservation.capacityDeferred = false
	return task, release, true
}

func (r *dispatchAdmissionRegistry) CapacityRetryExpired(
	reservation *dispatchReservation,
	now time.Time,
) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return reservation != nil && reservation.capacityDeferred &&
		!reservation.deadline.IsZero() && !now.Before(reservation.deadline)
}

func (r *dispatchAdmissionRegistry) RecordPendingCancel(
	sessionID string,
	ref jobExecutionRef,
	reason string,
) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneLocked(time.Now().UTC())
	if r.shuttingDown || (r.sessionID != "" && sessionID != "" && sessionID != r.sessionID) ||
		strings.TrimSpace(ref.AttemptID) == "" ||
		strings.TrimSpace(ref.JobID) == "" || strings.TrimSpace(ref.SubtaskID) == "" {
		return false
	}
	if r.sessionID == "" && sessionID != "" {
		r.sessionID = sessionID
	}
	if existing := r.byAttempt[ref.AttemptID]; existing != nil {
		return false
	}
	r.pendingCancel[ref.AttemptID] = pendingDispatchCancel{
		jobID:     ref.JobID,
		subtaskID: ref.SubtaskID,
		reason:    reason,
		createdAt: time.Now().UTC(),
	}
	return true
}

func (r *dispatchAdmissionRegistry) PendingCancel(
	reservation *dispatchReservation,
) (reason string, published bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reservation == nil {
		return "", false
	}
	return reservation.cancelReason, reservation.cancelPublished
}

func (r *dispatchAdmissionRegistry) MarkPendingCancelPublished(
	reservation *dispatchReservation,
) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reservation == nil || reservation.stale.Load() || reservation.cancelReason == "" {
		return false
	}
	reservation.cancelPublished = true
	reservation.state = dispatchAdmissionTerminal
	reservation.finishedAt = time.Now().UTC()
	return true
}

func (r *dispatchAdmissionRegistry) CompactTerminal(reservation *dispatchReservation) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reservation == nil || reservation.state != dispatchAdmissionTerminal {
		return
	}
	reservation.task = nil
	reservation.release = nil
	reservation.preparing = false
	reservation.capacityDeferred = false
}

func (r *dispatchAdmissionRegistry) pruneLocked(now time.Time) {
	cutoff := now.Add(-dispatchReservationRetention)
	for attemptID, reservation := range r.byAttempt {
		if reservation == nil || reservation.state != dispatchAdmissionTerminal ||
			reservation.finishedAt.IsZero() || reservation.finishedAt.After(cutoff) {
			continue
		}
		delete(r.byAttempt, attemptID)
		if r.byCommand[reservation.ref.CommandID] == attemptID {
			delete(r.byCommand, reservation.ref.CommandID)
		}
		if items := r.bySubtask[reservation.ref.SubtaskID]; items != nil {
			delete(items, attemptID)
			if len(items) == 0 {
				delete(r.bySubtask, reservation.ref.SubtaskID)
			}
		}
	}
	for attemptID, cancelled := range r.pendingCancel {
		if !cancelled.createdAt.After(cutoff) {
			delete(r.pendingCancel, attemptID)
		}
	}
}
