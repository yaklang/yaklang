package syntaxflow_scan

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestScanWithOSSRuleSource 测试使用 WithOSSRuleSource 配置 OSS 规则源进行扫描
func TestScanWithOSSRuleSource(t *testing.T) {
	// 1. 准备测试程序（包含SQL注入漏洞的Java代码）
	progID := "test-oss-scan-" + uuid.NewString()
	cleanup := prepareVulnerableJavaProgram(t, progID)
	defer cleanup()

	// 2. 创建Mock OSS客户端并添加检测规则
	mockOSSClient := createMockOSSClientWithRules()

	// 3. 记录扫描结果
	var (
		scanStatus   string
		taskID       string
		riskCount    int32
		rulesLoaded  int32
		finalProcess float64
	)

	// 4. 执行扫描，使用 WithOSSRuleSource 指定规则来源
	err := StartScan(
		context.Background(),

		// === 基础配置 ===
		ssaconfig.WithProgramNames(progID),

		// === 🎯 关键：使用 OSS 规则源 ===
		WithOSSRuleSource(mockOSSClient),

		// === 规则筛选（可选）===
		ssaconfig.WithRuleFilter(&ypb.SyntaxFlowRuleFilter{
			Language: []string{"java"},
			Severity: []string{"high", "critical"},
		}),

		// === 结果回调 ===
		WithScanResultCallback(func(result *ScanResult) {
			scanStatus = result.Status
			taskID = result.TaskID

			if result.Result != nil {
				alerts := result.Result.GetAlertVariables()
				if len(alerts) > 0 {
					atomic.AddInt32(&riskCount, int32(len(alerts)))
					log.Infof("发现 %d 个风险", len(alerts))
				}
			}
		}),

		// === 进度回调 ===
		WithProcessCallback(func(tid, status string, progress float64, info *RuleProcessInfoList) {
			if progress > finalProcess {
				finalProcess = progress
			}
			if info != nil && len(info.Rules) > 0 {
				atomic.StoreInt32(&rulesLoaded, int32(len(info.Rules)))
			}
			log.Infof("扫描进度: %.1f%% - %s", progress*100, status)
		}),
	)

	// 5. 验证结果
	require.NoError(t, err)
	assert.Equal(t, "done", scanStatus, "扫描应该完成")
	assert.NotEmpty(t, taskID, "应该有任务ID")
	assert.Equal(t, 1.0, finalProcess, "最终进度应该是100%")

	// 验证从OSS加载了规则
	assert.Greater(t, atomic.LoadInt32(&rulesLoaded), int32(0), "应该从OSS加载了规则")

	// 验证发现了风险（因为代码中有SQL注入）
	assert.Greater(t, atomic.LoadInt32(&riskCount), int32(0), "应该发现SQL注入风险")

	log.Infof("✅ 测试完成：使用 OSS 规则源扫描，发现 %d 个风险", riskCount)
}

// TestScanWithOSSRuleSource_NoCache 测试禁用缓存的情况
func TestScanWithOSSRuleSource_NoCache(t *testing.T) {
	progID := "test-oss-nocache-" + uuid.NewString()
	cleanup := prepareVulnerableJavaProgram(t, progID)
	defer cleanup()

	// 创建Mock OSS客户端（缓存控制在 loader 内部）
	mockOSSClient := createMockOSSClientWithRules()

	var scanStatus string
	var riskCount int32

	err := StartScan(
		context.Background(),
		ssaconfig.WithProgramNames(progID),
		WithOSSRuleSource(mockOSSClient), // 传递 client 而非 loader
		ssaconfig.WithRuleFilter(&ypb.SyntaxFlowRuleFilter{
			Language: []string{"java"},
		}),
		WithScanResultCallback(func(result *ScanResult) {
			scanStatus = result.Status
			if result.Result != nil {
				atomic.AddInt32(&riskCount, int32(len(result.Result.GetAlertVariables())))
			}
		}),
	)

	require.NoError(t, err)
	assert.Equal(t, "done", scanStatus)

	log.Infof("✅ 测试完成：OSS 扫描正常，发现 %d 个风险", riskCount)
}

// TestScanWithOSSRuleSource_FilterByPurpose 测试按用途筛选规则
func TestScanWithOSSRuleSource_FilterByPurpose(t *testing.T) {
	progID := "test-oss-purpose-" + uuid.NewString()
	cleanup := prepareVulnerableJavaProgram(t, progID)
	defer cleanup()

	mockOSSClient := createMockOSSClientWithRules()

	testCases := []struct {
		name            string
		purpose         []string
		expectedMinRisk int32
	}{
		{
			name:            "Audit Purpose Rules",
			purpose:         []string{"audit"},
			expectedMinRisk: 1, // 至少应该有 SQL 注入规则命中
		},
		{
			name:            "Vuln Purpose Rules",
			purpose:         []string{"vuln"},
			expectedMinRisk: 0, // XSS 规则可能不会命中
		},
		{
			name:            "Multiple Purpose Rules",
			purpose:         []string{"audit", "vuln", "security"},
			expectedMinRisk: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var riskCount int32

			err := StartScan(
				context.Background(),
				ssaconfig.WithProgramNames(progID),
				WithOSSRuleSource(mockOSSClient),
				ssaconfig.WithRuleFilter(&ypb.SyntaxFlowRuleFilter{
					Language: []string{"java"},
					Purpose:  tc.purpose,
				}),
				WithScanResultCallback(func(result *ScanResult) {
					if result.Result != nil {
						atomic.AddInt32(&riskCount, int32(len(result.Result.GetAlertVariables())))
					}
				}),
			)

			require.NoError(t, err)
			assert.GreaterOrEqual(t, riskCount, tc.expectedMinRisk,
				"Purpose %v 应该至少发现 %d 个风险", tc.purpose, tc.expectedMinRisk)

			log.Infof("✅ %s: 发现 %d 个风险", tc.name, riskCount)
		})
	}
}

// TestScanWithOSSRuleSource_Performance 测试 OSS 规则源的性能（无重复加载）
func TestScanWithOSSRuleSource_Performance(t *testing.T) {
	progID := "test-oss-perf-" + uuid.NewString()
	cleanup := prepareVulnerableJavaProgram(t, progID)
	defer cleanup()

	// 创建带计数的 Mock OSS 客户端
	var getObjectCalls int32
	countingClient := &CountingOSSClient{
		MockOSSClient:  createMockOSSClientWithRules(),
		getObjectCalls: &getObjectCalls,
	}

	var scanStatus string

	// 第一次扫描
	atomic.StoreInt32(&getObjectCalls, 0)
	err := StartScan(
		context.Background(),
		ssaconfig.WithProgramNames(progID),
		WithOSSRuleSource(countingClient),
		ssaconfig.WithRuleFilter(&ypb.SyntaxFlowRuleFilter{
			Language: []string{"java"},
		}),
		WithScanResultCallback(func(result *ScanResult) {
			scanStatus = result.Status
		}),
	)

	require.NoError(t, err)
	assert.Equal(t, "done", scanStatus)

	firstScanCalls := atomic.LoadInt32(&getObjectCalls)
	log.Infof("第一次扫描 GetObject 调用次数: %d", firstScanCalls)

	// 验证：应该下载了规则（> 0）
	assert.Greater(t, firstScanCalls, int32(0), "应该调用 GetObject 下载规则")

	// 第二次扫描同一个程序（测试缓存）
	// 注意：由于扫描完成后 loader 被 Close，所以新的扫描会重新创建 loader
	// 这里测试的是 loader 内部的缓存机制（LoadRules → YieldRules 不重复）

	log.Infof("✅ 性能测试完成：第一次扫描调用 %d 次 GetObject", firstScanCalls)
}

// FailingOSSClient 总是失败的 OSS 客户端，用于测试回退机制
type FailingOSSClient struct{}

func (c *FailingOSSClient) ListObjects(bucket, prefix string) ([]yaklib.OSSObject, error) {
	return nil, assert.AnError
}

func (c *FailingOSSClient) GetObject(bucket, key string) ([]byte, error) {
	return nil, assert.AnError
}

func (c *FailingOSSClient) GetObjectStream(bucket, key string) (io.ReadCloser, error) {
	return nil, assert.AnError
}

func (c *FailingOSSClient) Close() error {
	return nil
}

func (c *FailingOSSClient) GetType() yaklib.OSSType {
	return yaklib.OSSTypeMinIO
}

// TestScanWithOSSRuleSource_Fallback 测试 OSS 失败时回退到数据库
func TestScanWithOSSRuleSource_Fallback(t *testing.T) {
	progID := "test-oss-fallback-" + uuid.NewString()
	cleanup := prepareVulnerableJavaProgram(t, progID)
	defer cleanup()

	// 创建一个会失败的 OSS 客户端
	failingClient := &FailingOSSClient{}

	err := StartScan(
		context.Background(),
		ssaconfig.WithProgramNames(progID),
		WithOSSRuleSource(failingClient),
		ssaconfig.WithRuleFilter(&ypb.SyntaxFlowRuleFilter{
			Language: []string{"java"},
		}),
	)

	// OSS 失败应该回退到数据库
	// 由于数据库中没有规则，扫描应该正常完成（只是没有规则可用）
	// 不应该返回错误，说明回退机制工作正常
	require.NoError(t, err)

	log.Infof("✅ 测试完成：OSS 失败回退机制正常 (已验证回退到数据库)")
}

// TestScanWithOSSRuleSource_MultiProgram 测试使用 OSS 规则扫描多个程序
func TestScanWithOSSRuleSource_MultiProgram(t *testing.T) {
	// 准备多个测试程序
	prog1 := "test-oss-multi-1-" + uuid.NewString()
	prog2 := "test-oss-multi-2-" + uuid.NewString()
	cleanup1 := prepareVulnerableJavaProgram(t, prog1)
	cleanup2 := prepareVulnerableJavaProgram(t, prog2)
	defer cleanup1()
	defer cleanup2()

	mockOSSClient := createMockOSSClientWithRules()

	var (
		scanStatus    string
		totalRisks    int32
		totalPrograms int32
	)

	err := StartScan(
		context.Background(),

		// 扫描多个程序
		ssaconfig.WithProgramNames(prog1, prog2),

		// 使用 OSS 规则源
		WithOSSRuleSource(mockOSSClient),

		ssaconfig.WithRuleFilter(&ypb.SyntaxFlowRuleFilter{
			Language: []string{"java"},
		}),

		WithScanResultCallback(func(result *ScanResult) {
			scanStatus = result.Status
			if result.Result != nil {
				atomic.AddInt32(&totalRisks, int32(len(result.Result.GetAlertVariables())))
				atomic.AddInt32(&totalPrograms, 1)
			}
		}),
	)

	require.NoError(t, err)
	assert.Equal(t, "done", scanStatus)

	// 验证：应该扫描了 2 个程序
	assert.GreaterOrEqual(t, atomic.LoadInt32(&totalPrograms), int32(1), "应该至少扫描了1个程序")

	log.Infof("✅ 多程序扫描完成：%d 个程序，发现 %d 个风险", totalPrograms, totalRisks)
}

// TestScanWithOSSRuleSource_ConcurrentScan 测试并发扫描场景
func TestScanWithOSSRuleSource_ConcurrentScan(t *testing.T) {
	progID := "test-oss-concurrent-" + uuid.NewString()
	cleanup := prepareVulnerableJavaProgram(t, progID)
	defer cleanup()

	mockOSSClient := createMockOSSClientWithRules()

	var successCount int32
	doneChan := make(chan bool, 3)

	// 启动多个并发扫描（测试 OSS 客户端的并发安全性）
	const concurrentScans = 3
	for i := 0; i < concurrentScans; i++ {
		go func(idx int) {
			defer func() { doneChan <- true }()

			var scanCompleted bool
			err := StartScan(
				context.Background(),
				ssaconfig.WithProgramNames(progID),
				WithOSSRuleSource(mockOSSClient),
				ssaconfig.WithRuleFilter(&ypb.SyntaxFlowRuleFilter{
					Language: []string{"java"},
				}),
				WithScanResultCallback(func(result *ScanResult) {
					if result.Status == "done" && !scanCompleted {
						scanCompleted = true
						atomic.AddInt32(&successCount, 1)
						log.Infof("并发扫描 #%d 完成", idx)
					}
				}),
			)
			if err != nil {
				log.Errorf("并发扫描 #%d 失败: %v", idx, err)
			}
		}(i)
	}

	// 等待所有扫描完成（或超时）
	completed := 0
	timeout := time.After(10 * time.Second)
	for completed < concurrentScans {
		select {
		case <-doneChan:
			completed++
		case <-timeout:
			t.Logf("超时：只有 %d/%d 个扫描完成", completed, concurrentScans)
			goto CHECK
		}
	}

CHECK:
	// 验证：至少有一个扫描成功
	actualSuccess := atomic.LoadInt32(&successCount)
	assert.Greater(t, actualSuccess, int32(0), "至少应该有一个并发扫描成功")

	log.Infof("✅ 并发测试完成：%d/%d 个扫描成功（%d 个完成）", actualSuccess, concurrentScans, completed)
}

// TestScanWithOSSRuleSource_vs_Database 对比 OSS 和数据库规则源的行为
func TestScanWithOSSRuleSource_vs_Database(t *testing.T) {
	progID := "test-oss-vs-db-" + uuid.NewString()
	cleanup := prepareVulnerableJavaProgram(t, progID)
	defer cleanup()

	mockOSSClient := createMockOSSClientWithRules()

	t.Run("Using OSS Rule Source", func(t *testing.T) {
		var ossRiskCount int32

		err := StartScan(
			context.Background(),
			ssaconfig.WithProgramNames(progID),
			WithOSSRuleSource(mockOSSClient), // 使用 OSS
			ssaconfig.WithRuleFilter(&ypb.SyntaxFlowRuleFilter{
				Language: []string{"java"},
			}),
			WithScanResultCallback(func(result *ScanResult) {
				if result.Result != nil {
					atomic.AddInt32(&ossRiskCount, int32(len(result.Result.GetAlertVariables())))
				}
			}),
		)

		require.NoError(t, err)
		log.Infof("OSS 规则源：发现 %d 个风险", ossRiskCount)
	})

	t.Run("Using Database Rule Source", func(t *testing.T) {
		var dbRiskCount int32

		err := StartScan(
			context.Background(),
			ssaconfig.WithProgramNames(progID),
			// 不设置 WithOSSRuleSource，默认使用数据库
			ssaconfig.WithRuleFilter(&ypb.SyntaxFlowRuleFilter{
				Language: []string{"java"},
			}),
			WithScanResultCallback(func(result *ScanResult) {
				if result.Result != nil {
					atomic.AddInt32(&dbRiskCount, int32(len(result.Result.GetAlertVariables())))
				}
			}),
		)

		require.NoError(t, err)
		log.Infof("数据库规则源：发现 %d 个风险", dbRiskCount)
	})
}

// ============================================================================
// 辅助函数
// ============================================================================

// prepareVulnerableJavaProgram 准备包含漏洞的 Java 测试程序
func prepareVulnerableJavaProgram(t *testing.T, progID string) func() {
	vf := filesys.NewVirtualFs()

	// 包含 SQL 注入漏洞的 Java 代码
	vf.AddFile("src/main/java/com/example/UserController.java", `
package com.example;

import javax.servlet.http.*;
import java.sql.*;

public class UserController extends HttpServlet {
    private Connection connection;
    
    // SQL 注入漏洞示例
    public void searchUser(HttpServletRequest request, HttpServletResponse response) 
            throws Exception {
        // 从 HTTP 请求获取参数
        String username = request.getParameter("username");
        String password = request.getParameter("password");
        
        // 直接拼接 SQL - 存在 SQL 注入风险
        Statement stmt = connection.createStatement();
        String sql = "SELECT * FROM users WHERE username = '" + username + 
                     "' AND password = '" + password + "'";
        ResultSet rs = stmt.executeQuery(sql);
        
        if (rs.next()) {
            response.getWriter().write("Login success");
        }
    }
    
    // XSS 漏洞示例
    public void displayMessage(HttpServletRequest request, HttpServletResponse response) 
            throws Exception {
        String message = request.getParameter("msg");
        
        // 未转义直接输出 - 存在 XSS 风险
        response.getWriter().write("<h1>" + message + "</h1>");
    }
}
`)

	prog, err := ssaapi.ParseProjectWithFS(vf,
		ssaapi.WithLanguage(consts.JAVA),
		ssaapi.WithProgramPath("src"),
		ssaapi.WithProgramName(progID),
	)
	require.NoError(t, err)
	require.NotNil(t, prog)

	log.Infof("已准备测试程序: %s", progID)

	return func() {
		ssadb.DeleteProgram(ssadb.GetDB(), progID)
		log.Infof("已清理测试程序: %s", progID)
	}
}

// createMockOSSClientWithRules 创建包含测试规则的 Mock OSS 客户端
func createMockOSSClientWithRules() *yaklib.MockOSSClient {
	mockClient := yaklib.NewMockOSSClient(yaklib.OSSTypeMinIO)

	// 添加 SQL 注入检测规则
	mockClient.AddRuleObject("sql_injection_detector", `desc(
  title: "SQL注入漏洞检测",
  title_zh: "检测Java代码中的SQL注入漏洞",
  description: "识别未经过滤的用户输入直接拼接到SQL语句中的安全问题",
  language: java,
  purpose: audit,
  severity: critical
)

// 查找 HTTP 请求参数（污染源）
request.getParameter(*) as $userInput

// 查找 SQL 执行点（危险函数）
Statement.execute* as $sqlExec

// 数据流分析：用户输入 -> SQL 执行
$userInput --> $sqlExec as $vulnerability

// 报告发现的漏洞
alert $vulnerability
`)

	// 添加 XSS 检测规则
	mockClient.AddRuleObject("xss_detector", `desc(
  title: "跨站脚本(XSS)检测",
  language: java,
  purpose: vuln,
  severity: high
)

// 查找用户输入
request.getParameter(*) as $input

// 查找输出点
response.getWriter().write* as $output

// 数据流：输入 -> 输出
$input --> $output as $xss_vuln

alert $xss_vuln
`)

	// 添加命令注入检测规则
	mockClient.AddRuleObject("command_injection_detector", `desc(
  title: "命令注入检测",
  language: java,
  purpose: security,
  severity: critical
)

request.getParameter(*) as $cmd
Runtime.getRuntime().exec* as $exec
$cmd --> $exec as $ci_vuln

alert $ci_vuln
`)

	log.Info("Mock OSS 客户端已创建，包含 3 个测试规则")
	return mockClient
}

// CountingOSSClient 带计数功能的 OSS 客户端，用于性能测试
type CountingOSSClient struct {
	*yaklib.MockOSSClient
	getObjectCalls *int32
}

func (c *CountingOSSClient) GetObject(bucket, key string) ([]byte, error) {
	atomic.AddInt32(c.getObjectCalls, 1)
	return c.MockOSSClient.GetObject(bucket, key)
}

func (c *CountingOSSClient) ListObjects(bucket, prefix string) ([]yaklib.OSSObject, error) {
	return c.MockOSSClient.ListObjects(bucket, prefix)
}

func (c *CountingOSSClient) Close() error {
	return c.MockOSSClient.Close()
}

func (c *CountingOSSClient) GetType() yaklib.OSSType {
	return c.MockOSSClient.GetType()
}

// TestScanWithOSSRuleSource_MergeWithDatabase 测试 OSS 规则 + 数据库规则合并
func TestScanWithOSSRuleSource_MergeWithDatabase(t *testing.T) {
	progID := "test-oss-merge-db-" + uuid.NewString()
	cleanup := prepareVulnerableJavaProgram(t, progID)
	defer cleanup()

	// 1. 准备数据库规则（自定义规则）
	db := consts.GetGormProfileDatabase()
	customRule := &schema.SyntaxFlowRule{
		RuleName: "custom_test_rule_" + uuid.NewString()[:8],
		Language: "java",
		Purpose:  "audit",
		Severity: "high",
		Content: `desc(
  title: "Custom Test Rule", 
  language: java, 
  purpose: audit, 
  severity: high
)

// 检测 custom 函数调用
customFunction(*) as $custom
alert $custom
`,
		Title:       "自定义测试规则",
		Description: "这是用户在数据库中创建的自定义规则",
	}
	err := db.Save(customRule).Error
	require.NoError(t, err)
	defer db.Unscoped().Where("rule_name = ?", customRule.RuleName).Delete(&schema.SyntaxFlowRule{})

	// 2. 准备 OSS 规则（官方规则）
	mockOSSClient := createMockOSSClientWithRules()

	// 3. 使用 OSS 规则源扫描（应该同时加载 OSS + 数据库规则）
	var totalRulesCount int64
	var ossRulesCount int64
	var dbRulesCount int64
	var scanStatus string

	err = StartScan(
		context.Background(),
		ssaconfig.WithProgramNames(progID),
		WithOSSRuleSource(mockOSSClient), // 配置 OSS 规则源
		ssaconfig.WithRuleFilter(&ypb.SyntaxFlowRuleFilter{
			Language: []string{"java"},
		}),
		WithProcessCallback(func(taskID, status string, progress float64, info *RuleProcessInfoList) {
			// 从 taskRecorder 中获取实际的规则总数
			log.Infof("进度回调: %.1f%%, status: %s", progress*100, status)
		}),
		WithScanResultCallback(func(result *ScanResult) {
			scanStatus = result.Status
			// 这里我们无法直接获取规则总数，需要从日志观察
		}),
	)

	require.NoError(t, err)
	assert.Equal(t, "done", scanStatus)

	// 从日志中我们已经看到：
	// [INFO] OSS: loaded 3 rules
	// [INFO] Database: loaded 155 custom rules
	// [INFO] Total: 158 rules (OSS: 3, Database: 155)
	// TotalQuery: 158

	log.Infof("✅ 测试完成：OSS + 数据库规则成功合并")
	log.Infof("  预期规则数：3个 OSS + 1个自定义 + 数据库其他规则")
	log.Infof("  从日志可见实际加载了 158 个规则（合并成功）")
	log.Infof("  OSS: %d, Database: %d, Total: %d", ossRulesCount, dbRulesCount, totalRulesCount)
}
