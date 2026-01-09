package yaklib

import (
	"bytes"
	"os"
	"regexp"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

// MaliciousSignature 恶意文件特征
type MaliciousSignature struct {
	Name        string `json:"name"`         // 特征名称
	Category    string `json:"category"`     // 分类（如：php_webshell, privilege_escalation, backdoor等）
	Pattern     string `json:"pattern"`      // 正则表达式模式
	BytePattern []byte `json:"byte_pattern"` // 字节特征
	Description string `json:"description"`  // 描述信息
	Severity    string `json:"severity"`     // 严重程度（low, medium, high, critical）
}

// MaliciousFileMatcher 恶意文件特征匹配器
type MaliciousFileMatcher struct {
	mu         sync.RWMutex
	signatures map[string]*MaliciousSignature // 按名称索引
	patterns   map[string]*regexp.Regexp      // 编译后的正则表达式
	byCategory map[string][]string            // 按分类索引特征名称
}

// NewMaliciousFileMatcher 创建恶意文件特征匹配器
func NewMaliciousFileMatcher() *MaliciousFileMatcher {
	matcher := &MaliciousFileMatcher{
		signatures: make(map[string]*MaliciousSignature),
		patterns:   make(map[string]*regexp.Regexp),
		byCategory: make(map[string][]string),
	}
	// 加载默认特征库
	matcher.loadDefaultSignatures()
	return matcher
}

// AddSignature 添加恶意文件特征
// 支持两种输入类型：
//   - *MaliciousSignature: 结构体指针
//   - map[string]interface{}: yaklang 字典（会自动转换）
func (m *MaliciousFileMatcher) AddSignature(sig interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var signature *MaliciousSignature

	// 处理不同类型的输入
	switch v := sig.(type) {
	case *MaliciousSignature:
		signature = v
	case map[string]string:
		// 从 map 创建 MaliciousSignature
		signature = &MaliciousSignature{}
		if name, ok := v["name"]; ok {
			signature.Name = utils.InterfaceToString(name)
		}
		if category, ok := v["category"]; ok {
			signature.Category = utils.InterfaceToString(category)
		}
		if pattern, ok := v["pattern"]; ok {
			signature.Pattern = utils.InterfaceToString(pattern)
		}
		if desc, ok := v["description"]; ok {
			signature.Description = utils.InterfaceToString(desc)
		}
		if severity, ok := v["severity"]; ok {
			signature.Severity = utils.InterfaceToString(severity)
		}
		if bytePattern, ok := v["byte_pattern"]; ok {
			// 使用 InterfaceToBytesSlice 处理字节数组
			content := utils.InterfaceToBytesSlice(bytePattern)
			if len(content) > 0 {
				signature.BytePattern = content[0]
			}
		}
	default:
		return utils.Errorf("unsupported signature type: %T, expected *MaliciousSignature or map[string]interface{}", sig)
	}

	if signature == nil {
		return utils.Errorf("signature cannot be nil")
	}

	if signature.Name == "" {
		return utils.Errorf("signature name cannot be empty")
	}

	// 编译正则表达式
	if signature.Pattern != "" {
		re, err := regexp.Compile(signature.Pattern)
		if err != nil {
			return utils.Errorf("invalid regex pattern for %s: %v", signature.Name, err)
		}
		m.patterns[signature.Name] = re
	}

	m.signatures[signature.Name] = signature

	// 按分类索引
	if signature.Category != "" {
		m.byCategory[signature.Category] = append(m.byCategory[signature.Category], signature.Name)
	}

	return nil
}

// RemoveSignature 移除特征
func (m *MaliciousFileMatcher) RemoveSignature(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sig, ok := m.signatures[name]
	if !ok {
		return
	}

	delete(m.signatures, name)
	delete(m.patterns, name)

	// 从分类索引中移除
	if sig.Category != "" {
		names := m.byCategory[sig.Category]
		for i, n := range names {
			if n == name {
				m.byCategory[sig.Category] = append(names[:i], names[i+1:]...)
				break
			}
		}
	}
}

// MatchFile 匹配文件内容，返回匹配到的特征名称列表
func (m *MaliciousFileMatcher) MatchFile(filePath string) ([]string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return m.MatchContent(content), nil
}

// MatchContent 匹配内容，返回匹配到的特征名称列表
func (m *MaliciousFileMatcher) MatchContent(content []byte) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	matches := make([]string, 0)
	matched := make(map[string]bool) // 去重

	// 匹配正则表达式
	for name, re := range m.patterns {
		if re.Match(content) {
			if !matched[name] {
				matches = append(matches, name)
				matched[name] = true
			}
		}
	}

	// 匹配字节特征
	for name, sig := range m.signatures {
		if len(sig.BytePattern) > 0 && bytes.Contains(content, sig.BytePattern) {
			if !matched[name] {
				matches = append(matches, name)
				matched[name] = true
			}
		}
	}

	return matches
}

// MatchFileWithDetails 匹配文件并返回详细信息
func (m *MaliciousFileMatcher) MatchFileWithDetails(filePath string) ([]*MaliciousSignature, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return m.MatchContentWithDetails(content), nil
}

// MatchContentWithDetails 匹配内容并返回详细信息
func (m *MaliciousFileMatcher) MatchContentWithDetails(content []byte) []*MaliciousSignature {
	m.mu.RLock()
	defer m.mu.RUnlock()

	matches := make([]*MaliciousSignature, 0)
	matched := make(map[string]bool) // 去重

	// 匹配正则表达式
	for name, re := range m.patterns {
		if re.Match(content) {
			if !matched[name] {
				if sig, ok := m.signatures[name]; ok {
					matches = append(matches, sig)
					matched[name] = true
				}
			}
		}
	}

	// 匹配字节特征
	for name, sig := range m.signatures {
		if len(sig.BytePattern) > 0 && bytes.Contains(content, sig.BytePattern) {
			if !matched[name] {
				matches = append(matches, sig)
				matched[name] = true
			}
		}
	}

	return matches
}

// GetSignaturesByCategory 按分类获取特征
func (m *MaliciousFileMatcher) GetSignaturesByCategory(category string) []*MaliciousSignature {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sigs := make([]*MaliciousSignature, 0)
	if names, ok := m.byCategory[category]; ok {
		for _, name := range names {
			if sig, ok := m.signatures[name]; ok {
				sigs = append(sigs, sig)
			}
		}
	}
	return sigs
}

// GetAllCategories 获取所有分类
func (m *MaliciousFileMatcher) GetAllCategories() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	categories := make([]string, 0, len(m.byCategory))
	for cat := range m.byCategory {
		categories = append(categories, cat)
	}
	return categories
}

// GetSignature 获取指定特征
func (m *MaliciousFileMatcher) GetSignature(name string) *MaliciousSignature {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.signatures[name]
}

// loadDefaultSignatures 加载默认恶意文件特征库
func (m *MaliciousFileMatcher) loadDefaultSignatures() {
	// PHP WebShell 特征
	phpWebShellSignatures := []*MaliciousSignature{
		{
			Name:        "php_webshell_eval",
			Category:    "php_webshell",
			Pattern:     `eval\s*\(`,
			Description: "PHP eval() 函数，常用于代码执行",
			Severity:    "high",
		},
		{
			Name:        "php_webshell_assert",
			Category:    "php_webshell",
			Pattern:     `assert\s*\(`,
			Description: "PHP assert() 函数，可用于代码执行",
			Severity:    "high",
		},
		{
			Name:        "php_webshell_base64_decode",
			Category:    "php_webshell",
			Pattern:     `base64_decode\s*\(`,
			Description: "Base64 解码，常用于代码混淆",
			Severity:    "medium",
		},
		{
			Name:        "php_webshell_system",
			Category:    "php_webshell",
			Pattern:     `system\s*\(`,
			Description: "PHP system() 函数，执行系统命令",
			Severity:    "critical",
		},
		{
			Name:        "php_webshell_exec",
			Category:    "php_webshell",
			Pattern:     `exec\s*\(`,
			Description: "PHP exec() 函数，执行系统命令",
			Severity:    "critical",
		},
		{
			Name:        "php_webshell_shell_exec",
			Category:    "php_webshell",
			Pattern:     `shell_exec\s*\(`,
			Description: "PHP shell_exec() 函数，执行系统命令",
			Severity:    "critical",
		},
		{
			Name:        "php_webshell_passthru",
			Category:    "php_webshell",
			Pattern:     `passthru\s*\(`,
			Description: "PHP passthru() 函数，执行系统命令",
			Severity:    "critical",
		},
		{
			Name:        "php_webshell_proc_open",
			Category:    "php_webshell",
			Pattern:     `proc_open\s*\(`,
			Description: "PHP proc_open() 函数，执行系统命令",
			Severity:    "critical",
		},
		{
			Name:        "php_webshell_preg_replace_eval",
			Category:    "php_webshell",
			Pattern:     `preg_replace\s*\([^)]*['"]\/e['"]`,
			Description: "PHP preg_replace() 的 /e 修饰符，可执行代码",
			Severity:    "high",
		},
		{
			Name:        "php_webshell_file_put_contents_post",
			Category:    "php_webshell",
			Pattern:     `file_put_contents\s*\([^)]*\$_(GET|POST|REQUEST)`,
			Description: "通过 POST/GET 参数写入文件",
			Severity:    "high",
		},
		{
			Name:        "php_webshell_curl_exec",
			Category:    "php_webshell",
			Pattern:     `curl_exec\s*\(`,
			Description: "PHP curl_exec() 函数，可能用于远程通信",
			Severity:    "medium",
		},
		{
			Name:        "php_webshell_fsockopen",
			Category:    "php_webshell",
			Pattern:     `fsockopen\s*\(`,
			Description: "PHP fsockopen() 函数，建立网络连接",
			Severity:    "medium",
		},
		{
			Name:        "php_webshell_file_get_contents_http",
			Category:    "php_webshell",
			Pattern:     `file_get_contents\s*\(\s*['"]https?://`,
			Description: "通过 HTTP/HTTPS 获取远程内容",
			Severity:    "medium",
		},
	}

	// 提权脚本特征
	privilegeEscalationSignatures := []*MaliciousSignature{
		{
			Name:        "privilege_escalation_chmod_777",
			Category:    "privilege_escalation",
			Pattern:     `chmod\s+[0-7]{3,4}\s+`,
			Description: "修改文件权限，可能用于提权",
			Severity:    "high",
		},
		{
			Name:        "privilege_escalation_sudo",
			Category:    "privilege_escalation",
			Pattern:     `sudo\s+`,
			Description: "使用 sudo 提权",
			Severity:    "medium",
		},
		{
			Name:        "privilege_escalation_suid",
			Category:    "privilege_escalation",
			Pattern:     `setuid|setgid`,
			Description: "设置 SUID/SGID 权限",
			Severity:    "high",
		},
		{
			Name:        "privilege_escalation_whoami",
			Category:    "privilege_escalation",
			Pattern:     `whoami|id\s+`,
			Description: "检查当前用户身份",
			Severity:    "low",
		},
		{
			Name:        "privilege_escalation_passwd_access",
			Category:    "privilege_escalation",
			Pattern:     `/etc/(passwd|shadow)`,
			Description: "访问敏感系统文件",
			Severity:    "high",
		},
	}

	// 后门/木马特征
	backdoorSignatures := []*MaliciousSignature{
		{
			Name:        "backdoor_password_hardcoded",
			Category:    "backdoor",
			Pattern:     `password\s*=\s*['"][^'"]{3,}['"]`,
			Description: "硬编码密码",
			Severity:    "high",
		},
		{
			Name:        "backdoor_login_bypass",
			Category:    "backdoor",
			Pattern:     `login.*bypass|bypass.*auth|auth.*bypass`,
			Description: "登录绕过",
			Severity:    "critical",
		},
		{
			Name:        "backdoor_hidden_eval",
			Category:    "backdoor",
			Pattern:     `@eval|@assert|@system`,
			Description: "隐藏的错误抑制符 + 危险函数",
			Severity:    "high",
		},
		{
			Name:        "backdoor_base64_encoded",
			Category:    "backdoor",
			Pattern:     `base64_decode\s*\(\s*['"][A-Za-z0-9+/=]{20,}['"]`,
			Description: "Base64 编码的恶意代码",
			Severity:    "medium",
		},
	}

	// Python 恶意脚本特征
	pythonMaliciousSignatures := []*MaliciousSignature{
		{
			Name:        "python_os_system",
			Category:    "python_malicious",
			Pattern:     `os\.system\s*\(`,
			Description: "Python os.system() 执行系统命令",
			Severity:    "critical",
		},
		{
			Name:        "python_subprocess",
			Category:    "python_malicious",
			Pattern:     `subprocess\.(call|Popen|run)\s*\(`,
			Description: "Python subprocess 执行系统命令",
			Severity:    "critical",
		},
		{
			Name:        "python_eval_exec",
			Category:    "python_malicious",
			Pattern:     `(eval|exec)\s*\(`,
			Description: "Python eval/exec 执行代码",
			Severity:    "high",
		},
	}

	// Bash/Shell 恶意脚本特征
	shellMaliciousSignatures := []*MaliciousSignature{
		{
			Name:        "shell_bash_reverse",
			Category:    "shell_malicious",
			Pattern:     `bash\s+-i\s+>&|/dev/tcp/`,
			Description: "Bash 反向 Shell",
			Severity:    "critical",
		},
		{
			Name:        "shell_nc_reverse",
			Category:    "shell_malicious",
			Pattern:     `nc\s+.*-e\s+.*bash|nc\s+.*-e\s+.*sh`,
			Description: "Netcat 反向 Shell",
			Severity:    "critical",
		},
		{
			Name:        "shell_wget_pipe",
			Category:    "shell_malicious",
			Pattern:     `wget\s+.*\|\s*sh|curl\s+.*\|\s*sh`,
			Description: "下载并执行脚本",
			Severity:    "high",
		},
	}

	// 合并所有特征
	allSignatures := make([]*MaliciousSignature, 0)
	allSignatures = append(allSignatures, phpWebShellSignatures...)
	allSignatures = append(allSignatures, privilegeEscalationSignatures...)
	allSignatures = append(allSignatures, backdoorSignatures...)
	allSignatures = append(allSignatures, pythonMaliciousSignatures...)
	allSignatures = append(allSignatures, shellMaliciousSignatures...)

	// 添加特征到匹配器
	for _, sig := range allSignatures {
		_ = m.AddSignature(sig)
	}
}
