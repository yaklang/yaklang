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
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// createMockProgram 创建模拟程序数据
func createMockProgram(programName, language, description string, fileCount, lineCount int) *ssadb.IrProgram {
	// 程序信息存储在SSA数据库
	db := ssadb.GetDB()
	program := &ssadb.IrProgram{
		ProgramName: programName,
		Description: description,
		Language:    ssaconfig.Language(language),
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
	return createMockRiskWithDisposal(taskId, programName, riskType, severity, fromRule, title, titleVerbose, description, solution, filePath, functionName, codeFragment, line, "")
}

// createMockRiskWithDisposal 创建带有处置状态的模拟风险数据
func createMockRiskWithDisposal(taskId, programName, riskType, severity, fromRule, title, titleVerbose, description, solution, filePath, functionName, codeFragment string, line int64, disposalStatus string) *schema.SSARisk {
	// SSA风险数据存储在SSA数据库
	db := consts.GetGormDefaultSSADataBase()
	risk := &schema.SSARisk{
		Title:                title,
		TitleVerbose:         titleVerbose,
		Description:          description,
		Solution:             solution,
		RiskType:             riskType,
		Severity:             schema.SyntaxFlowSeverity(severity),
		FromRule:             fromRule,
		ProgramName:          programName,
		RuntimeId:            taskId,
		CodeSourceUrl:        filePath,
		CodeRange:            fmt.Sprintf("%d-%d", line, line+2),
		CodeFragment:         codeFragment,
		FunctionName:         functionName,
		Line:                 line,
		Language:             "java",
		LatestDisposalStatus: disposalStatus,
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

// TestGenerateSSAReport_FromRiskIDs 测试从Filter生成报告
func TestGenerateSSAReport_FromRiskIDs(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
	}

	server, err := NewTestServer()
	if err != nil {
		t.Fatalf("创建测试服务器失败: %v", err)
	}

	taskId := uuid.New().String()
	programName1 := "test-program-1"
	programName2 := "test-program-2"

	defer cleanupMockData(taskId, programName1)
	defer cleanupMockData(taskId, programName2)

	// 创建两个不同的程序
	createMockProgram(programName1, "java", "测试项目1", 5, 1000)
	createMockProgram(programName2, "php", "测试项目2", 3, 500)

	// 创建任务
	riskCounts := map[string]int64{
		"total": 6, "critical": 1, "high": 2, "middle": 2, "low": 1, "info": 0,
	}
	createMockTask(taskId, programName1, schema.SYNTAXFLOWSCAN_DONE, riskCounts, 10)

	// 在不同项目中创建Risk
	var riskIDs []int64

	// 项目1的Risk
	risk1 := createMockRisk(taskId, programName1, "SQL注入", "critical", "sql-rule",
		"SQL Injection", "SQL注入漏洞", "存在SQL注入风险", "使用参数化查询",
		"/src/User.java", "getUser", "SELECT * FROM users WHERE id = "+"id", 10)
	riskIDs = append(riskIDs, int64(risk1.ID))

	risk2 := createMockRisk(taskId, programName1, "XSS", "high", "xss-rule",
		"XSS", "跨站脚本", "存在XSS风险", "对输出进行编码",
		"/src/Display.java", "showUser", "out.print(userInput)", 20)
	riskIDs = append(riskIDs, int64(risk2.ID))

	// 项目2的Risk
	risk3 := createMockRisk(taskId, programName2, "文件上传", "high", "upload-rule",
		"File Upload", "文件上传漏洞", "未验证文件类型", "验证文件类型和大小",
		"/upload.php", "handleUpload", "move_uploaded_file($_FILES['file'])", 15)
	riskIDs = append(riskIDs, int64(risk3.ID))

	risk4 := createMockRisk(taskId, programName2, "弱密码", "middle", "weak-pwd-rule",
		"Weak Password", "弱密码", "密码强度不足", "增强密码策略",
		"/auth.php", "validatePwd", "strlen($pwd) > 6", 30)
	riskIDs = append(riskIDs, int64(risk4.ID))

	// 测试：使用Filter生成报告
	req := &ypb.GenerateSSAReportRequest{
		Filter: &ypb.SSARisksFilter{
			ID: riskIDs,
		},
		ReportName: "用户选择的风险报告",
	}

	ctx := context.Background()
	resp, err := server.GenerateSSAReport(ctx, req)
	if err != nil {
		t.Fatalf("生成SSA报告失败: %v", err)
	}

	if !resp.Success {
		t.Fatalf("报告生成失败: %s", resp.Message)
	}

	t.Logf("✅ 从Filter生成报告成功!")
	t.Logf("   报告ID: %s", resp.ReportData)
	t.Logf("   包含 %d 个Risk，涉及 2 个项目", len(riskIDs))
}

// TestGenerateSSAReport_FromTaskID 测试从TaskID生成报告（原有功能）
func TestGenerateSSAReport_FromTaskID(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
	}

	server, err := NewTestServer()
	if err != nil {
		t.Fatalf("创建测试服务器失败: %v", err)
	}

	taskId := uuid.New().String()
	programName := "test-task-program"

	defer cleanupMockData(taskId, programName)

	// 创建程序
	createMockProgram(programName, "java", "测试任务项目", 10, 2000)

	// 创建任务
	riskCounts := map[string]int64{
		"total": 3, "critical": 0, "high": 2, "middle": 1, "low": 0, "info": 0,
	}
	createMockTask(taskId, programName, schema.SYNTAXFLOWSCAN_DONE, riskCounts, 5)

	// 创建Risk
	for i := 0; i < 3; i++ {
		severity := "high"
		if i == 2 {
			severity = "middle"
		}
		createMockRisk(taskId, programName,
			fmt.Sprintf("风险类型%d", i+1), severity, fmt.Sprintf("rule-%d", i+1),
			fmt.Sprintf("Risk %d", i+1), fmt.Sprintf("风险%d", i+1),
			fmt.Sprintf("描述%d", i+1), fmt.Sprintf("解决方案%d", i+1),
			fmt.Sprintf("/src/File%d.java", i+1), fmt.Sprintf("method%d", i+1),
			fmt.Sprintf("code snippet %d", i+1), int64(10+i*10))
	}

	// 测试：使用TaskID生成报告（原有功能）
	req := &ypb.GenerateSSAReportRequest{
		TaskID:     taskId,
		ReportName: "任务扫描报告",
	}

	ctx := context.Background()
	resp, err := server.GenerateSSAReport(ctx, req)
	if err != nil {
		t.Fatalf("生成SSA报告失败: %v", err)
	}

	if !resp.Success {
		t.Fatalf("报告生成失败: %s", resp.Message)
	}

	t.Logf("✅ 从TaskID生成报告成功!")
	t.Logf("   报告ID: %s", resp.ReportData)
}

// TestGenerateSSAReport_Validation 测试参数验证
func TestGenerateSSAReport_Validation(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
	}

	server, err := NewTestServer()
	if err != nil {
		t.Fatalf("创建测试服务器失败: %v", err)
	}

	// 测试：既没有TaskID也没有Filter
	req := &ypb.GenerateSSAReportRequest{
		ReportName: "无参数报告",
	}

	ctx := context.Background()
	_, err = server.GenerateSSAReport(ctx, req)

	if err == nil {
		t.Fatal("期望返回错误，但没有返回错误")
	}

	t.Logf("✅ 参数验证正确: %v", err)
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
		TaskID:     "44babc8b-3784-45fc-b75a-3a6812a088e8", // 使用实际存在的taskID
		ReportName: "Ecommerce-Ejherb项目扫描报告",
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

// TestGenerateSSAReport_WithDisposalStatus 测试包含处置状态的报告生成
func TestGenerateSSAReport_WithDisposalStatus(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
	}

	server, err := NewTestServer()
	if err != nil {
		t.Fatalf("创建测试服务器失败: %v", err)
	}

	taskId := uuid.New().String()
	programName := "test-disposal-status-project"

	defer cleanupMockData(taskId, programName)

	// 创建程序
	createMockProgram(programName, "java", "测试处置状态显示项目", 5, 2000)

	// 创建任务
	riskCounts := map[string]int64{
		"total": 4, "critical": 1, "high": 1, "middle": 1, "low": 1, "info": 0,
	}
	createMockTask(taskId, programName, schema.SYNTAXFLOWSCAN_DONE, riskCounts, 5)

	// 创建不同处置状态的风险，并创建对应的处置记录
	risk1 := createMockRisk(
		taskId, programName,
		"SQL注入", "critical", "sql-injection-rule",
		"SQL Injection", "SQL注入漏洞",
		"存在SQL注入风险", "使用参数化查询",
		"/src/dao/UserDao.java", "getUserById",
		`String sql = "SELECT * FROM users WHERE id = '" + userId + "'";`,
		10,
	)
	// 创建处置记录
	db := consts.GetGormDefaultSSADataBase()
	db.Create(&schema.SSARiskDisposals{
		SSARiskID:       int64(risk1.ID),
		RiskFeatureHash: risk1.RiskFeatureHash,
		Status:          "is_issue",
		Comment:         "确认为SQL注入漏洞，需要立即修复",
		TaskId:          taskId,
	})

	risk2 := createMockRisk(
		taskId, programName,
		"XSS", "high", "xss-rule",
		"Cross-site Scripting", "跨站脚本攻击",
		"输出未进行HTML编码", "对用户输入进行HTML编码",
		"/src/web/UserController.java", "showUserInfo",
		`response.getWriter().write("<div>" + username + "</div>");`,
		25,
	)
	db.Create(&schema.SSARiskDisposals{
		SSARiskID:       int64(risk2.ID),
		RiskFeatureHash: risk2.RiskFeatureHash,
		Status:          "suspicious",
		Comment:         "需要进一步确认是否存在XSS风险",
		TaskId:          taskId,
	})

	risk3 := createMockRisk(
		taskId, programName,
		"硬编码密钥", "middle", "hardcoded-key-rule",
		"Hardcoded Key", "硬编码密钥",
		"代码中存在硬编码的密钥", "将密钥存储在安全的配置文件中",
		"/src/config/SecurityConfig.java", "getEncryptionKey",
		`private static final String KEY = "abc123def456";`,
		15,
	)
	db.Create(&schema.SSARiskDisposals{
		SSARiskID:       int64(risk3.ID),
		RiskFeatureHash: risk3.RiskFeatureHash,
		Status:          "not_issue",
		Comment:         "这是测试环境的密钥，不是真实密钥，不构成风险",
		TaskId:          taskId,
	})

	_ = createMockRisk(
		taskId, programName,
		"HTTP头缺失", "low", "missing-header-rule",
		"Missing Security Header", "安全头缺失",
		"响应中缺少安全相关的HTTP头", "添加安全HTTP响应头",
		"/src/web/SecurityFilter.java", "doFilter",
		`response.setHeader("Content-Type", "text/html");`,
		40,
	)
	// 这个风险不创建处置记录，测试未处置状态

	// 测试报告生成
	req := &ypb.GenerateSSAReportRequest{
		TaskID:     taskId,
		ReportName: "处置状态显示测试报告",
	}

	ctx := context.Background()
	resp, err := server.GenerateSSAReport(ctx, req)
	if err != nil {
		t.Fatalf("生成SSA报告失败: %v", err)
	}

	if !resp.Success {
		t.Fatalf("报告生成失败: %s", resp.Message)
	}

	t.Logf("✅ 处置状态显示测试报告生成成功!")
	t.Logf("   报告ID: %s", resp.ReportData)
	t.Logf("   包含4个不同处置状态的风险:")
	t.Logf("   - SQL注入: 存在漏洞 (is_issue) - 确认为SQL注入漏洞，需要立即修复")
	t.Logf("   - XSS: 疑似问题 (suspicious) - 需要进一步确认是否存在XSS风险")
	t.Logf("   - 硬编码密钥: 不是问题 (not_issue) - 这是测试环境的密钥，不是真实密钥，不构成风险")
	t.Logf("   - HTTP头缺失: 未处置 (not_set) - 无处置备注")
	t.Logf("   请手动检查报告中的处置状态和备注是否正确显示")
}
