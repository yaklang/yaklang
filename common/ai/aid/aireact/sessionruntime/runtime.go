package sessionruntime

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiconfig"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/reactloops_yak"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	reActSessionAttachTimeout = time.Minute
	reActSessionStopTimeout   = 10 * time.Second
)

// ReActSessionRuntime is the process-local, transport-independent entry point
// for a persistent ReAct session. Its payloads intentionally remain the
// existing protobuf DTOs for now; the boundary removes the gRPC stream
// dependency without changing ReAct's public input model at the same time.
type ReActSessionRuntime interface {
	ReserveSession(ctx context.Context, sessionID, ownerID string) (SessionReservation, error)
	Connect(ctx context.Context, req ConnectRequest, onEvent ReActEventHandler) (ReActConnection, error)
	IsSessionBusy(sessionID string) bool
	QuiesceSessions(ctx context.Context, sessionIDs []string) (SessionQuiescence, error)
	QuiesceAll(ctx context.Context) (SessionQuiescence, error)
}

// ReActEventHandler reports a subscriber-local delivery failure. Normal ReAct
// output remains best-effort and never fails the shared runtime because one
// subscriber is gone; synchronous connection responses may return this error to
// the caller so they cannot report success after the response was lost.
type ReActEventHandler func(*schema.AiOutputEvent) error

type ConnectRequest struct {
	StartParams *ypb.AIStartParams
	Reservation SessionReservation
	Options     *ConnectOptions
}

// ConnectOptions contains caller-specific delivery and construction options.
// Production callers normally leave LoadBuiltinTools enabled; ConfigOptions is
// primarily useful to inject deterministic dependencies in lifecycle tests.
type ConnectOptions struct {
	LoadBuiltinTools bool
	ConfigOptions    []aicommon.ConfigOption
	OnEventError     func(error)
}

// SessionReservation is an execution lease, not merely a creation lock. A
// scheduler holds it from the trigger boundary until its job has completely
// finalized and unregistered. This lets session deletion cancel the owner and
// wait for the scheduled task's deferred persistence and cleanup to finish.
type SessionReservation interface {
	Context() context.Context
	SessionID() string
	OwnerID() string
	Release()
	runtimeReservation() *sessionReservation
}

type ReActConnection interface {
	Send(*ypb.AIInputEvent) error
	Close() error
	Done() <-chan struct{}
	CreatedRuntime() bool
}

// SessionQuiescence keeps one or more sessions closed to new reservations,
// connections and inputs while their durable data is being removed.
type SessionQuiescence interface {
	Release()
}

type reActSessionRuntime struct {
	projectDB func() *gorm.DB
	newReAct  func(...aicommon.ConfigOption) (*aireact.ReAct, error)

	mu           sync.Mutex
	entries      map[string]*reActSessionState
	changed      chan struct{}
	allQuiescing bool
}

type reActSessionState struct {
	reservation   *sessionReservation
	starting      *reActSessionStart
	runtime       *ownedReActRuntime
	admitting     int
	taskAdmitting int
	quiescing     bool
}

// reActSessionBusyReason explains why a session cannot currently be treated as
// idle. "Busy" is intentionally broader than "an AI task is running": session
// creation, scheduler ownership, input admission and shutdown are all boundary
// states in which starting another scheduled task would be unsafe.
type reActSessionBusyReason uint8

const (
	reActSessionAvailable reActSessionBusyReason = iota
	// Runtime/session shutdown barriers reject every new operation until their
	// quiescence guard is released.
	reActSessionBusyRuntimeQuiescing
	reActSessionBusySessionQuiescing
	// Ownership and creation states reserve the next task or ReAct instance even
	// though no task may be visible in aireact yet.
	reActSessionBusyReserved
	reActSessionBusyStarting
	// Boundary states close the gaps before task-queue publication and after
	// running-session unregistration.
	reActSessionBusyTaskAdmitting
	reActSessionBusyRuntimeTransition
	// The ReAct layer reports a process-wide start, queued task or running task.
	reActSessionBusyReAct
)

type reActSessionStart struct {
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	doneOnce sync.Once
}

func (s *reActSessionStart) finish() {
	if s != nil {
		s.doneOnce.Do(func() { close(s.done) })
	}
}

type ownedReActRuntime struct {
	sessionID string
	react     *aireact.ReAct
	cancel    context.CancelFunc
	done      chan struct{}
}

type sessionReservation struct {
	runtime   *reActSessionRuntime
	sessionID string
	ownerID   string
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	once      sync.Once
}

func (r *sessionReservation) Context() context.Context { return r.ctx }
func (r *sessionReservation) SessionID() string        { return r.sessionID }
func (r *sessionReservation) OwnerID() string          { return r.ownerID }
func (r *sessionReservation) runtimeReservation() *sessionReservation {
	return r
}
func (r *sessionReservation) Release() {
	if r == nil {
		return
	}
	r.once.Do(func() {
		r.cancel()
		r.runtime.releaseReservation(r)
		close(r.done)
	})
}

type reActSessionQuiescence struct {
	runtime *reActSessionRuntime
	ids     []string
	all     bool
	once    sync.Once
}

func (g *reActSessionQuiescence) Release() {
	if g == nil || g.runtime == nil {
		return
	}
	g.once.Do(func() { g.runtime.releaseQuiescence(g.ids, g.all) })
}

func New(projectDB func() *gorm.DB) ReActSessionRuntime {
	return &reActSessionRuntime{
		projectDB: projectDB,
		newReAct:  aireact.NewReAct,
		entries:   make(map[string]*reActSessionState),
		changed:   make(chan struct{}),
	}
}

func normalizeReActSessionID(sessionID string) string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return "default"
	}
	return sessionID
}

func (r *reActSessionRuntime) entryLocked(sessionID string) *reActSessionState {
	entry := r.entries[sessionID]
	if entry == nil {
		entry = &reActSessionState{}
		r.entries[sessionID] = entry
	}
	return entry
}

func (r *reActSessionRuntime) notifyLocked() {
	close(r.changed)
	r.changed = make(chan struct{})
}

func (r *reActSessionRuntime) cleanupEntryLocked(sessionID string, entry *reActSessionState) {
	if entry != nil && entry.reservation == nil && entry.starting == nil && entry.runtime == nil && entry.admitting == 0 && entry.taskAdmitting == 0 && !entry.quiescing {
		delete(r.entries, sessionID)
	}
}

// sessionCoordinationBusyReasonLocked reports Runtime-owned coordination state;
// callers must hold r.mu. It deliberately does not inspect ReAct task state so
// IsSessionBusy can release r.mu before taking locks owned by aireact.
func (r *reActSessionRuntime) sessionCoordinationBusyReasonLocked(sessionID string, entry *reActSessionState) reActSessionBusyReason {
	if r.allQuiescing {
		return reActSessionBusyRuntimeQuiescing
	}
	if entry == nil {
		return reActSessionAvailable
	}
	if entry.quiescing {
		return reActSessionBusySessionQuiescing
	}
	if entry.reservation != nil {
		return reActSessionBusyReserved
	}
	if entry.starting != nil {
		return reActSessionBusyStarting
	}
	if entry.taskAdmitting > 0 {
		return reActSessionBusyTaskAdmitting
	}
	if entry.runtime == nil {
		return reActSessionAvailable
	}

	// The running-session registry is removed when the ReAct input loop exits,
	// before the queue processor and deferred task cleanup necessarily finish.
	// A missing or different registry entry therefore remains busy until the
	// Runtime-owned lifecycle reaches owner.done and clears entry.runtime.
	running, ok := aireact.GetRunningSession(sessionID)
	if !ok || running != entry.runtime.react {
		return reActSessionBusyRuntimeTransition
	}
	return reActSessionAvailable
}

func (r *reActSessionRuntime) sessionCoordinationBusyReason(sessionID string) reActSessionBusyReason {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessionCoordinationBusyReasonLocked(sessionID, r.entries[sessionID])
}

func (r *reActSessionRuntime) sessionBusyReason(sessionID string) reActSessionBusyReason {
	if reason := r.sessionCoordinationBusyReason(sessionID); reason != reActSessionAvailable {
		return reason
	}
	// ReAct owns the actual task/queue state. Keep this check outside r.mu so a
	// read-only status query does not nest Runtime and ReAct lifecycle locks.
	if aireact.IsSessionBusy(sessionID) {
		return reActSessionBusyReAct
	}
	return reActSessionAvailable
}

func (r *reActSessionRuntime) ReserveSession(ctx context.Context, sessionID, ownerID string) (SessionReservation, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	sessionID = normalizeReActSessionID(sessionID)
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return nil, utils.Error("session reservation owner id is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	entry := r.entryLocked(sessionID)
	reason := r.sessionCoordinationBusyReasonLocked(sessionID, entry)
	if reason == reActSessionBusyRuntimeQuiescing || reason == reActSessionBusySessionQuiescing {
		return nil, utils.Errorf("AI ReAct session is stopping: %s", sessionID)
	}
	if reason != reActSessionAvailable || aireact.IsSessionBusy(sessionID) {
		return nil, utils.Errorf("AI ReAct session is busy: %s", sessionID)
	}
	reservationCtx, cancel := context.WithCancel(ctx)
	reservation := &sessionReservation{
		runtime:   r,
		sessionID: sessionID,
		ownerID:   ownerID,
		ctx:       reservationCtx,
		cancel:    cancel,
		done:      make(chan struct{}),
	}
	entry.reservation = reservation
	r.notifyLocked()
	return reservation, nil
}

func (r *reActSessionRuntime) releaseReservation(reservation *sessionReservation) {
	if reservation == nil {
		return
	}
	r.mu.Lock()
	entry := r.entries[reservation.sessionID]
	if entry != nil && entry.reservation == reservation {
		entry.reservation = nil
		r.cleanupEntryLocked(reservation.sessionID, entry)
		r.notifyLocked()
	}
	r.mu.Unlock()
}

func (r *reActSessionRuntime) validReservationLocked(entry *reActSessionState, reservation SessionReservation, sessionID string) (*sessionReservation, bool) {
	if reservation == nil {
		return nil, entry == nil || entry.reservation == nil
	}
	raw := reservation.runtimeReservation()
	if raw == nil || raw.runtime != r || raw.sessionID != sessionID || entry == nil || entry.reservation != raw {
		return nil, false
	}
	return raw, true
}

func (r *reActSessionRuntime) IsSessionBusy(sessionID string) bool {
	sessionID = normalizeReActSessionID(sessionID)
	return r.sessionBusyReason(sessionID) != reActSessionAvailable
}

func (r *reActSessionRuntime) Connect(ctx context.Context, req ConnectRequest, onEvent ReActEventHandler) (ReActConnection, error) {
	loadBuiltinTools := true
	var configOptions []aicommon.ConfigOption
	if req.Options != nil {
		loadBuiltinTools = req.Options.LoadBuiltinTools
		configOptions = req.Options.ConfigOptions
	}
	return r.connectWithOptions(ctx, req, onEvent, loadBuiltinTools, configOptions...)
}

func (r *reActSessionRuntime) connectWithOptions(
	ctx context.Context,
	req ConnectRequest,
	onEvent ReActEventHandler,
	loadBuiltinTools bool,
	additionalOptions ...aicommon.ConfigOption,
) (ReActConnection, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	startParams := req.StartParams
	if startParams == nil {
		startParams = &ypb.AIStartParams{}
	}
	sessionID := normalizeReActSessionID(startParams.GetTimelineSessionID())
	var onEventError func(error)
	if req.Options != nil {
		onEventError = req.Options.OnEventError
	}

	// 启动 ReAct 之前懒扫描用户的 ~/yakit-projects/ai-focus/，
	// 防止客户端跳过 QueryAIFocus 直接发带 FocusModeLoop 的 free input 时
	// 找不到注册项。冷却由 EnsureUserFocusModesLoaded 内部控制，失败只 log。
	// 关键词: start ai re-act ensure user focus modes
	if err := reactloops_yak.EnsureUserFocusModesLoaded(); err != nil {
		log.Warnf("ensure user yak focus modes failed: %v", err)
	}
	delivery := newReActEventDelivery(ctx, onEvent, onEventError)

	waitCtx := ctx
	var cancelWait context.CancelFunc
	if startParams.GetAttach() {
		waitCtx, cancelWait = context.WithTimeout(ctx, reActSessionAttachTimeout)
		defer cancelWait()
	}

	for {
		if err := waitCtx.Err(); err != nil {
			delivery.cancel()
			if startParams.GetAttach() && err == context.DeadlineExceeded {
				return nil, utils.Errorf("wait running aireact session timeout: %s", sessionID)
			}
			return nil, err
		}

		r.mu.Lock()
		entry := r.entryLocked(sessionID)
		if r.allQuiescing || entry.quiescing {
			r.mu.Unlock()
			delivery.cancel()
			return nil, utils.Errorf("AI ReAct session is stopping: %s", sessionID)
		}
		reservation, reservationValid := r.validReservationLocked(entry, req.Reservation, sessionID)
		if req.Reservation != nil && !reservationValid {
			r.mu.Unlock()
			delivery.cancel()
			return nil, utils.Errorf("invalid or expired AI ReAct session reservation: %s", sessionID)
		}

		// Do not attach to the registry entry that NewReAct publishes midway
		// through construction. Waiting for the start attempt makes the runtime
		// handle and its cancellation ownership visible atomically.
		if entry.starting == nil && !aireact.IsSessionStarting(sessionID) {
			if runningReAct, ok := aireact.GetRunningSession(sessionID); ok {
				owner := entry.runtime
				if owner != nil && owner.react != runningReAct {
					owner = nil
				}
				r.mu.Unlock()
				return r.attachConnection(ctx, delivery, sessionID, startParams, runningReAct, owner, reservation)
			}
		}
		// The compatibility registry is removed when the input loop exits, while
		// owned runtime shutdown also waits for the queue processor and hot-patch
		// loop. Do not create a replacement in that transition window.
		if entry.runtime != nil {
			changed := r.changed
			r.mu.Unlock()
			if err := waitForReActRuntimeChange(waitCtx, changed); err != nil {
				delivery.cancel()
				return nil, err
			}
			continue
		}

		// A scheduled run reserves its target session at the trigger boundary. A
		// user turn arriving in the narrow interval before that run publishes its
		// ReAct instance waits and attaches, so it cannot steal initialization and
		// make the scheduled occurrence execute late behind it.
		if entry.starting != nil || (entry.reservation != nil && reservation == nil) || startParams.GetAttach() {
			changed := r.changed
			r.mu.Unlock()
			if err := waitForReActRuntimeChange(waitCtx, changed); err != nil {
				delivery.cancel()
				if startParams.GetAttach() && err == context.DeadlineExceeded {
					return nil, utils.Errorf("wait running aireact session timeout: %s", sessionID)
				}
				return nil, err
			}
			continue
		}

		// This closes the small gap between the initial registry lookup and
		// NewReAct publishing its running session. Exactly one connection may
		// initialize a persistent session; racing connections wait and attach to
		// that instance.
		releaseGlobalStart, ownsGlobalStart := aireact.TryBeginSessionStart(sessionID)
		if !ownsGlobalStart {
			changed := r.changed
			r.mu.Unlock()
			if err := waitForReActRuntimeChange(waitCtx, changed); err != nil {
				delivery.cancel()
				return nil, err
			}
			continue
		}
		startCtx, startCancel := context.WithCancel(ctx)
		start := &reActSessionStart{ctx: startCtx, cancel: startCancel, done: make(chan struct{})}
		entry.starting = start
		r.notifyLocked()
		r.mu.Unlock()

		owner, err := func() (*ownedReActRuntime, error) {
			defer releaseGlobalStart()
			return r.createRuntime(start, delivery, sessionID, startParams, loadBuiltinTools, additionalOptions...)
		}()
		if err != nil {
			startCancel()
			r.finishStart(sessionID, start, nil)
			delivery.cancel()
			return nil, err
		}
		if err := r.publishRuntime(sessionID, start, owner); err != nil {
			delivery.cancel()
			return nil, err
		}
		return newReActConnection(r, ctx, delivery, sessionID, owner.react, owner, reservation, true, nil), nil
	}
}

func waitForReActRuntimeChange(ctx context.Context, changed <-chan struct{}) error {
	// The short fallback tick also observes legacy/programmatic ReAct sessions
	// that still publish only through aireact's compatibility registry.
	timer := time.NewTimer(50 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-changed:
		return nil
	case <-timer.C:
		return nil
	}
}

func (r *reActSessionRuntime) projectDatabase() *gorm.DB {
	if r == nil || r.projectDB == nil {
		return nil
	}
	return r.projectDB()
}

func (r *reActSessionRuntime) createRuntime(
	start *reActSessionStart,
	delivery *reActEventDelivery,
	sessionID string,
	startParams *ypb.AIStartParams,
	loadBuiltinTools bool,
	additionalOptions ...aicommon.ConfigOption,
) (*ownedReActRuntime, error) {
	resolvedStartParams, err := ResolveSessionStartParams(
		r.projectDatabase(),
		sessionID,
		startParams,
		startParams.GetPreferSessionCachedConfig(),
	)
	if err != nil {
		return nil, utils.Errorf("resolve session cached config failed: %v", err)
	}
	if _, err := yakit.CreateOrUpdateAISessionMetaOnStart(r.projectDatabase(), sessionID, resolvedStartParams, time.Now()); err != nil {
		log.Warnf("persist ai session start meta failed for %s: %v", sessionID, err)
	}

	inputEvent := chanx.NewUnlimitedChan[*ypb.AIInputEvent](start.ctx, 10)
	hotpatchChan := chanx.NewUnlimitedChan[aicommon.ConfigOption](start.ctx, 10)
	if aiconfig.IsTieredAIConfig() {
		log.Info("tiered ai config is enabled. the old-styled ai config is override")
	}
	defaultAI, err := aicommon.GetDefaultAIModelCallback()
	if err != nil {
		defaultAI, _ = aicommon.GetDefaultAIModelCallback()
		log.Warnf("get default AI model callback failed: %v", err)
	}

	configOptions := []aicommon.ConfigOption{
		aicommon.WithEventHandler(delivery.deliver),
		aicommon.WithEventInputChanx(inputEvent),
		aicommon.WithContext(start.ctx),
	}
	if loadBuiltinTools {
		configOptions = append(configOptions, aireact.WithBuiltinTools())
	}
	configOptions = append(configOptions,
		aicommon.WithEnhanceKnowledgeManager(rag.NewRagEnhanceKnowledgeManager()),
		aicommon.WithPersistentSessionId(sessionID),
		aicommon.WithHotPatchOptionChan(hotpatchChan),
		aicommon.WithEnablePETaskAnalyze(true),
		aicommon.WithEnableDispatchSubReactAgent(true), // 仅仅允许顶层 ReAct 分发子 ReAct Agent，子 Agent 仍然可以使用原始的 AI 回调。
	)
	// optsFromStartParams (containing WithAICallback) must be applied BEFORE
	// tiered overrides, otherwise WithAICallback overwrites all three callbacks
	// (Original, Quality, Speed) to the same frontend-selected model.
	configOptions = append(configOptions, ConvertStartParamsToReActConfig(resolvedStartParams)...)
	if aiconfig.IsTieredAIConfig() {
		configOptions = append(configOptions, aicommon.WithAutoTieredAICallback(defaultAI))
	}
	configOptions = append(configOptions, additionalOptions...)

	reAct, err := r.newReAct(configOptions...)
	if err != nil {
		log.Errorf("create re-act failed: %v", err)
		return nil, utils.Errorf("create re-act instance failed: %v", err)
	}
	reAct.GetConfig().SetConfig("MustProcessAttachedData", true)
	owner := &ownedReActRuntime{
		sessionID: sessionID,
		react:     reAct,
		cancel:    start.cancel,
		done:      make(chan struct{}),
	}
	go func() {
		reAct.WaitLifecycleStopped()
		close(owner.done)
	}()
	return owner, nil
}

func (r *reActSessionRuntime) finishStart(sessionID string, start *reActSessionStart, owner *ownedReActRuntime) {
	r.mu.Lock()
	entry := r.entries[sessionID]
	if entry != nil && entry.starting == start {
		entry.starting = nil
		if owner != nil {
			entry.runtime = owner
		}
		start.finish()
		r.cleanupEntryLocked(sessionID, entry)
		r.notifyLocked()
	}
	r.mu.Unlock()
}

func (r *reActSessionRuntime) publishRuntime(sessionID string, start *reActSessionStart, owner *ownedReActRuntime) error {
	r.mu.Lock()
	entry := r.entries[sessionID]
	if entry == nil || entry.starting != start {
		r.mu.Unlock()
		owner.cancel()
		<-owner.done
		start.finish()
		return utils.Errorf("AI ReAct session start ownership changed: %s", sessionID)
	}
	if r.allQuiescing || entry.quiescing {
		r.mu.Unlock()
		owner.cancel()
		<-owner.done
		r.finishStart(sessionID, start, nil)
		return utils.Errorf("AI ReAct session is stopping: %s", sessionID)
	}
	entry.runtime = owner
	entry.starting = nil
	start.finish()
	r.notifyLocked()
	r.mu.Unlock()

	go func() {
		<-owner.done
		r.mu.Lock()
		entry := r.entries[sessionID]
		if entry != nil && entry.runtime == owner {
			entry.runtime = nil
			r.cleanupEntryLocked(sessionID, entry)
			r.notifyLocked()
		}
		r.mu.Unlock()
	}()
	return nil
}

func (r *reActSessionRuntime) attachConnection(
	ctx context.Context,
	delivery *reActEventDelivery,
	sessionID string,
	startParams *ypb.AIStartParams,
	reAct *aireact.ReAct,
	owner *ownedReActRuntime,
	reservation *sessionReservation,
) (ReActConnection, error) {
	log.Infof("attach connection to running aireact session: %s", sessionID)
	if _, err := yakit.CreateOrUpdateAISessionMetaOnStart(r.projectDatabase(), sessionID, startParams, time.Now()); err != nil {
		log.Warnf("persist ai session start meta failed for %s: %v", sessionID, err)
	}
	unsubscribe, ok := aireact.SubscribeRunningSession(sessionID, delivery.deliver)
	if !ok {
		delivery.cancel()
		return nil, utils.Errorf("failed to subscribe running aireact session: %s", sessionID)
	}
	return newReActConnection(r, ctx, delivery, sessionID, reAct, owner, reservation, false, unsubscribe), nil
}

type reActEventDelivery struct {
	ctx          context.Context
	cancelFn     context.CancelFunc
	onEvent      ReActEventHandler
	onEventError func(error)
	printer      *aicommon.DebugStreamPrinter
}

func newReActEventDelivery(parent context.Context, onEvent ReActEventHandler, onEventError func(error)) *reActEventDelivery {
	ctx, cancel := context.WithCancel(parent)
	// debugStreamPrinter 在 DEBUG=1 时把流式 delta 合并到单行，避免每个
	// token 单独换行造成的刷屏；非流事件来临时先 FlushIfActive 收尾，让
	// 后续 log / 普通事件都从新行开始，消除"夹心"现象。
	// 关键词: DEBUG=1 流式输出体验, AI stream delta debug print
	debugStreamPrinter := aicommon.GetDefaultDebugStreamPrinter()
	// 同步把 common/log 默认输出包装上一层 flush, 让任何日志写入前先把
	// 流缓冲刷出, 彻底消灭日志被夹在流中间的视觉混乱。
	// 关键词: EnsureLogFlushWrapperInstalled grpc_ai_react entry
	aicommon.EnsureLogFlushWrapperInstalled()
	return &reActEventDelivery{
		ctx:          ctx,
		cancelFn:     cancel,
		onEvent:      onEvent,
		onEventError: onEventError,
		printer:      debugStreamPrinter,
	}
}

func (d *reActEventDelivery) cancel() {
	if d != nil && d.cancelFn != nil {
		d.cancelFn()
	}
}

func (d *reActEventDelivery) deliver(event *schema.AiOutputEvent) {
	if err := d.deliverWithResult(event); err != nil && d.onEventError != nil {
		d.onEventError(err)
	}
}

func (d *reActEventDelivery) deliverWithResult(event *schema.AiOutputEvent) error {
	if d == nil || event == nil || d.ctx.Err() != nil {
		return nil
	}
	if event.Timestamp <= 0 {
		event.Timestamp = time.Now().Unix() // fallback
	}
	utils.Debug(func() {
		if event.IsStream {
			d.printer.PrintStreamDelta(event)
		} else {
			d.printer.FlushIfActive()
		}
	})
	if d.onEvent != nil && d.ctx.Err() == nil {
		return d.onEvent(event)
	}
	return nil
}

type reActConnection struct {
	runtime     *reActSessionRuntime
	sessionID   string
	react       *aireact.ReAct
	owner       *ownedReActRuntime
	reservation *sessionReservation
	created     bool
	delivery    *reActEventDelivery
	unsubscribe func()
	done        chan struct{}
	closeOnce   sync.Once
	inputMu     sync.Mutex
	inputUUIDs  []string
	inputClosed bool
}

func newReActConnection(
	runtime *reActSessionRuntime,
	ctx context.Context,
	delivery *reActEventDelivery,
	sessionID string,
	react *aireact.ReAct,
	owner *ownedReActRuntime,
	reservation *sessionReservation,
	created bool,
	unsubscribe func(),
) *reActConnection {
	connection := &reActConnection{
		runtime:     runtime,
		sessionID:   sessionID,
		react:       react,
		owner:       owner,
		reservation: reservation,
		created:     created,
		delivery:    delivery,
		unsubscribe: unsubscribe,
		done:        make(chan struct{}),
	}
	go func() {
		if owner == nil {
			<-ctx.Done()
			connection.close(true)
			return
		}
		select {
		case <-ctx.Done():
			connection.close(true)
		case <-owner.done:
			connection.close(false)
		}
	}()
	return connection
}

func (c *reActConnection) CreatedRuntime() bool { return c != nil && c.created }
func (c *reActConnection) Done() <-chan struct{} {
	if c == nil {
		done := make(chan struct{})
		close(done)
		return done
	}
	return c.done
}

func (c *reActConnection) Close() error {
	if c != nil {
		c.close(true)
	}
	return nil
}

func (c *reActConnection) close(stopOwned bool) {
	c.closeOnce.Do(func() {
		c.delivery.cancel()
		if stopOwned && !c.created && c.reservation != nil && c.react != nil {
			c.inputMu.Lock()
			c.inputClosed = true
			inputUUIDs := append([]string(nil), c.inputUUIDs...)
			c.inputMu.Unlock()
			// Task status is emitted before its deferred timeline persistence and
			// cleanup finish. Keep the scheduler's reservation alive until the task
			// has actually left the shared ReAct runtime.
			for _, inputUUID := range inputUUIDs {
				if err := c.react.CancelTaskByUserInputUUIDAndWait(context.Background(), inputUUID); err != nil {
					log.Warnf("wait for attached ReAct task %s to stop failed: %v", inputUUID, err)
				}
			}
		}
		if c.unsubscribe != nil {
			c.unsubscribe()
		}
		if stopOwned && c.created && c.owner != nil {
			c.owner.cancel()
			<-c.owner.done
		}
		close(c.done)
	})
}

func (c *reActConnection) Send(event *ypb.AIInputEvent) error {
	if c == nil || c.runtime == nil || c.react == nil {
		return utils.Error("AI ReAct connection is closed")
	}
	if event == nil {
		return utils.Error("AI ReAct input event is nil")
	}
	select {
	case <-c.done:
		return utils.Error("AI ReAct connection is closed")
	default:
	}
	if event.GetIsStart() {
		return nil
	}
	if !c.created && event.GetIsSyncMessage() && event.GetSyncType() == aicommon.SYNC_TYPE_RECOVERY_HISTORY {
		return sendAttachedRecoveryHistory(c.delivery.ctx, c.runtime.projectDatabase(), c.delivery.deliverWithResult, c.sessionID, event)
	}
	return c.runtime.sendInput(c, event)
}

func (r *reActSessionRuntime) sendInput(connection *reActConnection, event *ypb.AIInputEvent) error {
	// Match Config.StartEventLoopEx/processInputEvent precedence. Events carrying
	// hot-patch, interactive or sync semantics are not considered task-producing
	// free input even if a malformed client also sets IsFreeInput.
	ordinaryFreeInput := event.GetIsFreeInput() &&
		!event.GetIsConfigHotpatch() &&
		!event.GetIsInteractiveMessage() &&
		!event.GetIsSyncMessage()
	for {
		if err := connection.delivery.ctx.Err(); err != nil {
			return err
		}
		r.mu.Lock()
		entry := r.entryLocked(connection.sessionID)
		if r.allQuiescing || entry.quiescing {
			r.mu.Unlock()
			return utils.Errorf("AI ReAct session is stopping: %s", connection.sessionID)
		}
		if current, ok := aireact.GetRunningSession(connection.sessionID); !ok || current != connection.react {
			r.mu.Unlock()
			return utils.Errorf("AI ReAct session is no longer running: %s", connection.sessionID)
		}
		if ordinaryFreeInput && entry.reservation != nil && entry.reservation != connection.reservation {
			changed := r.changed
			r.mu.Unlock()
			if err := waitForReActRuntimeChange(connection.delivery.ctx, changed); err != nil {
				return err
			}
			continue
		}
		if connection.reservation != nil && entry.reservation != connection.reservation {
			r.mu.Unlock()
			return utils.Errorf("AI ReAct session reservation expired: %s", connection.sessionID)
		}
		entry.admitting++
		if ordinaryFreeInput {
			entry.taskAdmitting++
		}
		r.notifyLocked()
		r.mu.Unlock()

		var err error
		if ordinaryFreeInput || event.GetIsConfigHotpatch() {
			err = connection.react.SendInputEventAndWaitAccepted(event)
		} else {
			err = connection.react.SendInputEvent(event)
		}

		r.mu.Lock()
		entry = r.entries[connection.sessionID]
		if entry != nil && entry.admitting > 0 {
			entry.admitting--
			if ordinaryFreeInput && entry.taskAdmitting > 0 {
				entry.taskAdmitting--
			}
			r.cleanupEntryLocked(connection.sessionID, entry)
			r.notifyLocked()
		}
		r.mu.Unlock()
		if err == nil && ordinaryFreeInput && connection.reservation != nil {
			for _, resource := range event.GetAttachedResourceInfo() {
				if resource.GetType() == aicommon.USER_FREE_INPUT_UUID && strings.TrimSpace(resource.GetValue()) != "" {
					inputUUID := strings.TrimSpace(resource.GetValue())
					connection.inputMu.Lock()
					closed := connection.inputClosed
					if !closed {
						connection.inputUUIDs = append(connection.inputUUIDs, inputUUID)
					}
					connection.inputMu.Unlock()
					if closed {
						connection.react.CancelTaskByUserInputUUID(inputUUID)
					}
					break
				}
			}
		}
		return err
	}
}

func (r *reActSessionRuntime) QuiesceSessions(ctx context.Context, sessionIDs []string) (SessionQuiescence, error) {
	// This generalizes the scheduler's former cancelSessionExecutionsAndWait:
	// session data is not removed until every matching reservation, start attempt,
	// owned ReAct runtime and in-flight input admission has stopped.
	ids := make([]string, 0, len(sessionIDs))
	seen := make(map[string]struct{}, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		sessionID = normalizeReActSessionID(sessionID)
		if _, ok := seen[sessionID]; ok {
			continue
		}
		seen[sessionID] = struct{}{}
		ids = append(ids, sessionID)
	}
	return r.quiesce(ctx, ids, false)
}

func (r *reActSessionRuntime) QuiesceAll(ctx context.Context) (SessionQuiescence, error) {
	return r.quiesce(ctx, nil, true)
}

func (r *reActSessionRuntime) quiesce(ctx context.Context, ids []string, all bool) (SessionQuiescence, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	waitCtx, cancel := context.WithTimeout(ctx, reActSessionStopTimeout)
	defer cancel()

	r.mu.Lock()
	if r.allQuiescing {
		r.mu.Unlock()
		return nil, utils.Error("all AI ReAct sessions are already stopping")
	}
	if all {
		ids = ids[:0]
		for sessionID := range r.entries {
			ids = append(ids, sessionID)
		}
		for _, sessionID := range aireact.EnumerateStartingSessions() {
			sessionID = normalizeReActSessionID(sessionID)
			if _, ok := r.entries[sessionID]; !ok {
				r.entries[sessionID] = &reActSessionState{}
				ids = append(ids, sessionID)
			}
		}
		for _, sessionID := range aireact.EnumerateRunningSessions() {
			sessionID = normalizeReActSessionID(sessionID)
			if _, ok := r.entries[sessionID]; !ok {
				r.entries[sessionID] = &reActSessionState{}
				ids = append(ids, sessionID)
			}
		}
	}
	for _, sessionID := range ids {
		entry := r.entryLocked(sessionID)
		if entry.quiescing {
			r.mu.Unlock()
			return nil, utils.Errorf("AI ReAct session is already stopping: %s", sessionID)
		}
		if entry.starting == nil && aireact.IsSessionStarting(sessionID) {
			r.mu.Unlock()
			return nil, utils.Errorf("AI ReAct session %s is starting outside this runtime", sessionID)
		}
		if entry.runtime == nil {
			if _, running := aireact.GetRunningSession(sessionID); running && entry.starting == nil {
				r.mu.Unlock()
				return nil, utils.Errorf("AI ReAct session %s is running outside this runtime", sessionID)
			}
		}
	}
	if all {
		r.allQuiescing = true
	}

	waits := make([]<-chan struct{}, 0, len(ids)*2)
	cancels := make([]context.CancelFunc, 0, len(ids)*2)
	for _, sessionID := range ids {
		entry := r.entryLocked(sessionID)
		entry.quiescing = true
		if entry.reservation != nil {
			cancels = append(cancels, entry.reservation.cancel)
			waits = append(waits, entry.reservation.done)
		}
		if entry.starting != nil {
			cancels = append(cancels, entry.starting.cancel)
			waits = append(waits, entry.starting.done)
		}
		if entry.runtime != nil {
			cancels = append(cancels, entry.runtime.cancel)
			waits = append(waits, entry.runtime.done)
		}
	}
	r.notifyLocked()
	r.mu.Unlock()

	for _, cancelFn := range cancels {
		cancelFn()
	}
	for {
		r.mu.Lock()
		admitting := false
		for _, sessionID := range ids {
			if entry := r.entries[sessionID]; entry != nil && entry.admitting > 0 {
				admitting = true
				break
			}
		}
		changed := r.changed
		r.mu.Unlock()
		if !admitting {
			break
		}
		if err := waitForReActRuntimeChange(waitCtx, changed); err != nil {
			r.releaseQuiescence(ids, all)
			return nil, utils.Errorf("wait for AI ReAct input admission to stop failed: %v", err)
		}
	}
	for _, done := range waits {
		select {
		case <-done:
		case <-waitCtx.Done():
			r.releaseQuiescence(ids, all)
			return nil, utils.Errorf("wait for AI ReAct session shutdown failed: %v", waitCtx.Err())
		}
	}
	return &reActSessionQuiescence{runtime: r, ids: ids, all: all}, nil
}

func (r *reActSessionRuntime) releaseQuiescence(ids []string, all bool) {
	r.mu.Lock()
	if all {
		r.allQuiescing = false
	}
	for _, sessionID := range ids {
		entry := r.entries[sessionID]
		if entry == nil {
			continue
		}
		entry.quiescing = false
		r.cleanupEntryLocked(sessionID, entry)
	}
	r.notifyLocked()
	r.mu.Unlock()
}
