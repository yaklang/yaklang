package yakit

import (
	"context"
	"sync"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// SSAComparisonItemConfig 比较项的配置信息
type SSAComparisonItemConfig struct {
	RuleName        string
	RuleContentHash string
	VariableName    string
	RuntimeId       string
	ProgramName     string
}

type SSAComparisonItemOption func(*SSAComparisonItemConfig)

func DiffWithProgram(name string) SSAComparisonItemOption {
	return func(itemConfig *SSAComparisonItemConfig) {
		itemConfig.ProgramName = name
	}
}
func DiffWithRuntimeId(runtimeId string) SSAComparisonItemOption {
	return func(itemConfig *SSAComparisonItemConfig) {
		itemConfig.RuntimeId = runtimeId
	}
}
func DiffWithRuleName(name string) SSAComparisonItemOption {
	return func(config *SSAComparisonItemConfig) {
		config.RuleName = name
	}
}
func DiffWithRuleContentHash(hash string) SSAComparisonItemOption {
	return func(config *SSAComparisonItemConfig) {
		config.RuleContentHash = hash
	}
}
func DiffWithVariableName(variable string) SSAComparisonItemOption {
	return func(config *SSAComparisonItemConfig) {
		config.VariableName = variable
	}
}

// 配置被比较的项
type CompareStatus string

const (
	Equal CompareStatus = "Equal"
	Add   CompareStatus = "Add"
	Del   CompareStatus = "Del"
)

// ComparisonItem is a generic struct that holds the configuration for comparing items in the SSA system.
type ComparisonItem[T any] struct {
	ProgramName string
	TaskId      string
	Kind        schema.SSADiffResultKind
	// GetComparisonValue 用于获取需要比较的值
	GetComparisonValue func(context.Context) <-chan T
}

func NewSSARiskComparisonItem(opts ...SSAComparisonItemOption) (*ComparisonItem[*schema.SSARisk], error) {
	s := new(SSAComparisonItemConfig)
	item := new(ComparisonItem[*schema.SSARisk])
	for _, opt := range opts {
		opt(s)
	}
	if s.ProgramName == "" && s.RuntimeId == "" {
		return nil, utils.Error("program name or runtime id must be set for compare risk item")
	}
	item.ProgramName = s.ProgramName
	item.TaskId = s.RuntimeId
	// item.Kind = schema.RuntimeId
	item.GetComparisonValue = func(ctx context.Context) <-chan *schema.SSARisk {
		//TODO: 感觉db不应该在这里声明一个
		db := consts.GetGormDefaultSSADataBase().Model(&schema.SSARisk{})
		db = bizhelper.ExactQueryString(db, "program_name", s.ProgramName)
		db = bizhelper.ExactQueryString(db, "runtime_id", s.RuntimeId)
		db = bizhelper.ExactQueryString(db, "from_rule", s.RuleName)
		db = bizhelper.ExactQueryString(db, "variable", s.VariableName)
		db = bizhelper.QueryOrder(db, "variable", "asc")
		return bizhelper.YieldModel[*schema.SSARisk](ctx, db)
	}
	return item, nil
}

func NewCompareCustomVariableItem(opts ...SSAComparisonItemOption) *ComparisonItem[*ssadb.AuditNode] {
	s := new(SSAComparisonItemConfig)
	item := new(ComparisonItem[*ssadb.AuditNode])
	for _, opt := range opts {
		opt(s)
	}
	item.GetComparisonValue = func(ctx context.Context) <-chan *ssadb.AuditNode {
		db := consts.GetGormDefaultSSADataBase().Model(&schema.SSARisk{})
		db = bizhelper.ExactQueryString(db, "program_name", s.ProgramName)
		db = bizhelper.ExactQueryString(db, "rule_name", s.RuleName)
		db = bizhelper.ExactQueryString(db, "result_variable", s.VariableName)
		db = bizhelper.QueryOrder(db, "variable", "asc")
		db = bizhelper.QueryByBool(db, "is_entry_node", true)
		return bizhelper.YieldModel[*ssadb.AuditNode](ctx, db)
	}
	return item
}

// ComparatorConfig 比较器运行时的配置项
type ComparatorConfig[T any] struct {
	// resultHandler 用于处理比较结果的函数
	resultHandler []func(result *ComparisonResult[T])
	// getComparisonBasisInfo 获取比较结果的信息
	getComparisonBasisInfo func(value T) (rule string, originHash string, diffHash string)
	// saveResultHandler 用于处理比较结果的函数
	saveResultHandler func(result []*ComparisonResult[T])
}

type ComparatorOptions[T any] func(options *ComparatorConfig[T])

// WithComparatorSaveResultHandler is used to set the function that handles the comparison results.
func WithComparatorSaveResultHandler[T any](f func([]*ComparisonResult[T])) func(options *ComparatorConfig[T]) {
	return func(options *ComparatorConfig[T]) {
		options.saveResultHandler = f
	}
}

func WithSSARiskDiffResultHandler(f func(result *ComparisonResult[*schema.SSARisk])) func(options *ComparatorConfig[*schema.SSARisk]) {
	return func(options *ComparatorConfig[*schema.SSARisk]) {
		options.resultHandler = append(options.resultHandler, f)
	}
}

// WithSSARiskDiffSaveResultHandler is used to set the function that handles the comparison results for SSARisk.
func WithSSARiskDiffSaveResultHandler(baseItem, compareItem string, kind string) func(options *ComparatorConfig[*schema.SSARisk]) {
	return WithComparatorSaveResultHandler(func(risks []*ComparisonResult[*schema.SSARisk]) {
		utils.GormTransactionReturnDb(consts.GetGormDefaultSSADataBase(), func(tx *gorm.DB) {
			for _, risk := range risks {
				result := &schema.SSADiffResult{
					BaseLine:         baseItem,
					Compare:          compareItem,
					RuleName:         risk.FromRule,
					BaseLineRiskHash: risk.BaseValHash,
					CompareRiskHash:  risk.NewValHash,
					Status:           string(risk.Status),
					CompareType:      schema.RiskDiff,
					DiffResultKind:   kind,
				}
				SaveSSADiffResult(consts.GetGormDefaultSSADataBase(), result)
			}
		})
	})
}

// WithComparatorGetBasisInfo is used to set the function that generates the comparison basis information.
func WithComparatorGetBasisInfo[T any](get func(value T) (
	rule string,
	originHash string,
	diffHash string,
)) func(options *ComparatorConfig[T]) {
	return func(options *ComparatorConfig[T]) {
		options.getComparisonBasisInfo = get
	}
}

// WithSSARiskComparisonInfoGenerate 设置用于生成SSARisk比较信息的函数
func WithSSARiskComparisonInfoGenerate(f func(risk *schema.SSARisk) (
	rule string,
	originHash string,
	diffHash string,
)) func(options *ComparatorConfig[*schema.SSARisk]) {
	return WithComparatorGetBasisInfo(f)
}

type SSAComparator[T any] struct {
	baseItem *ComparisonItem[T]
	config   *ComparatorConfig[T]
}

type ComparisonResultItem[T any] struct {
	Val  T
	Hash string

	rule string
}
type ComparisonResult[T any] struct {
	BaseValue T
	NewValue  T
	FromRule  string

	//riskHash
	BaseValHash string
	NewValHash  string
	Status      CompareStatus
}

func (s *SSAComparator[T]) Compare(
	ctx context.Context,
	item *ComparisonItem[T],
	opts ...ComparatorOptions[T],
) <-chan *ComparisonResult[T] {
	// todo: 切换更流式的compare算法
	diffHashMap := make(map[string]int)
	string2BaseItemValue := make(map[string]*ComparisonResultItem[T])
	string2CompareItemValue := make(map[string]*ComparisonResultItem[T])

	result := make(chan *ComparisonResult[T])
	for _, opt := range opts {
		opt(s.config)
	}

	if s.config.saveResultHandler == nil {
		log.Errorf("saveResultHandler function is not set for SSAComparator, using default saveResultHandler function")
		close(result)
		return result
	}
	if s.config.getComparisonBasisInfo == nil {
		log.Errorf("generateHash function is not set for SSAComparator, using default hash function")
		close(result)
		return result
	}

	for v := range s.baseItem.GetComparisonValue(ctx) {
		sfRule, riskHash, diffHash := s.config.getComparisonBasisInfo(v)
		string2BaseItemValue[diffHash] = &ComparisonResultItem[T]{
			Val:  v,
			Hash: riskHash,
			rule: sfRule,
		}
		diffHashMap[diffHash]++
	}
	for t := range item.GetComparisonValue(ctx) {
		sfRule, riskHash, diffHash := s.config.getComparisonBasisInfo(t)
		string2CompareItemValue[diffHash] = &ComparisonResultItem[T]{
			Val:  t,
			Hash: riskHash,
			rule: sfRule,
		}
		diffHashMap[diffHash]--
	}

	wg := new(sync.WaitGroup)
	taskChan := make(chan *ComparisonResult[T], 1)
	processor := utils.NewBatchProcessor[*ComparisonResult[T]](
		ctx,
		taskChan,
		utils.WithBatchProcessorCallBack[*ComparisonResult[T]](s.config.saveResultHandler),
	)

	processor.Start()

	addChannel := func(compareResult *ComparisonResult[T]) bool {
		wg.Add(1)
		//进行前置的保存操作
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case taskChan <- compareResult:
			}
		}()
		select {
		case <-ctx.Done():
			close(result)
			return false
		case result <- compareResult:
			for _, f := range s.config.resultHandler {
				f(compareResult)
			}
			return true
		}
	}

	go func() {
		defer func() {
			wg.Wait()
			close(taskChan)
			processor.Wait()
			close(result)
		}()
		var zeroValue T
		for s, i := range diffHashMap {
			switch {
			case i < 0:
				//只在compareItem中存在
				for y := i; y < 0; y++ {
					compareValue, ok := string2CompareItemValue[s]
					if !ok {
						continue
					}
					if !addChannel(&ComparisonResult[T]{
						BaseValue:   compareValue.Val,
						NewValue:    zeroValue,
						FromRule:    compareValue.rule,
						BaseValHash: compareValue.Hash,
						NewValHash:  "",
						Status:      Del,
					}) {
						return
					}
				}
			case i > 0:
				//只在BaseItem中存在
				for y := i; y > 0; y-- {
					baseValue, ok := string2BaseItemValue[s]
					if !ok {
						continue
					}
					if !addChannel(&ComparisonResult[T]{
						BaseValue:   zeroValue,
						NewValue:    baseValue.Val,
						FromRule:    baseValue.rule,
						BaseValHash: "",
						NewValHash:  baseValue.Hash,
						Status:      Add,
					}) {
						return
					}
				}
			default:
				baseValue, ok1 := string2BaseItemValue[s]
				compareValue, ok2 := string2CompareItemValue[s]
				if !(ok1 && ok2) {
					continue
				}
				if !addChannel(&ComparisonResult[T]{
					BaseValue:   baseValue.Val,
					NewValue:    compareValue.Val,
					FromRule:    compareValue.rule,
					BaseValHash: baseValue.Hash,
					NewValHash:  compareValue.Hash,
					Status:      Equal,
				}) {
					return
				}
			}
		}
	}()

	return result
}

// NewSSAComparator creates a new SSAComparator instance with the provided base item.
func NewSSAComparator[T any](item *ComparisonItem[T]) *SSAComparator[T] {
	return &SSAComparator[T]{
		baseItem: item,
		config:   new(ComparatorConfig[T]),
	}
}

func DoRiskDiff(context context.Context, base, compare *ypb.SSARiskDiffItem) (<-chan *ComparisonResult[*schema.SSARisk], error) {
	// 使用baseLine项目的risk作为对比的基础
	baseRiskItem, err := NewSSARiskComparisonItem(
		DiffWithVariableName(base.GetVariable()),
		DiffWithRuntimeId(base.GetRiskRuntimeId()),
		DiffWithRuleName(base.GetRuleName()),
		DiffWithProgram(base.GetProgramName()),
	)
	if err != nil {
		return nil, err
	}

	// 创建比较器
	resultComparator := NewSSAComparator[*schema.SSARisk](baseRiskItem)
	// 使用compare项目的risk进行对比
	compareRiskItem, err := NewSSARiskComparisonItem(
		DiffWithVariableName(compare.GetVariable()),
		DiffWithRuntimeId(compare.GetRiskRuntimeId()),
		DiffWithRuleName(compare.GetRuleName()),
		DiffWithProgram(compare.GetProgramName()))
	if err != nil {
		return nil, err
	}

	diffResults, err := GetSSADiffResult(
		consts.GetGormDefaultSSADataBase(),
		base.GetRiskRuntimeId(),
		compare.GetRiskRuntimeId(),
	)
	if err != nil {
		return nil, err
	}
	if len(diffResults) > 0 {
		res := make(chan *ComparisonResult[*schema.SSARisk])
		go func() {
			defer func() {
				close(res)
			}()
			for _, d := range diffResults {
				compareResult := &ComparisonResult[*schema.SSARisk]{
					BaseValue:   nil,
					NewValue:    nil,
					BaseValHash: d.BaseLineRiskHash,
					NewValHash:  d.CompareRiskHash,
					FromRule:    d.RuleName,
					Status:      CompareStatus(d.Status),
				}

				if hash := d.BaseLineRiskHash; hash != "" {
					if value, err := GetSSARiskByHash(consts.GetGormDefaultSSADataBase(), hash); err == nil {
						compareResult.BaseValue = value
					}
				}

				if hash := d.CompareRiskHash; hash != "" {
					if value, err := GetSSARiskByHash(consts.GetGormDefaultSSADataBase(), hash); err == nil {
						compareResult.NewValue = value
					}
				}

				res <- compareResult
			}
		}()

		return res, nil
	}

	// 执行对比
	res := resultComparator.Compare(context, compareRiskItem,
		// 对比结果保存到数据库
		WithComparatorSaveResultHandler(func(risks []*ComparisonResult[*schema.SSARisk]) {
			utils.GormTransactionReturnDb(consts.GetGormDefaultSSADataBase(), func(db *gorm.DB) {
				kind := schema.Unknown
				if base.RiskRuntimeId != "" {
					kind = schema.RuntimeId
				} else if base.ProgramName != "" {
					kind = schema.Program
				}
				for _, risk := range risks {
					result := &schema.SSADiffResult{
						BaseLine:         base.GetRiskRuntimeId(),
						Compare:          compare.GetRiskRuntimeId(),
						RuleName:         risk.FromRule,
						BaseLineRiskHash: risk.BaseValHash,
						CompareRiskHash:  risk.NewValHash,
						Status:           string(risk.Status),
						DiffResultKind:   string(kind),
					}
					SaveSSADiffResult(db, result)
				}
			})
		}),
		// 设置回调函数，返回一些信息作为对比的依据
		WithComparatorGetBasisInfo[*schema.SSARisk](func(risk *schema.SSARisk) (
			rule string,
			originHash string,
			diffHash string,
		) {
			return risk.FromRule, risk.Hash, risk.RiskFeatureHash
		}),
	)
	return res, nil
}

func CreateSSADiffResult(DB *gorm.DB, r *schema.SSADiffResult) error {
	if r == nil {
		return utils.Errorf("create error: ssa-diff-result is nil")
	}
	if db := DB.Create(r); db.Error != nil {
		return db.Error
	}
	return nil
}

func SaveSSADiffResult(DB *gorm.DB, r *schema.SSADiffResult) error {
	if r == nil {
		return utils.Errorf("save error: ssa-diff-result is nil")
	}
	if db := DB.Save(r); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteSSADiffResultByBaseLine(DB *gorm.DB, base []string, typs ...schema.SSADiffResultKind) error {
	db := DB.Model(&schema.SSADiffResult{})
	db = bizhelper.ExactQueryStringArrayOr(db, "base_line", base)
	if len(typs) > 0 {
		db = bizhelper.ExactQueryString(db, "diff_result_kind", string(typs[0]))
	}
	if db := db.Unscoped().Delete(&schema.SSADiffResult{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteSSADiffResultByCompare(DB *gorm.DB, compara []string, typs ...schema.SSADiffResultKind) error {
	db := DB.Model(&schema.SSADiffResult{})
	db = bizhelper.ExactQueryStringArrayOr(db, "compare", compara)
	if len(typs) > 0 {
		db = bizhelper.ExactQueryString(db, "diff_result_kind", string(typs[0]))
	}
	if db := db.Unscoped().Delete(&schema.SSADiffResult{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func DeleteSSADiffResultByRule(DB *gorm.DB, rules []string) error {
	db := DB.Model(&schema.SSADiffResult{})
	db = bizhelper.ExactQueryStringArrayOr(db, "rule_name", rules)
	if db := db.Unscoped().Delete(&schema.SSADiffResult{}); db.Error != nil {
		return db.Error
	}
	return nil
}

func GetSSADiffResult(DB *gorm.DB, baseline, compare string) ([]*schema.SSADiffResult, error) {
	var r []*schema.SSADiffResult
	if db := DB.Model(&schema.SSADiffResult{}).
		Where("base_line = ?", baseline).
		Where("compare = ?", compare).Find(&r); db.Error != nil {
		return nil, utils.Errorf("get ssa-diff-result failed: %s", db.Error)
	}
	return r, nil
}
