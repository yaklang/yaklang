//go:build linux

package netutil

import (
	"net"
	"regexp"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils/privileged"

	"github.com/yaklang/yaklang/common/utils"
)

// batchAddSpecificIPRouteToNetInterface 内部批量添加函数
func batchAddSpecificIPRouteToNetInterface(ipList []string, interfaceName string, isSingle bool) (success []string, failed map[string]error) {
	failed = make(map[string]error)

	if len(ipList) == 0 {
		return nil, failed
	}

	// Validate interface name
	if interfaceName == "" {
		for _, ip := range ipList {
			failed[ip] = utils.Errorf("interface name cannot be empty")
		}
		return nil, failed
	}

	interfaceNamePattern := regexp.MustCompile(`^[a-zA-Z0-9_-]{1,15}$`)
	if !interfaceNamePattern.MatchString(interfaceName) {
		err := utils.Errorf("invalid interface name format: %s", interfaceName)
		for _, ip := range ipList {
			failed[ip] = err
		}
		return nil, failed
	}

	// Check if the interface exists
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		err := utils.Errorf("failed to get interface by name: %s", interfaceName)
		for _, ip := range ipList {
			failed[ip] = err
		}
		return nil, failed
	}

	// Ensure the interface is up
	if iface.Flags&net.FlagUp == 0 {
		err := utils.Errorf("interface %s is not up", interfaceName)
		for _, ip := range ipList {
			failed[ip] = err
		}
		return nil, failed
	}

	// Validate IPs and construct commands
	var commands []string
	var validIPs []string
	for _, ipStr := range ipList {
		ipCIDR, err := formatIPStringAsCIDR(ipStr)
		if err != nil {
			failed[ipStr] = err
			continue
		}

		// Check for dangerous characters in IP CIDR
		dangerousChars := []string{";", "&", "|", "$", "`", "(", ")", "<", ">", "\"", "'", "\\", "\n", "\r", "\t", " "}
		hasDangerous := false
		for _, char := range dangerousChars {
			if strings.Contains(ipCIDR, char) {
				failed[ipStr] = utils.Errorf("IP CIDR contains dangerous characters: %s", ipCIDR)
				hasDangerous = true
				break
			}
		}
		if hasDangerous {
			continue
		}

		commands = append(commands, "ip route add "+ipCIDR+" dev "+interfaceName)
		validIPs = append(validIPs, ipStr)
	}

	if len(commands) == 0 {
		return nil, failed
	}

	// Combine commands into a single script
	combinedCmd := strings.Join(commands, " && ")

	// Execute the commands
	exc := privileged.NewExecutor("Route Add to " + interfaceName)
	output, err := exc.Execute(
		utils.TimeoutContextSeconds(120), combinedCmd,
		privileged.WithTitle("Batch Route Addition"),
		privileged.WithDescription("Adding routes to interface "+interfaceName),
	)
	if err != nil {
		spew.Dump(err)
		batchErr := utils.Errorf("(require root/administrator) failed to add routes: %s, output: %s", err, string(output))
		for _, ip := range validIPs {
			failed[ip] = batchErr
		}
		return nil, failed
	}
	spew.Dump(output)

	success = validIPs
	return success, failed
}

// batchDeleteSpecificIPRoute 内部批量删除函数
func batchDeleteSpecificIPRoute(ipList []string, isSingle bool) (success []string, failed map[string]error) {
	failed = make(map[string]error)

	if len(ipList) == 0 {
		return nil, failed
	}

	// Validate IPs and construct commands
	var commands []string
	var validIPs []string
	for _, ipStr := range ipList {
		ipCIDR, err := formatIPStringAsCIDR(ipStr)
		if err != nil {
			failed[ipStr] = err
			continue
		}

		// Check for dangerous characters in IP CIDR
		dangerousChars := []string{";", "&", "|", "$", "`", "(", ")", "<", ">", "\"", "'", "\\", "\n", "\r", "\t", " "}
		hasDangerous := false
		for _, char := range dangerousChars {
			if strings.Contains(ipCIDR, char) {
				failed[ipStr] = utils.Errorf("IP CIDR contains dangerous characters: %s", ipCIDR)
				hasDangerous = true
				break
			}
		}
		if hasDangerous {
			continue
		}

		commands = append(commands, "ip route del "+ipCIDR)
		validIPs = append(validIPs, ipStr)
	}

	if len(commands) == 0 {
		return nil, failed
	}

	// Combine commands into a single script
	combinedCmd := strings.Join(commands, " && ")

	// Execute the commands
	exc := privileged.NewExecutor("Route Delete")
	output, err := exc.Execute(
		utils.TimeoutContextSeconds(120), combinedCmd,
		privileged.WithTitle("Batch Route Deletion"),
		privileged.WithDescription("Deleting routes from routing table"),
	)
	if err != nil {
		spew.Dump(err)
		batchErr := utils.Errorf("(require root/administrator) failed to delete routes: %s, output: %s", err, string(output))
		for _, ip := range validIPs {
			failed[ip] = batchErr
		}
		return nil, failed
	}
	spew.Dump(output)

	success = validIPs
	return success, failed
}

// AddSpecificIPRouteToNetInterface 添加单个IP到特定网络接口的路由
func AddSpecificIPRouteToNetInterface(ipStr string, interfaceName string) error {
	success, failed := batchAddSpecificIPRouteToNetInterface([]string{ipStr}, interfaceName, true)
	if len(failed) > 0 {
		if err, ok := failed[ipStr]; ok {
			return err
		}
	}
	if len(success) == 0 {
		return utils.Errorf("failed to add route for %s", ipStr)
	}
	return nil
}

// DeleteSpecificIPRoute 删除单个IP的路由
func DeleteSpecificIPRoute(ipStr string) error {
	success, failed := batchDeleteSpecificIPRoute([]string{ipStr}, true)
	if len(failed) > 0 {
		if err, ok := failed[ipStr]; ok {
			return err
		}
	}
	if len(success) == 0 {
		return utils.Errorf("failed to delete route for %s", ipStr)
	}
	return nil
}

// BatchAddSpecificIPRouteToNetInterface 批量添加多个IP到特定网络接口的路由
func BatchAddSpecificIPRouteToNetInterface(ipList []string, interfaceName string) (success []string, failed map[string]error) {
	return batchAddSpecificIPRouteToNetInterface(ipList, interfaceName, false)
}

// BatchDeleteSpecificIPRoute 批量删除多个IP的路由
func BatchDeleteSpecificIPRoute(ipList []string) (success []string, failed map[string]error) {
	return batchDeleteSpecificIPRoute(ipList, false)
}

// DeleteAllRoutesForInterface 删除特定网络接口的所有路由
func DeleteAllRoutesForInterface(interfaceName string) (success []string, failed map[string]error, err error) {
	if interfaceName == "" {
		return nil, nil, utils.Errorf("interface name cannot be empty")
	}

	// Validate interface name
	interfaceNamePattern := regexp.MustCompile(`^[a-zA-Z0-9_-]{1,15}$`)
	if !interfaceNamePattern.MatchString(interfaceName) {
		return nil, nil, utils.Errorf("invalid interface name format: %s", interfaceName)
	}

	// Check if the interface exists
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return nil, nil, utils.Errorf("failed to get interface by name: %s", interfaceName)
	}

	// Ensure the interface is up
	if iface.Flags&net.FlagUp == 0 {
		return nil, nil, utils.Errorf("interface %s is not up", interfaceName)
	}

	// Construct the command to delete all routes for the interface
	cmd := "ip route flush dev " + interfaceName

	// Execute the command
	exc := privileged.NewExecutor("Flush Routes for " + interfaceName)
	output, err := exc.Execute(
		utils.TimeoutContextSeconds(120), cmd,
		privileged.WithTitle("Flush Routes"),
		privileged.WithDescription("Deleting all routes for interface "+interfaceName),
	)
	if err != nil {
		spew.Dump(err)
		return nil, nil, utils.Errorf("(require root/administrator) failed to delete all routes for interface %s: %s, output: %s", interfaceName, err, string(output))
	}
	spew.Dump(output)

	return nil, nil, nil
}

func formatIPStringAsCIDR(ipStr string) (string, error) {
	// 检查是否为空
	if ipStr == "" {
		return "", utils.Errorf("IP address cannot be empty")
	}

	// 解析IP地址
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "", utils.Errorf("invalid IP address: %s", ipStr)
	}

	// 不允许回环地址 (127.0.0.0/8)
	if ip.IsLoopback() {
		return "", utils.Errorf("loopback IP address is not allowed: %s", ipStr)
	}

	// 不允许组播地址
	if ip.IsMulticast() {
		return "", utils.Errorf("multicast IP address is not allowed: %s", ipStr)
	}

	// 不允许链路本地地址
	if ip.IsLinkLocalUnicast() {
		return "", utils.Errorf("link-local IP address is not allowed: %s", ipStr)
	}

	// 不允许未指定地址 (0.0.0.0 或 ::)
	if ip.IsUnspecified() {
		return "", utils.Errorf("unspecified IP address is not allowed: %s", ipStr)
	}

	// IPv4 特殊检查
	if ipv4 := ip.To4(); ipv4 != nil {
		// 检查广播地址 (255.255.255.255)
		if ipv4[0] == 255 && ipv4[1] == 255 && ipv4[2] == 255 && ipv4[3] == 255 {
			return "", utils.Errorf("broadcast IP address is not allowed: %s", ipStr)
		}
		// 返回 IPv4 CIDR 格式
		return ipStr + "/32", nil
	} else {
		// 返回 IPv6 CIDR 格式
		return ipStr + "/128", nil
	}
}
