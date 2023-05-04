package routewrapper

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"strconv"
)

type LinuxRouteWrapper struct {
	ipCommand     CommandSpec
	interfaces    map[string]*net.Interface
	routes        []Route
	defaultRoutes []Route
}

var familyMap = map[string]string{
	"inet":   "inet",
	"inet6":  "inet6",
	"ipx":    "ipx",
	"dnet":   "dnet",
	"mpls":   "mpls",
	"bridge": "bridge",
	"link":   "link",
}

func (wrapper *LinuxRouteWrapper) getRoutes() ([]Route, error) {
	stdoutBuf, stderrBuf, err := wrapper.ipCommand.Run()
	if err != nil {
		switch err := err.(type) {
		case *CommandExecError:
			if stderrBuf != nil {
				err.Message += fmt.Sprintf(" stderr:%s", stderrBuf)
			}
		}
		return nil, err
	}
	sc := bufio.NewScanner(bytes.NewReader(stdoutBuf))
	routes := make([]Route, 0, 16)
	for {
		if !sc.Scan() {
			break
		}
		t := sc.Text()
		columns := delimitedByWhitespaces.Split(t, -1)
		r := Route{
			Flags: make(map[string]string),
		}
		state := 0
		attrName := ""
		familyName := ""
		for _, v := range columns {
			if v == "" {
				continue
			}
			switch state {
			case 0:
				if v == "default" {
					r.Destination.IP = net.IPv4zero
					r.Destination.Mask = net.CIDRMask(0, 32)
				} else {
					dst, err := ourParseCIDRv4(v)
					if err != nil {
						return nil, err
					}
					r.Destination = *dst
				}
				state = 1
			case 1:
				switch v {
				case "via":
					state = 2
				case "dev":
					state = 4
				case "metric":
					state = 5
				case "proto",
					"scope",
					"realm",
					"mtu",
					"mtu_lock",
					"window",
					"rtt",
					"rttvar",
					"rio_min",
					"ssthresh",
					"cwnd",
					"initcwnd",
					"initrwnd",
					"features",
					"quickack",
					"advmss",
					"reordering",
					"protocol",
					"pref",
					"src":
					attrName = v
					state = 6
				case "congctl":
					attrName = v
					state = 7
				case "onlink":
					r.Flags["onlink"] = "onlink"
				case "linkdown":
					r.Flags["linkdown"] = "linkdown"
				case "nexthop":
					state = 9
				default:
					return nil, fmt.Errorf("Unknown attribute name: %s", v)
				}
			case 2:
				var ok bool
				familyName, ok = familyMap[v]
				if !ok {
					ip := net.ParseIP(v)
					if ip != nil {
						r.Gateway = ip
					}
					state = 1
				} else {
					state = 3
				}
			case 3:
				ip := net.ParseIP(v)
				if ip != nil {
					r.Gateway = ip
				}
				state = 1
			case 4:
				r.Interface = wrapper.interfaces[v]
				state = 1
			case 5:
				r.Metric, err = strconv.Atoi(v)
				if err != nil {
					return nil, err
				}
				state = 1
			case 6:
				r.Flags[attrName] = v
				state = 1
			case 7:
				if v == "lock" {
					state = 8
				} else {
					r.Flags["congctl"] = v
					state = 1
				}
			case 8:
				r.Flags["congctl_lock"] = v
				state = 1
			case 9:
				switch v {
				case "via":
					attrName = v
					state = 10
				case "dev", "weight":
					attrName = v
					state = 12
				default:
					return nil, fmt.Errorf("Unknown attribute name under \"nexthop\": %s", v)
				}
			case 10:
				var ok bool
				familyName, ok = familyMap[v]
				if !ok {
					r.Flags["nexthop_via"] = v
					state = 1
				} else {
					state = 11
				}
			case 11:
				r.Flags["nexthop_"+attrName] = familyName + " " + v
				state = 1
			case 12:
				r.Flags["nexthop_"+attrName] = v
				state = 1
			}
		}
		routes = append(routes, r)
	}
	return routes, nil
}

func (wrapper *LinuxRouteWrapper) populate() error {
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

func (wrapper *LinuxRouteWrapper) Routes() ([]Route, error) {
	err := wrapper.populate()
	if err != nil {
		return nil, err
	}
	return wrapper.routes, nil
}

func (wrapper *LinuxRouteWrapper) DefaultRoutes() ([]Route, error) {
	err := wrapper.populate()
	if err != nil {
		return nil, err
	}
	return wrapper.defaultRoutes, nil
}

func (wrapper *LinuxRouteWrapper) AddRoute(route Route) error {
	cmd := wrapper.ipCommand.Clone()
	cmd.Args = append(cmd.Args, "add")
	cmd.Args = append(cmd.Args, "to")
	if route.Destination.Mask == nil {
		cmd.Args = append(cmd.Args, route.Destination.IP.String())
	} else {
		cmd.Args = append(cmd.Args, route.Destination.String())
	}
	gatewayAddr := net.IP(nil)
	if route.Gateway != nil {
		gatewayAddr = route.Gateway
	} else {
		if route.Interface == nil {
			return fmt.Errorf("gateway is not specified while interface is not specified either")
		}
	}
	if gatewayAddr != nil {
		cmd.Args = append(cmd.Args, "via")
		cmd.Args = append(cmd.Args, gatewayAddr.String())
	}
	if route.Interface != nil {
		cmd.Args = append(cmd.Args, "dev")
		cmd.Args = append(cmd.Args, route.Interface.Name)
	}
	_, _, err := cmd.Run()
	if err != nil {
		return err
	}
	return nil
}

func (wrapper *LinuxRouteWrapper) GetInterface(name string) (*net.Interface, error) {
	if_, ok := wrapper.interfaces[name]
	if !ok {
		return nil, fmt.Errorf("No such interface: %s", name)
	}
	return if_, nil
}

func NewLinuxRouteWrapper(ipCommand string) (*LinuxRouteWrapper, error) {
	interfaces := make(map[string]*net.Interface)

	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for i := range ifs {
		pif := &ifs[i]
		_, ok := interfaces[pif.Name]
		if ok {
			return nil, fmt.Errorf("More than one Interfaces with the same name exists: %s", pif.Name)
		}
		interfaces[pif.Name] = pif
	}
	return &LinuxRouteWrapper{
		ipCommand:  CommandSpec{ipCommand, []string{"route"}},
		interfaces: interfaces,
		routes:     nil,
	}, nil
}
