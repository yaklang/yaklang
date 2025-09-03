package sfvm

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/utils/memedit"

	"github.com/yaklang/yaklang/common/log"
)

type RecursiveConfigKey string

const RecursiveMagicVariable = "__next__"

const (
	RecursiveConfig_NULL     RecursiveConfigKey = ""
	RecursiveConfig_Depth                       = "depth"
	RecursiveConfig_DepthMin                    = "depth_min"
	RecursiveConfig_DepthMax                    = "depth_max"
	// RecursiveConfig_Exclude 在匹配到不符合配置项的Value后，数据流继续流动，以匹配其它Value。
	RecursiveConfig_Exclude = "exclude"
	// RecursiveConfig_Include 在匹配到符合配置项的Value后，数据流继续流动，以匹配其它Value。
	RecursiveConfig_Include = "include"
	// RecursiveConfig_Until 会沿着数据流匹配每个Value，知道匹配到符合配置项的Value的时候，数据流停止流动。
	RecursiveConfig_Until = "until"
	// RecursiveConfig_Hook 会对匹配到的每个Value执行配置项的sfRule，但是不会影响最终结果，其数据流会持续流动。
	RecursiveConfig_Hook = "hook"
	// un-used now
	RecursiveConfig_Filter = "filter"
)

func FormatRecursiveConfigKey(i string) RecursiveConfigKey {
	switch strings.TrimSpace(strings.ToLower(i)) {
	case "depth":
		return RecursiveConfig_Depth
	case "depth_min", "depth-min", "min_depth", "min-depth", "mindepth", "depthmin":
		return RecursiveConfig_DepthMin
	case "depth_max", "depth-max", "max_depth", "max-depth", "maxdepth", "depthmax":
		return RecursiveConfig_DepthMax
	case "exclude":
		return RecursiveConfig_Exclude
	case "until":
		return RecursiveConfig_Until
	case "hook":
		return RecursiveConfig_Hook
	case "include":
		return RecursiveConfig_Include
	default:
		log.Warnf("unknown recursive config key: %s", i)
	}
	return RecursiveConfig_NULL
}

type RecursiveConfigItem struct {
	Key            string
	Value          string
	SyntaxFlowRule bool
}

type AnalysisContext struct {
	Step  int
	Label string
}

func NewDefaultAnalysisContext() *AnalysisContext {
	return &AnalysisContext{
		Step:  -1,
		Label: "",
	}
}

type AnalysisContextOption func(*AnalysisContext)

func WithAnalysisContext_Step(step int) AnalysisContextOption {
	return func(context *AnalysisContext) {
		context.Step = step
	}
}

func WithAnalysisContext_Label(label string) AnalysisContextOption {
	return func(context *AnalysisContext) {
		context.Label = label
	}
}

// type MatchMode int
const (
	NameMatch int = 1
	KeyMatch      = 1 << 1
	BothMatch     = NameMatch | KeyMatch
)

func MatchModeString(mode int) string {
	switch mode {
	case NameMatch:
		return "name"
	case KeyMatch:
		return "key"
	case BothMatch:
		return "name+key"
	}
	return "Unknown"
}

type ValueOperator interface {
	String() string
	IsMap() bool
	IsList() bool
	IsEmpty() bool
	GetOpcode() string
	GetBinaryOperator() string
	GetUnaryOperator() string
	// Len() int

	// Recursive will execute with handler for every list or map
	Recursive(func(ValueOperator) error) error

	// ExactMatch return ops, for OpPushSearchExact
	ExactMatch(context.Context, int, string) (bool, ValueOperator, error)
	// GlobMatch return opts, for OpPushSearchGlob
	GlobMatch(context.Context, int, string) (bool, ValueOperator, error)
	// RegexpMatch for OpPushSearchRegexp
	RegexpMatch(context.Context, int, string) (bool, ValueOperator, error)

	GetCalled() (ValueOperator, error)
	GetCallActualParams(int, bool) (ValueOperator, error)
	GetFields() (ValueOperator, error)

	// GetTopDef and GetBottomUse is for OpBottomUse
	// use and def
	GetSyntaxFlowUse() (ValueOperator, error)
	GetSyntaxFlowDef() (ValueOperator, error)
	// top and bottom
	GetSyntaxFlowTopDef(*SFFrameResult, *Config, ...*RecursiveConfigItem) (ValueOperator, error)
	GetSyntaxFlowBottomUse(*SFFrameResult, *Config, ...*RecursiveConfigItem) (ValueOperator, error)

	// ListIndex for OpListIndex, like a[1] a must be list...
	ListIndex(i int) (ValueOperator, error)

	Merge(...ValueOperator) (ValueOperator, error)
	Remove(...ValueOperator) (ValueOperator, error)

	AppendPredecessor(ValueOperator, ...AnalysisContextOption) error

	// fileFilter
	FileFilter(string, string, map[string]string, []string) (ValueOperator, error)

	CompareString(*StringComparator) (ValueOperator, []bool)
	CompareOpcode(*OpcodeComparator) (ValueOperator, []bool)
	CompareConst(*ConstComparator) []bool
	NewConst(any, ...*memedit.Range) ValueOperator
}
