//go:build windows
// +build windows

package hidsevent

//
//func ConvertGopsUtilProcessToProcessMeta(c context.Context, p *process.Process) *ProcessMeta {
//	var err error
//	var cpuPercent float64
//	var memPercent float32
//
//	data := &ProcessMeta{}
//	data.Pid = p.Pid
//
//	data.ProcessName, _ = p.Name()
//	data.CommandLine, _ = p.CmdlineWithContext(c)
//	if parent, err := p.ParentWithContext(c); err == nil {
//		data.ParentPid = parent.Pid
//	}
//
//	data.Status, _ = p.Status()
//	data.Username, _ = p.Username()
//	//cpuPercent, err = p.CPUPercentWithContext(c)
//	//if err != nil {
//	//	log.Warn(err)
//	//}
//	//
//	//memPercent, err = p.MemoryPercentWithContext(c)
//	//if err != nil {
//	//	log.Warn(err)
//	//}
//	//
//	//data.CPUPercent = cpuPercent
//	//data.MemoryPercent = float64(memPercent)
//
//	return data
//}
