//go:build windows

package netutil

import (
	"bytes"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/privileged"
)

/*
Windows 'route' command reference:
> route ADD destination_network MASK netmask gateway_ip METRIC metric IF interface_index

- To add a host route for a specific IP through an interface, we need:
  1. The destination IP (e.g., 8.8.8.8)
  2. The netmask, which is 255.255.255.255 for a /32 host route.
  3. A gateway IP. For routes bound to an interface (on-link), we can often use the interface's own IP.
  4. The interface index (a number), not the name.

- To delete a route:
> route DELETE destination_network

This is simpler and only requires the destination IP.
*/

// formatIPStringAsCIDR is platform-agnostic and can be reused directly.
// It validates an IP and returns it in CIDR format (/32 for IPv4, /128 for IPv6)
func formatIPStringAsCIDR(ipStr string) (string, error) {
	if ipStr == "" {
		return "", utils.Errorf("IP address cannot be empty")
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return "", utils.Errorf("invalid IP address: %s", ipStr)
	}
	if ip.IsLoopback() {
		return "", utils.Errorf("loopback IP address is not allowed: %s", ipStr)
	}
	if ip.IsMulticast() {
		return "", utils.Errorf("multicast IP address is not allowed: %s", ipStr)
	}
	if ip.IsLinkLocalUnicast() {
		return "", utils.Errorf("link-local IP address is not allowed: %s", ipStr)
	}
	if ip.IsUnspecified() {
		return "", utils.Errorf("unspecified IP address is not allowed: %s", ipStr)
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		if ipv4[0] == 255 && ipv4[1] == 255 && ipv4[2] == 255 && ipv4[3] == 255 {
			return "", utils.Errorf("broadcast IP address is not allowed: %s", ipStr)
		}
		return ipStr + "/32", nil
	} else {
		return ipStr + "/128", nil
	}
}

// batchAddSpecificIPRouteToNetInterface 内部批量添加函数 (Windows implementation)
// isSingle: 是否单个操作（影响提示信息）
func batchAddSpecificIPRouteToNetInterface(ipList []string, interfaceName string, isSingle bool) (success []string, failed map[string]error) {
	failed = make(map[string]error)

	if len(ipList) == 0 {
		return nil, failed
	}

	if interfaceName == "" {
		for _, ip := range ipList {
			failed[ip] = utils.Errorf("interface name cannot be empty")
		}
		return nil, failed
	}

	// On Windows, interface names can contain spaces and special characters.
	// We rely on net.InterfaceByName for validation, so a strict regex is less necessary.
	// But it's good practice to prevent obvious injection.
	if strings.ContainsAny(interfaceName, `"&|<>^`) {
		err := utils.Errorf("invalid interface name: contains prohibited characters")
		for _, ip := range ipList {
			failed[ip] = err
		}
		return nil, failed
	}

	// 检查网络接口是否存在且可用
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		err := utils.Errorf("failed to get interface by name '%s': %v", interfaceName, err)
		for _, ip := range ipList {
			failed[ip] = err
		}
		return nil, failed
	}

	if iface.Flags&net.FlagUp == 0 {
		err := utils.Errorf("interface %s is not up", interfaceName)
		for _, ip := range ipList {
			failed[ip] = err
		}
		return nil, failed
	}

	// Windows `route` command requires a gateway IP and an interface index.
	// We will use the interface's own IPv4 address as the gateway.
	var gatewayIP net.IP
	if addrs, err := iface.Addrs(); err != nil {
		err := utils.Errorf("failed to get interface addresses: %v", err)
		for _, ip := range ipList {
			failed[ip] = err
		}
		return nil, failed
	} else {
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
				if ipNet.IP.To4() != nil {
					gatewayIP = ipNet.IP
					break
				}
			}
		}
	}
	if gatewayIP == nil {
		err := utils.Errorf("net interface %s has no IPv4 address, cannot be used for routing", interfaceName)
		for _, ip := range ipList {
			failed[ip] = err
		}
		return nil, failed
	}

	// Get interface index
	interfaceIndex := iface.Index

	// 验证所有IP并构建命令
	var commands []string
	var validIPs []string
	for _, ipStr := range ipList {
		// Use ParseIP directly as we only need the IP string for the command,
		// and formatIPStringAsCIDR already does the validation.
		ip := net.ParseIP(ipStr)
		if ip == nil || ip.To4() == nil {
			failed[ipStr] = utils.Errorf("invalid or non-IPv4 address: %s", ipStr)
			continue
		}

		// Ensure no bad characters in the IP string itself (belt and suspenders)
		if strings.ContainsAny(ipStr, `&|;<>^"`) {
			failed[ipStr] = utils.Errorf("IP contains dangerous characters: %s", ipStr)
			continue
		}

		// Build the Windows-specific route command.
		// `route ADD [destination] MASK [netmask] [gateway] IF [interface_index]`
		cmd := fmt.Sprintf(
			"route ADD %s MASK 255.255.255.255 %s IF %d",
			ipStr, gatewayIP.String(), interfaceIndex,
		)
		commands = append(commands, cmd)
		validIPs = append(validIPs, ipStr)
	}

	if len(commands) == 0 {
		return nil, failed
	}

	// On Windows, `&&` is the command separator.
	combinedCmd := strings.Join(commands, " && ")

	var title, description string
	if isSingle {
		title = "Route Hijack"
		description = utils.MustRenderTemplate(
			`YAK wants to add a route for '{{.ipStr}}' via network interface '{{.interfaceName}}' to hijack traffic. This requires administrator privileges.`,
			map[string]interface{}{
				"ipStr":         ipList[0],
				"interfaceName": interfaceName,
			},
		)
	} else {
		title = "Batch Route Addition"
		description = utils.MustRenderTemplate(
			`YAK wants to add {{.count}} IP routes via network interface '{{.interfaceName}}' to hijack traffic. This requires administrator privileges. A 120s timeout is applied.`,
			map[string]interface{}{
				"count":         len(validIPs),
				"interfaceName": interfaceName,
			},
		)
	}

	exc := privileged.NewExecutor("Route Add to " + interfaceName)
	output, err := exc.Execute(
		utils.TimeoutContextSeconds(120), combinedCmd,
		privileged.WithTitle(title),
		privileged.WithDescription(description),
	)
	if err != nil {
		spew.Dump(err)
		// Check for a common error: "The route addition failed: The object already exists."
		// If so, we might not want to fail the entire batch.
		// For simplicity here, we fail the whole batch as the original code does.
		batchErr := utils.Errorf("(require administrator) failed to add routes: %s, output: %s", err, string(output))
		for _, ip := range validIPs {
			failed[ip] = batchErr
		}
		return nil, failed
	}
	spew.Dump(output)

	success = validIPs
	return success, failed
}

// batchDeleteSpecificIPRoute 内部批量删除函数 (Windows implementation)
// isSingle: 是否单个操作（影响提示信息）
func batchDeleteSpecificIPRoute(ipList []string, isSingle bool) (success []string, failed map[string]error) {
	failed = make(map[string]error)

	if len(ipList) == 0 {
		return nil, failed
	}

	var commands []string
	var validIPs []string
	for _, ipStr := range ipList {
		// We only need to validate it's a proper IP string.
		ip := net.ParseIP(ipStr)
		if ip == nil {
			failed[ipStr] = utils.Errorf("invalid IP address: %s", ipStr)
			continue
		}
		// Windows 'route delete' is simpler, only needs the destination.
		commands = append(commands, "route delete "+ipStr)
		validIPs = append(validIPs, ipStr)
	}

	if len(commands) == 0 {
		return nil, failed
	}

	combinedCmd := strings.Join(commands, " && ")

	var title, description string
	if isSingle {
		title = "Route Deletion"
		description = utils.MustRenderTemplate(
			`YAK wants to delete the route for '{{.ipStr}}' from the routing table. This requires administrator privileges.`,
			map[string]interface{}{"ipStr": ipList[0]},
		)
	} else {
		title = "Batch Route Deletion"
		description = utils.MustRenderTemplate(
			`YAK wants to delete {{.count}} IP routes from the routing table. This requires administrator privileges. A 120s timeout is applied.`,
			map[string]interface{}{"count": len(validIPs)},
		)
	}

	exc := privileged.NewExecutor("Route Delete")
	output, err := exc.Execute(
		utils.TimeoutContextSeconds(120), combinedCmd,
		privileged.WithTitle(title),
		privileged.WithDescription(description),
	)
	if err != nil {
		spew.Dump(err)
		batchErr := utils.Errorf("(require administrator) failed to delete routes: %s, output: %s", err, string(output))
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

// DeleteAllRoutesForInterface 删除特定网络接口的所有/32主机路由 (Windows implementation)
func DeleteAllRoutesForInterface(interfaceName string) (success []string, failed map[string]error, err error) {
	failed = make(map[string]error)

	if interfaceName == "" {
		return nil, failed, utils.Errorf("interface name cannot be empty")
	}

	if strings.ContainsAny(interfaceName, `&|;<>^"`) {
		return nil, failed, utils.Errorf("invalid interface name: contains prohibited characters")
	}

	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return nil, failed, utils.Errorf("failed to get interface by name '%s': %v", interfaceName, err)
	}

	// On Windows, 'route print' identifies interfaces by their IP address.
	// We need to get all IPv4 addresses for the target interface.
	var interfaceIPs []string
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, failed, utils.Errorf("could not get addresses for interface %s: %v", interfaceName, err)
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.To4() != nil {
			interfaceIPs = append(interfaceIPs, ipNet.IP.String())
		}
	}
	if len(interfaceIPs) == 0 {
		// No IPv4 addresses, so no IPv4 routes can be associated with it. Not an error.
		return nil, failed, nil
	}

	interfaceIpSet := make(map[string]struct{})
	for _, ip := range interfaceIPs {
		interfaceIpSet[ip] = struct{}{}
	}

	// Use `route print` to get routing table. -4 specifies IPv4 routes.
	cmd := exec.Command("route", "print", "-4")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, failed, utils.Errorf("failed to get routing table: %v", err)
	}

	lines := strings.Split(out.String(), "\n")
	var targetIPs []string

	// Regex to parse a line from the 'route print' output.
	// Example line: `1.1.1.1   255.255.255.255   10.0.0.1   10.0.0.100   20`
	// Groups:      (Dest IP)   (Netmask)        (Gateway)  (Interface IP)
	routePattern := regexp.MustCompile(`^\s*(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\s+(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\s+.*\s+(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})\s+\d+.*$`)

	for _, line := range lines {
		matches := routePattern.FindStringSubmatch(strings.TrimSpace(line))
		if len(matches) < 4 {
			continue
		}

		destIP := matches[1]
		netmask := matches[2]
		routeInterfaceIP := matches[3]

		// We are looking for host routes (/32), which have a netmask of 255.255.255.255
		if netmask != "255.255.255.255" {
			continue
		}

		// Check if the route's interface IP matches one of our target interface's IPs
		if _, ok := interfaceIpSet[routeInterfaceIP]; ok {
			// Validate it's a valid, non-special IP before adding
			if parsedIP := net.ParseIP(destIP); parsedIP != nil {
				if !parsedIP.IsLoopback() && !parsedIP.IsMulticast() &&
					!parsedIP.IsLinkLocalUnicast() && !parsedIP.IsUnspecified() &&
					parsedIP.To4() != nil {
					targetIPs = append(targetIPs, destIP)
				}
			}
		}
	}

	if len(targetIPs) == 0 {
		return nil, failed, nil // No matching routes to delete.
	}

	// Use the batch deletion function.
	success, failed = batchDeleteSpecificIPRoute(targetIPs, false)
	if len(failed) > 0 {
		return success, failed, utils.Errorf("some routes failed to delete")
	}

	return success, failed, nil
}
