package tests

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"time"
)

// Alert 告警信息结构
type Alert struct {
	ID          string    `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	Severity    string    `json:"severity"`
	Type        string    `json:"type"`
	Source      string    `json:"source"`
	Destination string    `json:"destination"`
	Message     string    `json:"message"`
	IsNoise     bool      `json:"is_noise"` // 标记是否为噪声，用于测试验证
	Tags        []string  `json:"tags"`
}

// AlertGeneratorConfig 告警生成器配置
type AlertGeneratorConfig struct {
	TotalCount      int     // 总告警数量
	NoiseRatio      float64 // 噪声比例 0.0-1.0
	OutputFile      string  // 输出文件路径
	TimeSpanMinutes int     // 时间跨度（分钟）
}

// GenerateTestAlerts 生成测试用的告警数据
func GenerateTestAlerts(config AlertGeneratorConfig) error {
	rand.Seed(time.Now().UnixNano())

	alerts := make([]Alert, 0, config.TotalCount)
	noiseCount := int(float64(config.TotalCount) * config.NoiseRatio)
	realAlertCount := config.TotalCount - noiseCount

	baseTime := time.Now().Add(-time.Duration(config.TimeSpanMinutes) * time.Minute)

	// 生成真实告警
	for i := 0; i < realAlertCount; i++ {
		alert := generateRealAlert(i, baseTime, config.TimeSpanMinutes)
		alerts = append(alerts, alert)
	}

	// 生成噪声告警
	for i := 0; i < noiseCount; i++ {
		alert := generateNoiseAlert(i+realAlertCount, baseTime, config.TimeSpanMinutes)
		alerts = append(alerts, alert)
	}

	// 随机打乱顺序
	rand.Shuffle(len(alerts), func(i, j int) {
		alerts[i], alerts[j] = alerts[j], alerts[i]
	})

	// 写入文件
	return writeAlertsToFile(alerts, config.OutputFile)
}

// generateRealAlert 生成真实告警
func generateRealAlert(id int, baseTime time.Time, timeSpan int) Alert {
	alertTypes := []struct {
		typ      string
		severity string
		message  string
		tags     []string
	}{
		{"SQL_INJECTION", "high", "Detected SQL injection attempt in parameter 'id'", []string{"web", "injection", "database"}},
		{"XSS_ATTACK", "high", "Cross-site scripting detected in user input", []string{"web", "xss", "client-side"}},
		{"BRUTE_FORCE", "critical", "Multiple failed login attempts from same IP", []string{"authentication", "brute-force"}},
		{"MALWARE_DETECTED", "critical", "Suspicious file upload detected", []string{"malware", "file-upload"}},
		{"UNAUTHORIZED_ACCESS", "high", "Unauthorized access attempt to admin panel", []string{"access-control", "privilege"}},
		{"DATA_EXFILTRATION", "critical", "Large data transfer to external IP detected", []string{"data-loss", "network"}},
		{"PORT_SCAN", "medium", "Port scanning activity detected from external IP", []string{"reconnaissance", "network"}},
		{"COMMAND_INJECTION", "high", "OS command injection attempt detected", []string{"injection", "system"}},
		{"PATH_TRAVERSAL", "high", "Directory traversal attempt in file path", []string{"web", "file-system"}},
		{"DDOS_ATTACK", "critical", "Distributed denial of service attack detected", []string{"availability", "network"}},
	}

	alertType := alertTypes[rand.Intn(len(alertTypes))]
	offset := rand.Intn(timeSpan)

	return Alert{
		ID:          fmt.Sprintf("REAL-%d", id),
		Timestamp:   baseTime.Add(time.Duration(offset) * time.Minute),
		Severity:    alertType.severity,
		Type:        alertType.typ,
		Source:      generateRandomIP(),
		Destination: generateInternalIP(),
		Message:     alertType.message,
		IsNoise:     false,
		Tags:        alertType.tags,
	}
}

// generateNoiseAlert 生成噪声告警
func generateNoiseAlert(id int, baseTime time.Time, timeSpan int) Alert {
	noiseTypes := []struct {
		typ      string
		severity string
		message  string
		tags     []string
	}{
		// 重复性噪声
		{"HEALTH_CHECK", "info", "Health check request from monitoring system", []string{"monitoring", "routine"}},
		{"BACKUP_ROUTINE", "info", "Scheduled backup operation completed", []string{"backup", "routine"}},
		{"LOG_ROTATION", "info", "Log rotation process executed", []string{"maintenance", "routine"}},

		// 误报
		{"FALSE_POSITIVE_SQL", "low", "Legitimate database query flagged as suspicious", []string{"false-positive", "database"}},
		{"SCANNER_ACTIVITY", "low", "Security scanner routine check", []string{"scanner", "internal"}},
		{"API_RATE_LIMIT", "low", "API rate limit warning (within threshold)", []string{"api", "rate-limit"}},

		// 低优先级事件
		{"USER_AGENT_ANOMALY", "info", "Unusual user agent string detected", []string{"anomaly", "low-priority"}},
		{"SLOW_QUERY", "info", "Database query execution time exceeded threshold", []string{"performance", "database"}},
		{"CACHE_MISS", "info", "High cache miss rate detected", []string{"performance", "cache"}},

		// 已知良性行为
		{"ADMIN_LOGIN", "info", "Administrator login from known IP", []string{"authentication", "admin"}},
		{"SYSTEM_UPDATE", "info", "System update check performed", []string{"maintenance", "update"}},
		{"SSL_CERT_CHECK", "info", "SSL certificate validation routine", []string{"ssl", "routine"}},

		// 测试活动
		{"PENETRATION_TEST", "low", "Authorized penetration testing activity", []string{"testing", "authorized"}},
		{"QA_TESTING", "info", "QA team testing environment", []string{"testing", "qa"}},

		// 重复告警
		{"DISK_SPACE_WARNING", "low", "Disk space usage at 70% (repeated)", []string{"resource", "repeated"}},
		{"MEMORY_WARNING", "low", "Memory usage elevated (repeated)", []string{"resource", "repeated"}},
	}

	noiseType := noiseTypes[rand.Intn(len(noiseTypes))]
	offset := rand.Intn(timeSpan)

	// 某些噪声会重复出现
	idSuffix := id
	if rand.Float64() < 0.3 { // 30% 的噪声是重复的
		idSuffix = rand.Intn(100)
	}

	return Alert{
		ID:          fmt.Sprintf("NOISE-%d", idSuffix),
		Timestamp:   baseTime.Add(time.Duration(offset) * time.Minute),
		Severity:    noiseType.severity,
		Type:        noiseType.typ,
		Source:      generateRandomIP(),
		Destination: generateInternalIP(),
		Message:     noiseType.message,
		IsNoise:     true,
		Tags:        noiseType.tags,
	}
}

// generateRandomIP 生成随机IP地址
func generateRandomIP() string {
	return fmt.Sprintf("%d.%d.%d.%d",
		rand.Intn(256),
		rand.Intn(256),
		rand.Intn(256),
		rand.Intn(256))
}

// generateInternalIP 生成内部IP地址
func generateInternalIP() string {
	return fmt.Sprintf("192.168.%d.%d",
		rand.Intn(256),
		rand.Intn(256))
}

// writeAlertsToFile 将告警写入文件
func writeAlertsToFile(alerts []Alert, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer file.Close()

	// 根据文件扩展名决定输出格式
	if len(filename) > 5 && filename[len(filename)-5:] == ".json" {
		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")
		return encoder.Encode(alerts)
	}

	// 默认输出为日志格式
	return writeAlertsAsLog(file, alerts)
}

// writeAlertsAsLog 将告警写入日志格式
func writeAlertsAsLog(file *os.File, alerts []Alert) error {
	for _, alert := range alerts {
		// 格式：[时间戳] [严重程度] [类型] src=源IP dst=目标IP id=告警ID tags=标签 msg="消息内容"
		logLine := fmt.Sprintf("[%s] [%s] [%s] src=%s dst=%s id=%s tags=%s msg=\"%s\"\n",
			alert.Timestamp.Format("2006-01-02 15:04:05"),
			formatSeverity(alert.Severity),
			alert.Type,
			alert.Source,
			alert.Destination,
			alert.ID,
			formatTags(alert.Tags),
			alert.Message,
		)

		_, err := file.WriteString(logLine)
		if err != nil {
			return fmt.Errorf("failed to write log line: %v", err)
		}
	}
	return nil
}

// formatSeverity 格式化严重程度，使其对齐
func formatSeverity(severity string) string {
	severityMap := map[string]string{
		"critical": "CRITICAL",
		"high":     "HIGH    ",
		"medium":   "MEDIUM  ",
		"low":      "LOW     ",
		"info":     "INFO    ",
	}
	if formatted, ok := severityMap[severity]; ok {
		return formatted
	}
	return severity
}

// formatTags 格式化标签列表
func formatTags(tags []string) string {
	if len(tags) == 0 {
		return "[]"
	}
	result := "["
	for i, tag := range tags {
		if i > 0 {
			result += ","
		}
		result += tag
	}
	result += "]"
	return result
}

// GenerateDefaultTestAlerts 使用默认配置生成测试告警
func GenerateDefaultTestAlerts(outputFile string) error {
	config := AlertGeneratorConfig{
		TotalCount:      1000,
		NoiseRatio:      0.6, // 60% 噪声
		OutputFile:      outputFile,
		TimeSpanMinutes: 1440, // 24小时
	}
	return GenerateTestAlerts(config)
}

// WriteAlertsAsJSON 将告警写入JSON格式文件
func WriteAlertsAsJSON(alerts []Alert, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(alerts)
}
