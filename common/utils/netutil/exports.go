package netutil

import "github.com/yaklang/yaklang/common/utils"

// AddIPRouteToNetInterface 添加IP路由到网络接口
// 支持单个IP（string）或多个IP（[]string 或任何可转换的切片类型）
// ipOrIPAddrs: IP地址，支持 string、[]string 或通过 InterfaceToStringSlice 转换的类型
// ifaceName: 网络接口名称
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

// DeleteIPRoute 删除IP路由
// 支持单个IP（string）或多个IP（[]string 或任何可转换的切片类型）
// ipOrIPAddrs: IP地址，支持 string、[]string 或通过 InterfaceToStringSlice 转换的类型
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

// DeleteIPRouteFromNetInterface 删除网络接口的所有/32主机路由
// ifaceName: 网络接口名称
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
