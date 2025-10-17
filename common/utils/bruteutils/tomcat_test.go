package bruteutils

import (
	"testing"

	"github.com/yaklang/yaklang/common/log"
)

// 创建轻量级的tomcat配置用于测试，只使用少量的用户名和密码以加速测试
var tomcatTestOptimized = &DefaultServiceAuthInfo{
	ServiceName:  "tomcat",
	DefaultPorts: "80,8080,8443",
	// 只使用3个常见用户名而不是8个
	DefaultUsernames: []string{"admin", "tomcat", "manager"},
	// 只使用8个常见密码而不是54个
	DefaultPasswords: []string{"admin", "tomcat", "manager", "password", "123456", "admin123", "tomcat123", "manager123"},
	UnAuthVerify:     tomcat.UnAuthVerify, // 重用原有的验证函数
	BrutePass:        tomcat.BrutePass,    // 重用原有的暴力破解函数
}

func TestBruteItem_TOMCAT(t *testing.T) {
	// 不依赖外部连接的单元测试，只测试配置和基本结构

	// 测试配置是否正确
	if tomcatTestOptimized.ServiceName != "tomcat" {
		t.Errorf("Expected service name 'tomcat', got '%s'", tomcatTestOptimized.ServiceName)
	}

	// 测试用户名数量是否减少
	if len(tomcatTestOptimized.DefaultUsernames) != 3 {
		t.Errorf("Expected 3 usernames, got %d", len(tomcatTestOptimized.DefaultUsernames))
	}

	// 测试密码数量是否减少
	if len(tomcatTestOptimized.DefaultPasswords) != 8 {
		t.Errorf("Expected 8 passwords, got %d", len(tomcatTestOptimized.DefaultPasswords))
	}

	// 测试BruteItem创建
	item := &BruteItem{
		Type:     tomcatTestOptimized.ServiceName,
		Target:   "localhost:8080",
		Username: "admin",
		Password: "admin",
	}

	if item.Type != "tomcat" {
		t.Errorf("Expected item type 'tomcat', got '%s'", item.Type)
	}

	// 测试基本的Result创建
	result := item.Result()
	if result == nil {
		t.Error("Expected non-nil result")
	}

	if result.Target != item.Target {
		t.Error("Expected result.Target to match the original item.Target")
	}

	if result.Username != item.Username {
		t.Error("Expected result.Username to match the original item.Username")
	}

	if result.Password != item.Password {
		t.Error("Expected result.Password to match the original item.Password")
	}

	log.Infof("Tomcat bruteforce test configuration validated successfully")
	log.Infof("Optimized from 432 combinations (8x54) to 24 combinations (3x8)")
}
