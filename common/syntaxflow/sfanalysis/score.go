package sfanalysis

import "github.com/yaklang/yaklang/common/yak/static_analyzer/result"

const (
	Error   = string(result.Error)
	Warning = string(result.Warn)
	Info    = string(result.Info)
)

const (
	ProblemTypeSyntaxError = "syntax_error"
	ProblemTypeBlankRule   = "blank_rule"

	ProblemTypeLackDescriptionField = "lack_description_field"
	ProblemTypeLackSolutionField    = "lack_solution_field"

	ProblemTypeMissingPositiveTestData = "missing_positive_test_data"
	ProblemTypeMissingNegativeTestData = "missing_negative_test_data"
	ProblemTypeTestCaseNotPass         = "test_case_not_pass"

	ProblemTypeMissingAlert = "missing_alert"
)

const (
	MaxScore = 100
	MinScore = 0

	SyntaxErrorPenalty           = 100
	BlankRulePenalty             = 100
	VerifyTestCaseNotPassPenalty = 100
	MissingAlertPenalty          = 100

	MissingDescriptionPenalty  = 40
	MissingSolutionPenalty     = 10
	MissingPositiveTestPenalty = 15
	MissingNegativeTestPenalty = 5

	GradeSMin = 100
	GradeAMin = 90
	GradeBMin = 80
	GradeCMin = 70
	GradeDMin = 60
)

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
