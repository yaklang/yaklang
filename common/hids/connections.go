package hids

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/guard"
	"github.com/yaklang/yaklang/common/log"
)

// ConnectionInfo 连接信息结构体
type ConnectionInfo struct {
	Fd         uint32  `json:"fd"`
	Family     string  `json:"family"`      // 地址族：IPv4, IPv6, Unix
	Type       string  `json:"type"`        // 套接字类型：TCP, UDP, Unix
	LocalAddr  string  `json:"local_addr"`  // 本地地址
	RemoteAddr string  `json:"remote_addr"` // 远程地址
	Status     string  `json:"status"`      // 连接状态
	Uids       []int32 `json:"uids"`        // 用户ID列表
	Pid        int     `json:"pid"`         // 进程ID
}

// GetAllConnections 获取所有网络连接
// 使用本地 guard 包的实现
// Example:
// ```
// conns = hids.GetAllConnections()
//
//	for conn in conns {
//	    println(conn.Pid, conn.LocalAddr, conn.RemoteAddr, conn.Status)
//	}
//
// ```
func GetAllConnections() []*ConnectionInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	netConns, err := guard.GetAllConns(ctx)
	if err != nil {
		log.Errorf("get connections failed: %v", err)
		return nil
	}

	var conns []*ConnectionInfo
	for _, netConn := range netConns {
		conn := netConnToConnectionInfo(netConn)
		conns = append(conns, conn)
	}

	return conns
}

// netConnToConnectionInfo 将 guard.NetConn 转换为 ConnectionInfo
func netConnToConnectionInfo(netConn *guard.NetConn) *ConnectionInfo {
	return &ConnectionInfo{
		Fd:         netConn.Fd,
		Family:     netConn.Family,
		Type:       netConn.Type,
		LocalAddr:  netConn.LocalAddr,
		RemoteAddr: netConn.RemoteAddr,
		Status:     netConn.Status,
		Uids:       netConn.Uids,
		Pid:        netConn.Pid,
	}
}

// GetConnectionsByPid 根据进程ID获取连接信息
// Example:
// ```
// conns = hids.GetConnectionsByPid(1234)
//
//	for conn in conns {
//	    println(conn.LocalAddr, conn.RemoteAddr)
//	}
//
// ```
func GetConnectionsByPid(pid int) []*ConnectionInfo {
	allConns := GetAllConnections()
	var conns []*ConnectionInfo
	for _, conn := range allConns {
		if conn.Pid == pid {
			conns = append(conns, conn)
		}
	}
	return conns
}

// GetConnectionCount 获取当前连接数量
// Example:
// ```
// count = hids.GetConnectionCount()
// println(count)
// ```
func GetConnectionCount() int {
	conns := GetAllConnections()
	return len(conns)
}
