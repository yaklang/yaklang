package sfreport

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// 常量定义
const (
	// 风险等级
	severityCritical = "critical"
	severityHigh     = "high"
	severityMiddle   = "middle"
	severityLow      = "low"
	severityInfo     = "info"

	// 风险等级中文名称
	severityTextCritical = "严重"
	severityTextHigh     = "高危"
	severityTextMiddle   = "中危"
	severityTextLow      = "低危"
	severityTextInfo     = "信息"

	// 风险等级颜色
	colorCritical = "#8B0000"
	colorHigh     = "#FF4500"
	colorMiddle   = "#FFA500"
	colorLow      = "#FFD700"
	colorInfo     = "#90EE90"

	// 默认描述
	defaultProjectDescription = "暂无项目描述信息"

	// 漏洞详情显示限制，避免文档过大
	defaultRiskDetailsLimit = 100
	defaultCodeSegmentLimit = 1000
)

// SeverityInfo 风险等级信息
type SeverityInfo struct {
	Text  string
	Color string
}

// severityMap 风险等级映射表
var severityMap = map[string]SeverityInfo{
	severityCritical: {Text: severityTextCritical, Color: colorCritical},
	severityHigh:     {Text: severityTextHigh, Color: colorHigh},
	severityMiddle:   {Text: severityTextMiddle, Color: colorMiddle},
	severityLow:      {Text: severityTextLow, Color: colorLow},
	severityInfo:     {Text: severityTextInfo, Color: colorInfo},
}

// getSeverityInfo 获取风险等级信息
func getSeverityInfo(severity string) SeverityInfo {
	if info, exists := severityMap[severity]; exists {
		return info
	}
	return SeverityInfo{}
}

// SSAProjectReport SSA项目扫描报告
type SSAProjectReport struct {
	// 封面信息
	ProgramName string    `json:"program_name"`
	ReportTime  time.Time `json:"report_time"`

	// 项目信息
	Language      ssaconfig.Language `json:"language"`
	Description   string             `json:"description"`
	RepositoryURL string             `json:"repository_url"`
	FileCount     int                `json:"file_count"`
	CodeLineCount int                `json:"code_line_count"`
	ScanStartTime time.Time          `json:"scan_start_time"`
	ScanEndTime   time.Time          `json:"scan_end_time"`
	TotalRules    int                `json:"total_rules"`

	// 漏洞信息
	TotalRisksCount    int `json:"total_risks_count"`
	CriticalRisksCount int `json:"critical_risks_count"`
	HighRisksCount     int `json:"high_risks_count"`
	MiddleRisksCount   int `json:"middle_risks_count"`
	LowRisksCount      int `json:"low_risks_count"`

	// 详细风险列表
	Risks []*SSAReportRisk `json:"risks"`

	// 文件列表
	Files []*SSAReportFile `json:"files"`

	// 规则列表
	Rules         map[string]*SSAReportRule `json:"rules"`
	EngineVersion string                    `json:"engine_version"`

	// 标志：是否是从用户选择的RiskIDs生成的报告
	// 如果为true，表示报告数据来源于用户选择的Risk，项目信息字段会被简化
	FromRiskSelection bool `json:"from_risk_selection"`
}

// GetProjectInfo 获取项目信息结构体
func (r *SSAProjectReport) GetProjectInfo() *ProjectInfo {
	return &ProjectInfo{
		ProgramName:       r.ProgramName,
		Language:          r.Language,
		Description:       r.Description,
		RepositoryURL:     r.RepositoryURL,
		FileCount:         r.FileCount,
		CodeLineCount:     r.CodeLineCount,
		ScanStartTime:     r.ScanStartTime,
		ScanEndTime:       r.ScanEndTime,
		TotalRules:        r.TotalRules,
		FromRiskSelection: r.FromRiskSelection,
	}
}

// GetRiskStatistics 获取风险统计结构体
func (r *SSAProjectReport) GetRiskStatistics() *RiskStatistics {
	return &RiskStatistics{
		TotalRisksCount:    r.TotalRisksCount,
		CriticalRisksCount: r.CriticalRisksCount,
		HighRisksCount:     r.HighRisksCount,
		MiddleRisksCount:   r.MiddleRisksCount,
		LowRisksCount:      r.LowRisksCount,
	}
}

// GetRiskTypeOverview 获取风险类型概览结构体
func (r *SSAProjectReport) GetRiskTypeOverview() *RiskTypeOverview {
	return NewRiskTypeOverview(r.Risks)
}

// GetSortedRules 获取排序后的规则列表
func (r *SSAProjectReport) GetSortedRules() []*SSAReportRule {
	rules := make([]*SSAReportRule, 0, len(r.Rules))
	ruleNames := make([]string, 0, len(r.Rules))

	// 收集规则名称并排序
	for ruleName := range r.Rules {
		ruleNames = append(ruleNames, ruleName)
	}
	sort.Strings(ruleNames)

	// 按排序后的顺序构建规则列表
	for _, ruleName := range ruleNames {
		rules = append(rules, r.Rules[ruleName])
	}

	return rules
}

// SSAReportRisk SSA报告中的风险项
type SSAReportRisk struct {
	Title        string `json:"title"`
	TitleVerbose string `json:"title_verbose"`
	Description  string `json:"description"`
	Solution     string `json:"solution"`
	RiskType     string `json:"risk_type"`
	Severity     string `json:"severity"`
	FromRule     string `json:"from_rule"`

	// 位置信息
	CodeSourceUrl string `json:"code_source_url"`
	CodeRange     string `json:"code_range"`
	CodeFragment  string `json:"code_fragment"`
	FunctionName  string `json:"function_name"`
	Line          int64  `json:"line"`

	// 处置信息（来自最新的处置记录）
	LatestDisposalStatus  string `json:"latest_disposal_status"`
	LatestDisposalComment string `json:"latest_disposal_comment"`
	RiskID                uint   `json:"risk_id"` // 用于查询处置记录
}

// SSAReportFile SSA报告中的文件信息
type SSAReportFile struct {
	FilePath      string             `json:"file_path"`
	Language      ssaconfig.Language `json:"language"`
	LineCount     int                `json:"line_count"`
	RiskCount     int                `json:"risk_count"`
	CriticalCount int                `json:"critical_count"`
	HighCount     int                `json:"high_count"`
	MiddleCount   int                `json:"middle_count"`
	LowCount      int                `json:"low_count"`
}

// SSAReportRule SSA报告中的规则信息
type SSAReportRule struct {
	RuleName    string `json:"rule_name"`
	Title       string `json:"title"`
	TitleZh     string `json:"title_zh"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	RiskCount   int    `json:"risk_count"`
}

// RiskGroup 风险分组结构，用于将相同类型和等级的风险归组
type RiskGroup struct {
	RiskType     string          `json:"risk_type"`     // 风险类型
	Severity     string          `json:"severity"`      // 风险等级
	Title        string          `json:"title"`         // 漏洞标题
	TitleVerbose string          `json:"title_verbose"` // 详细标题
	Description  string          `json:"description"`   // 漏洞描述
	Solution     string          `json:"solution"`      // 修复建议
	FromRule     string          `json:"from_rule"`     // 来源规则
	Instances    []*RiskInstance `json:"instances"`     // 该分组下的所有风险实例
	Count        int             `json:"count"`         // 实例数量
}

// RiskInstance 风险实例，表示具体的风险位置
type RiskInstance struct {
	CodeSourceUrl         string `json:"code_source_url"`         // 文件路径
	CodeRange             string `json:"code_range"`              // 代码范围
	CodeFragment          string `json:"code_fragment"`           // 代码片段
	FunctionName          string `json:"function_name"`           // 函数名
	Line                  int64  `json:"line"`                    // 行号
	LatestDisposalStatus  string `json:"latest_disposal_status"`  // 处置状态
	LatestDisposalComment string `json:"latest_disposal_comment"` // 处置备注
}

// ProjectInfo 项目基本信息
type ProjectInfo struct {
	ProgramName       string
	Language          ssaconfig.Language
	Description       string
	RepositoryURL     string
	FileCount         int
	CodeLineCount     int
	ScanStartTime     time.Time
	ScanEndTime       time.Time
	TotalRules        int
	FromRiskSelection bool // 是否从Risk选择生成
}

// ToMarkdownTable 将项目信息转换为Markdown表格
func (p *ProjectInfo) ToMarkdownTable() string {
	// 如果是从用户选择的Risk生成，只显示项目名称和检测规则数
	if p.FromRiskSelection {
		return fmt.Sprintf(`# 一、检测项目信息
| 项目名称     | %s  |
| :------------: | :---: |
| 检测规则数   | %d  |`,
			p.ProgramName, p.TotalRules)
	}

	// 从任务生成的完整报告，显示所有项目信息
	baseTable := fmt.Sprintf(`# 一、检测项目信息
| 项目名称     | %s  |
| :------------: | :---: |
| 检测语言     | %s  |
| 项目描述     | %s  |
| 文件数       | %d  |
| 代码量       | %d  |
| 检测开始时间 | %s  |
| 检测结束时间 | %s  |
| 检测规则数   | %d  |`,
		p.ProgramName, p.Language, safeStr(p.Description),
		p.FileCount, p.CodeLineCount,
		p.ScanStartTime.Format("2006.01.02 15:04"),
		p.ScanEndTime.Format("2006.01.02 15:04"),
		p.TotalRules)

	// 如果有仓库地址，添加到表格中
	if p.RepositoryURL != "" {
		// 在文件数行之前插入仓库地址行
		baseTable = strings.Replace(baseTable, "| 文件数       |", "| 仓库地址     | "+p.RepositoryURL+"  |\n| 文件数       |", 1)
	}

	return baseTable
}

// RiskStatistics 风险统计信息
type RiskStatistics struct {
	TotalRisksCount    int
	CriticalRisksCount int
	HighRisksCount     int
	MiddleRisksCount   int
	LowRisksCount      int
}

// GetInfoRiskCount 计算信息级风险数量
func (r *RiskStatistics) GetInfoRiskCount() int {
	return r.TotalRisksCount - r.CriticalRisksCount - r.HighRisksCount - r.MiddleRisksCount - r.LowRisksCount
}

// CalcPercentage 计算百分比
func (r *RiskStatistics) CalcPercentage(amount int) string {
	if r.TotalRisksCount == 0 || amount == 0 {
		return "0%"
	}

	percentage := float64(amount) / float64(r.TotalRisksCount) * 100

	// 根据百分比大小选择合适的显示格式
	if percentage >= 10 {
		return fmt.Sprintf("%.0f%%", percentage) // 10% 及以上显示整数
	} else if percentage >= 1 {
		return fmt.Sprintf("%.1f%%", percentage) // 1-10% 显示一位小数
	} else {
		return "<1%" // 小于1% 显示为 "<1%"
	}
}

// ToMarkdownTable 将风险统计转换为Markdown表格
func (r *RiskStatistics) ToMarkdownTable() string {
	infoRiskCount := r.GetInfoRiskCount()
	return fmt.Sprintf(`# 二、漏洞统计
## 2.1 漏洞数量统计
| 等级 | 数量 | 占比 |
| ---- | ---- | ---- |
| 总数 | %d   |      |
| 严重 | %d   | %s |
| 高危 | %d   | %s |
| 中危 | %d   | %s |
| 低危 | %d   | %s |
| 信息 | %d   | %s |`,
		r.TotalRisksCount,
		r.CriticalRisksCount, r.CalcPercentage(r.CriticalRisksCount),
		r.HighRisksCount, r.CalcPercentage(r.HighRisksCount),
		r.MiddleRisksCount, r.CalcPercentage(r.MiddleRisksCount),
		r.LowRisksCount, r.CalcPercentage(r.LowRisksCount),
		infoRiskCount, r.CalcPercentage(infoRiskCount))
}

// ToEChartsNightingale 将风险统计转换为ECharts南丁格尔玫瑰图配置
func (r *RiskStatistics) ToEChartsNightingale() *EChartsOption {
	if r.TotalRisksCount == 0 {
		return NewEChartsNightingaleOption(EChartsNightingaleOption{})
	}

	infoRiskCount := r.GetInfoRiskCount()

	// 原始数据
	rawData := []struct {
		key   string
		name  string
		value int
		color string
	}{
		{severityCritical, getSeverityInfo(severityCritical).Text, r.CriticalRisksCount, getSeverityInfo(severityCritical).Color},
		{severityHigh, getSeverityInfo(severityHigh).Text, r.HighRisksCount, getSeverityInfo(severityHigh).Color},
		{severityMiddle, getSeverityInfo(severityMiddle).Text, r.MiddleRisksCount, getSeverityInfo(severityMiddle).Color},
		{severityLow, getSeverityInfo(severityLow).Text, r.LowRisksCount, getSeverityInfo(severityLow).Color},
		{severityInfo, getSeverityInfo(severityInfo).Text, infoRiskCount, getSeverityInfo(severityInfo).Color},
	}

	// 过滤掉值为0的数据
	var validData []struct {
		key   string
		name  string
		value int
		color string
	}
	for _, item := range rawData {
		if item.value > 0 {
			validData = append(validData, item)
		}
	}

	if len(validData) == 0 {
		return NewEChartsNightingaleOption(EChartsNightingaleOption{})
	}

	// 视觉优化算法：处理极端数据比例
	chartData := r.optimizeNightingaleData(validData)

	option := EChartsNightingaleOption{
		Title: echartsTitle{
			Text: "",
			Left: "center",
		},
		Tooltip: echartsTooltip{
			Trigger: "item",
			//Formatter: "function(params) { return params.seriesName + '<br/>' + params.name + ': ' + (params.data.realValue !== undefined ? params.data.realValue : params.value) + ' (' + params.percent + '%)'; }",
		},
		Legend: echartsLegend{
			Data: []string{}, // 隐藏图例，因为标签直接显示在扇形外
		},
		Series: []echartsNightingaleSeries{
			{
				Name:     "风险等级分布",
				Type:     "pie",
				Radius:   []string{"20%", "80%"},
				Center:   []string{"50%", "50%"},
				RoseType: "area",
				ItemStyle: echartsNightingaleItemStyle{
					BorderRadius: 8,
				},
				Label: echartsNightingaleLabel{
					Show:     true,
					Position: "outside",
					//Formatter:  "function(params) { return params.name + '\n' + (params.data.realPercent !== undefined ? params.data.realPercent.toFixed(1) + '%' : params.percent + '%'); }",
					FontSize:   14,
					FontWeight: "bold",
					Color:      "#666",
				},
				LabelLine: echartsLabelLine{
					Show:    true,
					Length:  20,
					Length2: 15,
					Smooth:  false,
				},
				Data: chartData,
				Emphasis: echartsEmphasis{
					ItemStyle: echartsEmphasisItemStyle{
						ShadowBlur:    10,
						ShadowOffsetX: 0,
						ShadowColor:   "rgba(0, 0, 0, 0.5)",
					},
				},
			},
		},
	}

	return NewEChartsNightingaleOption(option)
}

// optimizeNightingaleData 优化南丁格尔玫瑰图数据，处理极端比例情况
func (r *RiskStatistics) optimizeNightingaleData(validData []struct {
	key   string
	name  string
	value int
	color string
}) []echartsNightingaleData {
	if len(validData) == 0 {
		return []echartsNightingaleData{}
	}

	// 计算总数和最大最小值
	total := 0
	maxValue := 0
	minValue := validData[0].value
	for _, item := range validData {
		total += item.value
		if item.value > maxValue {
			maxValue = item.value
		}
		if item.value < minValue {
			minValue = item.value
		}
	}

	chartData := make([]echartsNightingaleData, 0, len(validData))

	// 定义最小占比策略：确保最小数据项至少占总体的8%
	const minPercentage = 0.15 // 最小占比15%
	needOptimization := false

	// 检查是否需要优化：最小值占比小于8%，或者最大最小值比例超过15:1
	if minValue > 0 && (float64(minValue)/float64(total) < minPercentage || maxValue/minValue > 15) {
		needOptimization = true
	}

	if needOptimization {
		// 使用最小值策略优化算法
		// 1. 为每个数据项保证最小值
		// 2. 剩余的权重按原始比例分配

		minValueForChart := int(float64(total) * minPercentage) // 最小值对应的图表值
		reservedTotal := len(validData) * minValueForChart      // 为所有项目预留的最小值总和
		remainingTotal := total - reservedTotal                 // 剩余可分配的总值

		if remainingTotal < 0 {
			// 如果预留值过大，则采用均匀分配策略
			uniformValue := total / len(validData)
			for _, item := range validData {
				// 计算真实百分比
				realPercent := float64(item.value) / float64(total) * 100
				chartData = append(chartData, echartsNightingaleData{
					Value: uniformValue,
					Name:  item.name,
					ItemStyle: echartsItemStyle{
						Color: item.color,
					},
					RealValue:   item.value,
					RealPercent: realPercent,
				})
			}
		} else {
			// 正常的最小值策略
			for _, item := range validData {
				// 计算该项目的额外分配值（基于原始比例）
				extraValue := 0
				if remainingTotal > 0 {
					extraValue = int(float64(item.value) / float64(total) * float64(remainingTotal))
				}

				// 最终图表值 = 最小保证值 + 额外分配值
				chartValue := minValueForChart + extraValue

				// 确保最小值不会太小（至少为总数的3%）
				minChartValue := int(float64(total) * 0.03)
				if chartValue < minChartValue {
					chartValue = minChartValue
				}

				// 计算真实百分比
				realPercent := float64(item.value) / float64(total) * 100
				chartData = append(chartData, echartsNightingaleData{
					Value: chartValue,
					Name:  item.name,
					ItemStyle: echartsItemStyle{
						Color: item.color,
					},
					RealValue:   item.value,  // 保存真实值用于tooltip显示
					RealPercent: realPercent, // 保存真实百分比用于formatter显示
				})
			}
		}
	} else {
		// 数据比例正常，直接使用原始数据
		for _, item := range validData {
			// 计算真实百分比
			realPercent := float64(item.value) / float64(total) * 100
			chartData = append(chartData, echartsNightingaleData{
				Value: item.value,
				Name:  item.name,
				ItemStyle: echartsItemStyle{
					Color: item.color,
				},
				RealValue:   item.value,
				RealPercent: realPercent,
			})
		}
	}

	// 按严重程度排序（从高到低）
	severityPriority := map[string]int{
		getSeverityInfo(severityCritical).Text: 0,
		getSeverityInfo(severityHigh).Text:     1,
		getSeverityInfo(severityMiddle).Text:   2,
		getSeverityInfo(severityLow).Text:      3,
		getSeverityInfo(severityInfo).Text:     4,
	}

	sort.Slice(chartData, func(i, j int) bool {
		priority1, exists1 := severityPriority[chartData[i].Name]
		priority2, exists2 := severityPriority[chartData[j].Name]
		if !exists1 {
			priority1 = 999
		}
		if !exists2 {
			priority2 = 999
		}
		return priority1 < priority2
	})

	return chartData
}

// RiskTypeOverview 风险类型概览
type RiskTypeOverview struct {
	RiskMap map[string]map[string]int // 风险类型 -> 严重程度 -> 数量
}

// ToMarkdownTable 将风险类型概览转换为Markdown表格
func (r *RiskTypeOverview) ToMarkdownTable() string {
	if len(r.RiskMap) == 0 {
		return "## 2.2 漏洞类型统计\n暂无漏洞类型统计信息"
	}

	tableContent := `## 2.2 漏洞类型统计

| 漏洞类型 | 数量 |
| ---- | ---- |
`

	// 应该展示的风险级别
	severityLevels := []string{severityCritical, severityHigh, severityMiddle, severityLow, severityInfo}

	// 获取所有风险类型并排序，确保输出一致性
	riskTypes := make([]string, 0, len(r.RiskMap))
	for riskType := range r.RiskMap {
		riskTypes = append(riskTypes, riskType)
	}
	sort.Strings(riskTypes)

	for _, riskType := range riskTypes {
		severityMap := r.RiskMap[riskType]
		var counts []string
		for _, severity := range severityLevels {
			if count := severityMap[severity]; count > 0 {
				counts = append(counts, fmt.Sprintf("%s %d", getSeverityInfo(severity).Text, count))
			}
		}

		if len(counts) > 0 {
			tableContent += fmt.Sprintf("| %s | %s |\n", riskType, strings.Join(counts, " "))
		}
	}

	return tableContent
}

// ToEChartsStackedBar 将风险类型概览转换为ECharts堆叠柱状图配置
func (r *RiskTypeOverview) ToEChartsStackedBar() *EChartsOption {
	if len(r.RiskMap) == 0 {
		return NewEChartsBarOption(EChartsStackedBarOption{})
	}

	// 获取所有风险类型并排序，确保输出一致性
	riskTypes := make([]string, 0, len(r.RiskMap))
	for riskType := range r.RiskMap {
		riskTypes = append(riskTypes, riskType)
	}
	sort.Strings(riskTypes)

	// 过滤掉没有数据的风险类型
	validRiskTypes := make([]string, 0, len(riskTypes))
	for _, riskType := range riskTypes {
		severityMap := r.RiskMap[riskType]
		hasValidData := false
		for _, count := range severityMap {
			if count > 0 {
				hasValidData = true
				break
			}
		}
		if hasValidData {
			validRiskTypes = append(validRiskTypes, riskType)
		}
	}

	if len(validRiskTypes) == 0 {
		return NewEChartsBarOption(EChartsStackedBarOption{})
	}

	// 定义严重程度顺序和配置
	severityConfigs := []struct {
		key   string
		name  string
		color string
	}{
		{severityCritical, getSeverityInfo(severityCritical).Text, getSeverityInfo(severityCritical).Color},
		{severityHigh, getSeverityInfo(severityHigh).Text, getSeverityInfo(severityHigh).Color},
		{severityMiddle, getSeverityInfo(severityMiddle).Text, getSeverityInfo(severityMiddle).Color},
		{severityLow, getSeverityInfo(severityLow).Text, getSeverityInfo(severityLow).Color},
		{severityInfo, getSeverityInfo(severityInfo).Text, getSeverityInfo(severityInfo).Color},
	}

	// 计算每个风险类型的总数，用于计算该类型内部的占比
	riskTypeTotals := make(map[string]int)
	for _, riskType := range validRiskTypes {
		severityMap := r.RiskMap[riskType]
		total := 0
		for _, count := range severityMap {
			total += count
		}
		riskTypeTotals[riskType] = total
	}

	// 构建series数据
	var series []echartsSeries
	var legendData []string

	for _, config := range severityConfigs {
		// 构建该严重程度在各风险类型中的数据
		data := make([]echartsBarDataItem, len(validRiskTypes))
		hasData := false

		for i, riskType := range validRiskTypes {
			count := r.RiskMap[riskType][config.key]
			typeTotal := riskTypeTotals[riskType]

			if count > 0 && typeTotal > 0 {
				// 计算该严重程度在当前风险类型中的占比
				percentage := float64(count) / float64(typeTotal)
				data[i] = echartsBarDataItem{
					Value:     percentage,
					RealTotal: typeTotal,
				}
				hasData = true
			} else {
				data[i] = echartsBarDataItem{
					Value:     0,
					RealTotal: typeTotal,
				}
			}
		}

		// 只添加有数据的严重程度
		if hasData {
			series = append(series, echartsSeries{
				Name:  config.name,
				Type:  "bar",
				Stack: "总量",
				Data:  data,
				ItemStyle: echartsItemStyle{
					Color: config.color,
				},
				Label: echartsLabel{
					Show:     true,
					Position: "inside",
					//Formatter: "function(params) { return params.value > 0 ? (params.value * 100).toFixed(1) + '%' : ''; }",
					FontSize: 10,
					Color:    "white",
				},
			})
			legendData = append(legendData, config.name)
		}
	}

	// Y轴最大值固定为1.0（100%），因为每个柱子代表一个风险类型的100%分布
	maxValue := 1.0

	option := EChartsStackedBarOption{
		Title: echartsTitle{
			Text: "",
			Left: "center",
		},
		Tooltip: echartsTooltip{
			Trigger: "axis",
			AxisPointer: echartsAxisPointer{
				Type: "shadow",
			},
			//Formatter: "function(params) { let result = params[0].axisValue + '<br/>'; let total = 0; params.forEach(function(item) { if(item.value > 0) { let count = Math.round(item.value * item.data.realTotal); total += count; result += item.marker + ' ' + item.seriesName + ': ' + count + ' (' + (item.value * 100).toFixed(1) + '%)<br/>'; } }); if(total > 0) result += '总计: ' + total + '<br/>'; return result; }",
		},
		Legend: echartsLegend{
			Data:       legendData,
			Top:        10,
			ItemWidth:  18,
			ItemHeight: 12,
		},
		Grid: echartsGrid{
			Left:         "3%",
			Right:        "4%",
			Bottom:       "3%",
			Top:          "15%",
			ContainLabel: true,
		},
		XAxis: echartsXAxis{
			Type: "category",
			Data: validRiskTypes,
			AxisLabel: echartsAxisLabel{
				FontSize: 12,
			},
		},
		YAxis: echartsYAxis{
			Type: "value",
			Max:  maxValue,
			AxisLabel: echartsAxisLabel{
				Formatter: "{value}",
			},
			SplitLine: echartsSplitLine{
				Show: true,
				LineStyle: echartsLineStyle{
					Color: "#f0f0f0",
				},
			},
		},
		Series: series,
	}

	return NewEChartsBarOption(option)
}

// NewRiskTypeOverview 创建风险类型概览
func NewRiskTypeOverview(risks []*SSAReportRisk) *RiskTypeOverview {
	riskMap := make(map[string]map[string]int)

	for _, risk := range risks {
		if risk.RiskType == "" || risk.Severity == "" {
			continue
		}

		if riskMap[risk.RiskType] == nil {
			riskMap[risk.RiskType] = make(map[string]int)
		}
		riskMap[risk.RiskType][risk.Severity]++
	}

	return &RiskTypeOverview{RiskMap: riskMap}
}

func safeStr(i interface{}, items ...interface{}) string {
	s := utils.ParseStringToVisible(utils.InterfaceToString(i))
	return fmt.Sprintf(s, items...)
}

// GenerateSSAProjectReportFromTask 基于扫描任务生成SSA项目报告数据
func GenerateSSAProjectReportFromTask(ctx context.Context, task *schema.SyntaxFlowScanTask) (*SSAProjectReport, error) {
	// 获取程序名称列表（可能有多个程序）
	programs := strings.Split(task.Programs, schema.SYNTAXFLOWSCAN_PROGRAM_SPLIT)
	primaryProgram := ""
	if len(programs) > 0 {
		primaryProgram = programs[0]
	}

	report := &SSAProjectReport{
		ProgramName: primaryProgram,
		ReportTime:  time.Now(),
		Rules:       make(map[string]*SSAReportRule),

		// 从任务中获取统计信息
		TotalRules:         int(task.RulesCount),
		TotalRisksCount:    int(task.RiskCount),
		CriticalRisksCount: int(task.CriticalCount),
		HighRisksCount:     int(task.HighCount),
		MiddleRisksCount:   int(task.WarningCount),
		LowRisksCount:      int(task.LowCount),
		ScanStartTime:      task.CreatedAt,
		ScanEndTime:        task.UpdatedAt,
	}

	// 加载项目信息
	if primaryProgram != "" {
		err := loadProjectInfo(report, primaryProgram)
		if err != nil {
			log.Warnf("load project info failed: %v", err)
		}
	}

	// 获取任务相关的风险数据并进行批量处理
	risks, err := getRisksByTaskID(task.TaskId)
	if err != nil {
		return nil, utils.Wrapf(err, "get risks by task id failed")
	}

	// 批量处理风险数据（一次遍历完成所有统计）
	err = processRisksAndStats(report, risks)
	if err != nil {
		return nil, utils.Wrapf(err, "process risks and stats failed")
	}

	return report, nil
}

// GenerateSSAProjectReportFromFilter 基于SSARisksFilter生成SSA项目报告数据
// 支持使用完整的过滤器条件筛选风险
func GenerateSSAProjectReportFromFilter(ctx context.Context, filter *ypb.SSARisksFilter) (*SSAProjectReport, error) {
	if filter == nil {
		return nil, utils.Errorf("filter is nil")
	}

	// 通过Filter获取风险数据
	risks, err := getRisks(filter)
	if err != nil {
		return nil, utils.Wrapf(err, "get risks by filter failed")
	}

	if len(risks) == 0 {
		return nil, utils.Errorf("no risks found for the given filter")
	}

	// 从Risk列表中提取所有涉及的ProgramName
	programNameSet := make(map[string]bool)
	for _, risk := range risks {
		if risk.ProgramName != "" {
			programNameSet[risk.ProgramName] = true
		}
	}

	// 组合ProgramName（使用换行分隔，在markdown表格中更清晰）
	var programNames []string
	for name := range programNameSet {
		programNames = append(programNames, name)
	}
	sort.Strings(programNames) // 排序保证顺序一致
	// 使用 <br/> 换行符组合项目名称，在markdown表格中会换行显示
	combinedProgramName := strings.Join(programNames, "<br/>")

	// 创建报告结构（基于过滤器选择的Risk）
	report := &SSAProjectReport{
		ProgramName:       combinedProgramName,
		ReportTime:        time.Now(),
		Rules:             make(map[string]*SSAReportRule),
		FromRiskSelection: true, // 标记为从Risk选择生成
	}

	// 从Risk数据中统计规则数量
	ruleSet := make(map[string]bool)
	for _, risk := range risks {
		if risk.FromRule != "" {
			ruleSet[risk.FromRule] = true
		}
	}
	report.TotalRules = len(ruleSet)

	// 批量处理风险数据（一次遍历完成所有统计）
	err = processRisksAndStats(report, risks)
	if err != nil {
		return nil, utils.Wrapf(err, "process risks and stats failed")
	}

	// 从处理后的风险中计算统计信息（完全基于过滤器选择的Risk）
	report.TotalRisksCount = len(risks)
	report.CriticalRisksCount = 0
	report.HighRisksCount = 0
	report.MiddleRisksCount = 0
	report.LowRisksCount = 0

	// 统计各等级风险数量和时间范围
	var minTime, maxTime time.Time
	for i, risk := range risks {
		// 统计风险等级
		switch strings.ToLower(string(risk.Severity)) {
		case severityCritical:
			report.CriticalRisksCount++
		case severityHigh:
			report.HighRisksCount++
		case severityMiddle:
			report.MiddleRisksCount++
		case severityLow:
			report.LowRisksCount++
		}

		// 统计时间范围
		if i == 0 {
			minTime = risk.CreatedAt
			maxTime = risk.UpdatedAt
		} else {
			if risk.CreatedAt.Before(minTime) {
				minTime = risk.CreatedAt
			}
			if risk.UpdatedAt.After(maxTime) {
				maxTime = risk.UpdatedAt
			}
		}
	}

	report.ScanStartTime = minTime
	report.ScanEndTime = maxTime

	// 日志输出使用未转义的格式，更易读
	logProgramName := strings.Join(programNames, " | ")
	log.Infof("Generated report from filter, found %d risks, covering %d programs: %s",
		len(risks), len(programNames), logProgramName)

	return report, nil
}

// getRisksByIDs 通过RiskID列表获取风险
func getRisksByIDs(riskIDs []int64) ([]*schema.SSARisk, error) {
	if len(riskIDs) == 0 {
		return nil, utils.Errorf("riskIDs is empty")
	}

	log.Infof("getRisksByIDs: querying %d risks", len(riskIDs))
	filter := &ypb.SSARisksFilter{
		ID: riskIDs,
	}

	risks, err := getRisks(filter)
	if err != nil {
		log.Errorf("getRisksByIDs: failed to get risks, error: %v", err)
		return nil, err
	}

	log.Infof("getRisksByIDs: successfully retrieved %d risks", len(risks))
	return risks, nil
}

// GenerateYakitReportContent 生成yakit报告内容
func GenerateYakitReportContent(reportInstance *schema.Report, ssaReport *SSAProjectReport) error {
	// 生成报告各部分内容
	generateProjectOverview(reportInstance, ssaReport)
	generateRiskStatistics(reportInstance, ssaReport)
	generateRiskOverview(reportInstance, ssaReport)
	generateRiskDetails(reportInstance, ssaReport)
	return nil
}

// generateProjectOverview 生成项目概述
func generateProjectOverview(reportInstance *schema.Report, ssaReport *SSAProjectReport) {
	projectInfo := ssaReport.GetProjectInfo()
	reportInstance.Markdown(projectInfo.ToMarkdownTable())
}

// generateRiskStatistics 生成风险信息统计
func generateRiskStatistics(reportInstance *schema.Report, ssaReport *SSAProjectReport) {
	riskStats := ssaReport.GetRiskStatistics()

	// 生成Markdown表格
	reportInstance.Markdown(riskStats.ToMarkdownTable())

	// 生成风险统计图表
	if riskStats.TotalRisksCount > 0 {
		nightingaleChart := riskStats.ToEChartsNightingale()
		reportInstance.Raw(nightingaleChart)
	}
	riskTypeOverview := ssaReport.GetRiskTypeOverview()
	// 生成Markdown表格
	reportInstance.Markdown(riskTypeOverview.ToMarkdownTable())
	// 生成风险类型ECharts堆叠柱状图
	if riskStats.TotalRisksCount > 0 {
		stackedBarChart := riskTypeOverview.ToEChartsStackedBar()
		reportInstance.Raw(stackedBarChart)
	}
}

// generateRiskOverview 生成风险信息概览
func generateRiskOverview(reportInstance *schema.Report, ssaReport *SSAProjectReport) {
	if len(ssaReport.Risks) == 0 {
		reportInstance.Markdown("# 三、 漏洞概览\n\n暂无漏洞信息")
		return
	}

	// 构建Markdown表格内容
	tableContent := `# 三、 漏洞概览

| 漏洞类型 | 漏洞名称 | 位置 | 处置状态 | 处置备注 |
| ---- | ---- | ---- | ---- | ---- |
`

	// 注意：ssaReport.Risks 已经在 processRisksAndStats 中进行了稳定排序
	// 按风险类型(RiskType)排序为主要排序机制，相同风险类型按严重程度进行次要排序，确保输出一致性
	for _, risk := range ssaReport.Risks {
		if risk.CodeSourceUrl == "" {
			continue
		}
		// 构建位置信息
		location := risk.CodeSourceUrl
		if risk.Line > 0 {
			location = fmt.Sprintf("%s:%d", risk.CodeSourceUrl, risk.Line)
		}

		// 转义Markdown特殊字符，确保表格格式正确
		riskType := strings.ReplaceAll(risk.RiskType, "|", "\\|")
		titleVerbose := strings.ReplaceAll(risk.TitleVerbose, "|", "\\|")
		location = strings.ReplaceAll(location, "|", "\\|")

		// 格式化处置状态
		disposalStatus := formatDisposalStatus(risk.LatestDisposalStatus)

		// 处理处置备注
		disposalComment := risk.LatestDisposalComment
		if disposalComment == "" {
			disposalComment = "-"
		} else {
			// 转义特殊字符，处理换行符
			disposalComment = strings.ReplaceAll(disposalComment, "|", "\\|")
			disposalComment = strings.ReplaceAll(disposalComment, "\n", " ")
			// 如果备注太长，截断并添加省略号
			if len(disposalComment) > 50 {
				disposalComment = disposalComment[:50] + "..."
			}
		}

		tableContent += fmt.Sprintf("| %s | %s | %s | %s | %s |\n", riskType, titleVerbose, location, disposalStatus, disposalComment)
	}

	reportInstance.Markdown(tableContent)
}

// generateRiskDetails 生成风险详情
func generateRiskDetails(reportInstance *schema.Report, ssaReport *SSAProjectReport) {
	reportInstance.Markdown("# 四、漏洞详情")

	if len(ssaReport.Risks) == 0 {
		reportInstance.Markdown("暂无漏洞详情")
		return
	}

	// 将风险按类型和等级分组
	riskGroups := groupRisksByTypeAndSeverity(ssaReport.Risks)

	// 性能优化：限制显示的漏洞分组数量
	totalGroups := len(riskGroups)
	totalRisks := len(ssaReport.Risks)
	if totalGroups > defaultRiskDetailsLimit {
		reportInstance.Markdown(fmt.Sprintf("*注：为优化文档加载性能，漏洞详情仅显示前%d个漏洞类型，总计%d个漏洞类型，%d个漏洞实例*\n",
			defaultRiskDetailsLimit, totalGroups, totalRisks))
	}

	// 按风险等级和类型分组显示
	generateRiskDetailsByTypeGroups(reportInstance, riskGroups, ssaReport.Language)
}

// groupRisksByTypeAndSeverity 将风险按类型和等级分组
func groupRisksByTypeAndSeverity(risks []*SSAReportRisk) []*RiskGroup {
	// 使用map进行分组，key为 "RiskType|Severity|FromRule"
	groupMap := make(map[string]*RiskGroup)

	for _, risk := range risks {
		// 创建分组键，确保相同类型、等级和规则的风险归为一组
		groupKey := fmt.Sprintf("%s|%s|%s", risk.RiskType, risk.Severity, risk.FromRule)

		group, exists := groupMap[groupKey]
		if !exists {
			// 创建新的分组
			group = &RiskGroup{
				RiskType:     risk.RiskType,
				Severity:     risk.Severity,
				Title:        risk.Title,
				TitleVerbose: risk.TitleVerbose,
				Description:  risk.Description,
				Solution:     risk.Solution,
				FromRule:     risk.FromRule,
				Instances:    make([]*RiskInstance, 0),
				Count:        0,
			}
			groupMap[groupKey] = group
		}
		if risk.CodeSourceUrl != "" || risk.CodeFragment != "" {
			// 添加风险实例
			instance := &RiskInstance{
				CodeSourceUrl:         risk.CodeSourceUrl,
				CodeRange:             risk.CodeRange,
				CodeFragment:          risk.CodeFragment,
				FunctionName:          risk.FunctionName,
				Line:                  risk.Line,
				LatestDisposalStatus:  risk.LatestDisposalStatus,
				LatestDisposalComment: risk.LatestDisposalComment,
			}
			group.Instances = append(group.Instances, instance)
			group.Count++
		}
	}

	// 转换为切片并排序
	groups := make([]*RiskGroup, 0, len(groupMap))
	for _, group := range groupMap {
		groups = append(groups, group)
	}

	// 按严重程度、风险类型排序
	severityPriority := map[string]int{
		severityCritical: 0,
		severityHigh:     1,
		severityMiddle:   2,
		severityLow:      3,
		severityInfo:     4,
	}

	sort.Slice(groups, func(i, j int) bool {
		group1, group2 := groups[i], groups[j]

		// 首先按严重程度排序
		priority1, exists1 := severityPriority[group1.Severity]
		if !exists1 {
			priority1 = 999
		}
		priority2, exists2 := severityPriority[group2.Severity]
		if !exists2 {
			priority2 = 999
		}

		if priority1 != priority2 {
			return priority1 < priority2
		}

		// 相同严重程度按风险类型排序
		if group1.RiskType != group2.RiskType {
			return group1.RiskType < group2.RiskType
		}

		// 相同风险类型按规则名排序
		return group1.FromRule < group2.FromRule
	})

	return groups
}

// generateRiskDetailsByTypeGroups 按风险类型分组生成详情
func generateRiskDetailsByTypeGroups(reportInstance *schema.Report, riskGroups []*RiskGroup, language ssaconfig.Language) {
	displayedCount := 0

	for groupIndex, group := range riskGroups {
		// 达到显示限制时停止
		if displayedCount >= defaultRiskDetailsLimit {
			break
		}

		generateSingleRiskGroupDetail(reportInstance, group, groupIndex+1, language)
		displayedCount++
	}
}

// generateSingleRiskGroupDetail 生成单个风险分组的详情
func generateSingleRiskGroupDetail(reportInstance *schema.Report, group *RiskGroup, groupIndex int, language ssaconfig.Language) {
	// 使用漏洞名称和数量作为主标题
	reportInstance.Markdown(fmt.Sprintf("## 漏洞类型%d %s (%d个实例)", groupIndex, group.TitleVerbose, group.Count))

	// 生成漏洞基本信息表格（只显示一次）
	generateRiskGroupInfoTable(reportInstance, group)

	// 生成该分组下的所有风险实例代码片段
	reportInstance.Markdown("### 漏洞代码片段")
	generateRiskInstancesTable(reportInstance, group.Instances, language)
}

// generateRiskGroupInfoTable 生成风险分组基本信息表格
func generateRiskGroupInfoTable(reportInstance *schema.Report, group *RiskGroup) {
	severityInfo := getSeverityInfo(group.Severity)

	// 生成基本信息表格
	tableContent := `
| 项目 | 详情 |
| :----: | :----: |
`

	tableContent += fmt.Sprintf("| 漏洞等级 | <span style=\"color: %s\">%s</span> |\n", severityInfo.Color, severityInfo.Text)
	tableContent += fmt.Sprintf("| 漏洞类型 | %s |\n", strings.ReplaceAll(group.RiskType, "|", "\\|"))
	tableContent += fmt.Sprintf("| 扫描规则 | %s |\n", strings.ReplaceAll(group.FromRule, "|", "\\|"))
	tableContent += fmt.Sprintf("| 实例数量 | %d |\n", group.Count)

	if group.Description != "" {
		description := strings.ReplaceAll(group.Description, "|", "\\|")
		description = strings.ReplaceAll(description, "\n", "<br/>")
		tableContent += fmt.Sprintf("| 漏洞描述 | %s |\n", description)
	}

	if group.Solution != "" {
		solution := strings.ReplaceAll(group.Solution, "|", "\\|")
		solution = strings.ReplaceAll(solution, "\n", "<br/>")
		tableContent += fmt.Sprintf("| 修复建议 | %s |\n", solution)
	}

	reportInstance.Markdown(tableContent)
}

// generateRiskInstancesTable 生成风险实例代码片段
func generateRiskInstancesTable(reportInstance *schema.Report, instances []*RiskInstance, codeFragmentLanguage ssaconfig.Language) {
	if len(instances) == 0 {
		return
	}

	// 按文件路径和行号排序实例
	sort.Slice(instances, func(i, j int) bool {
		if instances[i].CodeSourceUrl != instances[j].CodeSourceUrl {
			return instances[i].CodeSourceUrl < instances[j].CodeSourceUrl
		}
		return instances[i].Line < instances[j].Line
	})

	// 为每个实例显示代码片段
	for i, instance := range instances {
		// 显示实例编号和文件路径信息
		reportInstance.Markdown(fmt.Sprintf("**%d. 文件路径：** %s:%d", i+1, instance.CodeSourceUrl, instance.Line))

		// 显示处置状态和备注
		if instance.LatestDisposalStatus != "" {
			disposalStatus := formatDisposalStatus(instance.LatestDisposalStatus)
			disposalInfo := fmt.Sprintf("**处置状态：** %s \n", disposalStatus)

			// 如果有处置备注，也显示出来
			if instance.LatestDisposalComment != "" {
				// 转义特殊字符
				comment := strings.ReplaceAll(instance.LatestDisposalComment, "\n", " ")
				disposalInfo += fmt.Sprintf("**处置备注：** %s", comment)
			}

			reportInstance.Markdown(disposalInfo)
		}

		// 显示代码片段
		if instance.CodeFragment != "" {
			reportInstance.Markdown("**代码片段：**")
			codeFragment := instance.CodeFragment
			if len(codeFragment) > defaultCodeSegmentLimit {
				codeFragment = codeFragment[:defaultCodeSegmentLimit] + "\n... (代码片段过长，已截断)"
			}
			reportInstance.Markdown(fmt.Sprintf("```%s\n%s\n```", codeFragmentLanguage, codeFragment))
		} else {
			// 如果没有代码片段，至少显示位置信息
			reportInstance.Markdown("*（无代码片段信息）*")
		}

		// 在实例之间添加分隔线（除了最后一个）
		if i < len(instances)-1 {
			reportInstance.Markdown("---")
		}
	}
}

// loadProjectInfo 加载项目信息
func loadProjectInfo(report *SSAProjectReport, programName string) error {
	db := ssadb.GetDB()
	var program ssadb.IrProgram

	if err := db.Where("program_name = ?", programName).First(&program).Error; err != nil {
		return utils.Wrapf(err, "get program %s from database failed", programName)
	}

	report.Language = program.Language
	if program.Description == "" {
		report.Description = defaultProjectDescription
	} else {
		report.Description = program.Description
	}
	report.EngineVersion = program.EngineVersion

	// 统计文件数量和代码行数
	if program.FileList != nil {
		report.FileCount = len(program.FileList)
		report.CodeLineCount = program.LineCount
	}

	return nil
}

// getRisks 通用风险获取函数，获取所有匹配条件的风险（无分页限制）
func getRisks(filter *ypb.SSARisksFilter) ([]*schema.SSARisk, error) {
	db := consts.GetGormDefaultSSADataBase()

	// 直接使用FilterSSARisk进行查询，避免分页限制
	db = db.Model(&schema.SSARisk{})
	db = yakit.FilterSSARisk(db, filter)
	// 使用多字段排序确保结果一致性：先按创建时间降序，再按ID升序
	db = db.Order("created_at DESC, id ASC")

	var risks []*schema.SSARisk
	if err := db.Find(&risks).Error; err != nil {
		return nil, utils.Wrapf(err, "query ssa risks failed")
	}

	log.Infof("getRisks: found %d risks with filter", len(risks))
	return risks, nil
}

// getRisksByTaskID 通过taskID获取风险（taskID存储在RuntimeId字段中）
func getRisksByTaskID(taskID string) ([]*schema.SSARisk, error) {
	log.Infof("getRisksByTaskID: querying risks for taskID=%s", taskID)
	filter := &ypb.SSARisksFilter{
		RuntimeID: []string{taskID}, // RuntimeID字段实际存储的是taskID
	}
	risks, err := getRisks(filter)
	if err != nil {
		log.Errorf("getRisksByTaskID: failed to get risks for taskID=%s, error: %v", taskID, err)
		return nil, err
	}
	log.Infof("getRisksByTaskID: successfully retrieved %d risks for taskID=%s", len(risks), taskID)
	return risks, nil
}

// getRisksByProgramName 通过项目名获取风险
func getRisksByProgramName(programName string) ([]*schema.SSARisk, error) {
	filter := &ypb.SSARisksFilter{
		ProgramName: []string{programName},
	}
	return getRisks(filter)
}

// processRisksAndStats 批量处理风险数据和统计信息（一次遍历完成所有处理）
func processRisksAndStats(report *SSAProjectReport, risks []*schema.SSARisk) error {
	log.Infof("processRisksAndStats: processing %d risks", len(risks))
	report.Risks = make([]*SSAReportRisk, 0, len(risks))
	fileMap := make(map[string]*SSAReportFile)
	ruleMap := make(map[string]*SSAReportRule)

	// 获取数据库连接
	db := ssadb.GetDB()

	// 一次遍历完成所有统计
	for _, risk := range risks {
		// 获取该风险的最新处置信息
		disposalStatus := risk.LatestDisposalStatus
		disposalComment := ""

		// 通过 GetSSARiskDisposalsWithInheritance 获取完整的处置记录
		disposals, err := yakit.GetSSARiskDisposalsWithInheritance(db, int64(risk.ID))
		if err == nil && len(disposals) > 0 {
			// 取第一个（最新的）处置记录
			latestDisposal := disposals[0]
			disposalStatus = latestDisposal.Status
			disposalComment = latestDisposal.Comment
		}

		// 处理风险详情
		reportRisk := &SSAReportRisk{
			Title:                 risk.Title,
			TitleVerbose:          risk.TitleVerbose,
			Description:           risk.Description,
			Solution:              risk.Solution,
			RiskType:              risk.RiskType,
			Severity:              string(risk.Severity),
			FromRule:              risk.FromRule,
			CodeSourceUrl:         risk.CodeSourceUrl,
			CodeRange:             risk.CodeRange,
			CodeFragment:          risk.CodeFragment,
			FunctionName:          risk.FunctionName,
			Line:                  risk.Line,
			LatestDisposalStatus:  disposalStatus,
			LatestDisposalComment: disposalComment,
			RiskID:                risk.ID,
		}
		report.Risks = append(report.Risks, reportRisk)

		// 统计文件信息
		if risk.CodeSourceUrl != "" {
			file, exists := fileMap[risk.CodeSourceUrl]
			if !exists {
				file = &SSAReportFile{
					FilePath: risk.CodeSourceUrl,
					Language: report.Language,
				}
				fileMap[risk.CodeSourceUrl] = file
			}
			file.RiskCount++
			updateSeverityCount(string(risk.Severity), file)
		}

		// 统计规则信息
		if risk.FromRule != "" {
			rule, exists := ruleMap[risk.FromRule]
			if !exists {
				rule = &SSAReportRule{
					RuleName:    risk.FromRule,
					Title:       risk.Title,
					TitleZh:     risk.TitleVerbose,
					Severity:    string(risk.Severity),
					Description: risk.Description,
				}
				ruleMap[risk.FromRule] = rule
			}
			rule.RiskCount++
		}
	}

	// 转换文件映射为切片并进行稳定排序
	report.Files = make([]*SSAReportFile, 0, len(fileMap))
	for _, file := range fileMap {
		report.Files = append(report.Files, file)
	}
	// 按文件路径排序，确保输出一致性
	sort.Slice(report.Files, func(i, j int) bool {
		return report.Files[i].FilePath < report.Files[j].FilePath
	})

	// 设置规则统计
	report.Rules = ruleMap
	report.TotalRules = len(ruleMap)

	// 对风险列表进行稳定排序，确保报告输出一致性
	// 按风险类型(RiskType)排序为主要排序机制，相同风险类型按严重程度进行次要排序
	sort.Slice(report.Risks, func(i, j int) bool {
		risk1, risk2 := report.Risks[i], report.Risks[j]

		// 首先按风险类型(RiskType)排序，这是表格中的第一列
		if risk1.RiskType != risk2.RiskType {
			return risk1.RiskType < risk2.RiskType
		}

		// 相同风险类型按严重程度排序
		// 定义严重程度优先级
		severityPriority := map[string]int{
			severityCritical: 0,
			severityHigh:     1,
			severityMiddle:   2,
			severityLow:      3,
			severityInfo:     4,
		}

		priority1, exists1 := severityPriority[risk1.Severity]
		if !exists1 {
			priority1 = 999 // 未知严重程度排在最后
		}

		priority2, exists2 := severityPriority[risk2.Severity]
		if !exists2 {
			priority2 = 999
		}

		// 相同风险类型和严重程度时，按详细标题排序确保完全确定性
		if priority1 == priority2 {
			return risk1.TitleVerbose < risk2.TitleVerbose
		}

		return priority1 < priority2
	})

	log.Infof("processRisksAndStats: completed processing - risks: %d, files: %d, rules: %d",
		len(report.Risks), len(report.Files), len(report.Rules))
	return nil
}

// updateSeverityCount 更新严重程度计数
func updateSeverityCount(severity string, file *SSAReportFile) {
	switch severity {
	case severityCritical:
		file.CriticalCount++
	case severityHigh:
		file.HighCount++
	case severityMiddle:
		file.MiddleCount++
	case severityLow:
		file.LowCount++
	}
}

// formatDisposalStatus 格式化处置状态为中文显示 和IRify前端保持一致
func formatDisposalStatus(status string) string {
	switch status {
	case "not_issue":
		return "不是问题"
	case "suspicious":
		return "存疑"
	case "is_issue":
		return "有问题"
	case "not_set", "":
		return "未处置"
	default:
		return "未处置"
	}
}
