package sfvm

import (
	"fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

func compileSyntaxFlow(text string) *SyntaxFlowVisitor {
	var errs antlr4util.SourceCodeErrors
	errHandler := antlr4util.SimpleSyntaxErrorHandler(func(msg string, start, end *memedit.Position) {
		errs = append(errs, antlr4util.NewSourceCodeError(msg, start, end))
	})
	errLis := antlr4util.NewErrorListener(func(self *antlr4util.ErrorListener, recognizer antlr.Recognizer, offendingSymbol interface{}, line, column int, msg string, e antlr.RecognitionException) {
		antlr4util.StringSyntaxErrorHandler(self, recognizer, offendingSymbol, line, column, msg, e)
		errHandler(self, recognizer, offendingSymbol, line, column, msg, e)
	})
	lexer := sf.NewSyntaxFlowLexer(antlr.NewInputStream(text))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errLis)
	astParser := sf.NewSyntaxFlowParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	astParser.RemoveErrorListeners()
	astParser.AddErrorListener(errLis)
	visitor := NewSyntaxFlowVisitor()
	visitor.VisitFlow(astParser.Flow())
	return visitor
}
func TestName(t *testing.T) {
	syntaxFlow := compileSyntaxFlow(`a*?{opcode: const}
`)
	for _, code := range syntaxFlow.codes {
		fmt.Println(Opcode2String[code.OpCode])
	}
}

func TestSFVMCompileRule(t *testing.T) {
	rule, err := CompileRule("desc(\n        title: \"Audit PHP OS Command Execution Functions\"\n        type: audit\n        level: info\n        lib: 'php-os-exec'\n        desc: <<<DESC\n### 1.规则目的\n该规则用于审计PHP代码中可能导致远程命令执行（RCE）或代码执行漏洞的危险函数使用。它通过识别直接或间接执行用户输入代码或命令的关键PHP函数，定位未经验证/转义的数据输入点。\n\n### 2.规则详细\n1. **作为基础检测库**\n   属于`php-os-exec`库规则（`lib`类型），需配合其他规则（如用户输入跟踪规则）共同检测命令执行漏洞，提供核心函数识别能力。\n\n2. **覆盖高危执行函数**\n   检测以下执行函数：\n   `eval()`, `exec()`, `assert()`, `system()`, `shell_exec()`, `pcntl_exec()`, `popen()`, `ob_start()`。\n\n   当这些函数接收了未经适当验证或转义的用户输入时，攻击者可以利用此漏洞执行任意代码或命令，进而完全控制服务器或执行恶意操作。因此，建议开发者避免使用这些危险函数，或在使用时对用户输入进行严格的验证和转义。\n\n### 触发场景\n// 存在漏洞的代码示例\n```php\n<?php\n    eval($_POST[1]);\n    exec($_POST[1]);\n    assert($_POST[1]);\n    system($_POST[1]);\n    shell_exec($_POST[1]);\n    pcntl_exec($_POST[1]);\n    popen($_POST[1]);\n    ob_start($_POST[1]);\n    ob_end($_POST[1]);\n?>\n```\n攻击者可以通过POST请求向服务器发送恶意PHP代码或系统命令，例如`?1=system('ls')`来列出服务器文件，或者注入恶意脚本，导致数据泄露、服务器被控等严重后果。\n\n### 潜在影响\n- **远程代码执行 (RCE)**: 攻击者可以直接在服务器上执行任意代码或命令。\n- **数据泄露/篡改**: 攻击者可以通过执行命令访问、修改或删除服务器上的敏感文件。\n- **服务器控制**: 攻击者可以进一步利用漏洞完全控制受影响的服务器，进行恶意活动。\n- **拒绝服务 (DoS)**: 攻击者可以执行消耗大量系统资源的命令，导致服务不可用。\nDESC\n        rule_id: \"4d56af61-28a4-48fd-812c-d28171f4ada7\"\n        title_zh: \"审计PHP命令执行函数\"\n        solution: <<<SOLUTION\n### 修复建议\n当规则命中这些危险函数时，并不能完全确定存在漏洞，这是lib规则的特性。需要结合其他规则来判断是否存在漏洞。但是，为了安全起见，可以采取以下措施来减少潜在风险：\n\n#### 1. 避免使用危险函数\n尽可能避免在生产环境中使用如 `eval()`、`exec()` 等可以直接执行代码或命令的函数。寻找更安全的替代方案。\n\n#### 2. 用户输入严格验证和过滤\n如果确实需要使用这些函数，必须对所有用户输入进行严格的验证、清洗和转义。永远不要直接将用户输入作为参数传递给这些函数。\n\n```php\n<?php\n// 示例：过滤 exec() 函数的输入\n$command = escapeshellcmd($_POST['cmd']); // 对输入进行命令转义\n$output = shell_exec($command); // 使用转义后的命令\n// ... 其他操作\n?>\n```\n\n#### 3. 使用安全的API或库\n优先使用PHP内置的安全API或受信的第三方库来处理文件操作、进程管理等，这些API通常提供了更严格的安全检查和参数处理。\n\n#### 4. 最小权限原则\n运行PHP应用的操作系统用户应遵循最小权限原则，只授予必要的权限，限制执行任意系统命令的能力。\n\n#### 5. Web应用防火墙 (WAF)\n部署WAF可以帮助检测和拦截包含潜在恶意代码或命令的请求，为应用提供一层安全防线。\nSOLUTION\n        reference: <<<REFERENCE\nnone\nREFERENCE\n)\n\n/^(eval|exec|assert|system|shell_exec|pcntl_exec|popen|ob_start)$/ as $output\n\nalert $output for {\n        title: \"\",\n        title_zh: \"\",\n        solution: <<<CODE\n\nCODE\n        desc: <<<CODE\n\nCODE\n        level: \"\",\n}\ndesc(\n        lang: php\n        alert_min:8\n        'file://unsafe.php': <<<UNSAFE\n<?php\n    eval($_POST[1]);\n    exec($_POST[1]);\n    assert($_POST[1]);\n    system($_POST[1]);\n    shell_exec($_POST[1]);\n    pcntl_exec($_POST[1]);\n    popen($_POST[1]);\n    ob_start($_POST[1]);\n    ob_end($_POST[1]);\nUNSAFE\n        \"safefile://save.php\": <<<SAFE\n<?php \n    evala($_POST[1]);\nSAFE\n)")
	require.NoError(t, err)
	spew.Dump(rule)
	//rule, err := FormatRule("desc(\n        title: \"Audit PHP OS Command Execution Functions\"\n        type: audit\n        level: info\n        lib: 'php-os-exec'\n        desc: <<<DESC\n### 1.规则目的\n该规则用于审计PHP代码中可能导致远程命令执行（RCE）或代码执行漏洞的危险函数使用。它通过识别直接或间接执行用户输入代码或命令的关键PHP函数，定位未经验证/转义的数据输入点。\n\n### 2.规则详细\n1. **作为基础检测库**\n   属于`php-os-exec`库规则（`lib`类型），需配合其他规则（如用户输入跟踪规则）共同检测命令执行漏洞，提供核心函数识别能力。\n\n2. **覆盖高危执行函数**\n   检测以下执行函数：\n   `eval()`, `exec()`, `assert()`, `system()`, `shell_exec()`, `pcntl_exec()`, `popen()`, `ob_start()`。\n\n   当这些函数接收了未经适当验证或转义的用户输入时，攻击者可以利用此漏洞执行任意代码或命令，进而完全控制服务器或执行恶意操作。因此，建议开发者避免使用这些危险函数，或在使用时对用户输入进行严格的验证和转义。\n\n### 触发场景\n// 存在漏洞的代码示例\n```php\n<?php\n    eval($_POST[1]);\n    exec($_POST[1]);\n    assert($_POST[1]);\n    system($_POST[1]);\n    shell_exec($_POST[1]);\n    pcntl_exec($_POST[1]);\n    popen($_POST[1]);\n    ob_start($_POST[1]);\n    ob_end($_POST[1]);\n?>\n```\n攻击者可以通过POST请求向服务器发送恶意PHP代码或系统命令，例如`?1=system('ls')`来列出服务器文件，或者注入恶意脚本，导致数据泄露、服务器被控等严重后果。\n\n### 潜在影响\n- **远程代码执行 (RCE)**: 攻击者可以直接在服务器上执行任意代码或命令。\n- **数据泄露/篡改**: 攻击者可以通过执行命令访问、修改或删除服务器上的敏感文件。\n- **服务器控制**: 攻击者可以进一步利用漏洞完全控制受影响的服务器，进行恶意活动。\n- **拒绝服务 (DoS)**: 攻击者可以执行消耗大量系统资源的命令，导致服务不可用。\nDESC\n        rule_id: \"4d56af61-28a4-48fd-812c-d28171f4ada7\"\n        title_zh: \"审计PHP命令执行函数\"\n        solution: <<<SOLUTION\n### 修复建议\n当规则命中这些危险函数时，并不能完全确定存在漏洞，这是lib规则的特性。需要结合其他规则来判断是否存在漏洞。但是，为了安全起见，可以采取以下措施来减少潜在风险：\n\n#### 1. 避免使用危险函数\n尽可能避免在生产环境中使用如 `eval()`、`exec()` 等可以直接执行代码或命令的函数。寻找更安全的替代方案。\n\n#### 2. 用户输入严格验证和过滤\n如果确实需要使用这些函数，必须对所有用户输入进行严格的验证、清洗和转义。永远不要直接将用户输入作为参数传递给这些函数。\n\n```php\n<?php\n// 示例：过滤 exec() 函数的输入\n$command = escapeshellcmd($_POST['cmd']); // 对输入进行命令转义\n$output = shell_exec($command); // 使用转义后的命令\n// ... 其他操作\n?>\n```\n\n#### 3. 使用安全的API或库\n优先使用PHP内置的安全API或受信的第三方库来处理文件操作、进程管理等，这些API通常提供了更严格的安全检查和参数处理。\n\n#### 4. 最小权限原则\n运行PHP应用的操作系统用户应遵循最小权限原则，只授予必要的权限，限制执行任意系统命令的能力。\n\n#### 5. Web应用防火墙 (WAF)\n部署WAF可以帮助检测和拦截包含潜在恶意代码或命令的请求，为应用提供一层安全防线。\nSOLUTION\n        reference: <<<REFERENCE\nnone\nREFERENCE\n)\n\n/^(eval|exec|assert|system|shell_exec|pcntl_exec|popen|ob_start)$/ as $output\n\nalert $output for {\n        title: \"\",\n        title_zh: \"\",\n        solution: <<<CODE\n\nCODE\n        desc: <<<CODE\n\nCODE\n        level: \"\",\n}\ndesc(\n        lang: php\n        alert_min:8\n        'file://unsafe.php': <<<UNSAFE\n<?php\n    eval($_POST[1]);\n    exec($_POST[1]);\n    assert($_POST[1]);\n    system($_POST[1]);\n    shell_exec($_POST[1]);\n    pcntl_exec($_POST[1]);\n    popen($_POST[1]);\n    ob_start($_POST[1]);\n    ob_end($_POST[1]);\nUNSAFE\n        \"safefile://save.php\": <<<SAFE\n<?php \n    evala($_POST[1]);\nSAFE\n)")
	//require.NoError(t, err)
	//fmt.Println(rule)
}
