package yaklib

import (
	"fmt"

	uuid "github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils"
)

func init() {
	YakitExports["EnableWebsiteTrees"] = yakitEnableCrawlerViewer
	YakitExports["EnableTable"] = yakitEnableFixedTable
	YakitExports["EnableDotGraphTab"] = yakitEnableDotGraphTab
	YakitExports["EnableText"] = yakitEnableText
	YakitExports["TableData"] = yakitTableData
	YakitExports["StatusCard"] = yakitStatusCard
	YakitExports["TextTabData"] = yakitTextTabData
	YakitExports["OutputDotGraph"] = yakitDotGraphData
}

type YakitFeature struct {
	Feature string                 `json:"feature"`
	Params  map[string]interface{} `json:"params"`
}

// EnableWebsiteTrees 在 Yakit UI 中启用「网站树」展示标签（导出名为 yakit.EnableWebsiteTrees）
// 用于在插件运行时让 Yakit 展示指定目标的网站结构树
//
// 参数:
//   - targets: 目标（如域名/URL），多个可用逗号等分隔
//
// Example:
// ```
// // 在 Yakit 中启用网站树展示（需在 Yakit 引擎环境下生效，示意性示例）
// yakit.EnableWebsiteTrees("example.com")
// ```
func yakitEnableCrawlerViewer(targets string) {
	yakitClientInstance.Output(&YakitFeature{
		Feature: "website-trees",
		Params: map[string]interface{}{
			"targets":          targets,
			"refresh_interval": 3,
		},
	})
}

// EnableTable 在 Yakit UI 中启用一个固定表格用于展示数据（导出名为 yakit.EnableTable）
// 启用后可配合 yakit.TableData 持续向该表格写入行数据
//
// 参数:
//   - tableName: 表格名称
//   - columns: 表格列名列表
//
// Example:
// ```
// // 启用一个固定表格并写入一行（需在 Yakit 引擎环境下生效，示意性示例）
// yakit.EnableTable("Result", ["name", "value"])
// yakit.TableData("Result", {"name": "a", "value": "1"})
// ```
func yakitEnableFixedTable(tableName string, columns []string) {
	yakitClientInstance.Output(&YakitFeature{
		Feature: "fixed-table",
		Params: map[string]interface{}{
			"table_name": tableName,
			"columns":    columns,
		},
	})
}

// EnableDotGraphTab 在 Yakit UI 中启用一个 DOT 图标签页（导出名为 yakit.EnableDotGraphTab）
// 启用后可配合 yakit.OutputDotGraph 向该标签页输出 Graphviz DOT 图
//
// 参数:
//   - tabName: 标签页名称
//
// Example:
// ```
// // 启用 DOT 图标签页并输出一张图（需在 Yakit 引擎环境下生效，示意性示例）
// yakit.EnableDotGraphTab("Graph")
// yakit.OutputDotGraph("Graph", "digraph G { a -> b }")
// ```
func yakitEnableDotGraphTab(tabName string) {
	yakitClientInstance.Output(&YakitFeature{
		Feature: "dot-graph-tab",
		Params: map[string]interface{}{
			"tab_name": tabName,
		},
	})
}

// EnableText 在 Yakit UI 中启用一个文本标签页（导出名为 yakit.EnableText）
// 启用后可配合 yakit.TextTabData 向该标签页追加文本内容
//
// 参数:
//   - tabName: 标签页名称
//
// Example:
// ```
// // 启用文本标签页并写入文本（需在 Yakit 引擎环境下生效，示意性示例）
// yakit.EnableText("Log")
// yakit.TextTabData("Log", "hello yak")
// ```
func yakitEnableText(tabName string) {
	yakitClientInstance.Output(&YakitFeature{
		Feature: "text",
		Params: map[string]interface{}{
			"tab_name": tabName,
		},
	})
}

type YakitFixedTableData struct {
	TableName string                 `json:"table_name"`
	Data      map[string]interface{} `json:"data"`
}

// TableData 向 Yakit UI 中已启用的固定表格写入一行数据（导出名为 yakit.TableData）
// 需先通过 yakit.EnableTable 启用同名表格；data 的键应与列名对应
//
// 参数:
//   - tableName: 目标表格名称（需与 EnableTable 一致）
//   - data: 行数据（map，键为列名）
//
// 返回值:
//   - 始终返回 nil（数据通过 Yakit 输出通道发送）
//
// Example:
// ```
// // 向已启用的表格写入一行（需在 Yakit 引擎环境下生效，示意性示例）
// yakit.EnableTable("Result", ["name", "value"])
// yakit.TableData("Result", {"name": "a", "value": "1"})
// ```
func yakitTableData(tableName string, data any) *YakitFixedTableData {
	tableData := &YakitFixedTableData{
		TableName: tableName,
		Data:      utils.InterfaceToGeneralMap(data),
	}
	if tableData.Data == nil {
		tableData.Data = map[string]interface{}{}
	}
	tableData.Data["uuid"] = uuid.New().String()
	if yakitClientInstance != nil {
		yakitClientInstance.Output(tableData)
	}
	return nil
}

type YakitDotGraphData struct {
	TabName string `json:"tab_name"`
	Data    string `json:"data"`
}

// OutputDotGraph 向 Yakit UI 中已启用的 DOT 图标签页输出一张 Graphviz DOT 图（导出名为 yakit.OutputDotGraph）
// 需先通过 yakit.EnableDotGraphTab 启用同名标签页
//
// 参数:
//   - tabName: 目标标签页名称（需与 EnableDotGraphTab 一致）
//   - data: Graphviz DOT 图字符串
//
// Example:
// ```
// // 输出一张简单的 DOT 图（需在 Yakit 引擎环境下生效，示意性示例）
// yakit.EnableDotGraphTab("Graph")
// yakit.OutputDotGraph("Graph", "digraph G { a -> b }")
// ```
func yakitDotGraphData(tabName string, data string) {
	tabData := &YakitDotGraphData{
		TabName: tabName,
		Data:    data,
	}
	if yakitClientInstance != nil {
		yakitClientInstance.Output(tabData)
	}
}

type YakitTextTabData struct {
	TableName string `json:"table_name"`
	Data      string `json:"data"`
}

// TextTabData 向 Yakit UI 中已启用的文本标签页追加文本内容（导出名为 yakit.TextTabData）
// 需先通过 yakit.EnableText 启用同名标签页
//
// 参数:
//   - tabName: 目标标签页名称（需与 EnableText 一致）
//   - data: 要追加的文本内容
//
// Example:
// ```
// // 向已启用的文本标签页写入内容（需在 Yakit 引擎环境下生效，示意性示例）
// yakit.EnableText("Log")
// yakit.TextTabData("Log", "hello yak")
// ```
func yakitTextTabData(tabName string, data string) {
	tabData := &YakitTextTabData{
		TableName: tabName,
		Data:      data,
	}
	if yakitClientInstance != nil {
		yakitClientInstance.Output(tabData)
	}
}

type YakitStatusCard struct {
	Id   string   `json:"id"`
	Data string   `json:"data"`
	Tags []string `json:"tags"`
}

// StatusCard 在 Yakit UI 中输出/更新一个状态卡片（导出名为 yakit.StatusCard）
// 状态卡片常用于展示扫描进度、统计计数等关键指标；相同 id 的卡片会被更新而非新增
//
// 参数:
//   - id: 卡片唯一标识（相同 id 会更新同一张卡片）
//   - data: 卡片展示的数据（会转为字符串展示）
//   - tags: 可选的标签，用于卡片分组/归类
//
// Example:
// ```
// // 输出一个状态卡片（需在 Yakit 引擎环境下生效，示意性示例）
// yakit.StatusCard("Open Ports", 12, "scan")
// ```
func yakitStatusCard(id string, data interface{}, tags ...string) {
	yakitClientInstance.StatusCard(id, data, tags...)
}

// StatusCard 在 Yakit UI 中输出/更新一个状态卡片（导出名为 yakit.StatusCard）
//
// 状态卡片是 Yakit 任务面板上方的“关键指标”小卡片，用于实时展示统计数字（已扫主机数、开放端口数、
// 发现漏洞数、成功/失败计数等）。核心特性：相同 id 的卡片会被“原地更新”而非新增，因此在循环里用同一个 id
// 反复调用即可实现指标的实时刷新。可选的 tags 用于把多张卡片分组归类展示。
//
// 参数:
//   - id: 卡片唯一标识（相同 id 更新同一张卡片，是实现实时刷新的关键）
//   - data: 卡片展示的数据（任意类型，会转为字符串展示）
//   - tags: 可选的分组标签（可变参数）
//
// Example:
// ```
// // 在扫描循环中实时更新统计卡片，并用 tags 把卡片分组
// total = 8; openPorts = 0; vulns = 0
// for i = 0; i < total; i++ {
//     openPorts += randn(0, 3)
//     if randn(1, 100) > 80 { vulns++ }
//     yakit.StatusCard("Scanned", sprintf("%d/%d", i + 1, total), "progress")
//     yakit.StatusCard("Open Ports", openPorts, "stats")     // 相同 id 原地刷新
//     yakit.StatusCard("Vulns",      vulns,     "stats")
//     sleep(0.02)
// }
// ```
func (c *YakitClient) StatusCard(id string, data interface{}, tags ...string) {
	// yakitStatusCard(id, data, tags...)
	c.Output(&YakitStatusCard{
		Id: id, Data: fmt.Sprint(data), Tags: tags,
	})
}
