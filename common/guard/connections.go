package guard

import (
	"context"
	"fmt"
	"time"

	gopsnet "github.com/shirou/gopsutil/v4/net"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type NetConn struct {
	Fd         uint32
	Family     string
	Type       string
	LocalAddr  string
	RemoteAddr string
	Status     string
	Uids       []int32
	Pid        int
}

func (s *NetConn) String() string {
	msg := fmt.Sprintf(
		"net.Conn[%v/%v-fd(%v)|pid(%v)]: %v",
		s.Family, s.Type, s.Fd, s.Pid, s.LocalAddr,
	)

	if s.RemoteAddr != "" {
		msg += " -> " + s.RemoteAddr
	}

	if s.Status != "" {
		msg += fmt.Sprintf(" (%v)", s.Status)
	}
	return msg
}

func GetAllConns(ctx context.Context) ([]*NetConn, error) {
	conns, err := gopsnet.ConnectionsWithContext(ctx, "all")
	if err != nil {
		return nil, nil
	}

	unixConn, _ := gopsnet.ConnectionsWithContext(ctx, "unix")
	conns = append(conns, unixConn...)

	var ret []*NetConn
	for _, conn := range conns {
		var (
			localAddr  = conn.Laddr.IP
			remoteAddr = conn.Raddr.IP
		)
		if conn.Laddr.Port > 0 {
			localAddr = utils.HostPort(conn.Laddr.IP, conn.Laddr.Port)
		}

		if conn.Raddr.Port > 0 {
			remoteAddr = utils.HostPort(conn.Raddr.IP, conn.Raddr.Port)
		}

		ret = append(ret, &NetConn{
			Fd:         conn.Fd,
			Family:     utils.AddressFamilyUint32ToString(conn.Family),
			Type:       utils.SocketTypeUint32ToString(conn.Type),
			LocalAddr:  localAddr,
			RemoteAddr: remoteAddr,
			Status:     conn.Status,
			Uids:       conn.Uids,
			Pid:        int(conn.Pid),
		})
	}
	return ret, nil
}

type NetConnEventType string

const (
	NetConnEvent_New       NetConnEventType = "new"
	NetConnEvent_Disappear NetConnEventType = "disappear"
)

type (
	NetConnCallback      func([]*NetConn)
	NetConnEventCallback func(eventType NetConnEventType, conn *NetConn)
)

type NetConnGuardOption func(t *NetConnGuardTarget) error

type NetConnGuardTarget struct {
	// intervalSeconds int
	// intervalOffset  int
	guardTargetBase

	eventCallbacks []NetConnEventCallback
	callbacks      []NetConnCallback

	cache *utils.Cache[*NetConn]
}

func NewNetConnGuardTarget(intervalSeconds int, options ...NetConnGuardOption) (*NetConnGuardTarget, error) {
	t := &NetConnGuardTarget{
		guardTargetBase: guardTargetBase{
			intervalSeconds: intervalSeconds,
		},
		cache: utils.NewTTLCache[*NetConn](),
	}
	t.children = t
	for _, option := range options {
		err := option(t)
		if err != nil {
			return nil, utils.Errorf("net conn guard execute option failed; %s", err)
		}
	}

	if t.eventCallbacks != nil {
		t.cache.SetExpirationCallback(func(key string, value *NetConn) {
			for _, i := range t.eventCallbacks {
				i(NetConnEvent_Disappear, value)
			}
		})
		t.cache.SetNewItemCallback(func(key string, value *NetConn) {
			for _, i := range t.eventCallbacks {
				i(NetConnEvent_New, value)
			}
		})
	}

	return t, nil
}

func SetNetConnEventCallback(f NetConnEventCallback) NetConnGuardOption {
	return func(t *NetConnGuardTarget) error {
		t.eventCallbacks = append(t.eventCallbacks, f)
		return nil
	}
}

func SetNetConnsCallback(f NetConnCallback) NetConnGuardOption {
	return func(t *NetConnGuardTarget) error {
		t.callbacks = append(t.callbacks, f)
		return nil
	}
}

func (p *NetConnGuardTarget) do() {
	ps, err := GetAllConns(utils.TimeoutContext(time.Duration(p.intervalSeconds) * time.Second))
	if err != nil {
		log.Errorf("query all conns failed: %s", err)
		return
	}

	for _, i := range p.callbacks {
		i(ps)
	}

	for _, conn := range ps {
		p.cache.SetWithTTL(utils.CalcSha1(
			conn.Fd, conn.Family, conn.Type,
			conn.LocalAddr, conn.RemoteAddr,
		), conn, time.Duration(2*p.intervalSeconds)*time.Second)
	}

	return
}
