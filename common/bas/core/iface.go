// Package core
// @Author bcy2007  2023/9/18 10:52
package core

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"os/exec"
	"regexp"
	"strings"

	"github.com/google/gopacket/pcap"
)

const (
	TestIP = "1.2.3.4"
)

func GetIfaceIPAddress() map[string]string {
	result := make(map[string]string)
	interfaces, err := net.Interfaces()
	if err != nil {
		fmt.Printf("get interfaces error: %v", err)
		return result
	}
	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			fmt.Printf("get interface address error: %v", err)
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				fmt.Printf("not ipnet: %v", addr)
				continue
			}
			if ipNet.IP.To4() == nil || ipNet.IP.IsLoopback() {
				continue
			}
			result[iface.Name] = ipNet.IP.String()
		}
	}
	return result
}

func GetIfaceIPAddressInWindows() (map[string]string, error) {
	result := make(map[string]string)
	devs, err := pcap.FindAllDevs()
	if err != nil {
		return result, utils.Errorf("find all devs error: %v", err)
	}
	for _, dev := range devs {
		addresses := dev.Addresses
		for _, addr := range addresses {
			if addr.IP.To4() == nil || addr.IP.IsLoopback() {
				continue
			}
			result[dev.Name] = addr.IP.String()
			break
		}
	}
	return result, err
}

func GetIfaceByName(ifaceName string) string {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		fmt.Printf("get interface %v error: %v", ifaceName, err)
		return ""
	}
	addrs, err := iface.Addrs()
	if err != nil {
		fmt.Printf("get interface address error: %v", err)
		return ""
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			fmt.Printf("not ipnet: %v", addr)
			continue
		}
		if ipNet.IP.To4() == nil || ipNet.IP.IsLoopback() {
			continue
		}
		fmt.Printf("interface: %v ", iface.Name)
		fmt.Printf("ipaddress: %v\n", ipNet.IP.String())
		return ipNet.IP.String()
	}
	return ""
}

func GetInterfaceInDarwin() (string, error) {
	cmd := exec.Command("route", "-n", "get", "default")
	resultBytes := new(bytes.Buffer)
	cmd.Stdout = resultBytes
	err := cmd.Run()
	if err != nil {
		return "", utils.Errorf("cmd running error: %v", err)
	}
	scanner := bufio.NewScanner(resultBytes)
	for scanner.Scan() {
		text := scanner.Text()
		if strings.Contains(text, "interface") && strings.Contains(text, ": ") {
			return strings.Split(text, ": ")[1], nil
		}
	}
	return "", nil
}

func GetInterfaceInLinux() (string, error) {
	cmd := exec.Command("route")
	resultBytes := new(bytes.Buffer)
	cmd.Stdout = resultBytes
	err := cmd.Run()
	if err != nil {
		return "", utils.Errorf("cmd running error: %v", err)
	}
	scanner := bufio.NewScanner(resultBytes)
	for scanner.Scan() {
		text := scanner.Text()
		if strings.Contains(text, "default") {
			reg := regexp.MustCompile(`\s+`)
			result := reg.Split(text, -1)
			return result[len(result)-1], nil
		}
	}
	return "", nil
}

func GetInterfaceInWindows() (string, string, error) {
	ifaces, err := GetIfaceIPAddressInWindows()
	if err != nil {
		return "", "", err
	}
	var ipaddress string
	cmd := exec.Command("route", "print", "0.0.0.0")
	resultBytes := new(bytes.Buffer)
	cmd.Stdout = resultBytes
	err = cmd.Run()
	if err != nil {
		return "", "", utils.Errorf("cmd running error: %v", err)
	}
	scanner := bufio.NewScanner(resultBytes)
	for scanner.Scan() {
		// fmt.Println(scanner.Text())
		text := scanner.Text()
		if strings.Contains(text, "0.0.0.0") {
			reg := regexp.MustCompile(`\s+`)
			result := reg.Split(text, -1)
			ipaddress = result[len(result)-2]
			break
		}
	}
	if ipaddress == "" {
		return "", "", utils.Error("cannot find default route")
	}
	for iface, ip := range ifaces {
		if ip == ipaddress {
			return iface, ipaddress, nil
		}
	}
	return "", "", utils.Errorf("cannot find %v iface", ipaddress)
}
