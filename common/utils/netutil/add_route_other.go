//go:build !darwin && !windows

package netutil

import "github.com/yaklang/yaklang/common/utils"

// AddSpecificIPRouteToNetInterface 添加单个IP到特定网络接口的路由（仅支持 macOS）
func AddSpecificIPRouteToNetInterface(ipStr string, interfaceName string) error {
	return utils.Errorf("AddSpecificIPRouteToNetInterface is only supported on macOS")
}

// DeleteSpecificIPRoute 删除单个IP的路由（仅支持 macOS）
func DeleteSpecificIPRoute(ipStr string) error {
	return utils.Errorf("DeleteSpecificIPRoute is only supported on macOS")
}

// BatchAddSpecificIPRouteToNetInterface 批量添加多个IP到特定网络接口的路由（仅支持 macOS）
func BatchAddSpecificIPRouteToNetInterface(ipList []string, interfaceName string) (success []string, failed map[string]error) {
	failed = make(map[string]error)
	err := utils.Errorf("BatchAddSpecificIPRouteToNetInterface is only supported on macOS")
	for _, ip := range ipList {
		failed[ip] = err
	}
	return nil, failed
}

// BatchDeleteSpecificIPRoute 批量删除多个IP的路由（仅支持 macOS）
func BatchDeleteSpecificIPRoute(ipList []string) (success []string, failed map[string]error) {
	failed = make(map[string]error)
	err := utils.Errorf("BatchDeleteSpecificIPRoute is only supported on macOS")
	for _, ip := range ipList {
		failed[ip] = err
	}
	return nil, failed
}

// DeleteAllRoutesForInterface 删除特定网络接口的所有/32主机路由（仅支持 macOS）
func DeleteAllRoutesForInterface(interfaceName string) (success []string, failed map[string]error, err error) {
	return nil, make(map[string]error), utils.Errorf("DeleteAllRoutesForInterface is only supported on macOS")
}
