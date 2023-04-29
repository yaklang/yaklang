package utils

import (
	"context"
	"github.com/pkg/errors"
	"net"
	"time"
)

func WaitConnect(addr string, timeout float64) error {
	ch := make(chan int)
	ctx, cancle := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				_, err := net.Dial("tcp", addr)
				if err == nil {
					ch <- 1
					return
				}
				//println("连接失败")
				time.Sleep(100 * time.Microsecond)
			}
		}
	}()

	select {
	case <-time.After(time.Duration(timeout) * time.Second):
		cancle()
		//println("超时")
		return errors.New("Connection attempt timed out")
	case <-ch:
		return nil
	}
}

func GetTargetAddrLocalAddr(targetAddr string) (string, error) {
	dialer := net.Dialer{Timeout: 3 * time.Second}
	conn, err := dialer.Dial("tcp", targetAddr)
	if err != nil {
		return "", errors.Errorf("dial failed: %s", err)
	}
	localAddr := conn.LocalAddr()
	_ = conn.Close()

	host, _, err := ParseStringToHostPort(localAddr.String())
	if err != nil {
		return "", errors.Errorf("%v parse to host failed: %s", localAddr.String(), err)
	}
	return host, nil
}

func GetTargetAddrInterfaceName(targetAddr string) (string, error) {
	host, err := GetTargetAddrLocalAddr(targetAddr)
	if err != nil {
		return "", err
	}

	ifs, err := net.Interfaces()
	if err != nil {
		return "", errors.Errorf("get interfaces failed: %s", err)
	}

	for _, iface := range ifs {
		addrs, err := iface.Addrs()
		if err != nil {
			return "", errors.Errorf("fetch iface addr failed: %s", err)
		}

		for _, addr := range addrs {
			_, _net, err := net.ParseCIDR(addr.String())
			if err != nil {
				continue
			}

			if i := net.ParseIP(FixForParseIP(host)); i != nil {
				if _net.Contains(i) {
					return iface.Name, nil
				}
			}
		}
	}
	return "", errors.Errorf("cannot found local interface name for %s", targetAddr)
}
