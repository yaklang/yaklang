package core

import (
	"sort"
	"sync"
)

// ServiceInfo 描述一个可注册的协议探测服务。
type ServiceInfo struct {
	// Name 协议类型名（注册键，小写）。
	Name string
	// DefaultPort 目标未带端口时使用的默认端口。
	DefaultPort int
	// DefaultUsernames / DefaultPasswords 内置字典。
	DefaultUsernames []string
	DefaultPasswords []string
	// Prober 最小认证探针。
	Prober Prober
	// UnAuthProber 可选：无凭证可达性/未授权访问探测。
	UnAuthProber Prober
	// NoUsername 表示该协议只验密码（用户名字段忽略）。
	NoUsername bool
}

// registry 是分协议注册表，替代全局静态 authFunc 表。
// 协议包通过 Register 自注册；核心包自身不 import 任何协议包。
var registry = struct {
	sync.RWMutex
	services map[string]ServiceInfo
}{services: make(map[string]ServiceInfo)}

// Register 注册（或覆盖）一个协议服务。重复注册以后注册者为准，
// 兼容构建可借此用驱动版实现覆盖默认最小探针。
func Register(info ServiceInfo) {
	if info.Name == "" {
		return
	}
	registry.Lock()
	defer registry.Unlock()
	registry.services[info.Name] = info
}

// Lookup 查询协议服务，第二个返回值表示是否存在。
func Lookup(name string) (ServiceInfo, bool) {
	registry.RLock()
	defer registry.RUnlock()
	info, ok := registry.services[normalizeServiceName(name)]
	return info, ok
}

// AvailableTypes 返回全部已注册协议名（有序）。
func AvailableTypes() []string {
	registry.RLock()
	defer registry.RUnlock()
	names := make([]string, 0, len(registry.services))
	for name := range registry.services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func normalizeServiceName(name string) string {
	out := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out = append(out, c)
	}
	return string(out)
}
