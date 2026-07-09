package sfvm

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/common/utils/omap"
)

func NewConfig(opts ...Option) *Config {
	c := &Config{
		ctx:      context.Background(),
		FailFast: true,
		Mutex:    sync.Mutex{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type ResultCapturedCallback func(name string, results Values) error

type Config struct {
	debug                     bool
	StrictMatch               bool
	FailFast                  bool
	initialContextVars        *omap.OrderedMap[string, Values]
	onResultCapturedCallbacks []ResultCapturedCallback
	ctx                       context.Context
	processCallback           func(idx int, msg string)
	Mutex                     sync.Mutex

	RuntimeOptions []any

	diagnosticsEnabled  bool
	diagnosticsRecorder *diagnostics.Recorder

	// workBudget bounds total fanout work in one rule execution. Heavy rules on
	// large projects drive SF opcode fanout (e.g. <typeName> over tens of
	// thousands of matched values) where each element does MergeAnchor(Clone) +
	// AppendPredecessor; without a structural bound the only limit is the
	// per-rule wall-clock (--rule-timeout, default 4h), so heavy rules hang for
	// hours. The budget is incremented per element in the hot fanout native
	// calls (sf_native_call.go) via EnterWork(); when it exceeds Limit the rule
	// ctx is cancelled (cancel) so the existing ctx-bail path surfaces partial
	// results instead of a hard failure. nil = no budget (legacy behavior).
	workBudget *RuleWorkBudget
}

// RuleWorkBudget is a per-rule atomic counter+limit that bounds total fanout
// work. cancel, if set, is invoked once when visited first exceeds Limit so the
// rule ctx (config.ctx) gets cancelled and execRule / native loops bail via
// their existing ctx.Done() checks. Created per Query in
// syntaxflow_scan/runtime.go and threaded to the sfvm.Config via
// QueryWithWorkBudget.
type RuleWorkBudget struct {
	visited  int64
	limit    int64
	cancel   context.CancelFunc
	exceeded int32 // set to 1 the first time visited > limit
}

// NewRuleWorkBudget builds a per-rule work budget. limit <= 0 means no budget
// (EnterWork is a no-op). cancel is invoked once when visited first exceeds
// limit; pass the rule ctx's CancelFunc so overflow cancels the rule and the
// existing ctx-bail path surfaces partial results.
func NewRuleWorkBudget(limit int64, cancel context.CancelFunc) *RuleWorkBudget {
	return &RuleWorkBudget{limit: limit, cancel: cancel}
}

// EnterWork atomically records one unit of fanout work and returns true if the
// budget has been exceeded (caller should abort the current loop). On the first
// overflow it invokes cancel once and marks exceeded. A nil/zero-limit budget
// (limit <= 0) is a no-op and never overflows.
func (b *RuleWorkBudget) EnterWork() bool {
	if b == nil || atomic.LoadInt64(&b.limit) <= 0 {
		return false
	}
	if atomic.AddInt64(&b.visited, 1) > atomic.LoadInt64(&b.limit) {
		if atomic.CompareAndSwapInt32(&b.exceeded, 0, 1) && b.cancel != nil {
			b.cancel()
		}
		return true
	}
	return false
}

// Exceeded reports whether the budget has ever overflowed (i.e. cancel was
// called). Used by the scan runner to treat the rule as partial-bailed rather
// than failed.
func (b *RuleWorkBudget) Exceeded() bool {
	if b == nil {
		return false
	}
	return atomic.LoadInt32(&b.exceeded) != 0
}

// EnterWork records one unit of fanout work against the config's work budget and
// returns true if the budget has been exceeded (caller should abort). A config
// with no budget never overflows.
func (c *Config) EnterWork() bool {
	if c == nil {
		return false
	}
	return c.workBudget.EnterWork()
}

// GetWorkBudget returns the config's work budget (may be nil).
func (c *Config) GetWorkBudget() *RuleWorkBudget {
	if c == nil {
		return nil
	}
	return c.workBudget
}

// BailCheck combines the per-rule context-cancellation check and the per-rule
// work-budget check into a single call. Returns a non-nil error when the rule
// context is done (deadline/budget cancel) or EnterWork reports the work budget
// exceeded, so hot per-element loops (e.g. <typeName> in sf_native_call.go) can
// bail with one call instead of repeating the select+EnterWork boilerplate.
func (c *Config) BailCheck() error {
	if c == nil {
		return nil
	}
	select {
	case <-c.GetContext().Done():
		return utils.Errorf("context done")
	default:
	}
	if c.EnterWork() {
		return utils.Errorf("work budget exceeded")
	}
	return nil
}

func (c *Config) GetContext() context.Context {
	return c.ctx
}

type Option func(*Config)

func WithInitialContextVars(o *omap.OrderedMap[string, Values]) Option {
	return func(config *Config) {
		config.initialContextVars = o
	}
}

func WithRuntimeOption(opt any) Option {
	return func(config *Config) {
		config.RuntimeOptions = append(config.RuntimeOptions, opt)
	}
}

func WithProcessCallback(p func(int, string)) Option {
	return func(config *Config) {
		config.processCallback = p
	}
}

func WithFailFast(b ...bool) Option {
	return func(config *Config) {
		if len(b) <= 0 {
			config.FailFast = true
			return
		}
		config.FailFast = b[0]
	}
}

func WithContext(ctx context.Context) Option {
	return func(config *Config) {
		if ctx != nil {
			config.ctx = ctx
		}
	}
}

// WithWorkBudget attaches a per-rule total-work budget to the config. The
// budget is shared across the rule's sub-queries (dataflow include etc.) so it
// bounds cumulative fanout work, not just one opcode. Created in the scan
// runner alongside the rule ctx; cancel should be the rule ctx's CancelFunc so
// overflow cancels the rule and the existing ctx-bail path surfaces partial
// results.
func WithWorkBudget(b *RuleWorkBudget) Option {
	return func(config *Config) {
		config.workBudget = b
	}
}

func WithEnableDebug(b ...bool) Option {
	return func(config *Config) {
		if len(b) <= 0 {
			config.debug = true
			return
		}
		config.debug = b[0]
	}
}

func WithStrictMatch(b ...bool) Option {
	return func(config *Config) {
		if len(b) > 0 {
			config.StrictMatch = b[0]
		} else {
			config.StrictMatch = true
		}
	}
}

func WithResultCaptured(c ResultCapturedCallback) Option {
	return func(config *Config) {
		config.onResultCapturedCallbacks = append(config.onResultCapturedCallbacks, c)
	}
}

func WithDiagnostics(enabled bool, recorder ...*diagnostics.Recorder) Option {
	return func(config *Config) {
		config.diagnosticsEnabled = enabled
		if len(recorder) > 0 && recorder[0] != nil {
			config.diagnosticsRecorder = recorder[0]
		} else if enabled && config.diagnosticsRecorder == nil {
			config.diagnosticsRecorder = diagnostics.NewRecorder()
		}
	}
}

func WithConfig(other *Config) Option {
	return func(self *Config) {
		self.StrictMatch = other.StrictMatch
		self.FailFast = other.FailFast
		self.initialContextVars = other.initialContextVars
		self.onResultCapturedCallbacks = other.onResultCapturedCallbacks
		self.ctx = other.ctx
		self.processCallback = other.processCallback
		self.diagnosticsEnabled = other.diagnosticsEnabled
		self.diagnosticsRecorder = other.diagnosticsRecorder
		self.workBudget = other.workBudget
	}
}

func (c *Config) Copy() *Config {
	ret := &Config{
		debug:                     c.debug,
		StrictMatch:               c.StrictMatch,
		FailFast:                  c.FailFast,
		initialContextVars:        c.initialContextVars,
		onResultCapturedCallbacks: c.onResultCapturedCallbacks,
		ctx:                       c.ctx,
		processCallback:           c.processCallback,
		diagnosticsEnabled:        c.diagnosticsEnabled,
		workBudget:                c.workBudget,
		// diagnosticsRecorder:       c.diagnosticsRecorder,
	}
	if ret.diagnosticsEnabled {
		ret.diagnosticsRecorder = diagnostics.NewRecorder()
	}
	return ret
}
