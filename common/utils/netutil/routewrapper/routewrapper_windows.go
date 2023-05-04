package routewrapper

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

const SEPARATOR = "==========================================================================="

const PERSISTENT = "PERSISTENT"

type WindowsRouteWrapper struct {
	routeCommand     string
	interfaces       map[int]*net.Interface
	interfacesByName map[string]*net.Interface
	routes           []Route
	defaultRoutes    []Route
}

func ourParseGatewayWinRoute(ip string) net.IP {
	if ip == "On-link" {
		return nil
	}
	return net.ParseIP(ip)
}

func splitWhitespacesWinRoute(t string) []string {
	t = strings.TrimLeft(t, " \t")
	i := 0
	s := 0
	chunks := make([]string, 0, 5)
outer:
	for {
		if i >= len(t) {
			chunk := t[s:i]
			chunks = append(chunks, chunk)
			break
		} else if t[i] == ' ' {
			if i+12 <= len(t) && t[i:i+12] == " Destination" {
				i += 12
			} else if i+8 <= len(t) && t[i:i+8] == " Address" {
				i += 8
			} else {
				chunk := t[s:i]
				chunks = append(chunks, chunk)
				for {
					if i >= len(t) {
						break outer
					} else if t[i] != ' ' {
						break
					}
					i++
				}
				s = i
			}
		} else {
			i++
		}
	}
	return chunks
}

func (wrapper *WindowsRouteWrapper) interfaceFromAddr(ip net.IP) (*net.Interface, error) {
	candidate := (*net.Interface)(nil)
	for _, if_ := range wrapper.interfaces {
		ifAddrs, err := if_.Addrs()
		if err != nil {
			return nil, err
		}
		for _, addr := range ifAddrs {
			ifAddr, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				return nil, err
			}
			if ifAddr.Equal(ip) {
				if candidate != nil {
					return nil, fmt.Errorf("Multiple interfaces that have the same IP address exist: %s", ip.String())
				}
				candidate = if_
			}
		}
	}
	if candidate == nil {
		return nil, fmt.Errorf("No applicable interface found for %s", ip.String())
	}
	return candidate, nil
}

func (wrapper *WindowsRouteWrapper) defaultRouteForInterface(if_ *net.Interface) (*net.Interface, *Route, error) {
	err := wrapper.populate()
	if err != nil {
		return nil, nil, err
	}
	ifAddrs, err := if_.Addrs()
	if err != nil {
		return nil, nil, err
	}
	candidate := (*Route)(nil)
	for i := range wrapper.routes {
		r := &wrapper.routes[i]
		if r.Destination.Mask != nil && !r.IsDefaultRoute() && r.Gateway != nil {
			for _, addr := range ifAddrs {
				ifAddr, _, err := net.ParseCIDR(addr.String())
				if err != nil {
					return nil, nil, err
				}
				if ifAddr.Mask(r.Destination.Mask).Equal(r.Destination.IP) {
					if candidate != nil {
						return nil, nil, fmt.Errorf("Multiple candidates exist for interface: %s", if_.Name)
					}
					candidate = r
				}
			}
		}
	}
	if candidate == nil {
		return nil, nil, fmt.Errorf("No applicable route found for %s", if_.Name)
	}
	return wrapper.interfaces[if_.Index], candidate, nil
}

func (wrapper *WindowsRouteWrapper) getRoutes() ([]Route, error) {
	routePrintCommand := CommandSpec{wrapper.routeCommand, []string{"PRINT"}}
	stdoutBuf, stderrBuf, err := routePrintCommand.Run()
	if err != nil {
		switch err := err.(type) {
		case *CommandExecError:
			if stderrBuf != nil {
				err.Message += fmt.Sprintf(" stderr:%s", stderrBuf)
			}
		}
		return nil, err
	}

	state := 0
	cont := 0
	sc := bufio.NewScanner(bytes.NewReader(stdoutBuf))
	header := []string(nil)
	undoBuf := make([]string, 0, 1)
	routes := make([]Route, 0, 16)
	var r Route
	for {
		t := ""
		if len(undoBuf) > 0 {
			t = undoBuf[len(undoBuf)-1]
			undoBuf = undoBuf[:len(undoBuf)-1]
		} else {
			if !sc.Scan() {
				break
			}
			t = sc.Text()
		}

		switch state {
		case 0:
			if t != SEPARATOR {
				return nil, errors.New("The result does not starts with the separator line")
			}
			state = 1
		case 1:
			if t != "Interface List" {
				return nil, fmt.Errorf("Expecting \"Interface List\", got \"%s\"", t)
			}
			state = 2
		case 2:
			if t == SEPARATOR {
				state = 3
			} else {
				t = strings.TrimLeft(t, " \t")
				i := strings.IndexByte(t, byte('.'))
				if i < 0 {
					return nil, fmt.Errorf("Invalid interface description: \"%s\"", t)
				}
				ifIndex, err := strconv.Atoi(t[0:i])
				if err != nil {
					return nil, fmt.Errorf("Invalid interface description: \"%s\"", t)
				}
				if i+3 > len(t) || t[i:i+3] != "..." {
					return nil, fmt.Errorf("Invalid interface description: \"%s\"", t)
				}
				i += 3
				j := 0
				mac := [8]byte{0, 0, 0, 0, 0, 0, 0, 0}
				for {
					if i >= len(t) {
						return nil, fmt.Errorf("Invalid interface description: \"%s\"", t)
					}
					if t[i] == '.' {
						break
					} else {
						if j == 8 {
							break
						}
						if i+3 >= len(t) {
							return nil, fmt.Errorf("Invalid interface description: \"%s\"", t)
						}
						b, err := strconv.ParseInt(t[i:i+2], 16, 16)
						if err != nil {
							return nil, fmt.Errorf("Invalid interface description: \"%s\"", t)
						}
						if j >= len(mac) {
							return nil, fmt.Errorf("Invalid interface description: \"%s\"", t)
						}
						mac[j] = byte(b)
						j++
						if t[i+2] != ' ' {
							return nil, fmt.Errorf("Invalid interface description: \"%s\"", t)
						}
						i += 3
					}
				}
				for {
					if i >= len(t) || t[i] != '.' {
						break
					}
					i++
				}
				ifName := t[i:]
				_, ok := wrapper.interfaces[ifIndex]
				if !ok {
					if_ := &net.Interface{
						Index:        ifIndex,
						MTU:          0,
						Name:         ifName,
						HardwareAddr: net.HardwareAddr(mac[:]),
						Flags:        0,
					}
					wrapper.interfaces[ifIndex] = if_
					wrapper.interfacesByName[ifName] = if_
				}
			}
		case 3:
			if t != "" {
				undoBuf = append(undoBuf, t)
				state = 4
			}
		case 4:
			if t == "IPv4 Route Table" {
				state = 5
			} else if t == "IPv6 Route Table" {
				state = 13
			} else {
				return nil, fmt.Errorf("Expecting \"IPv4 Route Table\", got \"%s\"", t)
			}
		case 5:
			if t != SEPARATOR {
				return nil, fmt.Errorf("Expecting a separator, got \"%s\"", t)
			}
			state = 6
		case 6:
			if t == "Active Routes:" {
				state = 7
			} else if t == "Persistent Routes:" {
				state = 11
			} else if t == "" {
				state = 4
			}
		case 7:
			if t == "  None" {
				state = 10
			} else {
				header = splitWhitespacesWinRoute(t)
				state = 8
			}
		case 8:
			if t == SEPARATOR {
				state = 6
			} else {
				columns := splitWhitespacesWinRoute(t)
				r = Route{
					Flags: make(map[string]string),
				}
				for i := 0; i < len(header); i++ {
					k := header[i]
					v := ""
					if i < len(columns) {
						v = columns[i]
					}
					switch k {
					case "Network Destination":
						dst := net.ParseIP(v)
						if dst == nil {
							return nil, fmt.Errorf("Invalid IPv4 address: %s", v)
						}
						r.Destination.IP = dst
					case "Netmask":
						nm := net.ParseIP(v)
						if nm == nil {
							return nil, fmt.Errorf("Invalid IPv4 address: %s", v)
						}
						nm4 := nm.To4()
						if nm4 != nil {
							r.Destination.Mask = net.IPMask(nm4)
						} else {
							r.Destination.Mask = net.IPMask(nm)
						}
					case "Gateway":
						ip := ourParseGatewayWinRoute(v)
						if ip != nil {
							r.Gateway = ip
						}
					case "Interface":
						ifAddr := net.ParseIP(v)
						if ifAddr == nil {
							return nil, fmt.Errorf("Invalid IPv4 address: %s", v)
						}
						r.Interface, err = wrapper.interfaceFromAddr(ifAddr)
						if err != nil {
							return nil, err
						}
					case "Metric":
						r.Metric, err = strconv.Atoi(v)
						if err != nil {
							return nil, err
						}
					}
				}
				routes = append(routes, r)
			}
		case 10:
			if t == SEPARATOR {
				state = 6
			} else if t == "" {
				state = 4
			}
		case 11:
			if t == "  None" {
				state = 10
			} else {
				header = splitWhitespacesWinRoute(t)
				state = 12
			}
		case 12:
			if t == SEPARATOR {
				state = 6
			} else {
				columns := splitWhitespacesWinRoute(t)
				r = Route{
					Flags: map[string]string{PERSISTENT: "persistent"},
				}
				for i := 0; i < len(header); i++ {
					k := header[i]
					v := ""
					if i < len(columns) {
						v = columns[i]
					}
					switch k {
					case "Network Address":
						dst := net.ParseIP(v)
						if dst == nil {
							return nil, fmt.Errorf("Invalid IPv4 address: %s", v)
						}
						r.Destination.IP = dst
					case "Netmask":
						nm := net.ParseIP(v)
						if nm == nil {
							return nil, fmt.Errorf("Invalid IPv4 address: %s", v)
						}
						nm4 := nm.To4()
						if nm4 != nil {
							r.Destination.Mask = net.IPMask(nm4)
						} else {
							r.Destination.Mask = net.IPMask(nm)
						}
					case "Gateway Address":
						ip := ourParseGatewayWinRoute(v)
						if ip != nil {
							r.Gateway = ip
						}
					case "Metric":
						if v == "Default" {
							r.Metric = -1
						} else {
							r.Metric, err = strconv.Atoi(v)
							if err != nil {
								return nil, err
							}
						}
					}
				}
				routes = append(routes, r)
			}
		case 13:
			if t != SEPARATOR {
				return nil, fmt.Errorf("Expecting a separator, got \"%s\"", t)
			}
			state = 14
		case 14:
			if t == "Active Routes:" {
				state = 15
			} else if t == "Persistent Routes:" {
				state = 18
			}
		case 15:
			if t == "  None" {
				state = 17
			} else {
				header = splitWhitespacesWinRoute(t)
				state = 16
			}
		case 16:
			if t == SEPARATOR {
				state = 14
			} else {
				columns := splitWhitespacesWinRoute(t)
				if cont == 0 {
					r = Route{
						Flags: make(map[string]string),
					}
				}
				i := cont
				for ; i < len(header); i++ {
					k := header[i]
					v := ""
					if i < len(columns) {
						v = columns[i]
					} else {
						break
					}
					switch k {
					case "If":
						ifIndex, err := strconv.Atoi(v)
						if err != nil {
							return nil, err
						}
						if_, ok := wrapper.interfaces[ifIndex]
						if !ok {
							return nil, fmt.Errorf("No such interface that has the index %d", ifIndex)
						}
						r.Interface = if_
					case "Metric":
						r.Metric, err = strconv.Atoi(v)
						if err != nil {
							return nil, err
						}
					case "Network Destination":
						dst, nm, err := net.ParseCIDR(v)
						if err != nil {
							return nil, err
						}
						r.Destination.IP = dst
						r.Destination.Mask = nm.Mask
					case "Gateway":
						ip := ourParseGatewayWinRoute(v)
						if ip != nil {
							r.Gateway = ip.To16()
						}
					}
				}
				if i == len(header) {
					routes = append(routes, r)
					cont = 0
				} else {
					cont = i
				}
			}
		case 17:
			if t == SEPARATOR {
				state = 14
			}
		case 18:
			if t == "  None" {
				state = 17
			} else {
				header = splitWhitespacesWinRoute(t)
				state = 19
			}
		case 19:
			if t == SEPARATOR {
				state = 14
			} else {
				columns := splitWhitespacesWinRoute(t)
				if cont == 0 {
					r = Route{
						Flags: map[string]string{PERSISTENT: "persistent"},
					}
				}
				i := cont
				for ; i < len(header); i++ {
					k := header[i]
					v := ""
					if i < len(columns) {
						v = columns[i]
					} else {
						break
					}
					switch k {
					case "If":
						ifIndex, err := strconv.Atoi(v)
						if err != nil {
							return nil, err
						}
						if_ := (*net.Interface)(nil)
						if ifIndex != 0 {
							var ok bool
							if_, ok = wrapper.interfaces[ifIndex]
							if !ok {
								return nil, fmt.Errorf("No such interface that has the index %d", ifIndex)
							}
						}
						r.Interface = if_
					case "Metric":
						r.Metric, err = strconv.Atoi(v)
						if err != nil {
							return nil, err
						}
					case "Network Destination":
						dst, nm, err := net.ParseCIDR(v)
						if err != nil {
							return nil, err
						}
						r.Destination.IP = dst
						r.Destination.Mask = nm.Mask
					case "Gateway":
						ip := ourParseGatewayWinRoute(v)
						if ip != nil {
							r.Gateway = ip.To16()
						}
					}
				}
				if i == len(header) {
					routes = append(routes, r)
					cont = 0
				} else {
					cont = i
				}
			}
		}
	}
	return routes, nil
}

func (wrapper *WindowsRouteWrapper) populate() error {
	if wrapper.routes == nil || wrapper.defaultRoutes == nil {
		routes, err := wrapper.getRoutes()
		if err != nil {
			return err
		}
		defaultRoutes := make([]Route, 0, 1)
		for _, route := range routes {
			if route.IsDefaultRoute() {
				defaultRoutes = append(defaultRoutes, route)
			}
		}
		wrapper.routes = routes
		wrapper.defaultRoutes = defaultRoutes
	}
	return nil
}

func (wrapper *WindowsRouteWrapper) Routes() ([]Route, error) {
	err := wrapper.populate()
	if err != nil {
		return nil, err
	}
	return wrapper.routes, nil
}

func (wrapper *WindowsRouteWrapper) DefaultRoutes() ([]Route, error) {
	err := wrapper.populate()
	if err != nil {
		return nil, err
	}
	return wrapper.defaultRoutes, nil
}

func (wrapper *WindowsRouteWrapper) AddRoute(route Route) error {
	args := []string{"ADD"}
	args = append(args, route.Destination.IP.String())
	if route.Destination.Mask != nil {
		args = append(args, "MASK")
		args = append(args, net.IP(route.Destination.Mask).String())
	}
	gatewayAddr := net.IP(nil)
	if route.Gateway != nil {
		gatewayAddr = route.Gateway
	} else {
		if route.Interface == nil {
			return fmt.Errorf("gateway is not specified while interface is not specified either")
		}
		_, routeForInterface, err := wrapper.defaultRouteForInterface(route.Interface)
		if err != nil {
			return fmt.Errorf("Could not determine the gateway address")
		}
		gatewayAddr = routeForInterface.Gateway
	}
	if gatewayAddr != nil {
		args = append(args, gatewayAddr.String())
	}
	if route.Interface != nil {
		args = append(args, "IF")
		args = append(args, strconv.FormatInt(int64(route.Interface.Index), 10))
	}
	_, _, err := CommandSpec{
		wrapper.routeCommand,
		args,
	}.Run()
	if err != nil {
		return err
	}
	return nil
}

func (wrapper *WindowsRouteWrapper) GetInterface(name string) (*net.Interface, error) {
	err := wrapper.populate()
	if err != nil {
		return nil, err
	}
	if_, ok := wrapper.interfacesByName[name]
	if !ok {
		return nil, fmt.Errorf("No such interface: %s", name)
	}
	return if_, nil
}

func NewWindowsRouteWrapper(routeCommand string) (*WindowsRouteWrapper, error) {
	interfaces := make(map[int]*net.Interface)
	interfacesByName := make(map[string]*net.Interface)

	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for i := range ifs {
		pif := &ifs[i]
		_, ok := interfaces[pif.Index]
		if ok {
			return nil, fmt.Errorf("More than one Interfaces with the same index exists: %s", pif.Index)
		}
		interfaces[pif.Index] = pif
		_, ok = interfacesByName[pif.Name]
		if ok {
			return nil, fmt.Errorf("More than one Interfaces with the same name exists: %s", pif.Name)
		}
		interfacesByName[pif.Name] = pif
	}
	return &WindowsRouteWrapper{
		routeCommand:     routeCommand,
		interfaces:       interfaces,
		interfacesByName: interfacesByName,
		routes:           nil,
	}, nil
}
