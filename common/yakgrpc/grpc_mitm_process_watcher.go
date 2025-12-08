package yakgrpc

import (
	"context"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/sysproc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"time"
)

func (s *Server) WatchProcessConnection(stream ypb.Yak_WatchProcessConnectionServer) error {
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	firstRequest, err := stream.Recv()
	if err != nil {
		log.Errorf("failed to receive first request: %v", err)
		return err
	}

	startParams := firstRequest.GetStartParams()
	if startParams.CheckIntervalSeconds <= 0 {
		startParams.CheckIntervalSeconds = 3
	}

	var dnsWatcher *pcapx.PcapReserveDNSCache
	if !startParams.DisableReserveDNS {
		dnsWatcher, err = pcapx.StartReserveDNSCache(ctx)
		if err != nil {
			return err
		}
	}

	feedback := func(ActionType string, p *sysproc.ProcessBasicInfo) {
		stream.Send(&ypb.WatchProcessResponse{
			Action:  ActionType,
			Process: ProcessInfo2GRPC(p),
		})
	}

	processWatcher := sysproc.NewProcessesWatcher()
	processWatcher.Start(
		ctx,
		func(ctx context.Context, p *sysproc.ProcessBasicInfo) {
			feedback("start", p)
		},
		func(ctx context.Context, p *sysproc.ProcessBasicInfo) {
			feedback("exit", p)
		},
		time.Duration(startParams.CheckIntervalSeconds)*time.Second,
	)

	for {
		event, err := stream.Recv()
		if err != nil {
			log.Errorf("error receiving from stream: %v", err)
			return err
		}

		pid := event.GetQueryPid()
		if pid == 0 {
			continue
		}

		connections, err := processWatcher.DetectPublicProcessConnections(pid, -1)
		if err != nil {
			log.Errorf("error detecting process connections: %v", err)
			continue
		}

		connectionMap := make(map[string][]string)
		if dnsWatcher != nil {
			for _, connection := range connections {
				rAddr := utils.HostPort(connection.Raddr.IP, int(connection.Raddr.Port))

				if _, ok := connectionMap[rAddr]; !ok {
					connectionMap[rAddr] = make([]string, 0)
				}
			}
			for key, _ := range connectionMap {
				domainList := dnsWatcher.ReserveResolve(key)
				connectionMap[key] = domainList
			}
		}

		var connectionsGRPC []*ypb.ConnectionInfo
		for _, connection := range connections {
			var domainList []string
			if dnsWatcher != nil {
				domainList = connectionMap[utils.HostPort(connection.Raddr.IP, int(connection.Raddr.Port))]
			}
			connInfo := ConnInfo2GRPC(connection, domainList)
			connectionsGRPC = append(connectionsGRPC, connInfo)
		}
		err = stream.Send(&ypb.WatchProcessResponse{
			Action:      "refresh_connections",
			Process:     ProcessInfo2GRPC(&sysproc.ProcessBasicInfo{Pid: pid}),
			Connections: connectionsGRPC,
		})
		if err != nil {
			return err
		}
	}
}

func ProcessInfo2GRPC(p *sysproc.ProcessBasicInfo) *ypb.ProcessInfo {
	return &ypb.ProcessInfo{
		Pid:     int32(p.Pid),
		Name:    p.Name,
		Exe:     p.Exe,
		Cmdline: p.Cmdline,
	}
}

func ConnInfo2GRPC(c net.ConnectionStat, domainList []string) *ypb.ConnectionInfo {
	return &ypb.ConnectionInfo{
		Domain:        domainList,
		LocalAddress:  utils.HostPort(c.Laddr.IP, int(c.Laddr.Port)),
		RemoteAddress: utils.HostPort(c.Raddr.IP, int(c.Raddr.Port)),
		Status:        c.Status,
	}
}
