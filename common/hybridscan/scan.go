package hybridscan

import (
	"context"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net"
)

//func (h *HyperScanCenter) SyncScan(ctx context.Context, target, port string, shuffle bool) {
//	hosts := hostsparser.NewHostsParser(ctx, target)
//	ports := utils.ParseStringToPorts(port)
//
//	total := hosts.Size() * len(ports)
//	config := filter.NewDefaultConfig()
//	targetsFilter := filter.NewStringFilter(config, filter.NewGenericCuckoo())
//
//	err := h.synScanner.WaitOpenPortAsync(ctx, func(ip net.IP, port int) {
//		targetsFilter.Exist()
//	})
//}

func (h *HyperScanCenter) WaitWriteChannelEmpty() {
	h.synScanner.WaitChannelEmpty()
}

func (h *HyperScanCenter) Scan(
	ctx context.Context, target string, port string,
	shuffle bool,
	noWait bool,
	openPortCallback func(ip net.IP, port int),
) error {
	hostFilter := utils.NewHostsFilter(target)
	portFilter := utils.NewPortsFilter(port)

	//cacher := new(sync.Map)
	addrFilter := filter.NewFilter()
	err := h.synScanner.WaitOpenPortAsync(ctx, func(ip net.IP, port int) {
		addr := utils.HostPort(ip.String(), port)
		if addrFilter.Exist(addr) {
			return
		}
		addrFilter.Insert(addr)

		if !(hostFilter.Contains(ip.String()) && portFilter.Contains(port)) {
			return
		}

		openPortCallback(ip, port)
		if h.config.DisableFingerprintMatch {
			return
		}

		select {
		case h.fpTargetStream <- &fp.PoolTask{
			Host: ip.String(),
			Port: port,
		}:
		default:
			log.Errorf("fingerprint buffer is filled")
		}
	})
	if err != nil {
		return errors.Errorf("syn scan port register callback failed: %s", err)
	}

	utils.Debug(func() {
		log.Infof("start to submit tasks for synscanner: %s port: %s", target, port)
	})
	if shuffle {
		err = h.synScanner.RandomScan(target, port, noWait)
	} else {
		err = h.synScanner.Scan(target, port, noWait)
	}
	if err != nil {
		return errors.Errorf("scan failed: %s", err)
	}

	return nil
}

func (h *HyperScanCenter) SubmitOpenPortScanTask(target string, port string, shuffle bool, noWait bool) error {
	var err error
	if shuffle {
		err = h.synScanner.RandomScan(target, port, noWait)
	} else {
		err = h.synScanner.Scan(target, port, noWait)
	}
	if err != nil {
		return errors.Errorf("submit task failed: %s", err)
	}

	return nil
}

func (h *HyperScanCenter) SubmitFingerprintMatchTask(ip net.IP, port int, async bool) {
	if !async {
		select {
		case h.fpTargetStream <- &fp.PoolTask{
			Host: ip.String(),
			Port: port,
		}:
		}
		return
	}

	select {
	case h.fpTargetStream <- &fp.PoolTask{
		Host: ip.String(),
		Port: port,
	}:
	default:
		log.Errorf("fingerprint buffer is full, drop: %v", utils.HostPort(ip.String(), port))
		return
	}
}
