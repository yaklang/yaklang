package mysql

import (
	"github.com/yaklang/yaklang/common/brute/core"
	"github.com/yaklang/yaklang/common/brute/dicts"
)

// defaultPasswords 是 MySQL 探针的默认密码字典（与旧 bruteutils 保持一致）。
var defaultPasswords = append(append([]string{}, dicts.CommonPasswords...), "")

// Register 把 MySQL 最小探针注册进 core 注册表。
// 由兼容层（bruteutils）显式调用，避免 import 副作用，保证精简构建可裁剪。
func Register() {
	core.Register(ServiceInfo())
}

// ServiceInfo 返回用于 core.Register 的服务描述。
func ServiceInfo() core.ServiceInfo {
	return core.ServiceInfo{
		Name:             "mysql",
		DefaultPort:      3306,
		DefaultUsernames: []string{"mysql", "root", "guest", "op", "ops"},
		DefaultPasswords: defaultPasswords,
		Prober:           Prober{},
	}
}
