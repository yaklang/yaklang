package sfanalyzer

import "github.com/yaklang/yaklang/common/yak/static_analyzer/result"

const (
	Error   = string(result.Error)
	Warning = string(result.Warn)
	Info    = string(result.Info)
)

// 问题类型常量定义
const (
	// 语法和编译相关问题
	ProblemTypeSyntaxError = "syntax_error" // 语法错误

	// 描述信息相关问题
	ProblemTypeLackDescriptionField = "lack_description_field" // 缺少描述字段
	ProblemTypeLackSolutionField    = "lack_solution_field"    // 缺少解决方案字段

	// 测试数据相关问题
	ProblemTypeMissingPositiveTestData = "missing_positive_test_data" // 缺少正向测试数据
	ProblemTypeMissingNegativeTestData = "missing_negative_test_data" // 缺少反向测试数据
	ProblemTypeTestCaseNotPass         = "test_case_not_pass"         // 测试用例不通过

	// 规则逻辑相关问题
	ProblemTypeMissingAlert = "missing_alert" // 缺少alert语句
)

const (
	// 基础分数设置
	MaxScore = 100 // 满分100分
	MinScore = 0   // 最低分0分

	// 严重错误扣分（直接导致不合格）
	SyntaxErrorPenalty           = 100 // 语法错误(SF规则编译出错)扣100分（直接0分）
	VerifyTestCaseNotPassPenalty = 100 // 测试用例不通过扣100分（直接0分）
	MissingAlertPenalty          = 100 // 缺少alert语句扣100分（直接0分）

	// 重要缺失项扣分
	MissingDescriptionPenalty  = 40 // 缺少详细描述扣40分
	MissingSolutionPenalty     = 10 // 缺少解决方案扣10分
	MissingPositiveTestPenalty = 15 // 缺少正向测试数据扣15分
	MissingNegativeTestPenalty = 5  // 缺少反向测试数据扣5分

	// 分数等级界限
	GradeSMin = 100
	GradeAMin = 90 // A级最低分数
	GradeBMin = 80 // B级最低分数
	GradeCMin = 70 // C级最低分数
	GradeDMin = 60 // D级最低分数
	// F级: 低于60分
)

// GetGrade 根据分数获取等级
func GetGrade(score int) string {
	switch {
	case score >= GradeSMin:
		return "S"
	case score >= GradeAMin:
		return "A"
	case score >= GradeBMin:
		return "B"
	case score >= GradeCMin:
		return "C"
	case score >= GradeDMin:
		return "D"
	default:
		return "F"
	}
}

// GetGradeDescription 获取等级描述
func GetGradeDescription(grade string) string {
	switch grade {
	case "S":
		return "完美 - 规则信息完整且包含正反测试且能通过"
	case "A":
		return "优秀 - 规则较为完整"
	case "B":
		return "良好 - 规则基本完整，有小问题"
	case "C":
		return "一般 - 规则可用，但缺少一些重要信息"
	case "D":
		return "较差 - 规则存在明显缺陷"
	case "F":
		return "不合格 - 规则存在严重问题"
	default:
		return "未知等级"
	}
}
