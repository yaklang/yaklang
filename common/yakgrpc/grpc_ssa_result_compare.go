package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

type SsaCompareConfig struct {
	RuleName        string
	RuleContentHash string
	VariableName    string
}

type WithCompareOpts func(*SsaCompareConfig)

func WithRuleName(name string) WithCompareOpts {
	return func(config *SsaCompareConfig) {
		config.RuleName = name
	}
}
func WithRuleContentHash(hash string) WithCompareOpts {
	return func(config *SsaCompareConfig) {
		config.RuleContentHash = hash
	}
}
func WithVariableName(variable string) WithCompareOpts {
	return func(config *SsaCompareConfig) {
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
	s := new(SsaCompareConfig)
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
	s := new(SsaCompareConfig)
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

type SsaCompare[T any] struct {
	generateHash func(value T) string
	baseItem     *Item[T]
}
type CompareResult[T any] struct {
	BaseValue T
	NewValue  T
	Status    CompareStatus
}

func (s *SsaCompare[T]) WithGenerateHash(f func(T) string) *SsaCompare[T] {
	s.generateHash = f
	return s
}

func (s *SsaCompare[T]) Compare(ctx context.Context, item *Item[T]) <-chan *CompareResult[T] {
	// todo: 切换更流式的compare算法
	hashMap := make(map[string]int)
	string2BaseItemValue := make(map[string]T)
	string2CompareItemValue := make(map[string]T)
	result := make(chan *CompareResult[T])
	if s.generateHash == nil {
		log.Errorf("generateHash function is not set for SsaCompare, using default hash function")
		close(result)
		return result
	}
	for v := range s.baseItem.GetCompareValue(ctx) {
		hash := s.generateHash(v)
		string2BaseItemValue[hash] = v
		hashMap[hash]++
	}
	for t := range item.GetCompareValue(ctx) {
		hash := s.generateHash(t)
		string2CompareItemValue[hash] = t
		hashMap[hash]--
	}
	addChannel := func(compareResult *CompareResult[T]) bool {
		select {
		case <-ctx.Done():
			close(result)
			return false
		case result <- compareResult:
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
					BaseValue: zeroValue,
					NewValue:  compareValue,
					Status:    Add,
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
					BaseValue: baseValue,
					NewValue:  zeroValue,
					Status:    Del,
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
					BaseValue: baseValue,
					NewValue:  compareValue,
					Status:    Equal,
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
		baseItem:     item,
		generateHash: nil,
	}
}
