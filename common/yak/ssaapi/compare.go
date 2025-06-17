package ssaapi

import (
	"context"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

type SsaCompareItemConfig struct {
	RuleName        string
	RuleContentHash string
	VariableName    string
}

type WithCompareOpts func(*SsaCompareItemConfig)

func WithRuleName(name string) WithCompareOpts {
	return func(config *SsaCompareItemConfig) {
		config.RuleName = name
	}
}
func WithRuleContentHash(hash string) WithCompareOpts {
	return func(config *SsaCompareItemConfig) {
		config.RuleContentHash = hash
	}
}
func WithVariableName(variable string) WithCompareOpts {
	return func(config *SsaCompareItemConfig) {
		config.VariableName = variable
	}
}

type CompareStatus int

const (
	Equal CompareStatus = iota
	Add
	Del
)

type Item[T any] struct {
	ProgramName string
	//两个项目比较时，根据RuleName/RuleId来确定
	GetCompareValue func(context.Context) <-chan T
}

func NewCompareRiskItem(progName string, opts ...WithCompareOpts) *Item[*schema.SSARisk] {
	s := new(SsaCompareItemConfig)
	item := new(Item[*schema.SSARisk])
	for _, opt := range opts {
		opt(s)
	}
	item.GetCompareValue = func(ctx context.Context) <-chan *schema.SSARisk {
		db := consts.GetGormDefaultSSADataBase().Model(&schema.SSARisk{})
		db = bizhelper.ExactQueryString(db, "program_name", progName)
		db = bizhelper.ExactQueryString(db, "from_rule", s.RuleName)
		db = bizhelper.ExactQueryString(db, "variable", s.VariableName)
		db = bizhelper.QueryOrder(db, "variable", "asc")
		return bizhelper.YieldModel[*schema.SSARisk](ctx, db)
	}
	return item
}
func NewCompareCustomVariableItem(progName string, opts ...WithCompareOpts) *Item[*ssadb.AuditNode] {
	s := new(SsaCompareItemConfig)
	item := new(Item[*ssadb.AuditNode])
	for _, opt := range opts {
		opt(s)
	}
	item.GetCompareValue = func(ctx context.Context) <-chan *ssadb.AuditNode {
		db := consts.GetGormDefaultSSADataBase().Model(&schema.SSARisk{})
		db = bizhelper.ExactQueryString(db, "program_name", progName)
		db = bizhelper.ExactQueryString(db, "rule_name", s.RuleName)
		db = bizhelper.ExactQueryString(db, "result_variable", s.VariableName)
		db = bizhelper.QueryOrder(db, "variable", "asc")
		db = bizhelper.QueryByBool(db, "is_entry_node", true)
		return bizhelper.YieldModel[*ssadb.AuditNode](ctx, db)
	}
	return item
}

type CompareOptions[T any] struct {
	onResultCallBack []func(result *CompareResult[T])
	getValueInfo     func(value T) (rule string, originHash string, diffHash string)
}

type CompareOpts[T any] func(options *CompareOptions[T])

func WithCompareResultCallback[T any](cb func(result *CompareResult[T])) func(options *CompareOptions[T]) {
	return func(options *CompareOptions[T]) {
		options.onResultCallBack = append(options.onResultCallBack, cb)
	}
}
func WithCompareResultGetValueInfo[T any](generate func(value T) (rule string, originHash string, diffHash string)) func(options *CompareOptions[T]) {
	return func(options *CompareOptions[T]) {
		options.getValueInfo = generate
	}
}

type SsaCompare[T any] struct {
	baseItem *Item[T]
	config   *CompareOptions[T]
}

type CompareResultItem[T any] struct {
	Val  T
	Hash string

	rule string
}
type CompareResult[T any] struct {
	BaseValue T
	NewValue  T
	FromRule  string

	//riskHash
	BaseValHash string
	NewValHash  string
	Status      CompareStatus
}

func (s *SsaCompare[T]) Compare(ctx context.Context, item *Item[T], opts ...CompareOpts[T]) <-chan *CompareResult[T] {
	// todo: 切换更流式的compare算法
	hashMap := make(map[string]int)
	string2BaseItemValue := make(map[string]*CompareResultItem[T])
	string2CompareItemValue := make(map[string]*CompareResultItem[T])
	result := make(chan *CompareResult[T])
	for _, opt := range opts {
		opt(s.config)
	}
	if s.config.getValueInfo == nil {
		log.Errorf("generateHash function is not set for SsaCompare, using default hash function")
		close(result)
		return result
	}
	for v := range s.baseItem.GetCompareValue(ctx) {
		rule, hash, diffHash := s.config.getValueInfo(v)
		string2BaseItemValue[diffHash] = &CompareResultItem[T]{
			Val:  v,
			Hash: hash,

			rule: rule,
		}
		hashMap[diffHash]++
	}
	for t := range item.GetCompareValue(ctx) {
		rule, hash, diffHash := s.config.getValueInfo(t)
		string2CompareItemValue[diffHash] = &CompareResultItem[T]{
			Val:  t,
			Hash: hash,

			rule: rule,
		}
		hashMap[diffHash]--
	}
	addChannel := func(compareResult *CompareResult[T]) bool {
		select {
		case <-ctx.Done():
			close(result)
			return false
		case result <- compareResult:
			for _, f := range s.config.onResultCallBack {
				f(compareResult)
			}
			return true
		}
	}
	go func() {
		defer close(result)
		var zeroValue T
		for s, i := range hashMap {
			switch {
			case i < 0:
				//只在compareItem中存在
				compareValue, ok := string2CompareItemValue[s]
				if !ok {
					continue
				}
				if !addChannel(&CompareResult[T]{
					BaseValue:   zeroValue,
					FromRule:    compareValue.rule,
					NewValue:    compareValue.Val,
					BaseValHash: "",
					NewValHash:  compareValue.Hash,
					Status:      Add,
				}) {
					return
				}
			case i > 0:
				//只在BaseItem中存在
				baseValue, ok := string2BaseItemValue[s]
				if !ok {
					continue
				}
				if !addChannel(&CompareResult[T]{
					BaseValue:   baseValue.Val,
					NewValue:    zeroValue,
					BaseValHash: baseValue.Hash,
					FromRule:    baseValue.rule,
					NewValHash:  "",
					Status:      Del,
				}) {
					return
				}
			default:
				baseValue, ok1 := string2BaseItemValue[s]
				compareValue, ok2 := string2CompareItemValue[s]
				if !(ok1 && ok2) {
					continue
				}
				if !addChannel(&CompareResult[T]{
					BaseValue:   baseValue.Val,
					NewValue:    compareValue.Val,
					BaseValHash: baseValue.Hash,
					NewValHash:  compareValue.Hash,
					FromRule:    compareValue.rule,
					Status:      Equal,
				}) {
					return
				}
			}
		}
	}()
	return result
}
func NewSsaCompare[T any](item *Item[T]) *SsaCompare[T] {
	return &SsaCompare[T]{
		baseItem: item,
		config:   new(CompareOptions[T]),
	}
}
