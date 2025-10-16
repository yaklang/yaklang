package main

import (
	"fmt"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/schema"
)

// getDatabase 获取数据库连接的辅助函数
func getDatabase(c *cli.Context) (*gorm.DB, error) {
	dbPath := c.String("db")
	db, err := gorm.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %v", err)
	}

	// 自动迁移
	db.AutoMigrate(&schema.VectorStoreDocument{}, &schema.VectorStoreCollection{}, &schema.YakScript{})
	return db, nil
}

// createTestScripts 创建测试脚本数据
func createTestScripts(db *gorm.DB) []*schema.YakScript {
	testScripts := []*schema.YakScript{
		{
			ScriptName: "sql_injection_detector",
			Type:       "poc",
			Author:     "security_team",
			Help:       "检测SQL注入漏洞的POC脚本，支持多种注入类型检测",
			Content:    "// SQL注入检测逻辑\nfunc detectSQLInjection() { ... }",
			Tags:       "sql,injection,database,security",
			Level:      "high",
		},
		{
			ScriptName: "xss_scanner",
			Type:       "poc",
			Author:     "web_security",
			Help:       "跨站脚本攻击(XSS)检测工具，支持反射型和存储型XSS检测",
			Content:    "// XSS检测逻辑\nfunc scanXSS() { ... }",
			Tags:       "xss,web,javascript,security",
			Level:      "medium",
		},
		{
			ScriptName: "port_scanner",
			Type:       "nuclei",
			Author:     "network_team",
			Help:       "高效的端口扫描工具，支持TCP和UDP端口扫描",
			Content:    "// 端口扫描逻辑\nfunc scanPorts() { ... }",
			Tags:       "port,scan,network,tcp,udp",
			Level:      "low",
		},
		{
			ScriptName: "file_upload_bypass",
			Type:       "poc",
			Author:     "pentest_team",
			Help:       "文件上传绕过检测脚本，测试各种文件上传限制绕过方法",
			Content:    "// 文件上传绕过逻辑\nfunc bypassFileUpload() { ... }",
			Tags:       "upload,bypass,file,web",
			Level:      "high",
		},
		{
			ScriptName: "directory_traversal",
			Type:       "poc",
			Author:     "security_team",
			Help:       "目录遍历漏洞检测工具，检测路径穿越攻击漏洞",
			Content:    "// 目录遍历检测逻辑\nfunc detectDirectoryTraversal() { ... }",
			Tags:       "directory,traversal,path,security",
			Level:      "medium",
		},
		{
			ScriptName: "web_shell_detector",
			Type:       "poc",
			Author:     "malware_team",
			Help:       "Web Shell检测工具，识别各种类型的Web后门",
			Content:    "// Web Shell检测逻辑\nfunc detectWebShell() { ... }",
			Tags:       "webshell,malware,backdoor,security",
			Level:      "high",
		},
		{
			ScriptName: "csrf_scanner",
			Type:       "poc",
			Author:     "web_security",
			Help:       "跨站请求伪造(CSRF)漏洞检测工具",
			Content:    "// CSRF检测逻辑\nfunc scanCSRF() { ... }",
			Tags:       "csrf,web,security,token",
			Level:      "medium",
		},
		{
			ScriptName: "weak_password_checker",
			Type:       "brute",
			Author:     "auth_team",
			Help:       "弱密码检测工具，支持多种服务的密码爆破",
			Content:    "// 弱密码检测逻辑\nfunc checkWeakPassword() { ... }",
			Tags:       "password,brute,auth,security",
			Level:      "low",
		},
	}

	// 保存到数据库
	for _, script := range testScripts {
		db.Save(script)
	}

	return testScripts
}

// truncateString 辅助函数：截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
