package sfvm

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
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
	Key            string `json:"key"`
	Value          string `json:"value"`
	SyntaxFlowRule bool   `json:"syntax_flow_rule"`
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

func MatchModeString(mode int) string {
	switch ssadb.MatchMode(mode) {
	case ssadb.NameMatch:
		return "name"
	case ssadb.KeyMatch:
		return "key"
	case ssadb.BothMatch:
		return "name+key"
	case ssadb.ConstType:
		return "const"
	}
	return "Unknown"
}

type ValueOperator interface {
	// Basic shape and debug text.
	String() string
	IsMap() bool
	IsList() bool
	IsEmpty() bool

	// Candidate-mode condition optimization switch.
	ShouldUseConditionCandidate() bool

	// IR/operator metadata for comparator opcodes.
	GetOpcode() string
	GetBinaryOperator() string
	GetUnaryOperator() string

	// Search and name/key matching.
	ExactMatch(context.Context, ssadb.MatchMode, string) (bool, Values, error)
	GlobMatch(context.Context, ssadb.MatchMode, string) (bool, Values, error)
	RegexpMatch(context.Context, ssadb.MatchMode, string) (bool, Values, error)

	// Graph navigation.
	GetCalled() (Values, error)
	GetCallActualParams(int, bool) (Values, error)
	GetFields() (Values, error)

	// Def-use queries.
	GetSyntaxFlowUse() (Values, error)
	GetSyntaxFlowDef() (Values, error)
	GetSyntaxFlowTopDef(*SFFrameResult, *Config, ...*RecursiveConfigItem) (Values, error)
	GetSyntaxFlowBottomUse(*SFFrameResult, *Config, ...*RecursiveConfigItem) (Values, error)

	// List projection for OpListIndex.
	ListIndex(i int) (ValueOperator, error)

	// Optional provenance edge tracking used by some traversals.
	AppendPredecessor(ValueOperator, ...AnalysisContextOption) error

	// File content filtering.
	FileFilter(string, string, map[string]string, []string) (Values, error)

	// Condition comparators.
	CompareString(*StringComparator) (Values, []bool)
	CompareOpcode(*OpcodeComparator) (Values, []bool)
	CompareConst(*ConstComparator) bool
	NewConst(any, ...*memedit.Range) ValueOperator

	// Anchor bitvector provenance for condition mask alignment and scoped grouping.
	GetAnchorBitVector() *utils.BitVector
	SetAnchorBitVector(*utils.BitVector)
}
