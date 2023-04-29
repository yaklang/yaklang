package routewrapper

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net"
	"strconv"
)

type BSDRouteWrapper struct {
	netstatCommand CommandSpec
	routeCommand   string
	interfaces     map[string]*net.Interface
	routes         []Route
	defaultRoutes  []Route
}

const (
	RTF_PROTO1    = '1'
	RTF_PROTO2    = '2'
	RTF_PROTO3    = '3'
	RTF_BLACKHOLE = 'B'
	RTF_BROADCAST = 'b'
	RTF_CLONING   = 'C'
	RTF_PRCLONING = 'c'
	RTF_DYNAMIC   = 'D'
	RTF_GATEWAY   = 'G'
	RTF_HOST      = 'H'
	RTF_IFSCOPE   = 'I'
	RTF_IFREF     = 'i'
	RTF_LLINFO    = 'L'
	RTF_MODIFIED  = 'M'
	RTF_MULTICAST = 'm'
	RTF_REJECT    = 'R'
	RTF_ROUTER    = 'r'
	RTF_STATIC    = 'S'
	RTF_UP        = 'U'
	RTF_WASCLONED = 'W'
	RTF_XRESOLVE  = 'X'
	RTF_PROXY     = 'Y'
)

var flagsMap = map[rune]string{
	RTF_PROTO1:    "PROTO1",
	RTF_PROTO2:    "PROTO2",
	RTF_PROTO3:    "PROTO3",
	RTF_BLACKHOLE: "BLACKHOLE",
	RTF_BROADCAST: "BROADCAST",
	RTF_CLONING:   "CLONING",
	RTF_PRCLONING: "PRCLONING",
	RTF_DYNAMIC:   "DYNAMIC",
	RTF_GATEWAY:   "GATEWAY",
	RTF_HOST:      "HOST",
	RTF_IFSCOPE:   "IFSCOPE",
	RTF_IFREF:     "IFREF",
	RTF_LLINFO:    "LLINFO",
	RTF_MODIFIED:  "MODIFIED",
	RTF_MULTICAST: "MULTICAST",
	RTF_REJECT:    "REJECT",
	RTF_ROUTER:    "ROUTER",
	RTF_STATIC:    "STATIC",
	RTF_UP:        "UP",
	RTF_WASCLONED: "WASCLONED",
	RTF_XRESOLVE:  "XRESOLVE",
	RTF_PROXY:     "PROXY",
}

func (wrapper *BSDRouteWrapper) getRoutes() ([]Route, error) {
	stdoutBuf, stderrBuf, err := wrapper.netstatCommand.Run()
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
	sc := bufio.NewScanner(bytes.NewReader(stdoutBuf))
	header := []string(nil)
	undoBuf := make([]string, 0, 1)
	routes := make([]Route, 0, 16)
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
			if t != "Routing tables" {
				return nil, errors.New("The result does not starts with the line \"Routing tables\"")
			}
			state = 1
		case 1:
			if t != "" {
				undoBuf = append(undoBuf, t)
				state = 2
			}
		case 2:
			if t == "Internet:" {
				state = 4
			} else if t == "Internet6:" {
				state = 6
			} else {
				state = 3
			}
		case 3:
			if t == "" {
				state = 1
			}
		case 4:
			header = delimitedByWhitespaces.Split(t, -1)
			state = 5
		case 5:
			if t == "" {
				state = 1
			} else {
				columns := delimitedByWhitespaces.Split(t, -1)
				r := Route{
					Flags: make(map[string]string),
				}
				for i := 0; i < len(header); i++ {
					k := header[i]
					v := ""
					if i < len(columns) {
						v = columns[i]
					}
					switch k {
					case "Destination":
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
					case "Gateway":
						ip := net.ParseIP(v)
						if ip != nil {
							r.Gateway = ip
						}
					case "Flags":
						for _, c := range v {
							flag, ok := flagsMap[c]
							if !ok {
								return nil, fmt.Errorf("Unknown flag: %c", c)
							}
							r.Flags[flag] = flag
						}
					case "Netif":
						r.Interface = wrapper.interfaces[v]
					case "Expire":
						if v != "" {
							r.Expire, err = strconv.Atoi(v)
							if err != nil {
								return nil, err
							}
						}
					}
				}
				routes = append(routes, r)
			}
		case 6:
			header = delimitedByWhitespaces.Split(t, -1)
			state = 7
		case 7:
			if t == "" {
				state = 1
			} else {
				columns := delimitedByWhitespaces.Split(t, -1)
				r := Route{
					Flags: make(map[string]string),
				}
				for i := 0; i < len(header); i++ {
					k := header[i]
					v := ""
					if i < len(columns) {
						v = columns[i]
					}
					switch k {
					case "Destination":
						if v == "default" {
							r.Destination.IP = net.IPv6zero
							r.Destination.Mask = net.CIDRMask(0, 128)
						} else {
							dst, err := ourParseCIDRv6(v)
							if err != nil {
								return nil, err
							}
							r.Destination = *dst
						}
					case "Gateway":
						ip := net.ParseIP(v)
						if ip != nil {
							r.Gateway = ip
						}
					case "Flags":
						for _, c := range v {
							flag, ok := flagsMap[c]
							if !ok {
								return nil, fmt.Errorf("Unknown flag: %c", c)
							}
							r.Flags[flag] = flag
						}
					case "Netif":
						r.Interface = wrapper.interfaces[v]
					case "Expire":
						if v != "" {
							r.Expire, err = strconv.Atoi(v)
							if err != nil {
								return nil, err
							}
						}
					}
				}
				routes = append(routes, r)
			}
		}
	}
	return routes, nil
}

func (wrapper *BSDRouteWrapper) populate() error {
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

func (wrapper *BSDRouteWrapper) Routes() ([]Route, error) {
	err := wrapper.populate()
	if err != nil {
		return nil, err
	}
	return wrapper.routes, nil
}

func (wrapper *BSDRouteWrapper) DefaultRoutes() ([]Route, error) {
	err := wrapper.populate()
	if err != nil {
		return nil, err
	}
	return wrapper.defaultRoutes, nil
}

func (wrapper *BSDRouteWrapper) AddRoute(route Route) error {
	args := []string{"add"}
	destinationIsNetwork := route.DestinationIsNetwork()
	if destinationIsNetwork {
		args = append(args, "-net")
	}
	args = append(args, route.Destination.IP.String())
	gatewayAddr := net.IP(nil)
	if route.Gateway != nil {
		gatewayAddr = route.Gateway
	} else {
		if route.Interface == nil {
			return fmt.Errorf("gateway is not specified while interface is not specified either")
		}
	}
	if gatewayAddr != nil {
		args = append(args, gatewayAddr.String())
	}
	if route.Interface != nil {
		args = append(args, "-interface")
		args = append(args, route.Interface.Name)
	}
	if destinationIsNetwork {
		args = append(args, route.Destination.Mask.String())
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

func (wrapper *BSDRouteWrapper) GetInterface(name string) (*net.Interface, error) {
	if_, ok := wrapper.interfaces[name]
	if !ok {
		return nil, fmt.Errorf("No such interface: %s", name)
	}
	return if_, nil
}

func NewBSDRouteWrapper(netstatCommand string, routeCommand string) (*BSDRouteWrapper, error) {
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
	return &BSDRouteWrapper{
		netstatCommand: CommandSpec{netstatCommand, []string{"-r", "-n"}},
		routeCommand:   routeCommand,
		interfaces:     interfaces,
		routes:         nil,
	}, nil
}
