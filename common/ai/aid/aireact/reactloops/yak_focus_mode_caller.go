package reactloops

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

// FocusModeYakHookCaller 封装一个独立的 Yak 引擎实例，用于专注模式 (Yak Focus Mode)
// 的 MITM 风格回调。每个 ReActLoop 实例都应当持有一个独立的 caller，避免并发污染。
//
// 关键词: yak focus mode caller, isolated yak engine, focus hooks dispatch
//
// 使用模式：
//  1. 通过 NewFocusModeYakHookCaller 创建（内部 ImportLibs + SafeEval bundle）。
//  2. 通过 GetVar / GetString / GetBool / GetInt 读取声明式 __DUNDER__ 常量。
//  3. 通过 HasHook / CallByName 调用 focusXxx 钩子。
//  4. 生命周期结束时调用 Close 释放 ctx + cancel。
type FocusModeYakHookCaller struct {
	sourceName  string
	bundleCode  string
	engine      *antlr4yak.Engine
	rootCtx     context.Context
	cancel      context.CancelFunc
	callTimeout time.Duration

	mu     sync.Mutex
	closed bool
}

// FocusModeCallerOption 调整 FocusModeYakHookCaller 行为
type FocusModeCallerOption func(*focusModeCallerConfig)

type focusModeCallerConfig struct {
	callTimeout time.Duration
	extraVars   map[string]any
	parentCtx   context.Context
}

// WithFocusModeCallerCallTimeout 设置每次 hook 调用的超时
func WithFocusModeCallerCallTimeout(d time.Duration) FocusModeCallerOption {
	return func(c *focusModeCallerConfig) {
		if d > 0 {
			c.callTimeout = d
		}
	}
}

// WithFocusModeCallerVars 注入额外的全局变量到 yak engine 中
// 例如：把 invoker / loop / ctx 等运行期对象提前 bind 进 yak script 域。
func WithFocusModeCallerVars(vars map[string]any) FocusModeCallerOption {
	return func(c *focusModeCallerConfig) {
		if c.extraVars == nil {
			c.extraVars = make(map[string]any)
		}
		for k, v := range vars {
			c.extraVars[k] = v
		}
	}
}

// WithFocusModeCallerParentContext 指定父 ctx，caller 自身派生子 ctx，
// 父 ctx 取消时所有 hook 调用被打断
func WithFocusModeCallerParentContext(ctx context.Context) FocusModeCallerOption {
	return func(c *focusModeCallerConfig) {
		c.parentCtx = ctx
	}
}

// NewFocusModeYakHookCaller 创建一个独立的 Yak 引擎实例，并执行 bundleCode
// （主 .ai-focus.yak + 同级 sidekick *.yak 的拼接体）。执行成功后引擎中的 vars
// 表里就会含有所有 __DUNDER__ 常量与 focusXxx 函数。
//
// 关键词: focus mode bundle execution, dunder extraction, focus hook registration
func NewFocusModeYakHookCaller(
	sourceName string,
	bundleCode string,
	opts ...FocusModeCallerOption,
) (*FocusModeYakHookCaller, error) {
	cfg := &focusModeCallerConfig{
		callTimeout: 30 * time.Second,
		parentCtx:   context.Background(),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	rootCtx, cancel := context.WithCancel(cfg.parentCtx)

	engine := yaklang.New()
	if engine == nil {
		cancel()
		return nil, utils.Error("yak focus mode caller: failed to create yak engine")
	}

	if sourceName != "" {
		engine.SetSourceFilePath(sourceName)
	}

	caller := &FocusModeYakHookCaller{
		sourceName:  sourceName,
		bundleCode:  bundleCode,
		engine:      engine,
		rootCtx:     rootCtx,
		cancel:      cancel,
		callTimeout: cfg.callTimeout,
	}

	if len(cfg.extraVars) > 0 {
		engine.SetVars(cfg.extraVars)
	}

	// 在执行 bundle 前确保 caller 自身可用作 panic recovery 的边界
	if err := caller.evalBundle(rootCtx, bundleCode); err != nil {
		cancel()
		return nil, utils.Wrapf(err, "yak focus mode caller: eval bundle for %s failed", sourceName)
	}

	return caller, nil
}

// evalBundle 在 caller 内部安全执行 bundleCode，捕获 panic
func (c *FocusModeYakHookCaller) evalBundle(ctx context.Context, code string) (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = utils.Errorf("yak focus mode caller: panic during eval bundle: %v", r)
		}
	}()
	if err := c.engine.SafeEval(ctx, code); err != nil {
		return err
	}
	return nil
}

// SourceName 返回主脚本名（一般是 *.ai-focus.yak 的相对路径）
func (c *FocusModeYakHookCaller) SourceName() string {
	return c.sourceName
}

// Engine 返回内部 yak engine。仅供高级集成使用，普通调用方应使用 CallByName / GetVar。
func (c *FocusModeYakHookCaller) Engine() *antlr4yak.Engine {
	return c.engine
}

// HasHook 判断给定名称的函数是否在 yak 脚本中定义且为函数类型
func (c *FocusModeYakHookCaller) HasHook(name string) bool {
	if c == nil || c.engine == nil {
		return false
	}
	raw, ok := c.engine.GetVar(name)
	if !ok {
		return false
	}
	_, ok = raw.(*yakvm.Function)
	return ok
}

// GetVar 读取 yak 脚本顶层变量，原值
func (c *FocusModeYakHookCaller) GetVar(name string) (any, bool) {
	if c == nil || c.engine == nil {
		return nil, false
	}
	return c.engine.GetVar(name)
}

// GetString 读取字符串型 dunder 常量；不存在返回空字符串
func (c *FocusModeYakHookCaller) GetString(name string) string {
	v, ok := c.GetVar(name)
	if !ok || utils.IsNil(v) {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return utils.InterfaceToString(v)
}

// GetBool 读取布尔型 dunder 常量
func (c *FocusModeYakHookCaller) GetBool(name string) (bool, bool) {
	v, ok := c.GetVar(name)
	if !ok || utils.IsNil(v) {
		return false, false
	}
	return utils.InterfaceToBoolean(v), true
}

// GetInt 读取整型 dunder 常量
func (c *FocusModeYakHookCaller) GetInt(name string) (int, bool) {
	v, ok := c.GetVar(name)
	if !ok || utils.IsNil(v) {
		return 0, false
	}
	return utils.InterfaceToInt(v), true
}

// GetSlice 读取列表型 dunder 常量（如 __ACTIONS__）
func (c *FocusModeYakHookCaller) GetSlice(name string) []any {
	v, ok := c.GetVar(name)
	if !ok || utils.IsNil(v) {
		return nil
	}
	if t, ok := v.([]any); ok {
		return t
	}
	return utils.InterfaceToSliceInterface(v)
}

// GetMap 读取 map 型 dunder 常量（如 __VARS__）
func (c *FocusModeYakHookCaller) GetMap(name string) map[string]any {
	v, ok := c.GetVar(name)
	if !ok || utils.IsNil(v) {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return utils.InterfaceToMapInterface(v)
}

// CallByName 调用 yak 脚本中名为 name 的 focusXxx 钩子函数。
// 若该函数不存在返回 (nil, errFocusModeHookNotFound)。
// 若调用过程中超时或 panic 返回相应 error。返回 yak 函数的返回值。
//
// 关键词: focus mode hook invocation, panic recovery, ctx timeout
func (c *FocusModeYakHookCaller) CallByName(name string, args ...interface{}) (any, error) {
	if c == nil || c.engine == nil {
		return nil, utils.Error("yak focus mode caller: nil caller or engine")
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, utils.Error("yak focus mode caller: already closed")
	}
	c.mu.Unlock()

	raw, ok := c.engine.GetVar(name)
	if !ok {
		return nil, errFocusModeHookNotFound
	}
	fn, ok := raw.(*yakvm.Function)
	if !ok {
		return nil, utils.Errorf("yak focus mode caller: %q is not a function", name)
	}

	subCtx, cancel := context.WithTimeout(c.rootCtx, c.callTimeout)
	defer cancel()

	resultCh := make(chan any, 1)
	errCh := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				errCh <- utils.Errorf("yak focus mode hook %q panic: %v", name, r)
			}
		}()
		val, err := c.engine.SafeCallYakFunctionNativeWithFrameCallback(subCtx, nil, fn, args...)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- val
	}()

	select {
	case val := <-resultCh:
		return val, nil
	case err := <-errCh:
		return nil, err
	case <-subCtx.Done():
		err := subCtx.Err()
		if errors.Is(err, context.DeadlineExceeded) {
			log.Errorf("yak focus mode hook %q timeout after %v", name, c.callTimeout)
			return nil, utils.Errorf("yak focus mode hook %q timeout after %v", name, c.callTimeout)
		}
		return nil, err
	}
}

// CallByNameIgnoreNotFound 与 CallByName 类似，但当函数不存在时返回 (nil, nil)
// 便于 hook 是可选的场景使用
func (c *FocusModeYakHookCaller) CallByNameIgnoreNotFound(name string, args ...interface{}) (any, error) {
	val, err := c.CallByName(name, args...)
	if err != nil && errors.Is(err, errFocusModeHookNotFound) {
		return nil, nil
	}
	return val, err
}

// CallFunction 直接调用一个已经从 yak engine 中取出的函数对象，与 CallByName
// 的区别是：本方法允许调用嵌入在 dict / 列表中的匿名闭包（例如
// __ACTIONS__ 列表里某条 action 的 verifier 字段）。
//
// 关键词: yak focus mode call closure, embedded function invocation
func (c *FocusModeYakHookCaller) CallFunction(label string, fn *yakvm.Function, args ...interface{}) (any, error) {
	if c == nil || c.engine == nil {
		return nil, utils.Error("yak focus mode caller: nil caller or engine")
	}
	if fn == nil {
		return nil, utils.Errorf("yak focus mode caller: function %q is nil", label)
	}

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, utils.Error("yak focus mode caller: already closed")
	}
	c.mu.Unlock()

	subCtx, cancel := context.WithTimeout(c.rootCtx, c.callTimeout)
	defer cancel()

	resultCh := make(chan any, 1)
	errCh := make(chan error, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				errCh <- utils.Errorf("yak focus mode closure %q panic: %v", label, r)
			}
		}()
		val, err := c.engine.SafeCallYakFunctionNativeWithFrameCallback(subCtx, nil, fn, args...)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- val
	}()

	select {
	case val := <-resultCh:
		return val, nil
	case err := <-errCh:
		return nil, err
	case <-subCtx.Done():
		err := subCtx.Err()
		if errors.Is(err, context.DeadlineExceeded) {
			log.Errorf("yak focus mode closure %q timeout after %v", label, c.callTimeout)
			return nil, utils.Errorf("yak focus mode closure %q timeout after %v", label, c.callTimeout)
		}
		return nil, err
	}
}

// Close 取消 caller 的 ctx，使所有 hook 调用立即结束
func (c *FocusModeYakHookCaller) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	if c.cancel != nil {
		c.cancel()
	}
}

// errFocusModeHookNotFound 当 yak 脚本里没有定义某个 focusXxx 钩子函数时返回此错误。
var errFocusModeHookNotFound = errors.New("yak focus mode hook not found")

// IsFocusModeHookNotFound 判断 err 是否为 hook 不存在错误
func IsFocusModeHookNotFound(err error) bool {
	return errors.Is(err, errFocusModeHookNotFound)
}

// BundleSidekicks 将主入口 yak 代码与 sidekick 代码按顺序拼接成一份 bundle。
// sidekicks 中的每个元素是一段独立 yak 代码（已读取的内容），它们会以注释 banner
// 的形式与主代码合并在同一个执行上下文中。
//
// 关键词: focus mode bundle, sidekick concatenate, single engine context
func BundleSidekicks(mainCode string, sidekicks ...string) string {
	if len(sidekicks) == 0 {
		return mainCode
	}
	var combined string
	for i, sk := range sidekicks {
		if sk == "" {
			continue
		}
		combined += fmt.Sprintf("// ===== sidekick #%d ===== //\n%s\n\n", i, sk)
	}
	combined += "// ===== main focus entry ===== //\n"
	combined += mainCode
	return combined
}
