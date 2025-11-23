package hids

var Exports = map[string]interface{}{
	// 基础设置
	"Init":                InitHealthManager,
	"SetMonitorInterval":  SetMonitorIntervalFloat,
	"ShowMonitorInterval": ShowMonitorInterval,

	// CPU 指标
	"CPUPercent":            CPUPercent,
	"MemoryPercent":         MemoryPercent,
	"CPUAverage":            CPUAverage,
	"CPUPercentCallback":    CPUPercentCallback,
	"CPUAverageCallback":    CPUAverageCallback,
	"MemoryPercentCallback": MemoryPercentCallback,

	// 进程监控
	"GetAllProcesses":          GetAllProcesses,
	"GetProcessByPid":          GetProcessByPid,
	"GetProcessCount":          GetProcessCount,
	"GetProcessOpenFiles":      GetProcessOpenFiles,
	"GetProcessOpenFilesCount": GetProcessOpenFilesCount,

	// 连接监控
	"GetAllConnections":   GetAllConnections,
	"GetConnectionsByPid": GetConnectionsByPid,
	"GetConnectionCount":  GetConnectionCount,
}
