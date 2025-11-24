package health

import (
	"bytes"
	"encoding/json"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/yaklang/yaklang/common/log"
	"net"
	"os/exec"
	"os/user"
	"runtime"
	"strings"
)

func GetSystemInfo() (*host.InfoStat, error) {
	var (
		stat *host.InfoStat
		err  error
	)
	stat, err = host.Info()
	return stat, err
}

func GetSystemUsers() ([]*host.UserStat, error) {
	var users []*host.UserStat

	userStats, err := host.Users()
	if err != nil {
		unameRaw, _ := exec.Command("uname", "-r").CombinedOutput()
		if bytes.Contains(unameRaw, []byte("microsoft")) {
			return []*host.UserStat{}, nil
		}
		return nil, errors.Errorf("get user info failed(gopsutil): %v", err)
	}

	for _, u := range userStats {
		users = append(users, &u)
	}

	usr, err := user.Current()
	if err != nil {
		return nil, errors.Errorf("fetch current user failed: %s", err)
	}

	users = append(users, &host.UserStat{
		User: usr.Username,
	})

	if len(users) <= 0 {
		return nil, errors.New("no users info")
	}

	return users, nil
}

type SystemMatrix struct {
	NodeId            string           `json:"node_id"`
	GOARCH            string           `json:"arch"`
	GOOS              string           `json:"os"`
	Version           string           `json:"version"`
	Users             []*host.UserStat `json:"users"`
	ExternalNetwork   string           `json:"external_network"`
	Network           *NetworkInfo     `json:"network"`
	Matrix            *host.InfoStat   `json:"matrix"`
	HealthInfos       []*HealthInfo    `json:"health_infos"`
	NodeAliveDuration uint64           `json:"node_alive_duration"`
}

func (p *SystemMatrix) String() (str string) {
	str = ""
	jsonByte, err := json.MarshalIndent(*p, "", "    ")
	if err != nil {
		return
	}
	str = string(jsonByte)
	return
}

// 获取主User
func (p *SystemMatrix) GetMainUser() (mainUser string) {
	if p.Users == nil {
		return
	}
	if len(p.Users) == 0 {
		return
	}
	mainUser = p.Users[0].User
	return
}

// 所有用户tostring
func (p *SystemMatrix) UserToString() (allUser string) {
	jsonByte, err := json.MarshalIndent(&p.Users, "", "    ")
	if err != nil {
		return
	}
	allUser = string(jsonByte)
	return
}

// 获取主mac
func (p *SystemMatrix) GetMainMacAndMainNetAddr() (mainMac, mainNetAddr string) {
	if p.Network == nil {
		return
	}

	if len(p.Network.Interfaces) == 0 {
		return
	}
	//first not lookback addr
	for _, v := range p.Network.Interfaces {

		if len(v.Addrs) == 0 {
			continue
		}

		for _, addr := range v.Addrs {
			// get first ipv4 addr
			if addr.Address != "127.0.0.1" && strings.Contains(addr.Address, ":") == false {
				mainMac = v.HardwareAddr
				mainNetAddr = addr.Address
				return
			}
		}
	}
	return
}

func NewSystemMatrixBase() (*SystemMatrix, error) {
	matrix, err := GetSystemInfo()
	if err != nil {
		return nil, err
	}

	users, err := GetSystemUsers()
	if err != nil {
		log.Debugf("get system users info failed: %v", err)
	}

	networkInfo, err := GetNetworkInfo()
	externalNetworkInfo, _ := GetExternalIp()
	if err != nil {
		log.Warnf("get network info failed: %v", err)
	}

	return &SystemMatrix{
		NodeId:          "",
		GOARCH:          runtime.GOARCH,
		GOOS:            runtime.GOOS,
		Version:         "dev",
		ExternalNetwork: externalNetworkInfo.Addr,
		Network:         networkInfo,
		Users:           users,
		Matrix:          matrix,
	}, nil

}

type HealthInfo struct {
	Timestamp     int64   `json:"timestamp"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryPercent float64 `json:"memory_percent"`
	// kb/sec
	NetworkUpload   float64 `json:"network_upload"`
	NetworkDownload float64 `json:"network_download"`
	// kb/sec
	DiskWrite float64   `json:"disk_write"`
	DiskRead  float64   `json:"disk_read"`
	DiskUsage *DiskStat `json:"disk_usage"`
}

type DiskStat struct {
	Total uint64 `json:"total,omitempty"`
	Used  uint64 `json:"used,omitempty"`
}

type NetworkInfo struct {
	Interfaces []*NetInterfaceInfo `json:"interfaces"`
}

type ExternalNetworkInfo struct {
	Addr string `json:"ex_addr"`
}

func GetExternalIp() (ExternalNetworkInfo, error) {
	var ifs = ExternalNetworkInfo{}
	ifs.Addr = "error"
	return ifs, nil
}

func GetNetworkInfo() (*NetworkInfo, error) {
	var ifs []*NetInterfaceInfo

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, errors.Errorf("fetch interface failed: %v", err)
	}
	for _, iface := range ifaces {
		info := &NetInterfaceInfo{
			Index:        iface.Index,
			MTU:          iface.MTU,
			Name:         iface.Name,
			HardwareAddr: iface.HardwareAddr.String(),
			Flags:        iface.Flags.String(),
		}

		addrs, err := iface.Addrs()
		if err != nil {
			log.Warnf("[%v] get addr failed: %v", iface.Name, err)
		} else {
			for _, addr := range addrs {
				ip_value, _, err := net.ParseCIDR(addr.String())
				if err != nil {
					continue
				}
				info.Addrs = append(info.Addrs, &NetInterfaceAddr{
					Network: addr.Network(),
					Address: ip_value.String(),
				})

			}
		}

		mcAddrs, err := iface.MulticastAddrs()
		if err != nil {
			log.Warnf("[%v] get multicase-addrs failed: %v", iface.Name, err)
		} else {
			for _, addr := range mcAddrs {
				ip_value, _, err := net.ParseCIDR(addr.String())
				if err != nil {
					continue
				}

				info.MulticastAddrs = append(info.MulticastAddrs, &NetInterfaceAddr{
					Network: addr.Network(),
					Address: ip_value.String(),
				})

			}
		}

		ifs = append(ifs, info)
	}

	return &NetworkInfo{
		Interfaces: ifs,
	}, nil
}

type ExternalNetInterfaceInfo struct {
	Addrs string `json:"addrs"`
}

type NetInterfaceInfo struct {
	Index          int                 `json:"index"`
	MTU            int                 `json:"mtu"`
	Name           string              `json:"name"`
	HardwareAddr   string              `json:"hardware_addr"`
	Flags          string              `json:"flags"`
	Addrs          []*NetInterfaceAddr `json:"addrs"`
	MulticastAddrs []*NetInterfaceAddr `json:"multicase_addrs"`
}

func (p *NetInterfaceInfo) String() (str string) {
	jsonByte, err := json.Marshal(*p)
	if err != nil {
		return
	}
	str = string(jsonByte)
	return
}

type NetInterfaceAddr struct {
	Network string `json:"network"`
	Address string `json:"address"`
}
