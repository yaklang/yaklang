package yakgrpc

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// createMockProgram 创建模拟程序数据
func createMockProgram(programName, language, description string, fileCount, lineCount int) *ssadb.IrProgram {
	// 程序信息存储在SSA数据库
	db := ssadb.GetDB()
	program := &ssadb.IrProgram{
		ProgramName: programName,
		Description: description,
		Language:    language,
		ProgramKind: ssadb.Application,
		FileList:    make(ssadb.StringMap),
		LineCount:   lineCount,
	}

	// 模拟文件列表
	for i := 0; i < fileCount; i++ {
		fileName := fmt.Sprintf("/src/file%d.%s", i+1, getFileExtension(language))
		program.FileList[fileName] = fmt.Sprintf("file%d", i+1)
	}

	db.Save(program)
	return program
}

// getFileExtension 根据语言获取文件扩展名
func getFileExtension(language string) string {
	switch language {
	case "java":
		return "java"
	case "php":
		return "php"
	case "javascript":
		return "js"
	case "python":
		return "py"
	case "go":
		return "go"
	default:
		return "txt"
	}
}

// createMockTask 创建模拟扫描任务
func createMockTask(taskId, programName string, status string, riskCounts map[string]int64, ruleCount int64) *schema.SyntaxFlowScanTask {
	db := consts.GetGormDefaultSSADataBase()
	task := &schema.SyntaxFlowScanTask{
		TaskId:        taskId,
		Programs:      programName,
		Status:        status,
		Kind:          schema.SFResultKindScan,
		RulesCount:    ruleCount,
		RiskCount:     riskCounts["total"],
		CriticalCount: riskCounts["critical"],
		HighCount:     riskCounts["high"],
		WarningCount:  riskCounts["middle"],
		LowCount:      riskCounts["low"],
		InfoCount:     riskCounts["info"],
		TotalQuery:    ruleCount,
		SuccessQuery:  ruleCount,
	}
	db.Save(task)
	return task
}

// createMockRisk 创建模拟风险数据
func createMockRisk(taskId, programName, riskType, severity, fromRule, title, titleVerbose, description, solution, filePath, functionName, codeFragment string, line int64) *schema.SSARisk {
	// SSA风险数据存储在SSA数据库
	db := consts.GetGormDefaultSSADataBase()
	risk := &schema.SSARisk{
		Title:         title,
		TitleVerbose:  titleVerbose,
		Description:   description,
		Solution:      solution,
		RiskType:      riskType,
		Severity:      schema.SyntaxFlowSeverity(severity),
		FromRule:      fromRule,
		ProgramName:   programName,
		RuntimeId:     taskId,
		CodeSourceUrl: filePath,
		CodeRange:     fmt.Sprintf("%d-%d", line, line+2),
		CodeFragment:  codeFragment,
		FunctionName:  functionName,
		Line:          line,
		Language:      "java",
	}
	risk.Hash = risk.CalcHash()
	db.Save(risk)
	return risk
}

// cleanupMockData 清理模拟数据
func cleanupMockData(taskId, programName string) {
	// yakit数据库 - 存储报告数据
	//projectDB := consts.GetGormProjectDatabase()
	// SSA数据库 - 存储程序信息和风险数据
	ssaDB := consts.GetGormDefaultSSADataBase()

	// 清理扫描任务数据
	ssaDB.Where("task_id = ?", taskId).Unscoped().Delete(&schema.SyntaxFlowScanTask{})

	// 清理SSA风险数据（存储在SSA数据库）
	ssaDB.Where("runtime_id = ?", taskId).Unscoped().Delete(&schema.SSARisk{})

	// 清理程序数据（存储在SSA数据库）
	ssaDB.Where("program_name = ?", programName).Unscoped().Delete(&ssadb.IrProgram{})

	// 清理报告数据（存储在yakit数据库）
	// projectDB.Where("title = ?", programName).Unscoped().Delete(&schema.ReportRecord{})
}

// TestGenerateSSAReport_NoRisks 测试无风险的极端情况
func TestGenerateSSAReport_NoRisks(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
	}

	server, err := NewTestServer()
	if err != nil {
		t.Fatalf("创建测试服务器失败: %v", err)
	}

	// 创建测试数据
	taskId := uuid.New().String()
	programName := "test-empty-project"

	// 清理数据（防止之前的测试数据影响）
	defer cleanupMockData(taskId, programName)

	// 创建程序
	createMockProgram(programName, "java", "测试空项目 - 无任何风险", 5, 1000)

	// 创建无风险的任务
	riskCounts := map[string]int64{
		"total": 0, "critical": 0, "high": 0, "middle": 0, "low": 0, "info": 0,
	}
	createMockTask(taskId, programName, schema.SYNTAXFLOWSCAN_DONE, riskCounts, 10)

	// 测试报告生成
	req := &ypb.GenerateSSAReportRequest{
		TaskID:     taskId,
		ReportName: "无风险项目测试报告",
	}

	ctx := context.Background()
	resp, err := server.GenerateSSAReport(ctx, req)
	if err != nil {
		t.Fatalf("生成SSA报告失败: %v", err)
	}

	// 验证响应
	if resp == nil {
		t.Fatal("响应为空")
	}

	if !resp.Success {
		t.Fatalf("报告生成失败: %s", resp.Message)
	}

	if resp.ReportData == "" {
		t.Fatal("报告数据为空")
	}

	t.Logf("无风险项目报告生成成功! 报告ID: %s", resp.ReportData)
}

// TestGenerateSSAReport_SingleRiskType 测试单一风险类型
func TestGenerateSSAReport_SingleRiskType(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
	}

	server, err := NewTestServer()
	if err != nil {
		t.Fatalf("创建测试服务器失败: %v", err)
	}

	taskId := uuid.New().String()
	programName := "test-single-risk-project"

	defer cleanupMockData(taskId, programName)

	// 创建程序
	createMockProgram(programName, "java", "测试单一风险类型项目", 3, 500)

	// 创建任务（只有SQL注入风险）
	riskCounts := map[string]int64{
		"total": 3, "critical": 0, "high": 3, "middle": 0, "low": 0, "info": 0,
	}
	createMockTask(taskId, programName, schema.SYNTAXFLOWSCAN_DONE, riskCounts, 5)

	// 创建3个相同类型的风险
	for i := 0; i < 3; i++ {
		createMockRisk(
			taskId, programName,
			"SQL注入", "high", "sql-injection-rule",
			"SQL Injection", "SQL注入漏洞",
			"应用程序直接将用户输入拼接到SQL查询中，可能导致SQL注入攻击",
			"使用参数化查询或预编译语句来防止SQL注入",
			fmt.Sprintf("/src/controller/UserController%d.java", i+1),
			fmt.Sprintf("getUserData%d", i+1),
			fmt.Sprintf(`public User getUserData%d(String id) {
    String sql = "SELECT * FROM users WHERE id = '" + id + "'";  // 危险的SQL拼接
    return database.query(sql);
}`, i+1),
			int64(15+i*10),
		)
	}

	// 测试报告生成
	req := &ypb.GenerateSSAReportRequest{
		TaskID:     taskId,
		ReportName: "单一风险类型测试报告",
	}

	ctx := context.Background()
	resp, err := server.GenerateSSAReport(ctx, req)
	if err != nil {
		t.Fatalf("生成SSA报告失败: %v", err)
	}

	if !resp.Success {
		t.Fatalf("报告生成失败: %s", resp.Message)
	}

	t.Logf("单一风险类型报告生成成功! 报告ID: %s", resp.ReportData)
}

// TestGenerateSSAReport_MultipleRiskTypes 测试多种风险类型和等级
func TestGenerateSSAReport_MultipleRiskTypes(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
	}

	server, err := NewTestServer()
	if err != nil {
		t.Fatalf("创建测试服务器失败: %v", err)
	}

	taskId := uuid.New().String()
	programName := "test-multiple-risks-project"

	defer cleanupMockData(taskId, programName)

	// 创建程序
	createMockProgram(programName, "java", "测试多种风险类型项目", 10, 5000)

	// 创建任务（包含各种风险等级）
	riskCounts := map[string]int64{
		"total": 15, "critical": 2, "high": 4, "middle": 5, "low": 3, "info": 1,
	}
	createMockTask(taskId, programName, schema.SYNTAXFLOWSCAN_DONE, riskCounts, 20)

	// 创建各种类型和等级的风险
	risks := []struct {
		riskType, severity, rule, title, titleVerbose, description, solution, filePath, functionName string
		line                                                                                         int64
	}{
		// 严重风险
		{"代码注入", "critical", "code-injection-rule", "Code Injection", "代码注入漏洞", "应用程序执行用户可控的代码", "严格验证和过滤用户输入，避免执行动态代码", "/src/eval/CodeExecutor.java", "executeCode", 25},
		{"命令注入", "critical", "command-injection-rule", "Command Injection", "命令注入漏洞", "应用程序执行用户可控的系统命令", "使用白名单验证，避免直接执行系统命令", "/src/system/CommandRunner.java", "runCommand", 42},

		// 高危风险
		{"SQL注入", "high", "sql-injection-rule", "SQL Injection", "SQL注入漏洞", "SQL查询中存在用户输入拼接", "使用参数化查询", "/src/dao/UserDao.java", "findUser", 18},
		{"XSS", "high", "xss-rule", "Cross-site Scripting", "跨站脚本攻击", "输出用户数据未进行HTML编码", "对用户输入进行HTML编码", "/src/web/UserController.java", "showUserInfo", 33},
		{"路径遍历", "high", "path-traversal-rule", "Path Traversal", "路径遍历漏洞", "文件路径可被用户控制", "验证和限制文件访问路径", "/src/file/FileHandler.java", "readFile", 67},
		{"LDAP注入", "high", "ldap-injection-rule", "LDAP Injection", "LDAP注入漏洞", "LDAP查询中存在用户输入拼接", "对LDAP查询参数进行转义", "/src/auth/LdapAuth.java", "authenticate", 89},

		// 中危风险
		{"弱密码", "middle", "weak-password-rule", "Weak Password", "弱密码策略", "密码复杂度要求不足", "实施强密码策略", "/src/auth/PasswordValidator.java", "validatePassword", 15},
		{"会话固定", "middle", "session-fixation-rule", "Session Fixation", "会话固定漏洞", "登录后未重新生成会话ID", "登录成功后重新生成会话ID", "/src/web/SessionManager.java", "login", 45},
		{"信息泄露", "middle", "info-disclosure-rule", "Information Disclosure", "信息泄露", "错误信息包含敏感信息", "自定义错误页面，避免泄露系统信息", "/src/exception/ErrorHandler.java", "handleError", 23},
		{"不安全的随机数", "middle", "weak-random-rule", "Weak Random", "弱随机数生成", "使用了不安全的随机数生成器", "使用加密安全的随机数生成器", "/src/util/RandomUtil.java", "generateToken", 12},
		{"硬编码密钥", "middle", "hardcoded-key-rule", "Hardcoded Key", "硬编码密钥", "代码中存在硬编码的密钥或密码", "将密钥存储在安全的配置文件中", "/src/crypto/AESUtil.java", "encrypt", 34},

		// 低危风险
		{"HTTP头缺失", "low", "missing-header-rule", "Missing Security Header", "安全头缺失", "响应中缺少安全相关的HTTP头", "添加安全HTTP响应头", "/src/web/SecurityFilter.java", "doFilter", 28},
		{"调试信息", "low", "debug-info-rule", "Debug Information", "调试信息泄露", "生产环境中存在调试信息", "在生产环境中禁用调试模式", "/src/config/AppConfig.java", "init", 19},
		{"版本信息泄露", "low", "version-disclosure-rule", "Version Disclosure", "版本信息泄露", "HTTP响应中包含服务器版本信息", "隐藏或修改服务器版本信息", "/src/web/ApiController.java", "getVersion", 41},

		// 信息级风险
		{"代码注释", "info", "code-comment-rule", "Sensitive Comment", "敏感注释信息", "代码注释中包含敏感信息", "清理代码注释中的敏感信息", "/src/service/PaymentService.java", "processPayment", 56},
	}

	for _, risk := range risks {
		codeFragment := fmt.Sprintf(`// %s 示例代码
public class Example {
    public void %s() {
        // 这里是有问题的代码实现
        String userInput = request.getParameter("input");
        // 直接使用用户输入，存在安全风险
        processData(userInput);
    }
}`, risk.titleVerbose, risk.functionName)

		createMockRisk(
			taskId, programName,
			risk.riskType, risk.severity, risk.rule,
			risk.title, risk.titleVerbose, risk.description, risk.solution,
			risk.filePath, risk.functionName, codeFragment, risk.line,
		)
	}

	// 测试报告生成
	req := &ypb.GenerateSSAReportRequest{
		TaskID:     taskId,
		ReportName: "多种风险类型综合测试报告",
	}

	ctx := context.Background()
	resp, err := server.GenerateSSAReport(ctx, req)
	if err != nil {
		t.Fatalf("生成SSA报告失败: %v", err)
	}

	if !resp.Success {
		t.Fatalf("报告生成失败: %s", resp.Message)
	}

	t.Logf("多种风险类型报告生成成功! 报告ID: %s", resp.ReportData)
}

// TestGenerateSSAReport_ExtremeDataRatio 测试极端数据比例（用于测试玫瑰图优化）
func TestGenerateSSAReport_ExtremeDataRatio(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
	}

	server, err := NewTestServer()
	if err != nil {
		t.Fatalf("创建测试服务器失败: %v", err)
	}

	taskId := uuid.New().String()
	programName := "test-extreme-ratio-project"

	defer cleanupMockData(taskId, programName)

	// 创建程序
	createMockProgram(programName, "java", "测试极端数据比例项目（1个严重 + 1000个信息）", 50, 20000)

	// 创建极端比例的任务（1个严重风险，1000个信息级风险）
	riskCounts := map[string]int64{
		"total": 1001, "critical": 1, "high": 0, "middle": 0, "low": 0, "info": 1000,
	}
	createMockTask(taskId, programName, schema.SYNTAXFLOWSCAN_DONE, riskCounts, 50)

	// 创建1个严重风险
	createMockRisk(
		taskId, programName,
		"远程代码执行", "critical", "rce-rule",
		"Remote Code Execution", "远程代码执行漏洞",
		"应用程序存在远程代码执行漏洞，攻击者可以执行任意代码",
		"立即修复代码执行漏洞，加强输入验证",
		"/src/core/Executor.java", "execute",
		`public void execute(String command) {
    // 极度危险：直接执行用户输入的命令
    Runtime.getRuntime().exec(command);
}`, 25)

	// 创建1000个信息级风险（模拟大量的代码规范问题）
	for i := 0; i < 1000; i++ {
		createMockRisk(
			taskId, programName,
			"代码规范", "info", "code-style-rule",
			"Code Style", "代码风格问题",
			"代码不符合团队编码规范",
			"按照团队编码规范调整代码格式",
			fmt.Sprintf("/src/util/Util%d.java", i%10+1), // 分布在10个文件中
			fmt.Sprintf("method%d", i),
			fmt.Sprintf(`// 代码风格问题示例 %d
public void method%d(){
    //缺少空格和注释规范
    int x=1+2;
}`, i, i),
			int64(10+i%50), // 行号分布
		)
	}

	// 测试报告生成
	req := &ypb.GenerateSSAReportRequest{
		TaskID:     taskId,
		ReportName: "极端数据比例测试报告",
	}

	ctx := context.Background()
	resp, err := server.GenerateSSAReport(ctx, req)
	if err != nil {
		t.Fatalf("生成SSA报告失败: %v", err)
	}

	if !resp.Success {
		t.Fatalf("报告生成失败: %s", resp.Message)
	}

	t.Logf("极端数据比例报告生成成功! 报告ID: %s", resp.ReportData)
}

// TestGenerateSSAReport_TaskNotFound 测试任务不存在的情况
func TestGenerateSSAReport_TaskNotFound(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
	}

	server, err := NewTestServer()
	if err != nil {
		t.Fatalf("创建测试服务器失败: %v", err)
	}

	// 使用不存在的任务ID
	req := &ypb.GenerateSSAReportRequest{
		TaskID:     "non-existent-task-id",
		ReportName: "不存在任务测试报告",
	}

	ctx := context.Background()
	_, err = server.GenerateSSAReport(ctx, req)

	// 应该返回错误
	if err == nil {
		t.Fatal("期望返回错误，但没有返回错误")
	}

	t.Logf("正确返回错误: %v", err)
}

// TestGenerateSSAReport_ProgramNotFound 测试程序不存在的情况
func TestGenerateSSAReport_ProgramNotFound(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
	}

	server, err := NewTestServer()
	if err != nil {
		t.Fatalf("创建测试服务器失败: %v", err)
	}

	taskId := uuid.New().String()
	programName := "non-existent-program"

	defer cleanupMockData(taskId, programName)

	// 只创建任务，不创建程序
	riskCounts := map[string]int64{
		"total": 1, "critical": 0, "high": 1, "middle": 0, "low": 0, "info": 0,
	}
	createMockTask(taskId, programName, schema.SYNTAXFLOWSCAN_DONE, riskCounts, 1)

	// 创建一个风险
	createMockRisk(
		taskId, programName,
		"测试风险", "high", "test-rule",
		"Test Risk", "测试风险",
		"这是一个测试风险", "修复测试风险",
		"/src/Test.java", "testMethod",
		"public void testMethod() { }", 10)

	// 测试报告生成（程序不存在，但应该能生成报告，只是项目信息为默认值）
	req := &ypb.GenerateSSAReportRequest{
		TaskID:     taskId,
		ReportName: "程序不存在测试报告",
	}

	ctx := context.Background()
	resp, err := server.GenerateSSAReport(ctx, req)
	if err != nil {
		t.Fatalf("生成SSA报告失败: %v", err)
	}

	if !resp.Success {
		t.Fatalf("报告生成失败: %s", resp.Message)
	}

	t.Logf("程序不存在情况下报告生成成功! 报告ID: %s", resp.ReportData)
}

// TestGenerateSSAReport 冒烟测试
func TestGenerateSSAReport(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
	}
	// 创建测试服务器
	server, err := NewTestServer()
	if err != nil {
		t.Fatalf("创建测试服务器失败: %v", err)
	}

	// 准备测试请求
	req := &ypb.GenerateSSAReportRequest{
		//TaskID:     "f6c5443f-0598-421c-8454-ba6ca621dd30", // 使用用户提供的taskID
		TaskID:     "a8c551d5-01a0-4cae-8c7f-3b799ad2cb7b", // 使用用户提供的taskID
		ReportName: "SSA扫描报告测试",
	}

	// 调用GenerateSSAReport方法
	ctx := context.Background()
	resp, err := server.GenerateSSAReport(ctx, req)
	if err != nil {
		t.Fatalf("生成SSA报告失败: %v", err)
	}

	// 验证响应
	if resp == nil {
		t.Fatal("响应为空")
	}

	if !resp.Success {
		t.Fatalf("报告生成失败: %s", resp.Message)
	}

	if resp.ReportData == "" {
		t.Fatal("报告数据为空")
	}

	// 输出结果供人工检查
	t.Logf("报告生成成功!")
	t.Logf("报告ID: %s", resp.ReportData)
	t.Logf("消息: %s", resp.Message)
	t.Logf("请手动检查数据库中ID为 %s 的报告内容", resp.ReportData)
}
