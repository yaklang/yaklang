package yakit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	// set broadcast schema
	schema.SetBroadCast_Data(BroadcastData)
}

const (
	serverPushQueueCapacity  = 128
	serverPushEnqueueTimeout = 50 * time.Millisecond
)

type serverPushDescription struct {
	Name string

	queue chan *ypb.DuplexConnectionResponse
	done  chan struct{}
	send  func(*ypb.DuplexConnectionResponse) error

	stopOnce sync.Once
	pending  map[string]*ypb.DuplexConnectionResponse
	pendingM sync.Mutex

	subscriptions  map[string]struct{}
	subscriptionsM sync.RWMutex
}

var (
	serverPushMutex    = new(sync.RWMutex)
	serverPushCallback = make(map[string]*serverPushDescription)

	broadcastWithTypeMutex   = new(sync.Mutex)
	broadcastTypeCallerTable = make(map[string]func(func()))

	signalWithTypeMutex   = new(sync.Mutex)
	signalTypeCallerTable = make(map[string]func(func()))

	ServerPushType_Global           = "global"
	ServerPushType_HttpFlow         = "httpflow"
	ServerPushType_YakScript        = "yakscript"
	ServerPushType_Risk             = "risk"
	ServerPushType_AIMemory         = "ai_memory"
	ServerPushType_File_Monitor     = "file_monitor"
	ServerPushType_Error            = "error"
	ServerPushType_Warning          = "warning"
	ServerPushType_RPS              = "rps"
	ServerPushType_CPS              = "cps"
	ServerPushType_Fuzzer           = "fuzzer_server_push"
	ServerPushType_WebFuzzerTab     = "web_fuzzer_tab"
	ServerPushType_Project          = "project"
	ServerPushType_OpenAPIParse     = "openapi_parse"
	ServerPushType_BrowserExtension = "browser_extension"

	ProjectPushActionPromptEnter = "prompt_enter"
	ProjectPushActionAutoEnter   = "auto_enter"
	ServerPushType_SlowInsertSQL = "httpflow_slow_insert_sql"
	ServerPushType_SlowQuerySQL  = "httpflow_slow_query_sql"
	ServerPushType_SlowRuleHook  = "mitm_slow_rule_hook"
)

func newServerPushDescription(
	name string,
	queueCapacity int,
	send func(*ypb.DuplexConnectionResponse) error,
) *serverPushDescription {
	if queueCapacity < 1 {
		queueCapacity = 1
	}
	return &serverPushDescription{
		Name:          name,
		queue:         make(chan *ypb.DuplexConnectionResponse, queueCapacity),
		done:          make(chan struct{}),
		send:          send,
		pending:       make(map[string]*ypb.DuplexConnectionResponse),
		subscriptions: make(map[string]struct{}),
	}
}

func (s *serverPushDescription) setSubscription(name string, enabled bool) {
	if name == "" {
		return
	}
	s.subscriptionsM.Lock()
	defer s.subscriptionsM.Unlock()
	if enabled {
		s.subscriptions[name] = struct{}{}
		return
	}
	delete(s.subscriptions, name)
}

func (s *serverPushDescription) hasSubscription(name string) bool {
	s.subscriptionsM.RLock()
	defer s.subscriptionsM.RUnlock()
	_, ok := s.subscriptions[name]
	return ok
}

func (s *serverPushDescription) stop() {
	s.stopOnce.Do(func() {
		close(s.done)
	})
}

func (s *serverPushDescription) next(ctx context.Context) (*ypb.DuplexConnectionResponse, bool) {
	select {
	case <-ctx.Done():
		return nil, false
	case <-s.done:
		return nil, false
	default:
	}

	// Preserve FIFO delivery for queued frames. Coalesced invalidations were
	// received only after this queue filled, so they must be delivered later.
	select {
	case response := <-s.queue:
		return response, true
	default:
	}

	s.pendingM.Lock()
	for messageType, response := range s.pending {
		delete(s.pending, messageType)
		s.pendingM.Unlock()
		return response, true
	}
	s.pendingM.Unlock()

	select {
	case <-ctx.Done():
		return nil, false
	case <-s.done:
		return nil, false
	case response := <-s.queue:
		return response, true
	}
}

func (s *serverPushDescription) run(ctx context.Context) {
	defer s.stop()
	for {
		response, ok := s.next(ctx)
		if !ok {
			return
		}
		if err := s.send(response); err != nil {
			return
		}
	}
}

func isCoalescibleServerPush(messageType string) bool {
	switch messageType {
	case ServerPushType_Global,
		ServerPushType_HttpFlow,
		ServerPushType_HTTPFlowCommitted,
		ServerPushType_YakScript,
		ServerPushType_Risk,
		ServerPushType_AIMemory,
		ServerPushType_RPS,
		ServerPushType_CPS:
		return true
	default:
		return false
	}
}

func (s *serverPushDescription) enqueue(response *ypb.DuplexConnectionResponse) bool {
	if response == nil {
		return false
	}
	select {
	case <-s.done:
		return false
	default:
	}
	select {
	case s.queue <- response:
		return true
	default:
	}

	if isCoalescibleServerPush(response.GetMessageType()) {
		s.pendingM.Lock()
		s.pending[response.GetMessageType()] = response
		s.pendingM.Unlock()
		return true
	}

	timer := time.NewTimer(serverPushEnqueueTimeout)
	defer timer.Stop()
	select {
	case <-s.done:
		return false
	case s.queue <- response:
		return true
	case <-timer.C:
		log.Warnf("drop server push frame for slow client %s: type=%s queue_capacity=%d",
			s.Name, response.GetMessageType(), cap(s.queue))
		return false
	}
}

const broadcastThrottleInterval = time.Second

// newLeadingTrailingThrottle sends the first invalidation immediately and
// keeps only the newest callback for the end of each throttle window. This is
// used for HTTP flow creation notifications so a burst cannot lose its final
// wake-up while still bounding the notification rate.
func newLeadingTrailingThrottle(wait time.Duration) func(func()) {
	var mutex sync.Mutex
	var timer *time.Timer
	var pending func()
	var flush func()

	flush = func() {
		mutex.Lock()
		callback := pending
		pending = nil
		if callback == nil {
			timer = nil
			mutex.Unlock()
			return
		}
		timer = time.AfterFunc(wait, flush)
		mutex.Unlock()
		callback()
	}

	return func(callback func()) {
		mutex.Lock()
		if timer == nil {
			timer = time.AfterFunc(wait, flush)
			mutex.Unlock()
			callback()
			return
		}
		pending = callback
		mutex.Unlock()
	}
}

func newBroadcastTypeCaller(typeString string, wait time.Duration) func(func()) {
	if typeString == ServerPushType_HttpFlow {
		return newLeadingTrailingThrottle(wait)
	}
	return utils.NewThrottle(wait.Seconds())
}

const (
	WebFuzzerTabPushActionCreate = "create"
	WebFuzzerTabPushActionUpdate = "update"
	WebFuzzerTabPushActionDelete = "delete"
)

type WebFuzzerTabPush struct {
	Action      string              `json:"action,omitempty"`
	OpenFlag    bool                `json:"openFlag"` // 创建 Web Fuzzer Tab 之后，要不要把左侧一级菜单切到「Web Fuzzer」并聚焦新 Tab
	Data        []*ypb.FuzzerConfig `json:"data,omitempty"`
	ChangedData []*ypb.FuzzerConfig `json:"changedData,omitempty"`
	PageIDs     []string            `json:"pageIds,omitempty"`
}

type ProjectPush struct {
	Action      string `json:"action"`
	ID          int64  `json:"id"`
	ProjectName string `json:"project_name,omitempty"`
	Type        string `json:"type,omitempty"`
}

func BroadcastWebFuzzerTab(openFlag bool, data ...*ypb.FuzzerConfig) {
	if len(data) == 0 {
		return
	}
	BroadcastData(ServerPushType_WebFuzzerTab, &WebFuzzerTabPush{
		Action:   WebFuzzerTabPushActionCreate,
		OpenFlag: openFlag,
		Data:     data,
	})
}

// BroadcastWebFuzzerTabChanged notifies action-aware Yakit clients about
// updates and deletions. ChangedData intentionally uses a different JSON field
// from Data so older clients do not mistake an update for a newly created tab.
func BroadcastWebFuzzerTabChanged(action string, openFlag bool, changedData []*ypb.FuzzerConfig, pageIDs []string) {
	if action != WebFuzzerTabPushActionUpdate && action != WebFuzzerTabPushActionDelete {
		return
	}
	if len(changedData) == 0 && len(pageIDs) == 0 {
		return
	}
	BroadcastData(ServerPushType_WebFuzzerTab, &WebFuzzerTabPush{
		Action:      action,
		OpenFlag:    openFlag,
		ChangedData: changedData,
		PageIDs:     pageIDs,
	})
}

func BroadcastProjectChanged(action string, id int64, projectName, projectType string) {
	if action == "" || id <= 0 {
		return
	}
	BroadcastData(ServerPushType_Project, &ProjectPush{
		Action:      action,
		ID:          id,
		ProjectName: projectName,
		Type:        projectType,
	})
}

func RegisterServerPushCallback(id string, stream ypb.Yak_DuplexConnectionServer) {
	registerServerPushCallback(id, stream.Context(), serverPushQueueCapacity, stream.Send)
}

func registerServerPushCallback(
	id string,
	ctx context.Context,
	queueCapacity int,
	send func(*ypb.DuplexConnectionResponse) error,
) {
	description := newServerPushDescription(id, queueCapacity, send)
	serverPushMutex.Lock()
	previous := serverPushCallback[id]
	serverPushCallback[id] = description
	serverPushMutex.Unlock()
	if previous != nil {
		previous.stop()
	}
	log.Infof("Register server push callback: %v", id)
	go description.run(ctx)
}

func UnRegisterServerPushCallback(id string) {
	serverPushMutex.Lock()
	description := serverPushCallback[id]
	delete(serverPushCallback, id)
	serverPushMutex.Unlock()
	if description != nil {
		description.stop()
	}
	log.Infof("UnRegister server push callback: %v", id)
}

func SetServerPushSubscription(id, subscription string, enabled bool) bool {
	serverPushMutex.RLock()
	description := serverPushCallback[id]
	serverPushMutex.RUnlock()
	if description == nil {
		return false
	}
	description.setSubscription(subscription, enabled)
	return true
}

func broadcastRaw(data *ypb.DuplexConnectionResponse) {
	serverPushMutex.RLock()
	callbacks := make([]*serverPushDescription, 0, len(serverPushCallback))
	for _, item := range serverPushCallback {
		callbacks = append(callbacks, item)
	}
	serverPushMutex.RUnlock()

	for _, item := range callbacks {
		item.enqueue(data)
	}
}

func broadcastRawToSubscribers(subscription string, data *ypb.DuplexConnectionResponse) {
	callbacks := snapshotServerPushSubscribers(subscription)
	for _, item := range callbacks {
		item.enqueue(data)
	}
}

func snapshotServerPushSubscribers(subscription string) []*serverPushDescription {
	serverPushMutex.RLock()
	callbacks := make([]*serverPushDescription, 0, len(serverPushCallback))
	for _, item := range serverPushCallback {
		if item.hasSubscription(subscription) {
			callbacks = append(callbacks, item)
		}
	}
	serverPushMutex.RUnlock()
	return callbacks
}

func BroadcastDataToSubscribers(subscription, typeString string, msg any) {
	broadcastDataToSubscribersLazy(subscription, typeString, func() any { return msg })
}

// broadcastDataToSubscribersLazy avoids constructing and serializing a
// high-frequency frame when no connected client negotiated the subscription.
func broadcastDataToSubscribersLazy(subscription, typeString string, build func() any) bool {
	if subscription == "" || typeString == "" {
		return false
	}
	callbacks := snapshotServerPushSubscribers(subscription)
	if len(callbacks) == 0 || build == nil {
		return false
	}
	data := &ypb.DuplexConnectionResponse{
		Data:        utils.Jsonify(build()),
		MessageType: typeString,
		Timestamp:   time.Now().UnixNano(),
	}
	for _, item := range callbacks {
		item.enqueue(data)
	}
	return true
}

func BroadcastData(typeString string, msg any) {
	broadcastWithTypeMutex.Lock()
	defer broadcastWithTypeMutex.Unlock()

	jsonMsg := utils.Jsonify(msg)
	data := &ypb.DuplexConnectionResponse{
		Data:        jsonMsg,
		MessageType: typeString,
		Timestamp:   time.Now().UnixNano(),
	}
	switch msg.(type) {
	case string:
	default:
		// if complex object, broadcast now, no need to throttle
		broadcastRaw(data)
		return
	}

	hash := utils.CalcMd5(typeString, jsonMsg)

	if caller, ok := broadcastTypeCallerTable[hash]; ok {
		caller(func() {
			broadcastRaw(data)
		})
	} else {
		broadcastTypeCallerTable[hash] = newBroadcastTypeCaller(typeString, broadcastThrottleInterval)
		broadcastTypeCallerTable[hash](func() {
			broadcastRaw(data)
		})
	}
}

func signalRaw(id string, data *ypb.DuplexConnectionResponse) {
	serverPushMutex.RLock()
	callback := serverPushCallback[id]
	serverPushMutex.RUnlock()
	if callback != nil {
		callback.enqueue(data)
	}
}

func SignalDate(id string, typeString string, data any) {
	signalWithTypeMutex.Lock()
	defer signalWithTypeMutex.Unlock()

	Data := &ypb.DuplexConnectionResponse{
		Data:        utils.Jsonify(data),
		MessageType: typeString,
		Timestamp:   time.Now().UnixNano(),
	}
	signalIndex := fmt.Sprintf("%s_%s", id, typeString)

	if caller, ok := signalTypeCallerTable[signalIndex]; ok {
		caller(func() {
			signalRaw(id, Data)
		})
	} else {
		signalTypeCallerTable[signalIndex] = utils.NewThrottle(1)
		signalTypeCallerTable[signalIndex](func() {
			signalRaw(id, Data)
		})
	}
}
