package contextmenu

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	PluginType               = "context-menu"
	LegacyPluginType         = "codec"
	MaxCustomPluginsPerScene = 15

	ActionHistorySingle = "history-single"
	ActionHistoryMulti  = "history-multi"
	ActionHTTPPacket    = "http-packet"

	ActionLegacyHistorySingle = "legacy-codec-history-single"
	ActionLegacyHistoryMulti  = "legacy-codec-history-multi"
	ActionLegacyPacketContext = "legacy-codec-http-packet-context"
	ActionLegacyPacketMutate  = "legacy-codec-http-packet-mutate"

	LegacyTagHistorySingle = "allow-custom-single-history-mutate"
	LegacyTagHistoryMulti  = "allow-custom-multiple-history-mutate"
	LegacyTagPacketContext = "allow-custom-context-menu-execute"
	LegacyTagPacketMutate  = "allow-custom-http-packet-mutate"

	ExecutionTypeContextMenu         = "context-menu"
	ExecutionTypeLegacyHistory       = "legacy-codec-history"
	ExecutionTypeLegacyPacketContext = "legacy-codec-context"
	ExecutionTypeLegacyPacketMutate  = "legacy-codec-mutate"

	HookHistorySingle = "handleOneHTTPFlow"
	HookHistoryMulti  = "handleMultiHTTPFlows"
	HookHTTPPacket    = "handleHTTPPacket"

	ResultModeAuto   = "auto"
	ResultModeDialog = "dialog"
	ResultModeDrawer = "drawer"
	ResultModeTab    = "tab"
)

type HttpsState string

const (
	HttpsStateUnknown HttpsState = "unknown"
	HttpsStateHTTP    HttpsState = "http"
	HttpsStateHTTPS   HttpsState = "https"
	HttpsStateMixed   HttpsState = "mixed"
)

type ActionContextOptions struct {
	Scene          string
	Source         string
	Trigger        string
	HttpsState     HttpsState
	HasRequest     bool
	HasResponse    bool
	RequestSize    int64
	ResponseSize   int64
	SelectionCount int
	Params         map[string]any
	RuntimeID      string
	PluginUUID     string
	PluginName     string
	ActionID       string
}

// ActionContext is the read-only invocation context passed as the first
// argument to every context-menu hook. It deliberately exposes execution
// context only; UI and binding management stay outside this object.
type ActionContext struct {
	context.Context

	scene          string
	source         string
	trigger        string
	httpsState     HttpsState
	hasRequest     bool
	hasResponse    bool
	requestSize    int64
	responseSize   int64
	selectionCount int
	params         map[string]any
	runtimeID      string
	pluginUUID     string
	pluginName     string
	actionID       string
}

func NewActionContext(parent context.Context, options ActionContextOptions) *ActionContext {
	if parent == nil {
		parent = context.Background()
	}
	params := make(map[string]any, len(options.Params))
	for key, value := range options.Params {
		params[key] = value
	}
	return &ActionContext{
		Context:        parent,
		scene:          options.Scene,
		source:         options.Source,
		trigger:        options.Trigger,
		httpsState:     NormalizeHttpsState(options.HttpsState),
		hasRequest:     options.HasRequest,
		hasResponse:    options.HasResponse,
		requestSize:    options.RequestSize,
		responseSize:   options.ResponseSize,
		selectionCount: options.SelectionCount,
		params:         params,
		runtimeID:      options.RuntimeID,
		pluginUUID:     options.PluginUUID,
		pluginName:     options.PluginName,
		actionID:       options.ActionID,
	}
}

func (c *ActionContext) Scene() string {
	if c == nil {
		return ""
	}
	return c.scene
}

func (c *ActionContext) Source() string {
	if c == nil {
		return ""
	}
	return c.source
}

func (c *ActionContext) Trigger() string {
	if c == nil {
		return ""
	}
	return c.trigger
}

func (c *ActionContext) HttpsState() string {
	if c == nil {
		return string(HttpsStateUnknown)
	}
	return string(c.httpsState)
}

func (c *ActionContext) HasHttpsInfo() bool {
	return c != nil && c.httpsState != HttpsStateUnknown
}

func (c *ActionContext) IsHttps() bool {
	return c != nil && c.httpsState == HttpsStateHTTPS
}

func (c *ActionContext) ContainsHttps() bool {
	return c != nil && (c.httpsState == HttpsStateHTTPS || c.httpsState == HttpsStateMixed)
}

func (c *ActionContext) HasRequest() bool {
	return c != nil && c.hasRequest
}

func (c *ActionContext) HasResponse() bool {
	return c != nil && c.hasResponse
}

func (c *ActionContext) RequestSize() int64 {
	if c == nil {
		return 0
	}
	return c.requestSize
}

func (c *ActionContext) ResponseSize() int64 {
	if c == nil {
		return 0
	}
	return c.responseSize
}

func (c *ActionContext) SelectionCount() int {
	if c == nil {
		return 0
	}
	return c.selectionCount
}

func (c *ActionContext) RuntimeID() string {
	if c == nil {
		return ""
	}
	return c.runtimeID
}

func (c *ActionContext) PluginUUID() string {
	if c == nil {
		return ""
	}
	return c.pluginUUID
}

func (c *ActionContext) PluginName() string {
	if c == nil {
		return ""
	}
	return c.pluginName
}

func (c *ActionContext) ActionID() string {
	if c == nil {
		return ""
	}
	return c.actionID
}

func (c *ActionContext) HasParam(name string) bool {
	if c == nil {
		return false
	}
	_, ok := c.params[name]
	return ok
}

func (c *ActionContext) Param(name string) any {
	if c == nil {
		return nil
	}
	return c.params[name]
}

func (c *ActionContext) ParamString(name string) string {
	value := c.Param(name)
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func (c *ActionContext) ParamBool(name string) bool {
	value := c.Param(name)
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(typed))
		return parsed
	default:
		parsed, _ := strconv.ParseBool(fmt.Sprint(value))
		return parsed
	}
}

func (c *ActionContext) ParamInt(name string) int64 {
	value := c.Param(name)
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed
	default:
		parsed, _ := strconv.ParseInt(fmt.Sprint(value), 10, 64)
		return parsed
	}
}

func (c *ActionContext) Params() map[string]any {
	if c == nil {
		return nil
	}
	result := make(map[string]any, len(c.params))
	for key, value := range c.params {
		result[key] = value
	}
	return result
}

// The explicit methods below keep ActionContext safe even when a nil pointer
// is passed through reflective Yak calls.
func (c *ActionContext) Deadline() (time.Time, bool) {
	if c == nil || c.Context == nil {
		return time.Time{}, false
	}
	return c.Context.Deadline()
}

func (c *ActionContext) Done() <-chan struct{} {
	if c == nil || c.Context == nil {
		return nil
	}
	return c.Context.Done()
}

func (c *ActionContext) Err() error {
	if c == nil || c.Context == nil {
		return nil
	}
	return c.Context.Err()
}

func (c *ActionContext) Value(key any) any {
	if c == nil || c.Context == nil {
		return nil
	}
	return c.Context.Value(key)
}

func NormalizeHttpsState(state HttpsState) HttpsState {
	switch HttpsState(strings.ToLower(strings.TrimSpace(string(state)))) {
	case HttpsStateHTTP:
		return HttpsStateHTTP
	case HttpsStateHTTPS:
		return HttpsStateHTTPS
	case HttpsStateMixed:
		return HttpsStateMixed
	default:
		return HttpsStateUnknown
	}
}

func IsValidResultMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", ResultModeAuto, ResultModeDialog, ResultModeDrawer, ResultModeTab:
		return true
	default:
		return false
	}
}

func NormalizeResultMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return ResultModeAuto
	}
	return mode
}

func IsKnownAction(actionID string) bool {
	_, ok := HookForAction(actionID)
	return ok
}

func IsKnownScene(scene string) bool {
	return IsKnownAction(scene)
}

func IsKnownBindingAction(actionID string) bool {
	return IsKnownAction(actionID) || IsLegacyAction(actionID)
}

func IsLegacyAction(actionID string) bool {
	switch actionID {
	case ActionLegacyHistorySingle, ActionLegacyHistoryMulti, ActionLegacyPacketContext, ActionLegacyPacketMutate:
		return true
	default:
		return false
	}
}

func SceneForAction(actionID string) (string, bool) {
	switch actionID {
	case ActionHistorySingle, ActionLegacyHistorySingle:
		return ActionHistorySingle, true
	case ActionHistoryMulti, ActionLegacyHistoryMulti:
		return ActionHistoryMulti, true
	case ActionHTTPPacket, ActionLegacyPacketContext, ActionLegacyPacketMutate:
		return ActionHTTPPacket, true
	default:
		return "", false
	}
}

func ExecutionTypeForAction(actionID string) (string, bool) {
	switch actionID {
	case ActionHistorySingle, ActionHistoryMulti, ActionHTTPPacket:
		return ExecutionTypeContextMenu, true
	case ActionLegacyHistorySingle, ActionLegacyHistoryMulti:
		return ExecutionTypeLegacyHistory, true
	case ActionLegacyPacketContext:
		return ExecutionTypeLegacyPacketContext, true
	case ActionLegacyPacketMutate:
		return ExecutionTypeLegacyPacketMutate, true
	default:
		return "", false
	}
}

func LegacyActionsForTags(tags string) []string {
	pairs := []struct {
		tag      string
		actionID string
	}{
		{LegacyTagHistorySingle, ActionLegacyHistorySingle},
		{LegacyTagHistoryMulti, ActionLegacyHistoryMulti},
		{LegacyTagPacketContext, ActionLegacyPacketContext},
		{LegacyTagPacketMutate, ActionLegacyPacketMutate},
	}
	actions := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		if HasTag(tags, pair.tag) {
			actions = append(actions, pair.actionID)
		}
	}
	return actions
}

func HasTag(tags, expected string) bool {
	for _, tag := range strings.Split(tags, ",") {
		if strings.TrimSpace(tag) == expected {
			return true
		}
	}
	return false
}

func LegacyScriptImplements(tags, actionID string) bool {
	for _, capability := range LegacyActionsForTags(tags) {
		if capability == actionID {
			return true
		}
	}
	return false
}

func LegacyActionName(actionID string) string {
	switch actionID {
	case ActionLegacyHistorySingle:
		return "History 单选"
	case ActionLegacyHistoryMulti:
		return "History 多选"
	case ActionLegacyPacketContext:
		return "数据包右键"
	case ActionLegacyPacketMutate:
		return "HTTP 数据包变形"
	default:
		return ""
	}
}

func IsKnownHook(hookName string) bool {
	switch hookName {
	case HookHistorySingle, HookHistoryMulti, HookHTTPPacket:
		return true
	default:
		return false
	}
}

func HookForAction(actionID string) (string, bool) {
	switch actionID {
	case ActionHistorySingle:
		return HookHistorySingle, true
	case ActionHistoryMulti:
		return HookHistoryMulti, true
	case ActionHTTPPacket:
		return HookHTTPPacket, true
	default:
		return "", false
	}
}

func ExpectedParameterCount(actionID string) int {
	switch actionID {
	case ActionHistorySingle, ActionHistoryMulti:
		return 2
	case ActionHTTPPacket:
		return 3
	default:
		return 0
	}
}

func KnownActions() []string {
	return []string{ActionHistorySingle, ActionHistoryMulti, ActionHTTPPacket}
}
