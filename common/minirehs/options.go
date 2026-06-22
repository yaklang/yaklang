package minirehs

import "github.com/yaklang/yaklang/common/log"

// Logger 是库内部日志的最小契约. 默认实现转发到 common/log, 全部为英文输出.
type Logger interface {
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Debugf(format string, args ...interface{})
}

type defaultLogger struct{}

func (defaultLogger) Infof(format string, args ...interface{})  { log.Infof(format, args...) }
func (defaultLogger) Warnf(format string, args ...interface{})  { log.Warnf(format, args...) }
func (defaultLogger) Errorf(format string, args ...interface{}) { log.Errorf(format, args...) }
func (defaultLogger) Debugf(format string, args ...interface{}) { log.Debugf(format, args...) }

// config 是编译期内部配置, 由 Option 构造.
type config struct {
	backend        BackendKind
	defaultPolicy  UnsupportedPolicy
	minLiteralLen  int  // 提取的必需字面量最小长度, 短于此值的 pattern 归入 always-on
	reportLocation bool // mvs 后端: 命中是否上报精确字节偏移与内容 (true), 还是仅存在性 (false)
	logger         Logger
}

func newDefaultConfig() *config {
	return &config{
		backend:        Auto,
		defaultPolicy:  Reject,
		minLiteralLen:  2,
		reportLocation: true,
		logger:         defaultLogger{},
	}
}

// Option 是 functional option.
type Option func(*config)

// WithBackend 强制指定后端 (Auto / BackendEngine / BackendStdlib).
func WithBackend(b BackendKind) Option {
	return func(c *config) { c.backend = b }
}

// WithDefaultUnsupportedPolicy 设定全局默认的不支持处理策略.
func WithDefaultUnsupportedPolicy(p UnsupportedPolicy) Option {
	return func(c *config) { c.defaultPolicy = p }
}

// WithMinLiteralLen 设定必需字面量的最小长度阈值. 字面量越长 prefilter 越精准,
// 但过长可能让更多 pattern 退化为 always-on. 取值需 >= 1.
func WithMinLiteralLen(n int) Option {
	return func(c *config) {
		if n >= 1 {
			c.minLiteralLen = n
		}
	}
}

// WithReportLocation 控制 mvs 后端命中时是否上报精确字节偏移 [From,To) 与匹配内容.
// 默认 true: 对可编入 NFA 的 pattern, 命中后由 NFA 自身定位每个非重叠匹配 (leftmost-longest),
// 上报 Match{ID,From,To}, 内容即 data[From:To]. 置 false 则只判存在性 (Match{ID,-1,-1}),
// 走纯位运算快路径以换取更高吞吐 (适合只需"哪些规则命中"的打标场景).
// (regexp2-only 兜底 pattern 无法可靠给字节偏移, 任一设置下都以 -1/-1 上报.)
func WithReportLocation(b bool) Option {
	return func(c *config) { c.reportLocation = b }
}

// WithLogger 注入日志实现.
func WithLogger(l Logger) Option {
	return func(c *config) {
		if l != nil {
			c.logger = l
		}
	}
}
