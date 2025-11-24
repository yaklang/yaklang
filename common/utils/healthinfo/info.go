package healthinfo

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/shirou/gopsutil/v4/cpu"
	_ "github.com/shirou/gopsutil/v4/disk"
	_ "github.com/shirou/gopsutil/v4/docker"
	_ "github.com/shirou/gopsutil/v4/host"
	_ "github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/spec/health"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var (
	// 缓存上次采样的网络 IO
	lastCheckNetIO       time.Time
	lastKBytesNetRecvAll uint64
	lastKBytesNetSendAll uint64

	// 缓存上次采样的硬盘 IO 数据
	lastCheckDiskIO        time.Time
	lastKBytesDiskWriteAll uint64
	lastKBytesDiskReadAll  uint64
)

func toRound2(f float64) float64 {
	r, _ := strconv.ParseFloat(fmt.Sprintf("%.2f", f), 64)
	return r
}

func runTop(ctx context.Context) (*health.HealthInfo, error) {
	detail := &health.HealthInfo{
		Timestamp:       time.Now().Unix(),
		CPUPercent:      0,
		MemoryPercent:   0,
		NetworkUpload:   0,
		NetworkDownload: 0,
		DiskWrite:       0,
		DiskRead:        0,
		DiskUsage:       nil,
	}

	switch runtime.GOOS {
	case "darwin":
		/*
			Processes: 539 total, 2 running, 537 sleeping, 3391 threads
			2020/11/17 15:23:19
			Load Avg: 3.58, 2.90, 3.06
			CPU usage: 4.78% user, 10.10% sys, 85.10% idle
			SharedLibs: 336M resident, 73M data, 45M linkedit.
			MemRegions: 198984 total, 7757M resident, 225M private, 8187M shared.
			PhysMem: 30G used (5393M wired), 2521M unused.
			VM: 3786G vsize, 1993M framework vsize, 7146(0) swapins, 9384(0) swapouts.
			Networks: packets: 1858195/1324M in, 2421504/719M out.
			Disks: 1079068/22G read, 267894/5420M written.
		*/
		cmd := exec.CommandContext(
			ctx,
			"top", "-l", "1", "-n", "0", // 只显示一次，并且不开放进程信息
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return nil, err
		}
		s := bufio.NewScanner(bytes.NewBuffer(output))
		s.Split(bufio.ScanLines)

		for s.Scan() {
			line := strings.TrimSpace(strings.ToLower(s.Text()))
			switch true {
			case strings.HasPrefix(line, "cpu usage:"):
				idlePercent := yaklib.Grok(s.Text(), `%{BASE10NUM:user}\%.*?%{BASE10NUM:sys}\%.*?%{BASE10NUM:idle}\%`).Get("idle")
				p, err := strconv.ParseFloat(idlePercent, 64)
				if err != nil {
					return nil, utils.Errorf("parse idle percent[%v] failed: %v", idlePercent, err)
				}
				detail.CPUPercent = 100 - p
			case strings.HasPrefix(line, "physmem:"):
				params := yaklib.Grok(s.Text(), `PhysMem: ?%{DATA:used} ?used ?\(%{DATA:wired} wired\),? ?%{DATA:unused} unuse`)
				used, err := utils.ToBytes(params.Get("used"))
				if err != nil {
					return nil, utils.Errorf("used mem bytes[%s] calc failed: %s", params.Get("used"), err)
				}

				wired, err := utils.ToBytes(params.GetOr("wired", "0M"))
				if err != nil {
					return nil, utils.Errorf("wired mem bytes[%s] calc failed: %s", params.Get("wired"), err)
				}

				unused, err := utils.ToBytes(params.Get("unused"))
				if err != nil {
					return nil, utils.Errorf("unused mem bytes[%s] calc failed: %s", params.Get("unused"), err)
				}
				detail.MemoryPercent = toRound2((float64(used-wired) / float64(used+unused)) * 100)
			case strings.HasPrefix(line, "disks"):
				params := yaklib.Grok(s.Text(), `Disks: ?%{DATA:read}/%{DATA} ?read,? ?%{DATA:write}/%{DATA} ?written`)
				readBytes, writeBytes := params.Get("read"), params.Get("write")
				read, err := strconv.ParseUint(readBytes, 10, 64)
				if err != nil {
					return nil, utils.Errorf("parse disk read failed[%s]: %s", readBytes, err)
				}

				write, err := strconv.ParseUint(writeBytes, 10, 64)
				if err != nil {
					return nil, utils.Errorf("parse disk write failed[%s]; %s", writeBytes, err)
				}

				rKb := read * 4096
				wKb := write * 4096
				if !lastCheckNetIO.IsZero() {
					interval := time.Now().Sub(lastCheckDiskIO)
					detail.DiskRead = toRound2(float64(rKb-lastKBytesDiskReadAll) / interval.Seconds())
					detail.DiskWrite = toRound2(float64(wKb-lastKBytesDiskWriteAll) / interval.Seconds())
				}
				lastCheckDiskIO = time.Now()
				lastKBytesDiskWriteAll = wKb
				lastKBytesDiskReadAll = rKb
			}
		}
		return detail, nil
	case "linux":
		/*
			top - 23:21:21 up 23 days, 57 min,  1 user,  load average: 0.20, 0.31, 0.25
			Tasks:   1 total,   0 running,   1 sleeping,   0 stopped,   0 zombie
			%Cpu(s):  0.1 us,  0.0 sy,  0.0 ni, 99.9 id,  0.0 wa,  0.0 hi,  0.0 si,  0.0 st
			KiB Mem : 16370144 total,  7391584 free,  2737464 used,  6241096 buff/cache
			KiB Swap:  2097148 total,  2097148 free,        0 used. 13208220 avail Mem

			   PID USER      PR  NI    VIRT    RES    SHR S  %CPU %MEM     TIME+ COMMAND
			     1 root      20   0  225860   9576   6660 S   0.0  0.1   0:43.27 systemd

		*/
		cmd := exec.CommandContext(
			ctx,
			"top", "-b", "-n", "1", "-p", "1", // 只显示一次
		)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return nil, err
		}
		s := bufio.NewScanner(bytes.NewBuffer(output))
		s.Split(bufio.ScanLines)

		for s.Scan() {
			line := strings.TrimSpace(strings.ToLower(s.Text()))
			switch true {
			case strings.Contains(line, "cpu(s)"):
				idlePercent := yaklib.Grok(s.Text(), `%{BASE10NUM:idle} id`).Get("idle")
				p, err := strconv.ParseFloat(idlePercent, 64)
				if err != nil {
					return nil, utils.Errorf("parse idle percent[%v] failed: %v", idlePercent, err)
				}
				detail.CPUPercent = toRound2(100 - p)
			case strings.HasPrefix(line, "kib mem"):
				params := yaklib.Grok(s.Text(), `%{BASE10NUM:total} total.*?%{BASE10NUM:free} free.*?%{BASE10NUM:used} used`)
				var (
					free, total uint64
					err         error
				)
				free, err = strconv.ParseUint(params.Get("free"), 10, 64)
				if err != nil {
					free, err = utils.ToBytes(params.Get("free"))
					if err != nil {
						return nil, utils.Errorf("free mem bytes[%s] calc failed: %s", params.Get("used"), err)
					}
				}

				total, err = strconv.ParseUint(params.Get("total"), 10, 64)
				if err != nil {
					total, err = utils.ToBytes(params.Get("total"))
					if err != nil {
						return nil, utils.Errorf("total mem bytes[%s] calc failed: %s", params.Get("total"), err)
					}
				}

				if total > 0 && total-free > 0 {
					detail.MemoryPercent = toRound2((float64(total) - float64(free)/float64(total)) * 100)
				}
			}
		}

		if stats, _ := ReadDiskstats(); len(stats) > 0 {
			var (
				rAll uint64
				wAll uint64
			)
			for _, stat := range stats {
				rAll += stat.ReadSectors * 512  // *SectorSize
				wAll += stat.WriteSectors * 512 // *SectorSize
			}

			rKb := rAll / 1000
			wKb := wAll / 1000
			if !lastCheckNetIO.IsZero() {
				interval := time.Now().Sub(lastCheckDiskIO)
				detail.DiskRead = toRound2(float64(rKb-lastKBytesDiskReadAll) / interval.Seconds())
				detail.DiskWrite = toRound2(float64(wKb-lastKBytesDiskWriteAll) / interval.Seconds())
			}
			lastCheckDiskIO = time.Now()
			lastKBytesDiskWriteAll = wKb
			lastKBytesDiskReadAll = rKb

		}

		return detail, nil
	default:
		return nil, utils.Errorf("unsupported os: %s", runtime.GOOS)
	}
}

func healthInfoFromGopsutil() (*health.HealthInfo, error) {
	detail := &health.HealthInfo{
		Timestamp: time.Now().Unix(),
	}
	cpuProfiles, err := cpu.Percent(0, false)
	if err != nil {
		return nil, utils.Errorf("gopsutil cannot fetch cpu percent: %s", err)
	}
	if len(cpuProfiles) > 0 {
		var avg float64 = cpuProfiles[0]
		for _, p := range cpuProfiles[1:] {
			if p > 0 {
				avg = (avg + p) / 2.0
			}
		}
		detail.CPUPercent = toRound2(avg)
	}

	stat, err := mem.VirtualMemory()
	if err != nil {
		return nil, utils.Errorf("gopsutil cannot fetch mem percent: %s", err)
	}
	detail.MemoryPercent = stat.UsedPercent
	return detail, nil
}

// NewHealthInfo 获取系统健康信息
//
//	2023.5.10: TODO: disk rate is waiting for fixing;
func NewHealthInfo(ctx context.Context) (*health.HealthInfo, error) {
	// 硬盘读写暂时有点问题
	info, err := healthInfoFromGopsutil()
	if err != nil {
		log.Infof("gopsutil(v3) native cannot fetch info: %v, try by top...", err)
		info, err = runTop(ctx)
		if err != nil {
			return nil, utils.Errorf("no top running: reason: %s", err)
		}
	}

	if info == nil {
		return nil, utils.Errorf("fetch system health info failed: %s", err)
	}

	// 计算网络 IO
	var ioUpload float64 = 0
	var ioDownload float64 = 0
	if stats, err := net.IOCountersWithContext(ctx, true); err == nil && len(stats) > 0 {
		var (
			recvAll uint64
			sendAll uint64
		)
		for _, stat := range stats {
			recvAll += stat.BytesRecv
			sendAll += stat.BytesSent
		}

		recvKb := recvAll / 1000
		sendKb := sendAll / 1000
		if !lastCheckNetIO.IsZero() {
			interval := time.Now().Sub(lastCheckNetIO)
			recvKbPerSec := toRound2(float64(recvKb-lastKBytesNetRecvAll) / interval.Seconds())
			sendKbPerSec := toRound2(float64(sendKb-lastKBytesNetSendAll) / interval.Seconds())
			ioUpload, ioDownload = sendKbPerSec, recvKbPerSec
		}

		lastCheckNetIO = time.Now()
		lastKBytesNetSendAll = sendKb
		lastKBytesNetRecvAll = recvKb
	}

	var diskUsage = &health.DiskStat{
		Total: 0,
		Used:  0,
	}
	if stats, err := DiskUsageWithContext(ctx, "/"); err == nil {
		diskUsage.Used = stats.Used
		diskUsage.Total = stats.Total
	}

	info.Timestamp = time.Now().Unix()
	info.NetworkDownload = ioDownload
	info.NetworkUpload = ioUpload
	info.DiskUsage = diskUsage

	return info, nil
}
