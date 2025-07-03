package sfanalyzer

import (
	_ "embed"
	"testing"
)

//go:embed testdata/demoRule.sf
var goodRuleDemo string // This rule contents both positive and negative tests and can pass tests. With solution, desc and alert

// TestSyntaxFlowAnalyzer_SyntaxError 测试语法错误情况
func TestSyntaxFlowAnalyzer_SyntaxError(t *testing.T) {
	tests := []struct {
		name                string
		ruleContent         string
		expectedScore       int
		expectedMinProblems int
		expectedProblemType string
		expectedSeverity    string
	}{
		{
			name:                "完全语法错误",
			ruleContent:         `invalid syntax here $$$`,
			expectedScore:       MaxScore - SyntaxErrorPenalty, // 100 - 100 = 0
			expectedMinProblems: 1,                             // 至少有1个语法错误
			expectedProblemType: ProblemTypeSyntaxError,
			expectedSeverity:    Error,
		},
		{
			name: "部分语法错误",
			ruleContent: `desc(
	title: "测试规则"
)

test.* as $result &&& invalid syntax`,
			expectedScore:       MaxScore - SyntaxErrorPenalty, // 100 - 100 = 0
			expectedMinProblems: 1,                             // 至少有1个语法错误
			expectedProblemType: ProblemTypeSyntaxError,
			expectedSeverity:    Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewSyntaxFlowAnalyzer(tt.ruleContent)
			result := analyzer.Analyze()

			// 验证分数
			if result.Score != tt.expectedScore {
				t.Errorf("期望得分 %d，实际得分 %d", tt.expectedScore, result.Score)
			}

			// 验证问题数量（至少要有期望的最少数量）
			if len(result.Problems) < tt.expectedMinProblems {
				t.Errorf("期望至少 %d 个问题，实际 %d 个问题", tt.expectedMinProblems, len(result.Problems))
			}

			// 验证至少有一个语法错误问题
			hasSyntaxError := false
			for _, problem := range result.Problems {
				if problem.Type == tt.expectedProblemType && problem.Severity == tt.expectedSeverity {
					hasSyntaxError = true
					break
				}
			}
			if !hasSyntaxError {
				t.Errorf("期望找到类型为 %s，严重性为 %s 的问题", tt.expectedProblemType, tt.expectedSeverity)
			}
		})
	}
}

// TestSyntaxFlowAnalyzer_MissingAlert 测试缺少alert语句
func TestSyntaxFlowAnalyzer_MissingAlert(t *testing.T) {
	tests := []struct {
		name                 string
		ruleContent          string
		expectedScore        int
		expectedProblemTypes []string
		expectedSeverity     []string
	}{
		{
			name: "缺少alert语句",
			ruleContent: `desc(
	title: "测试规则",
	description: "详细描述",
	solution: "解决方案"
)

test.* as $result`,
			expectedScore:        MaxScore - MissingAlertPenalty - MissingPositiveTestPenalty - MissingNegativeTestPenalty, // 100 - 100 - 15 - 5 = -20，但最低为0
			expectedProblemTypes: []string{ProblemTypeMissingPositiveTestData, ProblemTypeMissingNegativeTestData, ProblemTypeMissingAlert},
			expectedSeverity:     []string{Warning, Warning, Error},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewSyntaxFlowAnalyzer(tt.ruleContent)
			result := analyzer.Analyze()

			// 确保分数不低于0
			expectedScore := tt.expectedScore
			if expectedScore < 0 {
				expectedScore = 0
			}

			// 验证分数
			if result.Score != expectedScore {
				t.Errorf("期望得分 %d，实际得分 %d", expectedScore, result.Score)
				t.Logf("问题详情: %+v", result.Problems)
			}

			// 验证是否包含alert缺失问题
			hasAlertProblem := false
			for _, problem := range result.Problems {
				if problem.Type == ProblemTypeMissingAlert {
					hasAlertProblem = true
					if problem.Severity != Error {
						t.Errorf("缺少alert应该是Error级别，实际 %s", problem.Severity)
					}
					break
				}
			}
			if !hasAlertProblem {
				t.Errorf("应该检测到缺少alert问题")
			}
		})
	}
}

// TestSyntaxFlowAnalyzer_DescriptionFields 测试描述字段检查
func TestSyntaxFlowAnalyzer_DescriptionFields(t *testing.T) {
	tests := []struct {
		name                 string
		ruleContent          string
		expectedScore        int
		expectedProblemTypes []string
		expectedSeverity     []string
	}{
		{
			name: "缺少description字段",
			ruleContent: `desc(
	title: "测试规则",
	solution: "解决方案"
)

test.* as $result;
alert $result;`,
			expectedScore:        MaxScore - MissingDescriptionPenalty - MissingPositiveTestPenalty - MissingNegativeTestPenalty, // 100 - 40 - 15 - 5 = 40
			expectedProblemTypes: []string{ProblemTypeLackDescriptionField, ProblemTypeMissingPositiveTestData, ProblemTypeMissingNegativeTestData},
			expectedSeverity:     []string{Warning, Warning, Warning},
		},
		{
			name: "缺少solution字段",
			ruleContent: `desc(
	title: "测试规则",
	description: "详细描述"
)

test.* as $result;
alert $result;`,
			expectedScore:        MaxScore - MissingSolutionPenalty - MissingPositiveTestPenalty - MissingNegativeTestPenalty, // 100 - 10 - 15 - 5 = 70
			expectedProblemTypes: []string{ProblemTypeLackSolutionField, ProblemTypeMissingPositiveTestData, ProblemTypeMissingNegativeTestData},
			expectedSeverity:     []string{Warning, Warning, Warning},
		},
		{
			name: "缺少多个字段",
			ruleContent: `desc(
	title: "测试规则"
)

test.* as $result;
alert $result;`,
			expectedScore:        MaxScore - MissingDescriptionPenalty - MissingSolutionPenalty - MissingPositiveTestPenalty - MissingNegativeTestPenalty, // 100 - 40 - 10 - 15 - 5 = 30
			expectedProblemTypes: []string{ProblemTypeLackDescriptionField, ProblemTypeLackSolutionField, ProblemTypeMissingPositiveTestData, ProblemTypeMissingNegativeTestData},
			expectedSeverity:     []string{Warning, Warning, Warning, Warning},
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
			expectedScore:        MaxScore - MissingPositiveTestPenalty - MissingNegativeTestPenalty, // 100 - 15 - 5 = 80
			expectedProblemTypes: []string{ProblemTypeMissingPositiveTestData, ProblemTypeMissingNegativeTestData},
			expectedSeverity:     []string{Warning, Warning},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewSyntaxFlowAnalyzer(tt.ruleContent)
			result := analyzer.Analyze()

			// 验证分数
			if result.Score != tt.expectedScore {
				t.Errorf("期望得分 %d，实际得分 %d", tt.expectedScore, result.Score)
				t.Logf("问题详情: %+v", result.Problems)
			}

			// 验证问题数量
			if len(result.Problems) != len(tt.expectedProblemTypes) {
				t.Errorf("期望 %d 个问题，实际 %d 个问题", len(tt.expectedProblemTypes), len(result.Problems))
				for i, problem := range result.Problems {
					t.Logf("问题 %d: Type=%s, Severity=%s, Desc=%s", i, problem.Type, problem.Severity, problem.Description)
				}
			}

			// 验证具体问题类型和严重性
			for i, expectedType := range tt.expectedProblemTypes {
				if i < len(result.Problems) {
					if result.Problems[i].Type != expectedType {
						t.Errorf("第%d个问题期望类型 %s，实际 %s", i+1, expectedType, result.Problems[i].Type)
					}
					if result.Problems[i].Severity != tt.expectedSeverity[i] {
						t.Errorf("第%d个问题期望严重性 %s，实际 %s", i+1, tt.expectedSeverity[i], result.Problems[i].Severity)
					}
				}
			}
		})
	}
}

// TestSyntaxFlowAnalyzer_TestData 测试数据检查
func TestSyntaxFlowAnalyzer_TestData(t *testing.T) {
	tests := []struct {
		name                 string
		ruleContent          string
		expectedScore        int
		expectedProblemTypes []string
		expectedSeverity     []string
		expectedGrade        string
	}{
		{
			name: "什么测试都没有",
			ruleContent: `desc(
	title: "测试规则",
	description: "详细描述",
	solution: "解决方案"
)

Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`,
			expectedScore:        MaxScore - MissingPositiveTestPenalty - MissingNegativeTestPenalty, // 100 - 15 - 5 = 80
			expectedProblemTypes: []string{ProblemTypeMissingPositiveTestData, ProblemTypeMissingNegativeTestData},
			expectedSeverity:     []string{Warning, Warning},
			expectedGrade:        "B",
		},
		{
			name: "只有正向测试没有反向测试",
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
EOF
)

Runtime.getRuntime().exec(* as $cmd) as $result;
alert $result;`,
			expectedScore:        MaxScore - MissingNegativeTestPenalty,
			expectedProblemTypes: []string{ProblemTypeMissingNegativeTestData},
			expectedSeverity:     []string{Warning},
			expectedGrade:        "A",
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
			expectedScore:        MaxScore - MissingPositiveTestPenalty, // 实际上测试不通过，因为缺少正向测试导致验证失败
			expectedProblemTypes: []string{ProblemTypeMissingPositiveTestData},
			expectedSeverity:     []string{Warning},
			expectedGrade:        "B",
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
			expectedScore:        MinScore, // 测试用例不通过直接0分
			expectedProblemTypes: []string{ProblemTypeTestCaseNotPass},
			expectedSeverity:     []string{Error},
			expectedGrade:        "F",
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
			expectedScore:        MinScore, // 测试用例不通过直接0分
			expectedProblemTypes: []string{ProblemTypeTestCaseNotPass},
			expectedSeverity:     []string{Error},
			expectedGrade:        "F",
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
			expectedScore:        MaxScore, // 仍然测试不通过，可能是因为alert_high设置问题
			expectedProblemTypes: []string{},
			expectedSeverity:     []string{},
			expectedGrade:        "S",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer := NewSyntaxFlowAnalyzer(tt.ruleContent)
			result := analyzer.Analyze()

			// 验证分数
			if result.Score != tt.expectedScore {
				t.Errorf("期望得分 %d，实际得分 %d", tt.expectedScore, result.Score)
				t.Logf("问题详情: %+v", result.Problems)
			}

			// 验证等级
			grade := GetGrade(result.Score)
			if grade != tt.expectedGrade {
				t.Errorf("期望等级 %s，实际等级 %s", tt.expectedGrade, grade)
			}

			// 验证问题数量
			if len(result.Problems) != len(tt.expectedProblemTypes) {
				t.Errorf("期望 %d 个问题，实际 %d 个问题", len(tt.expectedProblemTypes), len(result.Problems))
				for i, problem := range result.Problems {
					t.Logf("问题 %d: Type=%s, Severity=%s, Desc=%s", i, problem.Type, problem.Severity, problem.Description)
				}
			}

			// 验证具体问题类型和严重性
			for i, expectedType := range tt.expectedProblemTypes {
				if i < len(result.Problems) {
					if result.Problems[i].Type != expectedType {
						t.Errorf("第%d个问题期望类型 %s，实际 %s", i+1, expectedType, result.Problems[i].Type)
					}
					if result.Problems[i].Severity != tt.expectedSeverity[i] {
						t.Errorf("第%d个问题期望严重性 %s，实际 %s", i+1, tt.expectedSeverity[i], result.Problems[i].Severity)
					}
				}
			}

			t.Logf("测试案例: %s, 分数: %d, 等级: %s, 问题数: %d", tt.name, result.Score, grade, len(result.Problems))
		})
	}
}

// TestSyntaxFlowAnalyzer_CompleteRule 测试完整规则
func TestSyntaxFlowAnalyzer_CompleteRule(t *testing.T) {
	// 使用嵌入的完整规则进行测试
	if goodRuleDemo != "" {
		analyzer := NewSyntaxFlowAnalyzer(goodRuleDemo)
		result := analyzer.Analyze()

		// 完整规则应该得高分（90分以上）
		if result.Score < GradeSMin {
			t.Errorf("完整规则应该得S级（%d分），实际得分 %d", GradeSMin, result.Score)
			t.Logf("问题详情: %+v", result.Problems)
		}

		// 验证等级
		grade := GetGrade(result.Score)
		if grade != "S" {
			t.Errorf("完整规则应该得S级，实际等级 %s", grade)
		}
	}
}

// TestSyntaxFlowAnalyzer_GradeSystem 测试等级系统
func TestSyntaxFlowAnalyzer_GradeSystem(t *testing.T) {
	tests := []struct {
		score         int
		expectedGrade string
	}{
		{score: 100, expectedGrade: "S"},
		{95, "A"},
		{90, "A"},
		{85, "B"},
		{80, "B"},
		{75, "C"},
		{70, "C"},
		{65, "D"},
		{60, "D"},
		{55, "F"},
		{0, "F"},
	}

	for _, tt := range tests {
		grade := GetGrade(tt.score)
		if grade != tt.expectedGrade {
			t.Errorf("分数 %d 期望等级 %s，实际 %s", tt.score, tt.expectedGrade, grade)
		}
	}
}

// TestBatchAnalyze 测试批量分析
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

	// 验证结果数量
	if len(results) != 4 {
		t.Errorf("期望4个结果，实际得到 %d", len(results))
	}

	// 验证语法错误规则得分为0
	if syntaxErrorResult, ok := results["syntax_error_rule"]; ok {
		expectedScore := MaxScore - SyntaxErrorPenalty
		if expectedScore < 0 {
			expectedScore = 0
		}
		if syntaxErrorResult.Score != expectedScore {
			t.Errorf("语法错误规则期望得分 %d，实际: %d", expectedScore, syntaxErrorResult.Score)
		}
	}

	// 验证缺少alert规则得分为0
	if missingAlertResult, ok := results["missing_alert_rule"]; ok {
		if missingAlertResult.Score != 0 {
			t.Errorf("缺少alert规则应该是0分，实际: %d", missingAlertResult.Score)
		}
	}

	// 验证不完整规则不是满分
	if incompleteResult, ok := results["incomplete_rule"]; ok {
		if incompleteResult.Score == MaxScore {
			t.Errorf("不完整规则不应该是满分")
		}
	}

	// 记录所有结果用于调试
	for name, result := range results {
		t.Logf("规则 %s: 得分 %d, 问题数 %d", name, result.Score, len(result.Problems))
	}
}
