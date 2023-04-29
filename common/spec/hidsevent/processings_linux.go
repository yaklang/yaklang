//go:build linux || freebsd || openbsd || darwin
// +build linux freebsd openbsd darwin

package hidsevent

//type ProcessData struct {
//	Pid            int32 `json:"pid"`
//	Name           string
//	Status         string
//	Parent         int32
//	NumCtxSwitches *process.NumCtxSwitchesStat
//	Uids           []int32
//	Gids           []int32
//	NumThreads     int32
//	MemInfo        *process.MemoryInfoStat
//	SigInfo        *process.SignalInfoStat
//	CreateTime     int64
//
//	//LastCPUTimes *cpu.TimesStat
//	//LastCPUTime  time.Time
//
//	Tgid int32
//}
//
//func (p *ProcessData) fillFromStatusWithContext(ctx context.Context) error {
//	pid := p.Pid
//	statPath := path.Join("/proc", strconv.Itoa(int(pid)), "status")
//	//log.Info(statPath)
//	contents, err := ioutil.ReadFile(statPath)
//	if err != nil {
//		return err
//	}
//	lines := strings.Split(string(contents), "\n")
//	p.NumCtxSwitches = &process.NumCtxSwitchesStat{}
//	p.MemInfo = &process.MemoryInfoStat{}
//	p.SigInfo = &process.SignalInfoStat{}
//	for _, line := range lines {
//		tabParts := strings.SplitN(line, "\t", 2)
//		if len(tabParts) < 2 {
//			continue
//		}
//		value := tabParts[1]
//		switch strings.TrimRight(tabParts[0], ":") {
//		case "Name":
//			p.Name = strings.Trim(value, " \t")
//
//		case "State":
//			p.Status = value[0:1]
//		case "PPid", "Ppid":
//			pval, err := strconv.ParseInt(value, 10, 32)
//			if err != nil {
//				return err
//			}
//			p.Parent = int32(pval)
//		case "Tgid":
//			pval, err := strconv.ParseInt(value, 10, 32)
//			if err != nil {
//				return err
//			}
//			p.Tgid = int32(pval)
//		case "Uid":
//			p.Uids = make([]int32, 0, 4)
//			for _, i := range strings.Split(value, "\t") {
//				v, err := strconv.ParseInt(i, 10, 32)
//				if err != nil {
//					return err
//				}
//				p.Uids = append(p.Uids, int32(v))
//			}
//		case "Gid":
//			p.Gids = make([]int32, 0, 4)
//			for _, i := range strings.Split(value, "\t") {
//				v, err := strconv.ParseInt(i, 10, 32)
//				if err != nil {
//					return err
//				}
//				p.Gids = append(p.Gids, int32(v))
//			}
//		case "Threads":
//			v, err := strconv.ParseInt(value, 10, 32)
//			if err != nil {
//				return err
//			}
//			p.NumThreads = int32(v)
//		case "voluntary_ctxt_switches":
//			v, err := strconv.ParseInt(value, 10, 64)
//			if err != nil {
//				return err
//			}
//			p.NumCtxSwitches.Voluntary = v
//		case "nonvoluntary_ctxt_switches":
//			v, err := strconv.ParseInt(value, 10, 64)
//			if err != nil {
//				return err
//			}
//			p.NumCtxSwitches.Involuntary = v
//		case "VmRSS":
//			value := strings.Trim(value, " kB") // remove last "kB"
//			v, err := strconv.ParseUint(value, 10, 64)
//			if err != nil {
//				return err
//			}
//			p.MemInfo.RSS = v * 1024
//		case "VmSize":
//			value := strings.Trim(value, " kB") // remove last "kB"
//			v, err := strconv.ParseUint(value, 10, 64)
//			if err != nil {
//				return err
//			}
//			p.MemInfo.VMS = v * 1024
//		case "VmSwap":
//			value := strings.Trim(value, " kB") // remove last "kB"
//			v, err := strconv.ParseUint(value, 10, 64)
//			if err != nil {
//				return err
//			}
//			p.MemInfo.Swap = v * 1024
//		case "VmHWM":
//			value := strings.Trim(value, " kB") // remove last "kB"
//			v, err := strconv.ParseUint(value, 10, 64)
//			if err != nil {
//				return err
//			}
//			p.MemInfo.HWM = v * 1024
//		case "VmData":
//			value := strings.Trim(value, " kB") // remove last "kB"
//			v, err := strconv.ParseUint(value, 10, 64)
//			if err != nil {
//				return err
//			}
//			p.MemInfo.Data = v * 1024
//		case "VmStk":
//			value := strings.Trim(value, " kB") // remove last "kB"
//			v, err := strconv.ParseUint(value, 10, 64)
//			if err != nil {
//				return err
//			}
//			p.MemInfo.Stack = v * 1024
//		case "VmLck":
//			value := strings.Trim(value, " kB") // remove last "kB"
//			v, err := strconv.ParseUint(value, 10, 64)
//			if err != nil {
//				return err
//			}
//			p.MemInfo.Locked = v * 1024
//		case "SigPnd":
//			v, err := strconv.ParseUint(value, 16, 64)
//			if err != nil {
//				return err
//			}
//			p.SigInfo.PendingThread = v
//		case "ShdPnd":
//			v, err := strconv.ParseUint(value, 16, 64)
//			if err != nil {
//				return err
//			}
//			p.SigInfo.PendingProcess = v
//		case "SigBlk":
//			v, err := strconv.ParseUint(value, 16, 64)
//			if err != nil {
//				return err
//			}
//			p.SigInfo.Blocked = v
//		case "SigIgn":
//			v, err := strconv.ParseUint(value, 16, 64)
//			if err != nil {
//				return err
//			}
//			p.SigInfo.Ignored = v
//		case "SigCgt":
//			v, err := strconv.ParseUint(value, 16, 64)
//			if err != nil {
//				return err
//			}
//			p.SigInfo.Caught = v
//		}
//
//	}
//	return nil
//}
//
//func ConvertGopsUtilProcessToProcessMeta(c context.Context, p *process.Process) *ProcessMeta {
//
//	data := &ProcessMeta{}
//	processData := &ProcessData{Pid: p.Pid}
//	processData.fillFromStatusWithContext(c)
//
//	data.Pid = processData.Pid
//	data.ProcessName = processData.Name
//	data.CommandLine, _ = p.CmdlineWithContext(c)
//
//	data.ParentPid = processData.Parent
//	data.Status = processData.Status
//
//	cP, err := p.CPUPercentWithContext(c)
//	if err != nil {
//		log.Warn(err)
//	}
//
//	mP, err := p.MemoryPercentWithContext(c)
//	if err != nil {
//		log.Warn(err)
//	}
//
//	data.CPUPercent = cP
//	data.MemoryPercent = float64(mP)
//
//	return data
//}
