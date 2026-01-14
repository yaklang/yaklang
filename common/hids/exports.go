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

	// ==================== 进程信息收集模块 ====================
	// 进程列表获取
	"PS":                    PS,                    // 获取进程列表，支持过滤
	"GetProcessByPid":       GetProcessByPid,       // 根据PID获取进程详细信息
	"GetCurrentProcessInfo": GetCurrentProcessInfo, // 获取当前进程信息
	"ProcessExists":         ProcessExists,         // 检查进程是否存在

	// 进程过滤
	"NewProcessFilter": NewProcessFilter, // 创建进程过滤器

	// 进程父子关系识别
	"GetProcessChildren":  GetProcessChildren,  // 获取子进程列表
	"GetProcessParent":    GetProcessParent,    // 获取父进程信息
	"GetProcessTree":      GetProcessTree,      // 获取进程树
	"GetProcessAncestors": GetProcessAncestors, // 获取所有祖先进程

	// 进程操作
	"KillProcess": KillProcess, // 终止进程

	// ==================== 进程行为监控模块 ====================
	// 进程监控器
	"NewProcessMonitor":          NewProcessMonitor,          // 创建进程监控器
	"WithProcessMonitorInterval": WithProcessMonitorInterval, // 设置监控间隔
	"WithOnProcessCreate":        WithOnProcessCreate,        // 设置进程创建回调
	"WithOnProcessExit":          WithOnProcessExit,          // 设置进程退出回调
	"WithWhitelist":              WithWhitelist,              // 设置白名单规则
	"WatchProcess":               WatchProcess,               // 简单进程监控函数

	// 白名单规则
	"NewWhitelistRule": NewWhitelistRule, // 创建白名单规则

	// 文件哈希计算
	"GetFileHashMD5":    GetFileHashMD5,    // 获取文件MD5哈希
	"GetFileHashSHA256": GetFileHashSHA256, // 获取文件SHA256哈希

	// ==================== 连接状态监控模块 ====================
	// 连接列表获取
	"Netstat":                   Netstat,                   // 获取网络连接列表
	"GetTCPConnections":         GetTCPConnections,         // 获取TCP连接
	"GetUDPConnections":         GetUDPConnections,         // 获取UDP连接
	"GetListeningPorts":         GetListeningPorts,         // 获取监听端口
	"GetEstablishedConnections": GetEstablishedConnections, // 获取已建立连接
	"GetConnectionsByPid":       GetConnectionsByPid,       // 获取指定进程连接
	"GetConnectionsByPort":      GetConnectionsByPort,      // 获取指定端口连接
	"GetConnectionStats":        GetConnectionStats,        // 获取连接统计

	// 连接过滤
	"NewConnectionFilter": NewConnectionFilter, // 创建连接过滤器

	// 连接监控器
	"NewConnectionMonitor":          NewConnectionMonitor,          // 创建连接监控器
	"WithConnectionMonitorInterval": WithConnectionMonitorInterval, // 设置监控间隔
	"WithConnectionFilter":          WithConnectionFilter,          // 设置连接过滤器
	"WithOnNewConnection":           WithOnNewConnection,           // 设置新连接回调
	"WithOnConnectionDisappear":     WithOnConnectionDisappear,     // 设置连接消失回调
	"WithConnectionHistory":         WithConnectionHistory,         // 启用历史记录
	"WatchConnections":              WatchConnections,              // 简单连接监控函数

	// ==================== Linux Audit 监控模块 ====================
	// Audit监控器 - 基于Linux audit子系统进行用户行为审计
	"NewAuditMonitor":         NewAuditMonitor,         // 创建Audit监控器
	"WithAuditMonitorLogin":   WithAuditMonitorLogin,   // 设置是否监控登录事件
	"WithAuditMonitorCommand": WithAuditMonitorCommand, // 设置是否监控命令执行事件
	"WithOnLoginEvent":        WithOnLoginEvent,        // 设置登录事件回调
	"WithOnCommandEvent":      WithOnCommandEvent,      // 设置命令执行事件回调
	"WithAuditFilterUsers":    WithAuditFilterUsers,    // 设置用户过滤器
	"WithAuditFilterCommands": WithAuditFilterCommands, // 设置命令过滤器
	"WithAuditBufferSize":     WithAuditBufferSize,     // 设置缓冲区大小
	"WatchAuditEvents":        WatchAuditEvents,        // 简化的audit监控函数
}
