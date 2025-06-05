package buildin_rule

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestSyntaxFlowVerifyFileSystem(t *testing.T) {
	yakCode := uuid.NewString()
	javaCode := uuid.NewString()
	phpCode := uuid.NewString()

	rule := fmt.Sprintf(`

desc(
	title: "test rule verify file system", 
)

desc (
	language: yaklang, 
	alert_min: 1, 
	'file://a.yak': "%s"
)

desc(
	language: java, 
	'file://a.java': "%s"
)

desc( 
	language: php,
	'safefile://a.php': '%s'
)

	`, yakCode, javaCode, phpCode)

	f, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(rule)
	if err != nil {
		t.Fatal(err)
	}
	_ = f

	verifyFS, err := f.ExtractVerifyFilesystemAndLanguage()
	require.NoError(t, err)
	require.Len(t, verifyFS, 2)

	yakVerify := verifyFS[0]
	require.Equal(t, (consts.Yak), yakVerify.GetLanguage())
	data, err := yakVerify.GetVirtualFs().ReadFile("a.yak")
	require.NoError(t, err)
	require.Equal(t, yakCode, string(data))
	require.Equal(t, 1, yakVerify.GetExtraInfoInt("alert_min"))
	_ = yakVerify

	javaVerify := verifyFS[1]
	require.Equal(t, (consts.JAVA), javaVerify.GetLanguage())
	data, err = javaVerify.GetVirtualFs().ReadFile("a.java")
	require.NoError(t, err)
	require.Equal(t, javaCode, string(data))
	_ = javaVerify

	verifyFS, err = f.ExtractNegativeFilesystemAndLanguage()
	require.NoError(t, err)
	require.Len(t, verifyFS, 1)

	phpNegative := verifyFS[0]
	require.Equal(t, (consts.PHP), phpNegative.GetLanguage())
	data, err = phpNegative.GetVirtualFs().ReadFile("a.php")
	require.NoError(t, err)
	require.Equal(t, phpCode, string(data))
	_ = phpNegative
}

func TestVerifiedRule(t *testing.T) {
	yakit.InitialDatabase()
	db := consts.GetGormProfileDatabase()
	db = db.Where("is_build_in_rule = ? ", true)
	for rule := range sfdb.YieldSyntaxFlowRules(db, context.Background()) {
		f, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(rule.Content)
		if err != nil {
			t.Fatalf("compile rule %s error: %s", rule.RuleName, err)
		}
		if len(f.VerifyFsInfo) == 0 {
			continue
		}
		t.Run(strings.Join(append(strings.Split(rule.Tag, "|"), rule.RuleName), "/"), func(t *testing.T) {
			t.Log("Start to verify: " + rule.RuleName)
			err := ssatest.EvaluateVerifyFilesystemWithRule(rule, t)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

/*
--- FAIL: TestVerifiedRule (65.99s)

	--- FAIL: TestVerifiedRule/golang/CWE-1336/ssti/golang_SSTI_漏洞(sprig) (0.45s)
	--- FAIL: TestVerifiedRule/golang/CWE-200/information-exposure/检测Golang_FTP信息泄露 (0.57s)
	--- FAIL: TestVerifiedRule/golang/CWE-200/information-exposure/golang_Json相关函数泄露服务器敏感信息 (0.19s)
	--- FAIL: TestVerifiedRule/golang/CWE-200/information-exposure/检测Golang_SQL信息泄露 (0.45s)
	--- FAIL: TestVerifiedRule/golang/CWE-259/hard-coded-password/检测_Golang_LDAP_凭证硬编码漏洞 (0.35s)
	--- FAIL: TestVerifiedRule/golang/CWE-287/improper-authentication/检测Golang空密码漏洞 (0.13s)
	--- FAIL: TestVerifiedRule/golang/CWE-434/file-upload/golang_文件路径未授权(beego) (0.10s)
	--- FAIL: TestVerifiedRule/golang/CWE-434/file-upload/审计Golang文件上传漏洞 (0.08s)
	--- FAIL: TestVerifiedRule/golang/CWE-601/open-redirect/检测_Golang_开放重定向漏洞 (0.13s)
	--- FAIL: TestVerifiedRule/golang/CWE-611/xxe/golang_XXE(beego) (0.30s)
	--- FAIL: TestVerifiedRule/golang/CWE-611/xxe/检测Golang_XXE任意文件读取漏洞 (0.15s)
	--- FAIL: TestVerifiedRule/golang/CWE-73/unfiltered-file-or-path/golang_未过滤的文件或路径(beego) (0.00s)
	--- FAIL: TestVerifiedRule/golang/CWE-73/unfiltered-file-or-path/检测Golang未过滤文件路径漏洞 (0.00s)
	--- FAIL: TestVerifiedRule/golang/CWE-77/command-injection/检测Golang命令注入漏洞 (0.28s)
	--- FAIL: TestVerifiedRule/golang/CWE-79/xss/golang_反射型跨站脚本攻击(gobee) (0.08s)
	--- FAIL: TestVerifiedRule/golang/CWE-79/xss/检测Golang模板引擎中的反射型XSS漏洞 (0.04s)
	--- FAIL: TestVerifiedRule/golang/CWE-79/xss/检测Golang反射型XSS漏洞 (0.18s)
	--- FAIL: TestVerifiedRule/golang/CWE-89/sql-injection/检测Golang_SQL注入漏洞 (0.10s)
	--- FAIL: TestVerifiedRule/golang/CWE-89/sql-injection/检测_Golang_SQL_注入漏洞(ent) (0.42s)
	--- FAIL: TestVerifiedRule/golang/CWE-89/sql-injection/检测Golang_SQL注入漏洞(GORM) (0.11s)
	--- FAIL: TestVerifiedRule/golang/CWE-89/sql-injection/检测Golang_SQL注入漏洞(pop) (0.25s)
	--- FAIL: TestVerifiedRule/golang/CWE-89/sql-injection/检测Golang_Reform_SQL注入漏洞 (0.10s)
	--- FAIL: TestVerifiedRule/golang/CWE-89/sql-injection/golang_SQL_语句拼接的不安全写法 (0.14s)
	--- FAIL: TestVerifiedRule/golang/CWE-89/sql-injection/检测Golang_Sqlx_SQL注入漏洞 (0.24s)
	--- FAIL: TestVerifiedRule/golang/CWE-90/ldap-injection/检测Golang_LDAP注入漏洞 (0.35s)
	--- FAIL: TestVerifiedRule/golang/CWE-918/ssrf/golang_服务器端请求伪造(beego) (0.05s)
	--- FAIL: TestVerifiedRule/golang/CWE-918/ssrf/检测Golang_HTTP_SSRF漏洞 (0.31s)
	--- FAIL: TestVerifiedRule/golang/CWE-93/crlf-injection.sf/golang_CRLF注入漏洞(beego) (0.09s)
	--- FAIL: TestVerifiedRule/golang/CWE-942/cors/golang_CORS_漏洞(beego) (0.16s)
	--- FAIL: TestVerifiedRule/golang/lib/审计Golang文件路径处理 (0.17s)
	--- FAIL: TestVerifiedRule/golang/lib/审计Golang文件读取bufio包使用 (0.58s)
	--- FAIL: TestVerifiedRule/golang/lib/审计Golang_ioutil文件读取方法 (0.22s)
	--- FAIL: TestVerifiedRule/golang/lib/审计Golang文件读取功能 (0.16s)
	--- FAIL: TestVerifiedRule/golang/lib/审计Golang使用bufio进行文件写入的代码 (0.65s)
	--- FAIL: TestVerifiedRule/golang/lib/审计Golang文件写入(ioutil) (0.28s)
	--- FAIL: TestVerifiedRule/golang/lib/审计Golang文件写入函数(os) (0.33s)
	--- FAIL: TestVerifiedRule/golang/lib/审计Golang_FTP库使用情况 (0.14s)
	--- FAIL: TestVerifiedRule/golang/lib/查找_Golang_LDAP_连接汇点 (0.25s)
	--- FAIL: TestVerifiedRule/golang/lib/审计Golang_OS_Exec命令使用 (0.30s)
	--- FAIL: TestVerifiedRule/golang/lib/审计Golang_os包使用 (0.28s)
	--- FAIL: TestVerifiedRule/golang/lib/审计Golang_html/template使用 (0.17s)
	--- FAIL: TestVerifiedRule/golang/lib/审计Golang_XML解析 (0.28s)
	--- FAIL: TestVerifiedRule/golang/lib/http/审计Golang_gin_HTTP_Handler (0.74s)
	--- FAIL: TestVerifiedRule/golang/lib/http/审计_Golang_net/http_请求处理函数 (0.48s)
	--- FAIL: TestVerifiedRule/golang/lib/http/审计Golang_HTTP输出点 (0.19s)
	--- FAIL: TestVerifiedRule/golang/lib/http/审计Golang_HTTP输入点 (0.30s)
	--- FAIL: TestVerifiedRule/golang/lib/sqldb/golang-database-from-param (0.15s)
	--- FAIL: TestVerifiedRule/golang/lib/sqldb/审计Golang_GORM库使用 (0.51s)
	--- FAIL: TestVerifiedRule/golang/lib/sqldb/审计_Go_语言_Database_Pop_库使用 (0.33s)
	--- FAIL: TestVerifiedRule/golang/lib/sqldb/审计Golang_Database_Reform的使用 (0.71s)
	--- FAIL: TestVerifiedRule/golang/lib/sqldb/审计Golang_Database/SQL使用 (0.57s)
	--- FAIL: TestVerifiedRule/golang/lib/sqldb/审计Golang_Sqlx库使用情况 (0.53s)
	--- FAIL: TestVerifiedRule/java/code-style/j2ee-bad-practices/检测Java_J2EE_使用DriverManager_getConnection (0.19s)
	--- FAIL: TestVerifiedRule/java/code-style/j2ee-bad-practices/审计Java_J2EE_标准使用线程规则 (0.17s)
	--- FAIL: TestVerifiedRule/java/components/actuator/检查Java_Spring_Boot_Actuator配置 (0.32s)
	--- FAIL: TestVerifiedRule/java/components/fastjson/SCA:_检测Java_FastJson依赖漏洞 (0.24s)
	--- FAIL: TestVerifiedRule/java/components/jwt/检查_Java_JWT安全问题 (0.39s)
	--- FAIL: TestVerifiedRule/java/components/log4j/检测Java_Log4j远程代码执行漏洞 (0.13s)
	--- FAIL: TestVerifiedRule/java/components/mytheleaf/config/审计Java_Thymeleaf配置 (0.11s)
	--- FAIL: TestVerifiedRule/java/components/quartz/审计_Java_Quartz_Job_类识别 (0.12s)
	--- FAIL: TestVerifiedRule/java/components/shiro/检测Java_Shiro硬编码加密密钥 (0.85s)
	--- FAIL: TestVerifiedRule/java/CWE-117/improper-output-neutralization-for-logs/检测Java_日志伪造攻击 (0.11s)
	--- FAIL: TestVerifiedRule/java/CWE-1336/ssti/检测Java_Freemarker模板注入漏洞 (0.15s)
	--- FAIL: TestVerifiedRule/java/CWE-1336/ssti/审计_Java_Spring_Framework_处理_ModelAndView_时直接传入_String_参数 (0.24s)
	--- FAIL: TestVerifiedRule/java/CWE-200/exposure-of-sensitive-information/检测Java_Spring资源处理程序位置 (0.01s)
	--- FAIL: TestVerifiedRule/java/CWE-200/exposure-of-sensitive-information/审计Java_Springfox配置 (0.23s)
	--- FAIL: TestVerifiedRule/java/CWE-209/information-exposure-through-an-error-message/检查Java通过PrintStackTrace泄露信息 (0.07s)
	--- FAIL: TestVerifiedRule/java/CWE-22/path-travel/检测Java路径穿越漏洞 (0.07s)
	--- FAIL: TestVerifiedRule/java/CWE-247/reliance-on-dns-lookups-in-a-security-decision/检测Java_java.net.InetAddress_进行DNS查询 (0.09s)
	--- FAIL: TestVerifiedRule/java/CWE-252/unchecked-return-value/检测Java_IO库未检查返回值的API (0.27s)
	--- FAIL: TestVerifiedRule/java/CWE-252/unchecked-return-value/检测Java_Lang库未检查返回值的API (0.03s)
	--- FAIL: TestVerifiedRule/java/CWE-252/unchecked-return-value/检测Java_javax.sql.rowset库未检查返回值的API (0.14s)
	--- FAIL: TestVerifiedRule/java/CWE-252/unchecked-return-value/检测Java_Util库未检查返回值的API (0.04s)
	--- FAIL: TestVerifiedRule/java/CWE-252/unchecked-return-value/检测Java_Zip未检查返回值的API (0.25s)
	--- FAIL: TestVerifiedRule/java/CWE-287/improper-authentication/检测Java配置文件中不当的密码配置 (0.05s)
	--- FAIL: TestVerifiedRule/java/CWE-287/improper-authentication/检测Java密码管理中使用空密码 (0.15s)
	--- FAIL: TestVerifiedRule/java/CWE-297/improper-validation-of-certificate-with-host-mismatch/检测Java_SimpleEmail证书校验 (0.08s)
	--- FAIL: TestVerifiedRule/java/CWE-327/risky-cryptographic-algorithm/检测Java_Cipher使用不安全或有风险的加密算法 (0.38s)
	--- FAIL: TestVerifiedRule/java/CWE-327/risky-cryptographic-algorithm/检测Java_javax.crypto.KEM.Encapsulator使用不安全的加密算法 (0.08s)
	--- FAIL: TestVerifiedRule/java/CWE-327/risky-cryptographic-algorithm/检测Java_java.security.AlgorithmParameters使用不安全的加密算法 (0.12s)
	--- FAIL: TestVerifiedRule/java/CWE-327/risky-cryptographic-algorithm/检测Java_java.security使用不安全的哈希算法 (0.26s)
	--- FAIL: TestVerifiedRule/java/CWE-352/csrf/检查_Java_Spring_Framework_CSRF_保护 (0.02s)
	--- FAIL: TestVerifiedRule/java/CWE-390/delection-error-without-action/检测Java空Catch块 (0.17s)
	--- FAIL: TestVerifiedRule/java/CWE-400/uncontrolled-resource-consumption/检测Java_cn.hutool.captcha验证码不受控资源消耗漏洞 (0.19s)
	--- FAIL: TestVerifiedRule/java/CWE-434/unrestricted-upload-file/审计Java_Spring_Framework文件上传保存 (0.54s)
	--- FAIL: TestVerifiedRule/java/CWE-434/unrestricted-upload-file/检测Spring_MVC任意文件上传漏洞 (0.37s)
	--- FAIL: TestVerifiedRule/java/CWE-470/reflection-call-security/检测_Java_反射调用的潜在威胁 (0.21s)
	--- FAIL: TestVerifiedRule/java/CWE-502/untrusted-unserialization/检测Java原生反序列化漏洞 (0.30s)
	--- FAIL: TestVerifiedRule/java/CWE-502/untrusted-unserialization/检测Java_SnakeYAML反序列化漏洞 (0.14s)
	--- FAIL: TestVerifiedRule/java/CWE-502/untrusted-unserialization/检测Java_XMLDecoder反序列化漏洞 (0.44s)
	--- FAIL: TestVerifiedRule/java/CWE-601/url-redirect/检测Java_URL重定向漏洞 (0.29s)
	--- FAIL: TestVerifiedRule/java/CWE-611/xxe/检测_Java_SAXBuilder_非安全使用 (0.11s)
	--- FAIL: TestVerifiedRule/java/CWE-611/xxe/检测_Java_SAXParserFactory_不安全使用 (0.30s)
	--- FAIL: TestVerifiedRule/java/CWE-611/xxe/检测_Java_SAXReader_未安全使用 (0.10s)
	--- FAIL: TestVerifiedRule/java/CWE-643/xpath-injection/检测Java_XPath注入漏洞 (0.43s)
	--- FAIL: TestVerifiedRule/java/CWE-690/unchecked-return-value-to-null-pointer-dereference/检测Java_未检测返回值是否为null导致空指针 (0.47s)
	--- FAIL: TestVerifiedRule/java/CWE-73/unfiltered-file-or-path/审计Java_SetHeader中文件下载位置配置点 (0.22s)
	--- FAIL: TestVerifiedRule/java/CWE-73/unfiltered-file-or-path/检测Java_Springboot文件下载漏洞 (0.22s)
	--- FAIL: TestVerifiedRule/java/CWE-749/exposed-dangerous-method-or-function/检测Java反射setAccessible函数滥用漏洞 (0.01s)
	--- FAIL: TestVerifiedRule/java/CWE-77/command-injection/检测Java_Servlet和SpringMVC中的命令注入漏洞 (1.37s)
	--- FAIL: TestVerifiedRule/java/CWE-772/missing-release-of-resource-after-effective-lifetime/检测Java_Hibernate_数据库Connection资源未释放 (0.70s)
	--- FAIL: TestVerifiedRule/java/CWE-772/missing-release-of-resource-after-effective-lifetime/检测Java_Hibernate_Session_数据库资源未释放 (0.43s)
	--- FAIL: TestVerifiedRule/java/CWE-772/missing-release-of-resource-after-effective-lifetime/检测Java_java.io_流资源未释放 (0.75s)
	--- FAIL: TestVerifiedRule/java/CWE-772/missing-release-of-resource-after-effective-lifetime/检测Java_Socket资源未释放 (1.25s)
	--- FAIL: TestVerifiedRule/java/CWE-772/missing-release-of-resource-after-effective-lifetime/检测Java_java.sql_Connection_资源未释放 (0.51s)
	--- FAIL: TestVerifiedRule/java/CWE-772/missing-release-of-resource-after-effective-lifetime/检测Java_java.sql_Statement查询结果集ResultSet资源未释放 (0.56s)
	--- FAIL: TestVerifiedRule/java/CWE-772/missing-release-of-resource-after-effective-lifetime/检测Java_java.sql_数据库Statement资源未释放 (0.37s)
	--- FAIL: TestVerifiedRule/java/CWE-772/missing-release-of-resource-after-effective-lifetime/检测Java_Zip_GetInputStream资源未释放 (1.09s)
	--- FAIL: TestVerifiedRule/java/CWE-79/xss/检测Java_EE的XSS漏洞 (1.64s)
	--- FAIL: TestVerifiedRule/java/CWE-79/xss/检测Java_Spring_Boot框架模板引擎XSS漏洞 (0.16s)
	--- FAIL: TestVerifiedRule/java/CWE-79/xss/检测Java_SpringBoot_RestController_XSS漏洞 (0.60s)
	--- FAIL: TestVerifiedRule/java/CWE-79/xss/审计_Java_XSS_白名单绕过 (0.13s)
	--- FAIL: TestVerifiedRule/java/CWE-89/sql-injection/检查_Java_SQL_语句拼接 (0.68s)
	--- FAIL: TestVerifiedRule/java/CWE-89/sql-injection/检测Java_SQL注入漏洞 (0.29s)
	--- FAIL: TestVerifiedRule/java/CWE-89/sql-injection/查找Java拼接SQL字符串 (1.89s)
	--- FAIL: TestVerifiedRule/java/CWE-89/sql-injection/检测Java_Hibernate_SQL注入漏洞 (0.40s)
	--- FAIL: TestVerifiedRule/java/CWE-89/sql-injection/查找_Java_MyBatis/iBatis_XML_Mapper_不安全(${...})参数 (0.68s)
	--- FAIL: TestVerifiedRule/java/CWE-89/sql-injection/检测Java_SQL字符串拼接查询 (1.70s)
	--- FAIL: TestVerifiedRule/java/CWE-918/ssrf/检测Java_SpringBoot_服务端请求伪造(SSRF)漏洞 (0.32s)
	--- FAIL: TestVerifiedRule/java/CWE-94/code-injection/检测Java动态代码执行中的脚本注入漏洞 (0.20s)
	--- FAIL: TestVerifiedRule/java/CWE-94/code-injection/审计Java_EL_Expression_Factory使用 (0.22s)
	--- FAIL: TestVerifiedRule/java/CWE-94/code-injection/检测Java_Servlet_Groovy_Shell代码注入漏洞 (1.73s)
	--- FAIL: TestVerifiedRule/java/CWE-94/code-injection/检测Java_Spring_Boot_Groovy_Shell代码注入漏洞 (0.67s)
	--- FAIL: TestVerifiedRule/java/CWE-94/code-injection/审计Java_Spring_EL使用 (0.07s)
	--- FAIL: TestVerifiedRule/java/CWE-94/code-injection/检测Java_Spring_SPEL_Parser表达式注入漏洞 (0.32s)
	--- FAIL: TestVerifiedRule/java/CWE-942/overly-permissive-cross-domain-whitelist/检测Java_Spring_Framework跨域白名单过于宽松 (0.23s)
	--- FAIL: TestVerifiedRule/java/insecure_data_convert/检测Java不安全的字节数组到字符串转换 (0.01s)
	--- FAIL: TestVerifiedRule/java/lib/code-exec-sink/审计Java_GroovyShell_代码执行Sink点 (0.45s)
	--- FAIL: TestVerifiedRule/java/lib/code-exec-sink/查找Java_javax.script.*_ScriptEngineManager_eval_Sink (0.24s)
	--- FAIL: TestVerifiedRule/java/lib/command-exec-sink/查找Java_ProcessBuilder_Sink点 (0.60s)
	--- FAIL: TestVerifiedRule/java/lib/file-operator/查找_Java_文件删除接收点 (0.26s)
	--- FAIL: TestVerifiedRule/java/lib/filter/查找Java含有contain方法的过滤器 (0.13s)
	--- FAIL: TestVerifiedRule/java/lib/http/审计Java_Alibaba_Druid_HttpClientUtils的使用 (0.05s)
	--- FAIL: TestVerifiedRule/java/lib/http/审计_Java_Apache_Commons_HttpClient_使用 (0.10s)
	--- FAIL: TestVerifiedRule/java/lib/http/审计Java_Apache_HttpClient请求执行点 (0.19s)
	--- FAIL: TestVerifiedRule/java/lib/http/审计Java_HTTP_Fluent_Request (0.09s)
	--- FAIL: TestVerifiedRule/java/lib/http/查找Java中的HTTP_Sink_(多库) (0.28s)
	--- FAIL: TestVerifiedRule/java/lib/http/审计Java_ImageIo_读取_URL_的方法 (0.08s)
	--- FAIL: TestVerifiedRule/java/lib/http/审计Java_URL连接使用 (0.22s)
	--- FAIL: TestVerifiedRule/java/lib/http/查找Java_OkHttpClient使用及请求执行 (0.17s)
	--- FAIL: TestVerifiedRule/java/lib/log/查找Java日志记录方法 (0.18s)
	--- FAIL: TestVerifiedRule/java/lib/net/查找Java_TCP数据接收点 (0.49s)
	--- FAIL: TestVerifiedRule/java/lib/sql-operator/审计Java_JDBC_PreparedStatement_执行查询 (0.11s)
	--- FAIL: TestVerifiedRule/java/lib/sql-operator/检测_Java_JDBC_Statement.executeQuery_调用 (0.09s)
	--- FAIL: TestVerifiedRule/java/lib/user-input-http-source/审计Java_Servlet用户输入 (0.06s)
	--- FAIL: TestVerifiedRule/java/lib/user-input-http-source/查找Java_Spring_MVC_控制层用户可控输入参数 (1.30s)
	--- FAIL: TestVerifiedRule/java/sca/SCA:_检测Java_Fastjson依赖漏洞 (0.05s)
	--- FAIL: TestVerifiedRule/java/sca/SCA:_检测Java_Hessian依赖漏洞 (0.16s)
	--- FAIL: TestVerifiedRule/java/sca/SCA:_检测Java_shiro-core_依赖漏洞 (0.08s)
	--- FAIL: TestVerifiedRule/java/sca/SCA:_检测Java_Spring_Boot_Devtools使用 (0.02s)
	--- FAIL: TestVerifiedRule/php/CWE-259/hard-coded-password/审计PHP硬编码密码 (0.05s)
	--- FAIL: TestVerifiedRule/php/CWE-259/hard-coded-password/检测PHP中硬编码的LDAP凭据 (0.05s)
	--- FAIL: TestVerifiedRule/php/CWE-287/auth-byass/审计PHP_ThinkPHP认证绕过 (0.23s)
	--- FAIL: TestVerifiedRule/php/CWE-863/incorrect-authorization/检测PHP不安全的文件操作 (0.18s)
	--- FAIL: TestVerifiedRule/php/CWE-863/incorrect-authorization/检测PHP未验证FTP参数 (0.18s)
	--- FAIL: TestVerifiedRule/php/CWE-200/information-exposure/发现PHP_FTP信息泄露 (0.09s)
	--- FAIL: TestVerifiedRule/php/CWE-200/information-exposure/检测PHP信息泄漏漏洞 (0.13s)
	--- FAIL: TestVerifiedRule/php/CWE-200/information-exposure/检测PHP信息泄露风险 (0.04s)
	--- FAIL: TestVerifiedRule/php/CWE-434/file-upload/检测PHP不安全的文件上传漏洞 (0.20s)
	--- FAIL: TestVerifiedRule/php/CWE-434/file-upload/检测PHP_ThinkPHP框架不安全文件上传漏洞 (0.20s)
	--- FAIL: TestVerifiedRule/php/CWE-502/unserialize/检测PHP反序列化漏洞 (0.12s)
	--- FAIL: TestVerifiedRule/php/CWE-601/open-redirect/检测PHP开放重定向漏洞 (0.05s)
	--- FAIL: TestVerifiedRule/php/CWE-611/xxe/检测PHP_DOMDocument_load_XXE漏洞 (0.15s)
	--- FAIL: TestVerifiedRule/php/CWE-73/unfiltered-file-or-path/审计PHP未过滤文件或路径操作 (0.18s)
	--- FAIL: TestVerifiedRule/php/CWE-73/unfiltered-file-or-path/检测PHP恶意文件操作 (0.11s)
	--- FAIL: TestVerifiedRule/php/CWE-73/unfiltered-file-or-path/检测PHP未过滤目录读取 (0.07s)
	--- FAIL: TestVerifiedRule/php/CWE-73/unfiltered-file-or-path/审计PHP文件解压安全风险 (0.10s)
	--- FAIL: TestVerifiedRule/php/CWE-73/unfiltered-file-or-path/检测PHP文件解压漏洞 (0.13s)
	--- FAIL: TestVerifiedRule/php/CWE-73/unfiltered-file-or-path/检测PHP_Zip文件路径遍历漏洞 (0.13s)
	--- FAIL: TestVerifiedRule/php/CWE-77/common-injection/审计PHP文件包含漏洞 (0.23s)
	--- FAIL: TestVerifiedRule/php/CWE-78/os-command-injection/检测PHP命令注入漏洞 (0.08s)
	--- FAIL: TestVerifiedRule/php/CWE-78/os-command-injection/检测PHP代码执行漏洞 (0.24s)
	--- FAIL: TestVerifiedRule/php/CWE-79/xss/检测PHP跨站脚本漏洞 (0.20s)
	--- FAIL: TestVerifiedRule/php/CWE-89/sql-injection/检测PHP_MySQL注入漏洞 (0.19s)
	--- FAIL: TestVerifiedRule/php/CWE-89/sql-injection/检测PHP_PostgreSQL注入漏洞 (0.20s)
	--- FAIL: TestVerifiedRule/php/CWE-89/sql-injection/检测PHP_ThinkPHP_SQL注入漏洞 (0.15s)
	--- FAIL: TestVerifiedRule/php/CWE-918/ssrf/检测PHP_SSRF漏洞 (0.17s)
	--- FAIL: TestVerifiedRule/php/lib/审计PHP自定义过滤函数使用情况 (0.07s)
	--- FAIL: TestVerifiedRule/php/lib/审计PHP自定义外部变量使用 (0.24s)
	--- FAIL: TestVerifiedRule/php/lib/审计PHP文件读取函数 (0.12s)
	--- FAIL: TestVerifiedRule/php/lib/审计PHP文件写入方法 (0.13s)
	--- FAIL: TestVerifiedRule/php/lib/审计PHP_ThinkPHP_Param_参数使用 (0.10s)
*/
func TestVerify_DEBUG(t *testing.T) {
	if utils.InGithubActions() {
		t.SkipNow()
		return
	}
	yakit.InitialDatabase()
	err := sfbuildin.SyncEmbedRule()
	require.NoError(t, err)
	ruleName := "审计Golang FTP 硬编码密码"
	// ruleName := "审计Golang HTTP输出点"

	rule, err := sfdb.GetRulePure(ruleName)
	if err != nil {
		t.Fatal(err)
	}

	f, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(rule.Content)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.VerifyFsInfo) != 0 {
		t.Run(rule.RuleName, func(t *testing.T) {
			t.Log("Start to verify: " + rule.RuleName)
			err := ssatest.EvaluateVerifyFilesystemWithRule(rule, t)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBuildInRule_Verify_Negative_AlertMin(t *testing.T) {
	err := ssatest.EvaluateVerifyFilesystem(`
desc(
alert_min: '2',
language: yaklang,
'file://a.yak': <<<EOF
b = () => {
	a = 1;
}
EOF
)

a as $output;
check $output;
alert $output;

`, t)
	if err == nil {
		t.Fatal("expect error")
	}
}

func TestBuildInRule_Verify_Positive_AlertMin2(t *testing.T) {
	err := ssatest.EvaluateVerifyFilesystem(`
desc(
alert_min: 1,
language: yaklang,
'file://a.yak': <<<EOF
b = () => {
	a = 1;
}
EOF
)

a as $output;
check $output;
alert $output;

`, t)
	if err != nil {
		t.Fatal(err)
	}
}

func TestImport(t *testing.T) {
	_, err := sfdb.ImportRuleWithoutValid("test.sf", `
desc(
	level: "high",
	lang: "php",
)
$a #-> * as $param

alert $param for {"level": "high"}
`, true)
	require.NoError(t, err)
	rule, err := sfdb.GetRule("test.sf")
	require.NoError(t, err)
	var m map[string]*schema.SyntaxFlowDescInfo
	fmt.Println(rule.AlertDesc)
	err = json.Unmarshal(codec.AnyToBytes(rule.AlertDesc), &m)
	require.NoError(t, err)
	info, ok := m["param"]
	require.True(t, ok)
	require.True(t, info.Severity == schema.SFR_SEVERITY_HIGH)
	err = sfdb.DeleteRuleByRuleName("test.sf")
	require.NoError(t, err)
}

func TestJavaDependencies(t *testing.T) {
	code := `
__dependency__.*fastjson.version as $ver;
$ver?{version_in:(1.2.3,2.3.4]}  as $vulnVersion
alert $vulnVersion for {
	title:"存在fastjson 1.2.3-2.3.4漏洞",
};

desc(
lang: java,
'file://pom.xml': <<<CODE
<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>

    <groupId>com.example</groupId>
    <artifactId>vulnerable-fastjson-app</artifactId>
    <version>1.0-SNAPSHOT</version>

    <dependencies>
        <!-- Fastjson dependency with known vulnerabilities -->
        <dependency>
            <groupId>com.alibaba</groupId>
            <artifactId>fastjson</artifactId>
            <!-- An example version with known vulnerabilities, make sure to check for specific vulnerable versions -->
            <version>1.2.24</version>
        </dependency>
    </dependencies>
</project>
CODE
)`
	err := ssatest.EvaluateVerifyFilesystem(code, t)
	if err != nil {
		t.Fatal(err)
	}
}

const DEBUGCODE = `
desc(
    title: "Check for suspected SQL statement concatenation and execution in database queries",
    title_zh: "检查疑似 SQL 语句拼接并执行到数据库查询的代码"
)

e"SELECT COUNT(*) FROM qrtz_cron_triggers"<show> as $a;
alert $a

/(?i)\w+sql/ as $b;
alert $b


desc(
lang: java,
"file://a.java": <<<FILE
package com.itstyle.quartz.service.impl;


@Service("jobService")
public class JobServiceImpl implements IJobService {

	@Autowired
	private DynamicQuery dynamicQuery;
    @Autowired
    private Scheduler scheduler;
	@Override
	public Result listQuartzEntity(QuartzEntity quartz,
			Integer pageNo, Integer pageSize) throws SchedulerException {
	    String countSql = "SELECT COUNT(*) FROM qrtz_cron_triggers";
        if(!StringUtils.isEmpty(quartz.getJobName())){
            countSql+=" AND job.JOB_NAME = "+quartz.getJobName();
        }
        Long totalCount = dynamicQuery.nativeQueryCount(countSql);
        PageBean<QuartzEntity> data = new PageBean<>();
        if(totalCount>0){
            StringBuffer nativeSql = new StringBuffer();
            nativeSql.append("SELECT job.JOB_NAME as jobName,job.JOB_GROUP as jobGroup,job.DESCRIPTION as description,job.JOB_CLASS_NAME as jobClassName,");
            nativeSql.append("cron.CRON_EXPRESSION as cronExpression,tri.TRIGGER_NAME as triggerName,tri.TRIGGER_STATE as triggerState,");
            nativeSql.append("job.JOB_NAME as oldJobName,job.JOB_GROUP as oldJobGroup ");
            nativeSql.append("FROM qrtz_job_details AS job ");
            nativeSql.append("LEFT JOIN qrtz_triggers AS tri ON job.JOB_NAME = tri.JOB_NAME  AND job.JOB_GROUP = tri.JOB_GROUP ");
            nativeSql.append("LEFT JOIN qrtz_cron_triggers AS cron ON cron.TRIGGER_NAME = tri.TRIGGER_NAME AND cron.TRIGGER_GROUP= tri.JOB_GROUP ");
            nativeSql.append("WHERE tri.TRIGGER_TYPE = 'CRON'");
            Object[] params = new  Object[]{};
            if(!StringUtils.isEmpty(quartz.getJobName())){
                nativeSql.append(" AND job.JOB_NAME = ?");
                params = new Object[]{quartz.getJobName()};
            }
            Pageable pageable = PageRequest.of(pageNo-1,pageSize);
            List<QuartzEntity> list = dynamicQuery.nativeQueryPagingList(QuartzEntity.class,pageable, nativeSql.toString(), params);
            for (QuartzEntity quartzEntity : list) {
                JobKey key = new JobKey(quartzEntity.getJobName(), quartzEntity.getJobGroup());
                JobDetail jobDetail = scheduler.getJobDetail(key);
                quartzEntity.setJobMethodName(jobDetail.getJobDataMap().getString("jobMethodName"));
            }
            data = new PageBean<>(list, totalCount);
        }
        return Result.ok(data);
	}

	@Override
	public Long listQuartzEntity(QuartzEntity quartz) {
		StringBuffer nativeSql = new StringBuffer();
		nativeSql.append("SELECT COUNT(*)");
		nativeSql.append("FROM qrtz_job_details AS job LEFT JOIN qrtz_triggers AS tri ON job.JOB_NAME = tri.JOB_NAME ");
		nativeSql.append("LEFT JOIN qrtz_cron_triggers AS cron ON cron.TRIGGER_NAME = tri.TRIGGER_NAME ");
		nativeSql.append("WHERE tri.TRIGGER_TYPE = 'CRON'");
		return dynamicQuery.nativeQueryCount(nativeSql.toString(), new Object[]{});
	}

    @Override
    @Transactional
    public void save(QuartzEntity quartz) throws Exception{
        //如果是修改  展示旧的 任务
        if(quartz.getOldJobGroup()!=null){
            JobKey key = new JobKey(quartz.getOldJobName(),quartz.getOldJobGroup());
            scheduler.deleteJob(key);
        }
        Class cls = Class.forName(quartz.getJobClassName()) ;
        cls.newInstance();
        //构建job信息
        JobDetail job = JobBuilder.newJob(cls).withIdentity(quartz.getJobName(),
                quartz.getJobGroup())
                .withDescription(quartz.getDescription()).build();
        job.getJobDataMap().put("jobMethodName", quartz.getJobMethodName());
        // 触发时间点
        CronScheduleBuilder cronScheduleBuilder = CronScheduleBuilder.cronSchedule(quartz.getCronExpression());
        Trigger trigger = TriggerBuilder.newTrigger().withIdentity("trigger"+quartz.getJobName(), quartz.getJobGroup())
                .startNow().withSchedule(cronScheduleBuilder).build();
        //交由Scheduler安排触发
        scheduler.scheduleJob(job, trigger);
    }
}
FILE,
)
`

func TestJavaDEBUG(t *testing.T) {
	if utils.InGithubActions() {
		return
	}
	err := ssatest.EvaluateVerifyFilesystem(DEBUGCODE, t)
	if err != nil {
		t.Fatal(err)
	}
}
