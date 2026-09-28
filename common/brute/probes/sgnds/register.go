package sgnds

import "github.com/yaklang/yaklang/common/brute/core"

// Register 把 SG-JDBC 探针注册进 core 注册表。
// 默认端口 18600，匹配隔离装置外网服务端口。
func Register() {
	core.Register(ServiceInfo())
}

// ServiceInfo 返回用于 core.Register 的服务描述。
func ServiceInfo() core.ServiceInfo {
	return core.ServiceInfo{
		Name:             "sgnds",
		DefaultPort:      18600,
		DefaultUsernames: []string{"admin", "root", "system", "sa"},
		DefaultPasswords: []string{"", "admin", "123456", "password", "system", "oracle"},
		Prober:           Prober{},
	}
}
