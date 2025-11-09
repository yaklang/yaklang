//go:build darwin

package netutil

import (
	"bytes"
	"net"
	"os/exec"
	"regexp"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils/privileged"

	"github.com/yaklang/yaklang/common/utils"
)

/*
❯ sudo route add -net ip/32 -interface utun9
add net ip: gateway utun9
❯ sudo route delete -net ip/32
delete net ip
*/

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

// batchAddSpecificIPRouteToNetInterface 内部批量添加函数
// isSingle: 是否单个操作（影响提示信息）
func batchAddSpecificIPRouteToNetInterface(ipList []string, interfaceName string, isSingle bool) (success []string, failed map[string]error) {
	failed = make(map[string]error)

	if len(ipList) == 0 {
		return nil, failed
	}

	// 严格验证interfaceName，防止命令注入
	if interfaceName == "" {
		for _, ip := range ipList {
			failed[ip] = utils.Errorf("interface name cannot be empty")
		}
		return nil, failed
	}

	// 接口名称验证
	interfaceNamePattern := regexp.MustCompile(`^[a-zA-Z0-9_-]{1,15}$`)
	if !interfaceNamePattern.MatchString(interfaceName) {
		err := utils.Errorf("invalid interface name format: %s (only alphanumeric, underscore and hyphen allowed, max 15 chars)", interfaceName)
		for _, ip := range ipList {
			failed[ip] = err
		}
		return nil, failed
	}

	// 检查网络接口是否存在且可用
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		err := utils.Errorf("failed to get interface by name: %s", interfaceName)
		for _, ip := range ipList {
			failed[ip] = err
		}
		return nil, failed
	}

	// 检查接口是否UP
	if iface.Flags&net.FlagUp == 0 {
		err := utils.Errorf("interface %s is not up", interfaceName)
		for _, ip := range ipList {
			failed[ip] = err
		}
		return nil, failed
	}

	// 检查接口是否有IPv4地址
	haveIPv4 := false
	if addrs, err := iface.Addrs(); err != nil {
		err := utils.Errorf("failed to get interface addresses: %s", err)
		for _, ip := range ipList {
			failed[ip] = err
		}
		return nil, failed
	} else {
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok {
				if ipNet.IP.To4() != nil {
					haveIPv4 = true
					break
				}
			}
		}
	}
	if !haveIPv4 {
		err := utils.Errorf("net interface %s has no IPv4 address, cannot be routed", interfaceName)
		for _, ip := range ipList {
			failed[ip] = err
		}
		return nil, failed
	}

	// 验证所有IP并构建命令
	var commands []string
	var validIPs []string
	for _, ipStr := range ipList {
		ipCIDR, err := formatIPStringAsCIDR(ipStr)
		if err != nil {
			failed[ipStr] = err
			continue
		}

		// 检查是否包含危险字符
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

		commands = append(commands, "route add -net "+ipCIDR+" -interface "+interfaceName)
		validIPs = append(validIPs, ipStr)
	}

	if len(commands) == 0 {
		return nil, failed
	}

	// 将所有命令合并为一个脚本
	combinedCmd := strings.Join(commands, " && ")

	// 根据是否单个操作设置不同的提示
	var title, description string
	if isSingle {
		title = "Route Hijack"
		description = utils.MustRenderTemplate(
			`YAK want to add route for '{{.ipStr}}' to network interface '{{.interfaceName}}' to hijack traffic.`,
			map[string]interface{}{
				"ipStr":         ipList[0],
				"interfaceName": interfaceName,
			},
		)
	} else {
		title = "Batch Route Addition"
		description = utils.MustRenderTemplate(
			`YAK want to add {{.count}} IP routes to network interface '{{.interfaceName}}' to hijack traffic. 120s timeout automatically.`,
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
		// privileged.WithSkipConfirmDialog(),
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
// isSingle: 是否单个操作（影响提示信息）
func batchDeleteSpecificIPRoute(ipList []string, isSingle bool) (success []string, failed map[string]error) {
	failed = make(map[string]error)

	if len(ipList) == 0 {
		return nil, failed
	}

	// 验证所有IP并构建命令
	var commands []string
	var validIPs []string
	for _, ipStr := range ipList {
		ipCIDR, err := formatIPStringAsCIDR(ipStr)
		if err != nil {
			failed[ipStr] = err
			continue
		}

		commands = append(commands, "route delete -net "+ipCIDR)
		validIPs = append(validIPs, ipStr)
	}

	if len(commands) == 0 {
		return nil, failed
	}

	// 将所有命令合并为一个脚本
	combinedCmd := strings.Join(commands, " && ")

	// 根据是否单个操作设置不同的提示
	var title, description string
	if isSingle {
		title = "Route Deletion"
		description = utils.MustRenderTemplate(
			`YAK want to delete route for '{{.ipStr}}' from routing table.`,
			map[string]interface{}{
				"ipStr": ipList[0],
			},
		)
	} else {
		title = "Batch Route Deletion"
		description = utils.MustRenderTemplate(
			`YAK want to delete {{.count}} IP routes from routing table. 120s timeout automatically.`,
			map[string]interface{}{
				"count": len(validIPs),
			},
		)
	}

	exc := privileged.NewExecutor("Route Delete")
	output, err := exc.Execute(
		utils.TimeoutContextSeconds(120), combinedCmd,
		privileged.WithTitle(title),
		privileged.WithDescription(description),
		// privileged.WithSkipConfirmDialog(),
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
// 底层调用批量操作，但提示信息更简洁
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
// 底层调用批量操作，但提示信息更简洁
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
// 只需要用户输入一次密码，提高效率
func BatchAddSpecificIPRouteToNetInterface(ipList []string, interfaceName string) (success []string, failed map[string]error) {
	return batchAddSpecificIPRouteToNetInterface(ipList, interfaceName, false)
}

// BatchDeleteSpecificIPRoute 批量删除多个IP的路由
// 只需要用户输入一次密码
func BatchDeleteSpecificIPRoute(ipList []string) (success []string, failed map[string]error) {
	return batchDeleteSpecificIPRoute(ipList, false)
}

// DeleteAllRoutesForInterface 删除特定网络接口的所有/32主机路由
// 只删除掩码位为32的特定IP路由（非default、非网段路由）
func DeleteAllRoutesForInterface(interfaceName string) (success []string, failed map[string]error, err error) {
	failed = make(map[string]error)

	// 严格验证interfaceName，防止命令注入
	if interfaceName == "" {
		return nil, failed, utils.Errorf("interface name cannot be empty")
	}

	// 接口名称验证
	interfaceNamePattern := regexp.MustCompile(`^[a-zA-Z0-9_-]{1,15}$`)
	if !interfaceNamePattern.MatchString(interfaceName) {
		return nil, failed, utils.Errorf("invalid interface name format: %s (only alphanumeric, underscore and hyphen allowed, max 15 chars)", interfaceName)
	}

	// 检查网络接口是否存在
	_, err = net.InterfaceByName(interfaceName)
	if err != nil {
		return nil, failed, utils.Errorf("failed to get interface by name: %s", interfaceName)
	}

	// 获取路由表信息
	cmd := exec.Command("netstat", "-rn", "-f", "inet")
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	if err != nil {
		return nil, failed, utils.Errorf("failed to get routing table: %s", err)
	}

	// 解析路由表，提取该接口的所有主机路由（带/32的）
	lines := strings.Split(out.String(), "\n")
	var targetIPs []string

	// 匹配主机路由的正则表达式
	// 格式类似: 123.56.31.221/32   utun9              USc                 utun9
	// 或: 123.56.31.221      utun9              UH                  utun9
	routePattern := regexp.MustCompile(`^(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})(?:/32)?\s+.*\s+` + regexp.QuoteMeta(interfaceName) + `\s*$`)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		matches := routePattern.FindStringSubmatch(line)
		if len(matches) >= 2 {
			ip := matches[1]
			// 跳过特殊IP地址
			if ip == "0.0.0.0" || ip == "127.0.0.1" {
				continue
			}

			// 验证这是一个有效的IP地址
			if parsedIP := net.ParseIP(ip); parsedIP != nil && parsedIP.To4() != nil {
				// 确保不是回环、组播等特殊地址
				if !parsedIP.IsLoopback() && !parsedIP.IsMulticast() &&
					!parsedIP.IsLinkLocalUnicast() && !parsedIP.IsUnspecified() {
					targetIPs = append(targetIPs, ip)
				}
			}
		}
	}

	if len(targetIPs) == 0 {
		return nil, failed, nil // 没有需要删除的路由，不是错误
	}

	// 使用批量删除函数
	success, failed = batchDeleteSpecificIPRoute(targetIPs, false)

	if len(failed) > 0 {
		return success, failed, utils.Errorf("some routes failed to delete")
	}

	return success, failed, nil
}
