package sfanalysis

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

//go:embed testdata/demoRule.sf
var goodRuleDemo string // This rule contents both positive and negative tests and can pass tests. With solution, desc and alert

type wantProblem struct {
	typ      string
	severity string
}

type checkcase struct {
	name        string
	ruleContent string

	wantScore       int
	wantGrade       string
	wantMinProblems int

	// wantTotalProblems enforces exact problem count when >= 0.
	// Use -1 to skip exact count check (still enforces wantMinProblems).
	wantTotalProblems int

	// wantProblemsPrefix enforces problem list order for the first N problems.
	wantProblemsPrefix []wantProblem

	// wantContains asserts problems include these items (order-insensitive).
	wantContains []wantProblem

	// want*Count enforces exact severity counts when >= 0. Use -1 to skip.
	wantErrorCount   int
	wantWarningCount int
	wantInfoCount    int
}

type gradeCase struct {
	score     int
	wantGrade string
}

func clampScore(score int) int {
	if score < MinScore {
		return MinScore
	}
	return score
}

func hasProblem(problems []SyntaxFlowRuleProblem, want wantProblem) bool {
	for _, p := range problems {
		if p.Type == want.typ && p.Severity == want.severity {
			return true
		}
	}
	return false
}

func severityCounts(problems []SyntaxFlowRuleProblem) (errors, warnings, infos int) {
	for _, p := range problems {
		switch p.Severity {
		case Error:
			errors++
		case Warning:
			warnings++
		case Info:
			infos++
		}
	}
	return
}

func formatProblems(problems []SyntaxFlowRuleProblem) string {
	if len(problems) == 0 {
		return "(no problems)"
	}
	var b strings.Builder
	for i, p := range problems {
		fmt.Fprintf(&b, "%d) Type=%s Severity=%s Desc=%s Suggestion=%s\n", i+1, p.Type, p.Severity, p.Description, p.Suggestion)
	}
	return b.String()
}

func analyzeQualityContent(t *testing.T, ruleContent string) *SyntaxFlowRuleAnalyzeResult {
	t.Helper()

	report := Analyze(context.Background(), ruleContent, DefaultOptions(ProfileQuality))
	require.NotNil(t, report)
	require.NotNil(t, report.Quality)
	return report.Quality
}

func check(t *testing.T, cases []checkcase) {
	t.Helper()

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			result := analyzeQualityContent(t, tc.ruleContent)

			require.Equal(t, tc.wantScore, result.Score, "问题详情:\n%s", formatProblems(result.Problems))

			grade := GetGrade(result.Score)
			require.Equal(t, tc.wantGrade, grade, "score=%d\n问题详情:\n%s", result.Score, formatProblems(result.Problems))

			require.GreaterOrEqual(t, len(result.Problems), tc.wantMinProblems, "问题详情:\n%s", formatProblems(result.Problems))
			require.True(t, tc.wantTotalProblems < 0 || len(result.Problems) == tc.wantTotalProblems, "期望问题数 %d，实际 %d\n问题详情:\n%s", tc.wantTotalProblems, len(result.Problems), formatProblems(result.Problems))

			require.GreaterOrEqual(t, len(result.Problems), len(tc.wantProblemsPrefix), "问题详情:\n%s", formatProblems(result.Problems))
			for i := range tc.wantProblemsPrefix {
				got := result.Problems[i]
				want := tc.wantProblemsPrefix[i]
				require.Equal(t, want.typ, got.Type, "第%d个问题\n问题详情:\n%s", i+1, formatProblems(result.Problems))
				require.Equal(t, want.severity, got.Severity, "第%d个问题\n问题详情:\n%s", i+1, formatProblems(result.Problems))
			}

			for _, want := range tc.wantContains {
				require.True(t, hasProblem(result.Problems, want), "期望找到问题 Type=%s Severity=%s\n问题详情:\n%s", want.typ, want.severity, formatProblems(result.Problems))
			}

			errors, warnings, infos := severityCounts(result.Problems)
			require.True(t, tc.wantErrorCount < 0 || errors == tc.wantErrorCount, "期望 error 数量 %d，实际 %d\n问题详情:\n%s", tc.wantErrorCount, errors, formatProblems(result.Problems))
			require.True(t, tc.wantWarningCount < 0 || warnings == tc.wantWarningCount, "期望 warning 数量 %d，实际 %d\n问题详情:\n%s", tc.wantWarningCount, warnings, formatProblems(result.Problems))
			require.True(t, tc.wantInfoCount < 0 || infos == tc.wantInfoCount, "期望 info 数量 %d，实际 %d\n问题详情:\n%s", tc.wantInfoCount, infos, formatProblems(result.Problems))
		})
	}
}

func TestSyntaxFlowQuality_SyntaxError(t *testing.T) {
	check(t, []checkcase{
		{
			name:              "完全语法错误",
			ruleContent:       `invalid syntax here $$$`,
			wantScore:         clampScore(MaxScore - SyntaxErrorPenalty), // 100 - 100 = 0
			wantGrade:         "F",
			wantMinProblems:   1,
			wantTotalProblems: -1,
			wantContains: []wantProblem{
				{typ: ProblemTypeSyntaxError, severity: Error},
			},
			wantErrorCount:   -1,
			wantWarningCount: 0,
			wantInfoCount:    0,
		},
		{
			name: "部分语法错误",
			ruleContent: `desc(
	title: "测试规则"
)

test.* as $result &&& invalid syntax`,
			wantScore:         clampScore(MaxScore - SyntaxErrorPenalty),
			wantGrade:         "F",
			wantMinProblems:   1,
			wantTotalProblems: -1,
			wantContains: []wantProblem{
				{typ: ProblemTypeSyntaxError, severity: Error},
			},
			wantErrorCount:   -1,
			wantWarningCount: 0,
			wantInfoCount:    0,
		},
	})
}

func TestSyntaxFlowQuality_MissingAlert(t *testing.T) {
	check(t, []checkcase{
		{
			name: "缺少alert语句",
			ruleContent: `desc(
	title: "测试规则",
	description: "详细描述",
	solution: "解决方案"
)

test.* as $result`,
			wantScore:         clampScore(MaxScore - MissingAlertPenalty - MissingPositiveTestPenalty - MissingNegativeTestPenalty),
			wantGrade:         "F",
			wantMinProblems:   3,
			wantTotalProblems: 3,
			wantProblemsPrefix: []wantProblem{
				{typ: ProblemTypeMissingPositiveTestData, severity: Warning},
				{typ: ProblemTypeMissingNegativeTestData, severity: Warning},
				{typ: ProblemTypeMissingAlert, severity: Error},
			},
			wantErrorCount:   1,
			wantWarningCount: 2,
			wantInfoCount:    0,
		},
	})
}

func TestSyntaxFlowQuality_EmptyRule_NoError(t *testing.T) {
	check(t, []checkcase{
		{
			name:              "空字符串规则不应产生Error",
			ruleContent:       "",
			wantScore:         MinScore,
			wantGrade:         "F",
			wantMinProblems:   5,
			wantTotalProblems: 5,
			wantProblemsPrefix: []wantProblem{
				{typ: ProblemTypeLackDescriptionField, severity: Warning},
				{typ: ProblemTypeLackSolutionField, severity: Warning},
				{typ: ProblemTypeMissingPositiveTestData, severity: Warning},
				{typ: ProblemTypeMissingNegativeTestData, severity: Warning},
				{typ: ProblemTypeMissingAlert, severity: Warning},
			},
			wantErrorCount:   0,
			wantWarningCount: 5,
			wantInfoCount:    0,
		},
		{
			name:              "纯空白规则不应产生Error",
			ruleContent:       " \n\t ",
			wantScore:         MinScore,
			wantGrade:         "F",
			wantMinProblems:   5,
			wantTotalProblems: 5,
			wantProblemsPrefix: []wantProblem{
				{typ: ProblemTypeLackDescriptionField, severity: Warning},
				{typ: ProblemTypeLackSolutionField, severity: Warning},
				{typ: ProblemTypeMissingPositiveTestData, severity: Warning},
				{typ: ProblemTypeMissingNegativeTestData, severity: Warning},
				{typ: ProblemTypeMissingAlert, severity: Warning},
			},
			wantErrorCount:   0,
			wantWarningCount: 5,
			wantInfoCount:    0,
		},
	})
}

func TestSyntaxFlowQuality_DescriptionFields(t *testing.T) {
	check(t, []checkcase{
		{
			name: "缺少description字段",
			ruleContent: `desc(
	title: "测试规则",
	solution: "解决方案"
)

test.* as $result;
alert $result;`,
			wantScore:         MaxScore - MissingDescriptionPenalty - MissingPositiveTestPenalty - MissingNegativeTestPenalty, // 100 - 40 - 15 - 5 = 40
			wantGrade:         "F",
			wantMinProblems:   3,
			wantTotalProblems: 3,
			wantProblemsPrefix: []wantProblem{
				{typ: ProblemTypeLackDescriptionField, severity: Warning},
				{typ: ProblemTypeMissingPositiveTestData, severity: Warning},
				{typ: ProblemTypeMissingNegativeTestData, severity: Warning},
			},
			wantErrorCount:   0,
			wantWarningCount: 3,
			wantInfoCount:    0,
		},
		{
			name: "缺少solution字段",
			ruleContent: `desc(
	title: "测试规则",
	description: "详细描述"
)

test.* as $result;
alert $result;`,
			wantScore:         MaxScore - MissingSolutionPenalty - MissingPositiveTestPenalty - MissingNegativeTestPenalty, // 100 - 10 - 15 - 5 = 70
			wantGrade:         "C",
			wantMinProblems:   3,
			wantTotalProblems: 3,
			wantProblemsPrefix: []wantProblem{
				{typ: ProblemTypeLackSolutionField, severity: Warning},
				{typ: ProblemTypeMissingPositiveTestData, severity: Warning},
				{typ: ProblemTypeMissingNegativeTestData, severity: Warning},
			},
			wantErrorCount:   0,
			wantWarningCount: 3,
			wantInfoCount:    0,
		},
		{
			name: "缺少多个字段",
			ruleContent: `desc(
	title: "测试规则"
)

test.* as $result;
alert $result;`,
			wantScore:         MaxScore - MissingDescriptionPenalty - MissingSolutionPenalty - MissingPositiveTestPenalty - MissingNegativeTestPenalty, // 100 - 40 - 10 - 15 - 5 = 30
			wantGrade:         "F",
			wantMinProblems:   4,
			wantTotalProblems: 4,
			wantProblemsPrefix: []wantProblem{
				{typ: ProblemTypeLackDescriptionField, severity: Warning},
				{typ: ProblemTypeLackSolutionField, severity: Warning},
				{typ: ProblemTypeMissingPositiveTestData, severity: Warning},
				{typ: ProblemTypeMissingNegativeTestData, severity: Warning},
			},
			wantErrorCount:   0,
			wantWarningCount: 4,
			wantInfoCount:    0,
		},
		{
			name: "完整描述字段",
			ruleContent: `desc(
	title: "测试规则",
	description: "详细描述",
	solution: "解决方案"
)

test.* as $result;
alert $result;`,
			wantScore:         MaxScore - MissingPositiveTestPenalty - MissingNegativeTestPenalty, // 100 - 15 - 5 = 80
			wantGrade:         "B",
			wantMinProblems:   2,
			wantTotalProblems: 2,
			wantProblemsPrefix: []wantProblem{
				{typ: ProblemTypeMissingPositiveTestData, severity: Warning},
				{typ: ProblemTypeMissingNegativeTestData, severity: Warning},
			},
			wantErrorCount:   0,
			wantWarningCount: 2,
			wantInfoCount:    0,
		},
	})
}

func TestSyntaxFlowQuality_TestData(t *testing.T) {
	check(t, []checkcase{
		{
			name: "什么测试都没有",
			ruleContent: `desc(
	title: "测试规则",
	description: "详细描述",
	solution: "解决方案"
)

Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`,
			wantScore:         MaxScore - MissingPositiveTestPenalty - MissingNegativeTestPenalty, // 100 - 15 - 5 = 80
			wantGrade:         "B",
			wantMinProblems:   2,
			wantTotalProblems: 2,
			wantProblemsPrefix: []wantProblem{
				{typ: ProblemTypeMissingPositiveTestData, severity: Warning},
				{typ: ProblemTypeMissingNegativeTestData, severity: Warning},
			},
			wantErrorCount:   0,
			wantWarningCount: 2,
			wantInfoCount:    0,
		},
		{
			name: "只有正向测试没有反向测试",
			ruleContent: `desc(
	title: "测试规则",
	description: "详细描述",
	solution: "解决方案",
	alert_min: 1,
	alert_min: 1,
	language: java,
	'file://positive.java': <<<EOF
public class Test {
    public void vulnerable() {
        Runtime.getRuntime().exec("ls"); // 这里应该匹配规则
    }
}
EOF
)

Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`,
			wantScore:         MaxScore - MissingNegativeTestPenalty,
			wantGrade:         "A",
			wantMinProblems:   1,
			wantTotalProblems: 1,
			wantProblemsPrefix: []wantProblem{
				{typ: ProblemTypeMissingNegativeTestData, severity: Warning},
			},
			wantErrorCount:   0,
			wantWarningCount: 1,
			wantInfoCount:    0,
		},
		{
			name: "只有反向测试没有正向测试",
			ruleContent: `desc(
	title: "测试规则",
	description: "详细描述",
	solution: "解决方案",
	alert_min: 0,
	language: java,
	'safefile://negative.java': <<<SAFE
public class Safe {
    public void safe() {
        System.out.println("safe operation"); // 这里不应该匹配规则
    }
}
SAFE
)

Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`,
			wantScore:         MaxScore - MissingPositiveTestPenalty,
			wantGrade:         "B",
			wantMinProblems:   1,
			wantTotalProblems: 1,
			wantProblemsPrefix: []wantProblem{
				{typ: ProblemTypeMissingPositiveTestData, severity: Warning},
			},
			wantErrorCount:   0,
			wantWarningCount: 1,
			wantInfoCount:    0,
		},
		{
			name: "正反测试都有但正向测试不通过",
			ruleContent: `desc(
	title: "测试规则",
	description: "详细描述",
	solution: "解决方案",
	alert_min: 1,
	language: java,
	'file://positive.java': <<<EOF
public class Test {
    public void safe() {
        System.out.println("safe operation"); // 这里不会匹配规则，导致正向测试失败
    }
}
EOF,
	'safefile://negative.java': <<<SAFE
public class Safe {
    public void safe() {
        System.out.println("safe operation"); // 这里不应该匹配规则
    }
}
SAFE
)

Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`,
			wantScore:         MinScore, // 测试用例不通过直接0分
			wantGrade:         "F",
			wantMinProblems:   1,
			wantTotalProblems: 1,
			wantProblemsPrefix: []wantProblem{
				{typ: ProblemTypeTestCaseNotPass, severity: Error},
			},
			wantErrorCount:   1,
			wantWarningCount: 0,
			wantInfoCount:    0,
		},
		{
			name: "正反测试都有但反向测试不通过",
			ruleContent: `desc(
	title: "测试规则",
	description: "详细描述",
	solution: "解决方案",
	alert_min: 1,
	language: java,
	'file://positive.java': <<<EOF
public class Test {
    public void vulnerable() {
        Runtime.getRuntime().exec("ls"); // 这里应该匹配规则
    }
}
EOF,
	'safefile://negative.java': <<<SAFE
public class NotSafe {
    public void notSafe() {
        Runtime.getRuntime().exec("rm -rf /"); // 这里也会匹配规则，导致反向测试失败（误报）
    }
}
SAFE
)

Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`,
			wantScore:         MinScore, // 测试用例不通过直接0分
			wantGrade:         "F",
			wantMinProblems:   1,
			wantTotalProblems: 1,
			wantProblemsPrefix: []wantProblem{
				{typ: ProblemTypeTestCaseNotPass, severity: Error},
			},
			wantErrorCount:   1,
			wantWarningCount: 0,
			wantInfoCount:    0,
		},
		{
			name: "正反测试都有且都通过",
			ruleContent: `desc(
	title: "测试规则",
	description: "详细描述",
	solution: "解决方案",
	alert_min: 1,
	language: java,
	'file://positive.java': <<<EOF
public class Test {
    public void vulnerable() {
        Runtime.getRuntime().exec("ls"); // 这里应该匹配规则
    }
}
EOF,
	'safefile://negative.java': <<<SAFE
public class Safe {
    public void safe() {
        System.out.println("safe operation"); // 这里不应该匹配规则
    }
}
SAFE
)

Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`,
			wantScore:          MaxScore,
			wantGrade:          "S",
			wantMinProblems:    0,
			wantTotalProblems:  0,
			wantProblemsPrefix: []wantProblem{},
			wantErrorCount:     0,
			wantWarningCount:   0,
			wantInfoCount:      0,
		},
	})
}

func TestSyntaxFlowQuality_CompleteRule(t *testing.T) {
	require.NotEmpty(t, strings.TrimSpace(goodRuleDemo), "goodRuleDemo is empty")

	check(t, []checkcase{
		{
			name:               "demoRule.sf",
			ruleContent:        goodRuleDemo,
			wantScore:          MaxScore,
			wantGrade:          "S",
			wantMinProblems:    0,
			wantTotalProblems:  0,
			wantProblemsPrefix: []wantProblem{},
			wantErrorCount:     0,
			wantWarningCount:   0,
			wantInfoCount:      0,
		},
	})
}

func TestSyntaxFlowQuality_GradeSystem(t *testing.T) {
	tests := []gradeCase{
		{score: 100, wantGrade: "S"},
		{score: 95, wantGrade: "A"},
		{score: 90, wantGrade: "A"},
		{score: 85, wantGrade: "B"},
		{score: 80, wantGrade: "B"},
		{score: 75, wantGrade: "C"},
		{score: 70, wantGrade: "C"},
		{score: 65, wantGrade: "D"},
		{score: 60, wantGrade: "D"},
		{score: 55, wantGrade: "F"},
		{score: 0, wantGrade: "F"},
	}

	for _, tt := range tests {
		grade := GetGrade(tt.score)
		require.Equal(t, tt.wantGrade, grade, "score=%d", tt.score)
	}
}

func TestBatchAnalyze(t *testing.T) {
	rules := map[string]string{
		"perfect_rule": `desc(
	title: "完美规则",
	description: "这是一个完整的规则描述",
	solution: "修复建议",
	'file://positive.java': <<<JAVA
public class Test {
    public void vulnerable() {
        exec("rm -rf /");
    }
}
JAVA,
	'file://negative.java': <<<JAVA
public class Safe {
    public void safe() {
        System.out.println("safe");
    }
}
JAVA
)

exec as $sink;
alert $sink;`,
		"syntax_error_rule": `invalid syntax $$$`,
		"missing_alert_rule": `desc(
	title: "缺少alert",
	description: "描述",
	solution: "方案"
)

test.* as $result;`,
		"incomplete_rule": `desc(title: "不完整")
test.* as $result;
alert $result;`,
	}

	results := BatchAnalyze(rules)

	require.Len(t, results, 4)

	syntaxErrorResult, ok := results["syntax_error_rule"]
	require.True(t, ok)
	expectedScore := clampScore(MaxScore - SyntaxErrorPenalty)
	require.Equal(t, expectedScore, syntaxErrorResult.Score)

	missingAlertResult, ok := results["missing_alert_rule"]
	require.True(t, ok)
	require.Equal(t, 0, missingAlertResult.Score)

	incompleteResult, ok := results["incomplete_rule"]
	require.True(t, ok)
	require.NotEqual(t, MaxScore, incompleteResult.Score)

	for name, result := range results {
		t.Logf("规则 %s: 得分 %d, 问题数 %d", name, result.Score, len(result.Problems))
	}
}
