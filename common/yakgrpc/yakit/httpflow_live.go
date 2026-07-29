package yakit

import (
	"sync"

	"github.com/yaklang/yaklang/common/schema"
)

const (
	HTTPFlowLiveProtocolVersion         = 1
	HTTPFlowLiveReplayCapacity          = 2048
	HTTPFlowLiveSubscriberQueueCapacity = 256
	httpFlowLiveRecentProjectSlotCount  = 4
)

type HTTPFlowLiveGapReason string

const (
	HTTPFlowLiveGapReplayWindowExceeded HTTPFlowLiveGapReason = "replay_window_exceeded"
	HTTPFlowLiveGapSlowConsumer         HTTPFlowLiveGapReason = "slow_consumer"
	HTTPFlowLiveGapCursorAhead          HTTPFlowLiveGapReason = "cursor_ahead"
	HTTPFlowLiveGapProjectEvicted       HTTPFlowLiveGapReason = "project_evicted"
)

// HTTPFlowLiveRecord is the process-local representation of one committed
// MITM list row. Request and Response are always cleared before the record is
// retained, so the replay window cannot pin packet bodies in memory.
type HTTPFlowLiveRecord struct {
	Sequence                uint64
	ProjectGeneration       uint64
	DatabaseIdentity        string
	RequestHijackAtUnixMs   int64
	ResponseMirrorAtUnixMs  int64
	FlowBuiltAtUnixMs       int64
	PersistEnqueuedAtUnixMs int64
	PersistStartedAtUnixMs  int64
	CommittedAtUnixMs       int64
	HighWaterID             uint64
	Flow                    schema.HTTPFlow
}

type HTTPFlowLiveGap struct {
	Reason                  HTTPFlowLiveGapReason
	RequestedSequence       uint64
	OldestAvailableSequence uint64
	LatestSequence          uint64
	HighWaterID             uint64
}

type HTTPFlowLiveState struct {
	OldestAvailableSequence uint64
	LatestSequence          uint64
	HighWaterID             uint64
}

type httpFlowLiveSubscriber struct {
	events chan HTTPFlowLiveRecord
	gaps   chan HTTPFlowLiveGap
	gapped bool
}

type httpFlowLiveProject struct {
	databaseIdentity   string
	projectGeneration  uint64
	lastUsed           uint64
	nextSequence       uint64
	highWaterID        uint64
	evictedHighWaterID uint64
	events             []HTTPFlowLiveRecord
	head               int
	subscribers        map[*httpFlowLiveSubscriber]struct{}
}

type httpFlowLiveBroker struct {
	mu                 sync.Mutex
	replayCapacity     int
	subscriberCapacity int
	clock              uint64
	projects           [httpFlowLiveRecentProjectSlotCount]*httpFlowLiveProject
}

type HTTPFlowLiveSubscription struct {
	broker     *httpFlowLiveBroker
	project    *httpFlowLiveProject
	subscriber *httpFlowLiveSubscriber
	initial    HTTPFlowLiveState
	closeOnce  sync.Once
}

var globalHTTPFlowLiveBroker = newHTTPFlowLiveBroker(
	HTTPFlowLiveReplayCapacity,
	HTTPFlowLiveSubscriberQueueCapacity,
)

func newHTTPFlowLiveBroker(replayCapacity, subscriberCapacity int) *httpFlowLiveBroker {
	if replayCapacity < 1 {
		replayCapacity = 1
	}
	if subscriberCapacity < 1 {
		subscriberCapacity = 1
	}
	return &httpFlowLiveBroker{
		replayCapacity:     replayCapacity,
		subscriberCapacity: subscriberCapacity,
	}
}

func (s *HTTPFlowLiveSubscription) Events() <-chan HTTPFlowLiveRecord {
	if s == nil || s.subscriber == nil {
		return nil
	}
	return s.subscriber.events
}

func (s *HTTPFlowLiveSubscription) Gaps() <-chan HTTPFlowLiveGap {
	if s == nil || s.subscriber == nil {
		return nil
	}
	return s.subscriber.gaps
}

func (s *HTTPFlowLiveSubscription) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		if s.broker == nil || s.project == nil || s.subscriber == nil {
			return
		}
		s.broker.mu.Lock()
		delete(s.project.subscribers, s.subscriber)
		s.broker.mu.Unlock()
	})
}

func (s *HTTPFlowLiveSubscription) Snapshot() HTTPFlowLiveState {
	if s == nil || s.broker == nil || s.project == nil {
		return HTTPFlowLiveState{}
	}
	s.broker.mu.Lock()
	defer s.broker.mu.Unlock()
	return snapshotHTTPFlowLiveProject(s.project)
}

// InitialState is the broker cutoff captured atomically with subscriber
// registration. Records after this cutoff are queued on Events, so the RPC can
// finish replay without a heartbeat overtaking those queued records.
func (s *HTTPFlowLiveSubscription) InitialState() HTTPFlowLiveState {
	if s == nil {
		return HTTPFlowLiveState{}
	}
	return s.initial
}

func PublishHTTPFlowLiveCommitted(flow *schema.HTTPFlow) (HTTPFlowLiveRecord, bool) {
	return globalHTTPFlowLiveBroker.publishCommitted(flow)
}

func SubscribeHTTPFlowLive(
	databaseIdentity string,
	projectGeneration uint64,
	lastSeenSequence uint64,
	lastSeenID uint64,
) (*HTTPFlowLiveSubscription, []HTTPFlowLiveRecord, *HTTPFlowLiveGap) {
	return globalHTTPFlowLiveBroker.subscribe(
		databaseIdentity,
		projectGeneration,
		lastSeenSequence,
		lastSeenID,
	)
}

func SnapshotHTTPFlowLive(databaseIdentity string, projectGeneration uint64) HTTPFlowLiveState {
	return globalHTTPFlowLiveBroker.snapshot(databaseIdentity, projectGeneration)
}

func (b *httpFlowLiveBroker) publishCommitted(flow *schema.HTTPFlow) (HTTPFlowLiveRecord, bool) {
	if flow == nil || flow.ID == 0 || flow.SourceType != schema.HTTPFlow_SourceType_MITM || flow.RuntimeTiming == nil {
		return HTTPFlowLiveRecord{}, false
	}
	timing := flow.RuntimeTiming
	if timing.DatabaseIdentity == "" || timing.ProjectGeneration == 0 || timing.PersistedAtUnixMs == 0 {
		return HTTPFlowLiveRecord{}, false
	}

	summary := *flow
	summary.Request = ""
	summary.Response = ""
	summary.Payload = ""
	summary.AfterSaveHandlers = nil
	summary.RuntimeTiming = nil

	b.mu.Lock()
	defer b.mu.Unlock()
	project := b.projectLocked(timing.DatabaseIdentity, timing.ProjectGeneration)
	project.nextSequence++
	if uint64(flow.ID) > project.highWaterID {
		project.highWaterID = uint64(flow.ID)
	}
	record := HTTPFlowLiveRecord{
		Sequence:                project.nextSequence,
		ProjectGeneration:       timing.ProjectGeneration,
		DatabaseIdentity:        timing.DatabaseIdentity,
		RequestHijackAtUnixMs:   timing.RequestHijackAtUnixMs,
		ResponseMirrorAtUnixMs:  timing.ResponseMirrorAtUnixMs,
		FlowBuiltAtUnixMs:       timing.FlowBuiltAtUnixMs,
		PersistEnqueuedAtUnixMs: timing.PersistEnqueuedAtUnixMs,
		PersistStartedAtUnixMs:  timing.PersistStartedAtUnixMs,
		CommittedAtUnixMs:       timing.PersistedAtUnixMs,
		HighWaterID:             project.highWaterID,
		Flow:                    summary,
	}
	project.append(record, b.replayCapacity)

	for subscriber := range project.subscribers {
		if subscriber.gapped {
			continue
		}
		select {
		case subscriber.events <- record:
		default:
			subscriber.gapped = true
			b.signalGapLocked(subscriber, HTTPFlowLiveGap{
				Reason:                  HTTPFlowLiveGapSlowConsumer,
				OldestAvailableSequence: project.oldestAvailableSequence(),
				LatestSequence:          project.nextSequence,
				HighWaterID:             project.highWaterID,
			})
		}
	}
	return record, true
}

func (b *httpFlowLiveBroker) subscribe(
	databaseIdentity string,
	projectGeneration uint64,
	lastSeenSequence uint64,
	lastSeenID uint64,
) (*HTTPFlowLiveSubscription, []HTTPFlowLiveRecord, *HTTPFlowLiveGap) {
	b.mu.Lock()
	defer b.mu.Unlock()
	project := b.projectLocked(databaseIdentity, projectGeneration)
	state := snapshotHTTPFlowLiveProject(project)
	gap := func(reason HTTPFlowLiveGapReason) *HTTPFlowLiveGap {
		return &HTTPFlowLiveGap{
			Reason:                  reason,
			RequestedSequence:       lastSeenSequence,
			OldestAvailableSequence: state.OldestAvailableSequence,
			LatestSequence:          state.LatestSequence,
			HighWaterID:             state.HighWaterID,
		}
	}

	if lastSeenSequence > project.nextSequence {
		return nil, nil, gap(HTTPFlowLiveGapCursorAhead)
	}
	if lastSeenSequence > 0 && project.eventCount() > 0 && lastSeenSequence+1 < state.OldestAvailableSequence {
		return nil, nil, gap(HTTPFlowLiveGapReplayWindowExceeded)
	}
	if lastSeenSequence == 0 && lastSeenID < project.evictedHighWaterID {
		return nil, nil, gap(HTTPFlowLiveGapReplayWindowExceeded)
	}

	resumeSequence := lastSeenSequence
	if resumeSequence == 0 {
		// HighWaterID is monotonic even if concurrent insert callbacks ever expose
		// IDs out of sequence order. Resume after the last fully observed
		// high-water boundary, then replay a contiguous Sequence suffix.
		for _, record := range project.orderedEvents() {
			if record.HighWaterID > lastSeenID {
				break
			}
			resumeSequence = record.Sequence
		}
	}

	replay := make([]HTTPFlowLiveRecord, 0, project.eventCount())
	for _, record := range project.orderedEvents() {
		if record.Sequence > resumeSequence {
			replay = append(replay, record)
		}
	}

	subscriber := &httpFlowLiveSubscriber{
		events: make(chan HTTPFlowLiveRecord, b.subscriberCapacity),
		gaps:   make(chan HTTPFlowLiveGap, 1),
	}
	project.subscribers[subscriber] = struct{}{}
	return &HTTPFlowLiveSubscription{
		broker:     b,
		project:    project,
		subscriber: subscriber,
		initial:    state,
	}, replay, nil
}

func (b *httpFlowLiveBroker) snapshot(databaseIdentity string, projectGeneration uint64) HTTPFlowLiveState {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, project := range b.projects {
		if project != nil && project.databaseIdentity == databaseIdentity &&
			project.projectGeneration == projectGeneration {
			return snapshotHTTPFlowLiveProject(project)
		}
	}
	return HTTPFlowLiveState{}
}

func (b *httpFlowLiveBroker) projectLocked(databaseIdentity string, projectGeneration uint64) *httpFlowLiveProject {
	b.clock++
	var emptyIndex = -1
	oldestIndex := 0
	oldestUse := ^uint64(0)
	for index, project := range b.projects {
		if project == nil {
			if emptyIndex < 0 {
				emptyIndex = index
			}
			continue
		}
		if project.databaseIdentity == databaseIdentity && project.projectGeneration == projectGeneration {
			project.lastUsed = b.clock
			return project
		}
		if project.lastUsed < oldestUse {
			oldestUse = project.lastUsed
			oldestIndex = index
		}
	}

	index := emptyIndex
	if index < 0 {
		index = oldestIndex
		previous := b.projects[index]
		for subscriber := range previous.subscribers {
			if subscriber.gapped {
				continue
			}
			subscriber.gapped = true
			b.signalGapLocked(subscriber, HTTPFlowLiveGap{
				Reason:                  HTTPFlowLiveGapProjectEvicted,
				OldestAvailableSequence: previous.oldestAvailableSequence(),
				LatestSequence:          previous.nextSequence,
				HighWaterID:             previous.highWaterID,
			})
		}
	}
	project := &httpFlowLiveProject{
		databaseIdentity:  databaseIdentity,
		projectGeneration: projectGeneration,
		lastUsed:          b.clock,
		events:            make([]HTTPFlowLiveRecord, 0, b.replayCapacity),
		subscribers:       make(map[*httpFlowLiveSubscriber]struct{}),
	}
	b.projects[index] = project
	return project
}

func (b *httpFlowLiveBroker) signalGapLocked(subscriber *httpFlowLiveSubscriber, gap HTTPFlowLiveGap) {
	select {
	case subscriber.gaps <- gap:
	default:
	}
}

func (p *httpFlowLiveProject) append(record HTTPFlowLiveRecord, capacity int) {
	if len(p.events) < capacity {
		p.events = append(p.events, record)
		return
	}
	evicted := p.events[p.head]
	if evicted.HighWaterID > p.evictedHighWaterID {
		p.evictedHighWaterID = evicted.HighWaterID
	}
	p.events[p.head] = record
	p.head = (p.head + 1) % capacity
}

func (p *httpFlowLiveProject) eventCount() int {
	return len(p.events)
}

func (p *httpFlowLiveProject) orderedEvents() []HTTPFlowLiveRecord {
	if len(p.events) == 0 {
		return nil
	}
	ordered := make([]HTTPFlowLiveRecord, 0, len(p.events))
	for offset := 0; offset < len(p.events); offset++ {
		ordered = append(ordered, p.events[(p.head+offset)%len(p.events)])
	}
	return ordered
}

func (p *httpFlowLiveProject) oldestAvailableSequence() uint64 {
	if len(p.events) == 0 {
		return p.nextSequence + 1
	}
	return p.events[p.head].Sequence
}

func snapshotHTTPFlowLiveProject(project *httpFlowLiveProject) HTTPFlowLiveState {
	if project == nil {
		return HTTPFlowLiveState{}
	}
	return HTTPFlowLiveState{
		OldestAvailableSequence: project.oldestAvailableSequence(),
		LatestSequence:          project.nextSequence,
		HighWaterID:             project.highWaterID,
	}
}

func resetHTTPFlowLiveBrokerForTest() {
	globalHTTPFlowLiveBroker.mu.Lock()
	defer globalHTTPFlowLiveBroker.mu.Unlock()
	globalHTTPFlowLiveBroker.clock = 0
	globalHTTPFlowLiveBroker.projects = [httpFlowLiveRecentProjectSlotCount]*httpFlowLiveProject{}
}
