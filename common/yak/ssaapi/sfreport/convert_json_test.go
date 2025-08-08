package sfreport_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestReport(t *testing.T) {

	vf := filesys.NewVirtualFs()
	vf.AddFile("a.java", `
package org.joychou.controller;

public class SQLI {
    @RequestMapping("/jdbc/vuln")
    public String jdbc_sqli_vul(@RequestParam("username") String username) {

        StringBuilder result = new StringBuilder();

        try {
            Class.forName(driver);
            Connection con = DriverManager.getConnection(url, user, password);

            if (!con.isClosed())
                System.out.println("Connect to database successfully.");

            // sqli vuln code
            Statement statement = con.createStatement();
            String sql = "select * from users where username = '" + username + "'";
            logger.info(sql);
            ResultSet rs = statement.executeQuery(sql);

            while (rs.next()) {
                String res_name = rs.getString("username");
                String res_pwd = rs.getString("password");
                String info = String.format("%s: %s\n", res_name, res_pwd);
                result.append(info);
                logger.info(info);
            }
            rs.close();
            con.close();


        } catch (ClassNotFoundException e) {
            logger.error("Sorry, can't find the Driver!");
        } catch (SQLException e) {
            logger.error(e.toString());
        }
        return result.toString();
    }

	@RequestMapping("/jdbc/vuln")
    public String jdbc_sqli_vul(@RequestParam("username") String username) {

        StringBuilder result = new StringBuilder();

        try {
            Class.forName(driver);
            Connection con = DriverManager.getConnection(url, user, password);

            if (!con.isClosed())
                System.out.println("Connect to database successfully.");

            // sqli vuln code
            Statement statement = con.createStatement();
            String sql = "select * from users where username = '" + username + "'";
            logger.info(sql);
            ResultSet rs = statement.executeQuery(sql);

            while (rs.next()) {
                String res_name = rs.getString("username");
                String res_pwd = rs.getString("password");
                String info = String.format("%s: %s\n", res_name, res_pwd);
                result.append(info);
                logger.info(info);
            }
            rs.close();
            con.close();


        } catch (ClassNotFoundException e) {
            logger.error("Sorry, can't find the Driver!");
        } catch (SQLException e) {
            logger.error(e.toString());
        }
        return result.toString();
    }


}
	`)

	progName := uuid.NewString()
	prog, err := ssaapi.ParseProject(ssaapi.WithFileSystem(vf), ssaapi.WithLanguage(consts.JAVA), ssaapi.WithProgramName(progName))
	require.NoError(t, err)

	rule := `
g"SELECT*" as $sqlConst;
g"select*" as $sqlConst;

// 检测 SQL 字符串被传入到了某一个执行函数中，执行函数符合常见的 SQL 执行命名规范
$sqlConst -{
	until: <<<CODE
*?{opcode: call && <getCallee><name>?{have: /(?i)(query)|(execut)|(insert)|(native)|(update)/}<show>}<var(sink)> as $__next__;
CODE
}->;
check $sink;

// 检测 SQL 字符串是否被 add 操作拼接，add 操作是字符串拼接的常见操作
// 这里虽然会不全面，但是可以作为一个案例，可以支持更多规则来实现这个细节检测
$sqlConst?{<self>#>?{opcode: add}<var(op)> || <self>->?{opcode: add}<var(op)>};
check $op;

alert $op for {
	title_zh: "SQL 字符串拼接位置：疑似 SQL 语句拼接并执行到数据库查询的代码",
	type: audit,
	severity: medium,
	desc: "疑似 SQL 语句拼接并执行到数据库查询的代码",
};
`
	res, err := prog.SyntaxFlowWithError(rule)
	require.NoError(t, err)

	id, err := res.Save(schema.SFResultKindDebug)
	require.NoError(t, err)
	_ = id

	/*
		{
		  "report_type": "irify",
		  "engine_version": "dev",
		  "report_time": "2025-04-17T15:41:44.631769+08:00",
		  "program_name": "85c90742-d190-4434-a74e-db8dd561b6dd",
		  "Rules": [
		    {
		      "rule_name": "",
		      "language": "",
		      "description": "",
		      "solution": "",
		      "content": "\ng\"SELEC....",
		      "risks": [
		        "07dece1c8c7a56c1b7ca0ebeb83ed2a6ba952da3",
		        "f0481807c1b3d08d9e56fa15b76170bfbcb10898"
		      ]
		    }
		  ],
		  "Risks": {
		    "07dece1c8c7a56c1b7ca0ebeb83ed2a6ba952da3": {
		      "id": 1710,
		      "hash": "07dece1c8c7a56c1b7ca0ebeb83ed2a6ba952da3",
		      "title": "",
		      "title_verbose": "SQL 字符串拼接位置：疑似 SQL 语句拼接并执行到数据库查询的代码",
		      "description": "",
		      "solution": "",
		      "severity": "middle",
		      "risk_type": "其他",
		      "details": "",
		      "cve": "",
		      "time": "2025-04-17T15:41:44.628129+08:00",
		      "code_source_url": "a.java",
		      "line": 19,
		      "code_range": "{\"url\":\"/85c90742-d190-4434-a74e-db8dd561b6dd/a.java\",\"start_line\":19,\"start_column\":26,\"end_line\":19,\"end_column\":77,\"source_code_line\":15}",
		      "rule_name": "",
		      "program_name": "85c90742-d190-4434-a74e-db8dd561b6dd"
		    },
		    "f0481807c1b3d08d9e56fa15b76170bfbcb10898": {
		      "id": 1711,
		      "hash": "f0481807c1b3d08d9e56fa15b76170bfbcb10898",
		      "title": "",
		      "title_verbose": "SQL 字符串拼接位置：疑似 SQL 语句拼接并执行到数据库查询的代码",
		      "description": "",
		      "solution": "",
		      "severity": "middle",
		      "risk_type": "其他",
		      "details": "",
		      "cve": "",
		      "time": "2025-04-17T15:41:44.62919+08:00",
		      "code_source_url": "a.java",
		      "line": 56,
		      "code_range": "{\"url\":\"/85c90742-d190-4434-a74e-db8dd561b6dd/a.java\",\"start_line\":56,\"start_column\":26,\"end_line\":56,\"end_column\":77,\"source_code_line\":52}",
		      "rule_name": "",
		      "program_name": "85c90742-d190-4434-a74e-db8dd561b6dd"
		    }
		  },
		  "File": [
		    {
		      "path": "a.java",
		      "length": 2561,
		      "hash": {
		        "md5": "f239e50e36e4b402df4119d4b6aabe86",
		        "sha1": "f9010aa419c7c094722218ca1cee2332169df3bb",
		        "sha256": "2882ba1aff1e56775a199973613b556d6d3b2ea22c06cb18550e75f6c8577b3b"
		      },
		      "content": "\npackage org.joychou.controller;\n\npublic class SQLI {\n    @RequestMapping(\"/jdbc/vuln\")\n    public S...",
		      "risks": [
		        "07dece1c8c7a56c1b7ca0ebeb83ed2a6ba952da3",
		        "f0481807c1b3d08d9e56fa15b76170bfbcb10898"
		      ]
		    }
		  ]
		}
	*/
	report := sfreport.NewReport(sfreport.IRifyReportType)
	report.AddSyntaxFlowResult(res)

	err = report.PrettyWrite(os.Stdout)
	require.NoError(t, err)

	// check report
	require.Len(t, report.Risks, 2)
	// check report.risk
	for hash, risk := range report.Risks {
		riskDB, err := yakit.GetSSARiskByHash(ssadb.GetDB(), hash)
		require.NoError(t, err)
		require.Equal(t, risk.GetProgramName(), riskDB.ProgramName)
		require.Equal(t, risk.GetHash(), riskDB.Hash)
		require.Equal(t, risk.GetTitle(), riskDB.Title)
		require.Equal(t, risk.GetTitleVerbose(), riskDB.TitleVerbose)
		require.Equal(t, risk.GetDescription(), riskDB.Description)
		require.Equal(t, risk.GetSolution(), riskDB.Solution)
		require.Equal(t, risk.GetSeverity(), string(riskDB.Severity))
		require.Equal(t, risk.GetRiskType(), riskDB.RiskType)
	}

	// check report.rule
	require.Equal(t, len(report.Rules), 1)
	require.Equal(t, report.Rules[0].Content, rule)

	// check report.file
	require.Equal(t, len(report.File), 1)
	require.Equal(t, report.File[0].Path, "a.java")

	// check report.program
	require.Equal(t, report.ProgramName, progName)

}

func TestRiskExport_CompleteFlow(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("FileUploader.java", `import java.io.File;
import java.io.IOException;
import java.io.InputStream;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.nio.file.StandardCopyOption;

public class FileUploader {
    // 允许上传的文件扩展名白名单
    private static final String[] ALLOWED_EXTENSIONS = {".jpg", ".jpeg", ".png", ".gif", ".pdf", ".txt"};
    // 上传文件存储的基础目录
    private static final String UPLOAD_BASE_DIR = "/var/www/uploads/";

    /**
     * 安全上传文件方法
     * @param inputStream 文件输入流
     * @param fileName 原始文件名
     * @param subDir 子目录（可选）
     * @return 上传后的文件路径
     * @throws IOException 如果上传过程中发生错误
     * @throws SecurityException 如果检测到不安全操作
     */
    public static String uploadFile(InputStream inputStream, String fileName, String subDir)
            throws IOException, SecurityException {

        // 1. 检查文件名是否合法
        if (fileName == null || fileName.isEmpty()) {
            throw new SecurityException("文件名不能为空");
        }

        // 2. 防止路径穿越攻击
        if (fileName.contains("../") || fileName.contains("..\\")) {
            throw new SecurityException("文件名包含非法路径字符");
        }

        // 如果指定了子目录，同样检查子目录是否合法
        if (subDir != null && !subDir.isEmpty()) {
            if (subDir.contains("../") || subDir.contains("..\\")) {
                throw new SecurityException("子目录包含非法路径字符");
            }
        }

        // 3. 检查文件扩展名是否合法
        String fileExtension = getFileExtension(fileName).toLowerCase();
        boolean allowed = false;
        for (String ext : ALLOWED_EXTENSIONS) {
            if (ext.equalsIgnoreCase(fileExtension)) {
                allowed = true;
                break;
            }
        }
        if (!allowed) {
            throw new SecurityException("不允许的文件类型: " + fileExtension);
        }

        // 4. 创建目标目录
        Path uploadDir = Paths.get(UPLOAD_BASE_DIR, subDir != nil ? subDir : "");
        if (!Files.exists(uploadDir)) {
            Files.createDirectories(uploadDir);
        }

        // 5. 生成安全的文件名（避免覆盖现有文件）
        String safeFileName = System.currentTimeMillis() + "_" + fileName;
        Path destination = uploadDir.resolve(safeFileName);

        // 6. 保存文件
        Files.copy(inputStream, destination, StandardCopyOption.REPLACE_EXISTING);

        // 7. 返回相对路径（不暴露服务器绝对路径）
        return Paths.get(subDir != nil ? subDir : "", safeFileName).toString();
    }

    /**
     * 获取文件扩展名
     * @param fileName 文件名
     * @return 文件扩展名（包含点）
     */
    private static String getFileExtension(String fileName) {
        int dotIndex = fileName.lastIndexOf('.');
        if (dotIndex > 0 && dotIndex < fileName.length() - 1) {
            return fileName.substring(dotIndex);
        }
        return "";
    }

    // 示例用法
    public static void main(String[] args) {
        try {
            // 模拟文件上传
            InputStream fileStream = FileUploader.class.getResourceAsStream("/test.txt");
            String uploadedPath = uploadFile(fileStream, "test.txt", "user_docs");
            System.out.println("文件上传成功，路径: " + uploadedPath);
        } catch (Exception e) {
            e.printStackTrace();
        }
    }
}`)

	ssatest.CheckProfileWithFS(vf, t, func(p ssatest.ParseStage, prog ssaapi.Programs, start time.Time) error {
		if p != ssatest.OnlyDatabase {
			return nil
		}
		result, err := prog.SyntaxFlowWithError("desc(\n\ttitle: \"Check Java Path Traversal Vulnerability\"\n\ttitle_zh: \"检测Java路径穿越漏洞\"\n\ttype: vuln\n\trisk: \"path-traversal\"\n\tdesc: <<<DESC\n### 漏洞描述\n\n1. **漏洞原理**\n   路径 Traversal（也称为目录遍历）漏洞允许攻击者通过操纵文件路径参数，访问或执行服务器上受限目录之外的任意文件。在 Java 应用程序中，当应用程序直接使用用户提供的文件名或路径片段构建文件操作路径，且未对用户输入进行充分验证或清理时（例如去除 `../` 或其他目录遍历符），攻击者即可构造包含 `../` 等特殊字符的输入，向上遍历目录结构，访问位于应用程序根目录之外的文件，如配置文件、源代码、敏感数据文件甚至系统文件（如 `/etc/passwd`）。\n\n2. **触发场景**\n   以下代码示例未对用户输入的 `fileName` 进行充分验证，直接将其拼接在基本路径后创建文件对象并进行读取，存在路径穿越风险：\n   ```java\n   import java.io.File;\n   import java.io.FileReader;\n   import java.io.IOException;\n   import java.io.OutputStream;\n   import javax.servlet.ServletException;\n   import javax.servlet.http.HttpServlet;\n   import javax.servlet.http.HttpServletRequest;\n   import javax.servlet.http.HttpServletResponse;\n\n   public class InsecureFileReaderServlet extends HttpServlet {\n       @Override\n       protected void doGet(HttpServletRequest request, HttpServletResponse response) throws ServletException, IOException {\n           String fileName = request.getParameter(\"file\");\n           String filePath = \"path/to/safe/directory/\" + fileName; // 未对fileName进行检查或清理\n\n           File file = new File(filePath);\n           // ... 后续文件读取操作\n       }\n   }\n   ```\n   攻击者可以通过构造 `fileName` 为 `../../../../etc/passwd` 来尝试读取系统密码文件。\n\n3. **潜在影响**\n   - **信息泄露**: 攻击者可以读取任意敏感文件，包括配置文件、源代码、用户上传文件、私钥等。\n   - **文件篡改或删除**: 如果应用程序允许写入或删除文件，攻击者可能利用此漏洞修改或删除服务器上的关键文件，导致拒绝服务或进一步入侵。\n   - **远程代码执行（RCE）**: 在某些情况下，如果攻击者能够上传或修改可执行文件并诱导服务器执行，可能导致远程代码执行。\n   - **进一步攻击**: 获取的敏感信息可能被用于进行更复杂的攻击，如提权、内网渗透等。\nDESC\n\trule_id: \"7b798768-13e1-4dcd-8ab5-99a6f9635605\"\n\tsolution: <<<SOLUTION\n### 修复建议\n\n#### 1. 验证和清理用户输入\n在将用户输入用于构建文件路径之前，必须进行严格的验证和清理，移除目录穿越字符（如 `../`）。可以使用正则表达式或特定的安全库函数。\n\n```java\n// 修复代码示例 (简单清理示例，更健壮的清理需要考虑多种编码和操作系统差异)\nString fileName = request.getParameter(\"file\");\nif (fileName != null) {\n    // 移除 '../' 和 '..\\\\' 等目录穿越字符\n    fileName = fileName.replace(\"../\", \"\").replace(\"..\\\\\", \"\");\n    // 还可以进一步限制文件名只能包含字母、数字和特定安全字符\n    if (!fileName.matches(\"^[a-zA-Z0-9_\\\\-\\\\|\\\\.\\\\u4e00-\\\\u9fa5]+$\")) {\n         response.sendError(HttpServletResponse.SC_FORBIDDEN, \"Invalid file name.\");\n         return;\n    }\n}\nString filePath = \"path/to/safe/directory/\" + fileName;\n```\n\n#### 2. 使用标准库方法验证规范路径\n在文件操作前，获取文件的规范路径（Canonical Path），并检查该规范路径是否位于预期的安全目录下。这是更推荐和健壮的方法。\n\n```java\n// 修复代码示例 (使用 Canonical Path 验证)\nprivate static final String BASE_DIR = \"/usr/local/apache-tomcat/webapps/ROOT/safe_directory/\";\n\nprotected void doGet(HttpServletRequest request, HttpServletResponse response) throws ServletException, IOException {\n    String requestedFile = request.getParameter(\"file\");\n\n    // 构建潜在的完整路径\n    File file = new File(BASE_DIR, requestedFile);\n\n    // 获取文件的规范路径，此方法会解析并消除目录穿透符\n    String canonicalRequestedPath = file.getCanonicalPath();\n    String canonicalBaseDirPath = new File(BASE_DIR).getCanonicalPath();\n\n    // 检查文件的规范路径是否以安全目录的规范路径开头\n    if (!canonicalRequestedPath.startsWith(canonicalBaseDirPath)) {\n        response.sendError(HttpServletResponse.SC_FORBIDDEN, \"Access denied\");\n        return;\n    }\n\n    // ... 后续的文件读取操作，现在可以安全地使用 file 对象\n    if (!file.exists()) {\n        response.sendError(HttpServletResponse.SC_NOT_FOUND, \"File not found\");\n        return;\n    }\n    // ... 安全的文件操作\n}\n```\n\n#### 3. 限制文件访问范围\n配置应用程序或 Web 服务器，限制其只能访问特定的目录，或者使用沙箱机制隔离文件操作。\n\n#### 4. 使用白名单验证\n如果可能，不要接受用户输入的完整文件名或路径，而是让用户选择预定义的安全文件列表中的文件（白名单方式）。\nSOLUTION\n\treference: <<<REFERENCE\n[CWE-22: Improper Limitation of a Pathname to a Restricted Directory ('Path Traversal')](https://cwe.mitre.org/data/definitions/22.html)\nREFERENCE\n)\n\nfileName as $source\nFiles.copy() as $sink\n$sink #{\n    until:`* & $source`\n}-> as $result\n\nalert $result for {\n\tdesc: <<<CODE\n### 漏洞描述\n\n1. **漏洞原理**\n   路径 Traversal（也称为目录遍历）漏洞允许攻击者通过操纵文件路径参数，访问或执行服务器上受限目录之外的任意文件。在 Java 应用程序中，当应用程序直接使用用户提供的文件名或路径片段构建文件操作路径，且未对用户输入进行充分验证或清理时（例如去除 `../` 或其他目录遍历符），攻击者即可构造包含 `../` 等特殊字符的输入，向上遍历目录结构，访问位于应用程序根目录之外的文件，如配置文件、源代码、敏感数据文件甚至系统文件（如 `/etc/passwd`）。\n\n2. **触发场景**\n   以下代码示例未对用户输入的 `fileName` 进行充分验证，直接将其拼接在基本路径后创建文件对象并进行读取，存在路径穿越风险：\n   ```java\n   import java.io.File;\n   import java.io.FileReader;\n   import java.io.IOException;\n   import java.io.OutputStream;\n   import javax.servlet.ServletException;\n   import javax.servlet.http.HttpServlet;\n   import javax.servlet.http.HttpServletRequest;\n   import javax.servlet.http.HttpServletResponse;\n\n   public class InsecureFileReaderServlet extends HttpServlet {\n       @Override\n       protected void doGet(HttpServletRequest request, HttpServletResponse response) throws ServletException, IOException {\n           String fileName = request.getParameter(\"file\");\n           String filePath = \"path/to/safe/directory/\" + fileName; // 未对fileName进行检查或清理\n\n           File file = new File(filePath);\n           // ... 后续文件读取操作\n       }\n   }\n   ```\n   攻击者可以通过构造 `fileName` 为 `../../../../etc/passwd` 来尝试读取系统密码文件。\n\n3. **潜在影响**\n   - **信息泄露**: 攻击者可以读取任意敏感文件，包括配置文件、源代码、用户上传文件、私钥等。\n   - **文件篡改或删除**: 如果应用程序允许写入或删除文件，攻击者可能利用此漏洞修改或删除服务器上的关键文件，导致拒绝服务或进一步入侵。\n   - **远程代码执行（RCE）**: 在某些情况下，如果攻击者能够上传或修改可执行文件并诱导服务器执行，可能导致远程代码执行。\n   - **进一步攻击**: 获取的敏感信息可能被用于进行更复杂的攻击，如提权、内网渗透等。\nCODE\n\tlevel: \"high\",\n\ttype: \"vuln\",\n\tmessage: \"Java代码中发现路径穿越漏洞，并且数据流中间没有进行任何过滤。\",\n\ttitle: \"Check Java Path Traversal Vulnerability\",\n\ttitle_zh: \"检测Java路径穿越漏洞\",\n\tsolution: <<<CODE\n### 修复建议\n\n#### 1. 验证和清理用户输入\n在将用户输入用于构建文件路径之前，必须进行严格的验证和清理，移除目录穿越字符（如 `../`）。可以使用正则表达式或特定的安全库函数。\n\n```java\n// 修复代码示例 (简单清理示例，更健壮的清理需要考虑多种编码和操作系统差异)\nString fileName = request.getParameter(\"file\");\nif (fileName != null) {\n    // 移除 '../' 和 '..\\\\' 等目录穿越字符\n    fileName = fileName.replace(\"../\", \"\").replace(\"..\\\\\", \"\");\n    // 还可以进一步限制文件名只能包含字母、数字和特定安全字符\n    if (!fileName.matches(\"^[a-zA-Z0-9_\\\\-\\\\|\\\\.\\\\u4e00-\\\\u9fa5]+$\")) {\n         response.sendError(HttpServletResponse.SC_FORBIDDEN, \"Invalid file name.\");\n         return;\n    }\n}\nString filePath = \"path/to/safe/directory/\" + fileName;\n```\n\n#### 2. 使用标准库方法验证规范路径\n在文件操作前，获取文件的规范路径（Canonical Path），并检查该规范路径是否位于预期的安全目录下。这是更推荐和健壮的方法。\n\n```java\n// 修复代码示例 (使用 Canonical Path 验证)\nprivate static final String BASE_DIR = \"/usr/local/apache-tomcat/webapps/ROOT/safe_directory/\";\n\nprotected void doGet(HttpServletRequest request, HttpServletResponse response) throws ServletException, IOException {\n    String requestedFile = request.getParameter(\"file\");\n\n    // 构建潜在的完整路径\n    File file = new File(BASE_DIR, requestedFile);\n\n    // 获取文件的规范路径，此方法会解析并消除目录穿透符\n    String canonicalRequestedPath = file.getCanonicalPath();\n    String canonicalBaseDirPath = new File(BASE_DIR).getCanonicalPath();\n\n    // 检查文件的规范路径是否以安全目录的规范路径开头\n    if (!canonicalRequestedPath.startsWith(canonicalBaseDirPath)) {\n        response.sendError(HttpServletResponse.SC_FORBIDDEN, \"Access denied\");\n        return;\n    }\n\n    // ... 后续的文件读取操作，现在可以安全地使用 file 对象\n    if (!file.exists()) {\n        response.sendError(HttpServletResponse.SC_NOT_FOUND, \"File not found\");\n        return;\n    }\n    // ... 安全的文件操作\n}\n```\n\n#### 3. 限制文件访问范围\n配置应用程序或 Web 服务器，限制其只能访问特定的目录，或者使用沙箱机制隔离文件操作。\n\n#### 4. 使用白名单验证\n如果可能，不要接受用户输入的完整文件名或路径，而是让用户选择预定义的安全文件列表中的文件（白名单方式）。\nCODE\n}\n")
		require.NoError(t, err)

		resultId, err := result.Save(schema.SFResultKindDebug)
		require.NoError(t, err)
		require.NotZero(t, resultId)

		_, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
			ProgramName: []string{result.GetProgramName()},
		}, nil)
		require.NoError(t, err)
		require.NotEmpty(t, risks, "应该生成风险数据")

		//outputPath := filepath.Join("D:\\GoProject\\yaklang\\common\\yak\\ssaapi\\sfreport", "risk_export_test.json")

		data, err := sfreport.ConvertRisksToJson(risks)
		require.NoError(t, err)
		//file, err := os.OpenFile(outputPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0777)
		//defer file.Close()
		//file.Write(data)
		//_, err = os.Stat(outputPath)
		require.NoError(t, err)

		require.NoError(t, err)
		var exportData sfreport.Report
		err = json.Unmarshal(data, &exportData)
		require.NoError(t, err)

		// 验证基本结构
		require.NotZero(t, exportData.ReportTime)
		require.Equal(t, len(risks), exportData.RiskNums)
		require.Len(t, exportData.Risks, len(risks))

		i := 0
		for _, riskItem := range exportData.Risks {
			require.Equal(t, result.GetProgramName(), riskItem.GetProgramName())
			require.Equal(t, "java", riskItem.GetLanguage())
			require.Equal(t, risks[i].Title, riskItem.GetTitle())
			require.Equal(t, risks[i].RiskType, riskItem.GetRiskType())
			require.Equal(t, string(risks[i].Severity), riskItem.GetSeverity())
			i++
		}
		return nil
	})
}
