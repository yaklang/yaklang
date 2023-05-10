package hids

var Exports = map[string]interface{}{
	// 基础设置
	"Init":                InitHealthManager,
	"SetMonitorInterval":  SetMonitorIntervalFloat,
	"ShowMonitorInterval": ShowMonitorInterval,

	// CPU 指标
	"CPUPercent":            CPUPercent,
	"MemoryPercent":         MemoryPercent,
	"CPUPercentCallback":    CPUPercentCallback,
	"CPUAverageCallback":    CPUAverageCallback,
	"MemoryPercentCallback": MemoryPercentCallback,
}
