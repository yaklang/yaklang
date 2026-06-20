package netutil

import "github.com/yaklang/yaklang/common/utils"

// AddIPRouteToNetInterface 添加 IP 主机路由到指定网络接口（导出名为 netutils.AddIPRouteToNetInterface）
// 需要管理员/root 权限；支持单个 IP(string) 或多个 IP([]string 等可转换切片)，会为每个 IP 添加 /32 主机路由
//
// 参数:
//   - ipOrIPAddrs: IP 地址，支持 string、[]string 或可被 InterfaceToStringSlice 转换的类型
//   - ifaceName: 目标网络接口名称（如 "en0"、"eth0"）
//
// 返回值:
//   - 错误信息（权限不足、接口不存在或添加失败时返回）
//
// Example:
// ```
// // 真实功能示例：把若干 IP 的流量定向到指定网卡（需要 root 权限，示意性用法）
// netutils.AddIPRouteToNetInterface(["1.1.1.1", "8.8.8.8"], "en0")~
// ```
func AddIPRouteToNetInterface(ipOrIPAddrs any, ifaceName string) error {
	// 类型断言处理
	switch v := ipOrIPAddrs.(type) {
	case string:
		// 单个IP地址
		return AddSpecificIPRouteToNetInterface(v, ifaceName)
	case []string:
		// 字符串切片
		if len(v) == 0 {
			return utils.Errorf("IP list cannot be empty")
		}
		success, failed := BatchAddSpecificIPRouteToNetInterface(v, ifaceName)
		if len(failed) > 0 {
			// 返回第一个错误
			for ip, err := range failed {
				return utils.Errorf("failed to add route for %s: %v (succeeded: %d, failed: %d)", ip, err, len(success), len(failed))
			}
		}
		return nil
	default:
		// 尝试使用 InterfaceToStringSlice 转换
		ips := utils.InterfaceToStringSlice(ipOrIPAddrs)
		if len(ips) == 0 {
			return utils.Errorf("invalid IP address type or empty list")
		}
		success, failed := BatchAddSpecificIPRouteToNetInterface(ips, ifaceName)
		if len(failed) > 0 {
			// 返回第一个错误
			for ip, err := range failed {
				return utils.Errorf("failed to add route for %s: %v (succeeded: %d, failed: %d)", ip, err, len(success), len(failed))
			}
		}
		return nil
	}
}

// DeleteIPRoute 删除此前添加的 IP 主机路由（导出名为 netutils.DeleteIPRoute）
// 需要管理员/root 权限；支持单个 IP(string) 或多个 IP([]string 等可转换切片)
//
// 参数:
//   - ipOrIPAddrs: IP 地址，支持 string、[]string 或可被 InterfaceToStringSlice 转换的类型
//
// 返回值:
//   - 错误信息（权限不足或删除失败时返回）
//
// Example:
// ```
// // 真实功能示例：删除之前添加的主机路由（需要 root 权限，示意性用法）
// netutils.DeleteIPRoute(["1.1.1.1", "8.8.8.8"])~
// ```
func DeleteIPRoute(ipOrIPAddrs any) error {
	// 类型断言处理
	switch v := ipOrIPAddrs.(type) {
	case string:
		// 单个IP地址
		return DeleteSpecificIPRoute(v)
	case []string:
		// 字符串切片
		if len(v) == 0 {
			return utils.Errorf("IP list cannot be empty")
		}
		success, failed := BatchDeleteSpecificIPRoute(v)
		if len(failed) > 0 {
			// 返回第一个错误
			for ip, err := range failed {
				return utils.Errorf("failed to delete route for %s: %v (succeeded: %d, failed: %d)", ip, err, len(success), len(failed))
			}
		}
		return nil
	default:
		// 尝试使用 InterfaceToStringSlice 转换
		ips := utils.InterfaceToStringSlice(ipOrIPAddrs)
		if len(ips) == 0 {
			return utils.Errorf("invalid IP address type or empty list")
		}
		success, failed := BatchDeleteSpecificIPRoute(ips)
		if len(failed) > 0 {
			// 返回第一个错误
			for ip, err := range failed {
				return utils.Errorf("failed to delete route for %s: %v (succeeded: %d, failed: %d)", ip, err, len(success), len(failed))
			}
		}
		return nil
	}
}

// DeleteIPRouteFromNetInterface 删除指定网络接口上的所有 /32 主机路由（导出名为 netutils.DeleteIPRouteFromNetInterface）
// 需要管理员/root 权限；常用于清理之前通过 AddIPRouteToNetInterface 添加到该接口的所有路由
//
// 参数:
//   - ifaceName: 网络接口名称（如 "en0"、"eth0"）
//
// 返回值:
//   - 错误信息（权限不足、接口不存在或删除失败时返回）
//
// Example:
// ```
// // 真实功能示例：清理某网卡上所有由本程序添加的主机路由（需要 root 权限，示意性用法）
// netutils.DeleteIPRouteFromNetInterface("en0")~
// ```
func DeleteIPRouteFromNetInterface(ifaceName string) error {
	success, failed, err := DeleteAllRoutesForInterface(ifaceName)
	if err != nil {
		return err
	}
	if len(failed) > 0 {
		// 返回第一个错误
		for ip, e := range failed {
			return utils.Errorf("failed to delete route for %s: %v (succeeded: %d, failed: %d)", ip, e, len(success), len(failed))
		}
	}
	return nil
}

var Exports = map[string]any{
	// 用户友好的高级接口（推荐使用）
	"AddIPRouteToNetInterface":      AddIPRouteToNetInterface,
	"DeleteIPRoute":                 DeleteIPRoute,
	"DeleteIPRouteFromNetInterface": DeleteIPRouteFromNetInterface,

	// 底层接口（高级用户使用）
	"AddSpecificIPRouteToNetInterface":      AddSpecificIPRouteToNetInterface,
	"DeleteSpecificIPRoute":                 DeleteSpecificIPRoute,
	"BatchAddSpecificIPRouteToNetInterface": BatchAddSpecificIPRouteToNetInterface,
	"BatchDeleteSpecificIPRoute":            BatchDeleteSpecificIPRoute,
	"DeleteAllRoutesForInterface":           DeleteAllRoutesForInterface,
}

var (
	Action_Add    = "add"
	Action_Delete = "delete"
)

type RouteModifyResult struct {
	SuccessList []string          `json:"success_list"`
	FailMap     map[string]string `json:"fail_map"`
	Error       string            `json:"error,omitempty"`
}

type RouteModifyMessage struct {
	Action  string   `json:"action"` // "add" 或 "delete"
	IpList  []string `json:"ip_list"`
	TunName string   `json:"tun_name"`
}

func (r *RouteModifyMessage) IsAdd() bool {
	return r.Action == "add"
}

func (r *RouteModifyMessage) IsDelete() bool {
	return r.Action == "delete"
}
