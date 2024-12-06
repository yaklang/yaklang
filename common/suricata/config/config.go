package config

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	yaml "github.com/yaklang/yaklang/common/openapi/openapiyaml"
	"golang.org/x/exp/maps"
	"net"
	"strconv"
	"strings"
)

const DefaultConfigYaml = `
vars:
  # more specific is better for alert accuracy and performance
  address-groups:
    HOME_NET: "[192.168.0.1/16,10.0.0.0/8,172.16.0.0/12]"
    #HOME_NET: "[192.168.0.0/16]"
    #HOME_NET: "[10.0.0.0/8]"
    #HOME_NET: "[172.16.0.0/12]"
    #HOME_NET: "any"

    EXTERNAL_NET: "!$HOME_NET"
    #EXTERNAL_NET: "any"

    HTTP_SERVERS: "$HOME_NET"
    SMTP_SERVERS: "$HOME_NET"
    SQL_SERVERS: "$HOME_NET"
    DNS_SERVERS: "$HOME_NET"
    TELNET_SERVERS: "$HOME_NET"
    AIM_SERVERS: "$EXTERNAL_NET"
    DC_SERVERS: "$HOME_NET"
    DNP3_SERVER: "$HOME_NET"
    DNP3_CLIENT: "$HOME_NET"
    MODBUS_CLIENT: "$HOME_NET"
    MODBUS_SERVER: "$HOME_NET"
    ENIP_CLIENT: "$HOME_NET"
    ENIP_SERVER: "$HOME_NET"

  port-groups:
    HTTP_PORTS: "80"
    SHELLCODE_PORTS: "!80"
    ORACLE_PORTS: 1521
    SSH_PORTS: 22
    DNP3_PORTS: 20000
    MODBUS_PORTS: 502
    FILE_DATA_PORTS: "[$HTTP_PORTS,110,143]"
    FTP_PORTS: 21
    GENEVE_PORTS: 6081
    VXLAN_PORTS: 4789
    TEREDO_PORTS: 3544`

type Config struct {
	Vars map[string]*scope
}

func (c *Config) RandPortVar(varName string) int {
	val := c.Vars[varName]
	if val == nil {
		return 0
	}
	return int(val.randInt())
}
func (c *Config) RandIpVar(varName string) string {
	val := c.Vars[varName]
	if val == nil {
		return ""
	}
	n := val.randInt()
	return fmt.Sprintf("%d.%d.%d.%d", n>>24, n>>16&0xff, n>>8&0xff, n&0xff)
}
func (c *Config) HasVar(varName string) bool {
	_, ok := c.Vars[varName]
	return ok
}
func (c *Config) MatchVar(varName string, s any) bool {
	if c.Vars == nil {
		return false
	}
	val, ok := c.Vars[varName]
	if !ok {
		return false
	}
	switch ret := s.(type) {
	case string:
		ip := net.ParseIP(ret)
		if ip != nil {
			return val.hasNumber(ipToUint32(ip))
		}
		return false
	case int:
		return val.hasNumber(uint32(ret))
	case int64:
		return val.hasNumber(uint32(ret))
	case uint32:
		return val.hasNumber(uint32(ret))
	}
	return false
}

func NewConfig() *Config {
	cfg := &Config{
		Vars: map[string]*scope{},
	}
	cfg.addVarWithVarGetter("port", "ANY_PORT", "any", func(typ, name string) error {
		return nil
	})
	cfg.addVarWithVarGetter("port", "ANY_IP", "any", func(typ, name string) error {
		return nil
	})
	return cfg
}

func ipToUint32(ipIns net.IP) uint32 {
	ip := ipIns.To4()
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}
func (c *Config) addVarWithVarGetter(varType string, name string, v any, getter func(typ, name string) error) error {
	req := func(name string) error {
		err := getter(varType, name)
		if err != nil {
			return fmt.Errorf("get var `%s` failed: %v", name, err)
		}
		if _, ok := c.Vars[name]; !ok {
			return fmt.Errorf("get var `%s` failed: not found", name)
		}
		return nil
	}
	var getVar func(v any) (*scope, bool, error)
	getVar = func(v any) (*scope, bool, error) {
		switch ret := v.(type) {
		case string:
			if v == "any" {
				switch varType {
				case "ip":
					return newScope(0, 0xfffffffe), true, nil
				case "port":
					return newScope(0, 0xffff), true, nil
				default:
					return newScope(0, 0xfffffffe), true, nil
				}
			}
			n, err := strconv.Atoi(ret)
			if err == nil {
				return newScope(uint32(n), uint32(n)), true, nil
			}
			ret = strings.TrimSpace(ret)
			if v := net.ParseIP(ret); v != nil {
				n := ipToUint32(v)
				return newScope(uint32(n), uint32(n)), true, nil
			}
			_, ipNet, err := net.ParseCIDR(ret)
			if err == nil {
				startIP := ipNet.IP.To4()
				var endIP = make(net.IP, 4)
				copy(endIP, startIP)
				for i := range ipNet.Mask {
					endIP[i] |= ^ipNet.Mask[i]
				}
				return newScope(ipToUint32(startIP), ipToUint32(endIP)), true, nil
			}
			if strings.HasPrefix(ret, "$") {
				refVarName := ret[1:]
				err := req(refVarName)
				if err != nil {
					return nil, false, err
				}
				return c.Vars[refVarName], true, nil
			}
			if strings.HasPrefix(ret, "!") {
				refVarName := ret[1:]
				if strings.HasPrefix(refVarName, "$") {
					varName := strings.Trim(refVarName, "$")
					err := req(varName)
					if err != nil {
						return nil, false, err
					}
					sc := c.Vars[varName]
					return sc.not(), true, nil
				} else {
					val, ok, err := getVar(refVarName)
					if err != nil {
						return nil, false, err
					}
					if !ok {
						return nil, false, nil
					}
					return val.not(), true, nil
				}
			}
			if strings.HasPrefix(ret, "[") && strings.HasSuffix(ret, "]") {
				ret = strings.Trim(ret, "[]")
				eles := strings.Split(ret, ",")
				res := newEmptyScope()
				for _, ele := range eles {
					ele = strings.TrimSpace(ele)
					val, ok, err := getVar(ele)
					if err != nil {
						return nil, false, err
					}
					if !ok {
						return nil, false, fmt.Errorf("parse suricata config var element `%s` failed", ele)
					}
					res.add(val)
				}

				return res, true, nil
			}
		case int:
			return newScope(uint32(ret), uint32(ret)), true, nil
		case int64:
			return newScope(uint32(ret), uint32(ret)), true, nil
		case float64:
			return newScope(uint32(ret), uint32(ret)), true, nil
		}
		return nil, false, nil
	}
	newVal, ok, err := getVar(v)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("parse suricata config var `%s` failed", name)
	}
	c.Vars[name] = newVal
	return nil
}
func (c *Config) AddVar(name string, v any) error {
	return c.addVarWithVarGetter("", name, v, func(typ, name string) error {
		return nil
	})
}

var DefaultConfig = NewConfig()

func init() {
	config, err := ParseSuricataConfig(DefaultConfigYaml)
	if err != nil {
		log.Errorf("initing suricata default config failed: %v", err)
		return
	}
	DefaultConfig = config
}
func ParseSuricataConfig(yamlContent string) (*Config, error) {
	config := NewConfig()
	configMap := map[string]any{}
	err := yaml.Unmarshal([]byte(yamlContent), &configMap)
	if err != nil {
		log.Errorf("initing suricata default config failed: %v", err)
	}
	ipVarMaps := map[string]any{}
	portVarMaps := map[string]any{}
	varsData := configMap["vars"]
	if varsData != nil {
		if varGroupMap, ok := varsData.(map[string]any); ok {
			addressGroup := varGroupMap["address-groups"]
			if addressGroup != nil {
				if addressGroupMap, ok := addressGroup.(map[string]any); ok {
					maps.Copy(ipVarMaps, addressGroupMap)
				}
			}
			portGroup := varGroupMap["port-groups"]
			if portGroup != nil {
				if portGroupMap, ok := portGroup.(map[string]any); ok {
					maps.Copy(portVarMaps, portGroupMap)
				}
			}
		}
	}

	keys := maps.Keys(ipVarMaps)
	keys = append(keys, maps.Keys(portVarMaps)...)

	var varGetter func(typ string, key string) error
	varGetter = func(typ string, key string) error {
		if _, ok := config.Vars[key]; ok {
			return nil
		}
		var val any
		if typ == "ip" {
			v, ok := ipVarMaps[key]
			if !ok {
				return fmt.Errorf("get var `%s` failed: not found", key)
			}
			val = v
		} else {
			v, ok := portVarMaps[key]
			if !ok {
				return fmt.Errorf("get var `%s` failed: not found", key)
			}
			val = v
		}

		err = config.addVarWithVarGetter(typ, key, val, varGetter)
		if err != nil {
			return fmt.Errorf("get var `%s` failed: %w", key, err)
		}
		return nil
	}
	for _, key := range maps.Keys(ipVarMaps) {
		err := config.addVarWithVarGetter("ip", key, ipVarMaps[key], varGetter)
		if err != nil {
			return nil, fmt.Errorf("initing suricata default config failed: %v", err)
		}
	}
	for _, key := range maps.Keys(portVarMaps) {
		err := config.addVarWithVarGetter("port", key, portVarMaps[key], varGetter)
		if err != nil {
			return nil, fmt.Errorf("initing suricata default config failed: %v", err)
		}
	}
	return config, nil
}
