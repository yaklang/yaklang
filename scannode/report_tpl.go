package scannode

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mq"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/scannode/scanrpc"
)

//go:embed yak_scripts/gen_report.yak
var EmbedGenReport []byte

const GENREPORT_KEY = "JznQXuFDSepeNWHbiLGEwONiaBxhvj_SERVER_SCAN_MANAGER"

type TaskMeta struct {
	TotalNodes int    `json:"total_nodes"`
	RuntimeID  string `json:"runtime_id"`
}

type NodeData struct {
	HostTotal  int    `json:"host_total"`
	PortTotal  int    `json:"port_total"`
	Plugins    int    `json:"plugins"`
	ScriptType string `json:"script_type"`
}

type MultiNodeReport struct {
	TaskMeta  TaskMeta             `json:"task_meta"`
	NodesData map[string]*NodeData `json:"nodes_data"`
}

// 合并所有节点数据生成最终报告数据
func mergeAllNodesData(report *MultiNodeReport) map[string]interface{} {
	merged := make(map[string]interface{})

	var totalHosts int
	var portTotal, plugins int
	var scriptType string

	// 只累加host_total，因为node是按host分配的
	// port_total和plugins在所有node中都相同，取第一个即可
	for _, nodeData := range report.NodesData {
		totalHosts += nodeData.HostTotal
		if portTotal == 0 { // 取第一个节点的值
			portTotal = nodeData.PortTotal
			plugins = nodeData.Plugins
			scriptType = nodeData.ScriptType

		}
	}

	merged["runtime_id"] = report.TaskMeta.RuntimeID
	merged["host_total"] = totalHosts
	merged["port_total"] = portTotal
	merged["plugins"] = plugins
	merged["script_type"] = scriptType

	return merged
}

// 检查多节点是否都完成了
func checkAllNodesCompleted(report *MultiNodeReport) bool {
	// 当前已完成的节点数量就是nodes_data中的节点数量
	completedCount := len(report.NodesData)

	// 检查是否所有节点都完成
	return completedCount >= report.TaskMeta.TotalNodes && report.TaskMeta.TotalNodes > 0
}

func genReportFromKey(ctx context.Context, node string, helper *scanrpc.SCANServerHelper, broker *mq.Broker, req *scanrpc.SCAN_InvokeScriptRequest) error {
	if value := yakit.GetKey(consts.GetGormProfileDatabase(), GENREPORT_KEY); value != "" {
		// 首先尝试解析新的多节点格式
		var report MultiNodeReport
		if err := json.Unmarshal([]byte(value), &report); err != nil {
			// 如果解析失败，可能是旧格式，使用原有逻辑
			log.Debugf("failed to parse as multi-node format, trying legacy logic: %v", err)
			return genReportFromKeyLegacy(ctx, node, helper, broker, req, value)
		}

		// 检查是否所有节点都完成了
		if checkAllNodesCompleted(&report) {
			// 所有节点完成，生成报告
			yakit.DelKey(consts.GetGormProfileDatabase(), GENREPORT_KEY)

			// 合并所有节点数据
			mergedData := mergeAllNodesData(&report)
			mergedBytes, _ := json.Marshal(mergedData)

			genReport := &scanrpc.SCAN_InvokeScriptRequest{
				TaskId:          req.TaskId,
				RuntimeId:       req.RuntimeId,
				SubTaskId:       req.SubTaskId,
				ScriptContent:   string(EmbedGenReport),
				ScriptJsonParam: string(mergedBytes),
			}

			_, err := helper.DoSCAN_InvokeScript(ctx, node, genReport, broker)
			if err != nil {
				return err
			}

			log.Infof("genReportFromKey success for multi-node task, merged data from %d nodes",
				report.TaskMeta.TotalNodes)
		} else {
			log.Infof("waiting for more nodes to complete: %d/%d",
				len(report.NodesData), report.TaskMeta.TotalNodes)
		}
	}
	return nil
}

// 兼容旧格式的处理逻辑
func genReportFromKeyLegacy(ctx context.Context, node string, helper *scanrpc.SCANServerHelper, broker *mq.Broker, req *scanrpc.SCAN_InvokeScriptRequest, value string) error {
	vj := gjson.Parse(value)
	nodeNum := vj.Get("node_num").Int()

	if nodeNum == 0 {
		yakit.DelKey(consts.GetGormProfileDatabase(), GENREPORT_KEY)
		genReport := &scanrpc.SCAN_InvokeScriptRequest{
			TaskId:          req.TaskId,
			RuntimeId:       req.RuntimeId,
			SubTaskId:       req.SubTaskId,
			ScriptContent:   string(EmbedGenReport),
			ScriptJsonParam: value,
		}
		_, err := helper.DoSCAN_InvokeScript(ctx, node, genReport, broker)
		if err != nil {
			return err
		}
		log.Info("genReportFromKey success (legacy mode)")
	} else {
		// 原有的减1逻辑
		if nodeNum > 0 {
			var jsonData map[string]interface{}
			if err := json.Unmarshal([]byte(value), &jsonData); err != nil {
				log.Errorf("failed to unmarshal json: %v", err)
				return err
			}

			jsonData["node_num"] = nodeNum - 1
			updatedBytes, err := json.Marshal(jsonData)
			if err != nil {
				log.Errorf("failed to marshal json: %v", err)
				return err
			}

			yakit.SetKey(consts.GetGormProfileDatabase(), GENREPORT_KEY, string(updatedBytes))
			log.Infof("updated node_num from %d to %d (legacy mode)", nodeNum, nodeNum-1)
		}
	}
	return nil
}
