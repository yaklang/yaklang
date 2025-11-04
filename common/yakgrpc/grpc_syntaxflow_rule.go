package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QuerySyntaxFlowRule(ctx context.Context, req *ypb.QuerySyntaxFlowRuleRequest) (*ypb.QuerySyntaxFlowRuleResponse, error) {
	p, data, err := yakit.QuerySyntaxFlowRule(s.GetProfileDatabase(), req)
	if err != nil {
		return nil, err
	}
	rsp := &ypb.QuerySyntaxFlowRuleResponse{
		Pagination: req.GetPagination(),
		Total:      uint64(p.TotalRecord),
		DbMessage: &ypb.DbOperateMessage{
			TableName: "syntax_flow_rule",
			Operation: DbOperationQuery,
		},
	}
	for _, d := range data {
		rsp.Rule = append(rsp.Rule, d.ToGRPCModel())
	}
	return rsp, nil
}

func (s *Server) CreateSyntaxFlowRuleEx(ctx context.Context, req *ypb.CreateSyntaxFlowRuleRequest) (*ypb.CreateSyntaxFlowRuleResponse, error) {
	if req == nil || req.GetSyntaxFlowInput() == nil {
		return nil, utils.Error("create syntax flow rule failed: request is nil")
	}

	input := req.GetSyntaxFlowInput()
	rule, err := yakit.ParseSyntaxFlowInput(input)
	if err != nil {
		return nil, err
	}
	_, err = sfdb.CreateRuleWithDefaultGroup(rule, input.GetGroupNames()...)
	if err != nil {
		return nil, err
	}
	return &ypb.CreateSyntaxFlowRuleResponse{
		Rule: rule.ToGRPCModel(),
		Message: &ypb.DbOperateMessage{
			TableName:  "syntax_flow_rule",
			Operation:  DbOperationCreate,
			EffectRows: 1,
		},
	}, nil
}

func (s *Server) CreateSyntaxFlowRule(ctx context.Context, req *ypb.CreateSyntaxFlowRuleRequest) (*ypb.DbOperateMessage, error) {
	if ret, err := s.CreateSyntaxFlowRuleEx(ctx, req); err != nil {
		return nil, err
	} else {
		return ret.Message, nil
	}
}

func (s *Server) UpdateSyntaxFlowRuleEx(ctx context.Context, req *ypb.UpdateSyntaxFlowRuleRequest) (*ypb.UpdateSyntaxFlowRuleResponse, error) {
	if req == nil || req.SyntaxFlowInput == nil {
		return nil, utils.Error("update syntax flow rule failed: request is nil")
	}
	updatedRule, err := yakit.UpdateSyntaxFlowRule(s.GetProfileDatabase(), req.SyntaxFlowInput)
	if err != nil {
		return nil, err
	}
	return &ypb.UpdateSyntaxFlowRuleResponse{
		Message: &ypb.DbOperateMessage{
			TableName:  "syntax_flow_rule",
			Operation:  DbOperationCreateOrUpdate,
			EffectRows: 1,
		},
		Rule: updatedRule.ToGRPCModel(),
	}, nil
}

func (s *Server) UpdateSyntaxFlowRule(ctx context.Context, req *ypb.UpdateSyntaxFlowRuleRequest) (*ypb.DbOperateMessage, error) {
	if ret, err := s.UpdateSyntaxFlowRuleEx(ctx, req); err != nil {
		return nil, err
	} else {
		return ret.Message, nil
	}
}

func (s *Server) DeleteSyntaxFlowRule(ctx context.Context, req *ypb.DeleteSyntaxFlowRuleRequest) (*ypb.DbOperateMessage, error) {
	msg := &ypb.DbOperateMessage{
		TableName:    "syntax_flow_rule",
		Operation:    DbOperationDelete,
		EffectRows:   0,
		ExtraMessage: "",
	}
	count, err := yakit.DeleteSyntaxFlowRule(s.GetProfileDatabase(), req)
	msg.EffectRows = count
	return msg, err
}

type msgType string

const (
	SUCCESS msgType = "success"
	WARNING msgType = "warning"
	ERROR   msgType = "error"
	INFO    msgType = "info"
	DATA    msgType = "data"
)

type conflictInfo struct {
	Local  string `json:"local"`
	Remote string `json:"remote"`
}

// SyntaxFlowRuleToOnline 上传本地规则到在线服务
//
// 上传规则时的处理逻辑：
//
//	┌────────────┬──────────┬──────────┬─────────┬──────────────────────────────────┐
//	│ 远程规则   │ 本地版本  │ 远程版本  │ Dirty   │ 处理动作                          │
//	├────────────┼──────────┼──────────┼─────────┼──────────────────────────────────┤
//	│ 不存在     │ v1.0      │ -        │ - 	  │ 上传规则（首次发布）           	  │
//	│ 存在       │ v3.0      │ v2.0     │ true    │ 覆盖上传（本地有修改且版本更新）   │
//	│ 存在       │ v3.0      │ v2.0     │ false   │ 逻辑错误				        │
//	│ 存在       │ v1.0/v2.0 │ v2.0     │ true    │ 冲突-跳过（本地有修改）      	  │
//	│ 存在       │ v1.0/v2.0 │ v2.0     │ false   │ 跳过上传（提示需要更新）          │
//	└────────────┴──────────┴──────────┴─────────┴──────────────────────────────────┘
//
// 版本比较规则：
//   - CheckNewerVersion(remote, local) 返回 true 表示远程版本 > 本地版本
//   - 使用字符串字典序比较
//   - 内置规则（IsBuildInRule=true）不允许上传
//
// Dirty 标记说明（上传场景）：
//   - true: 本地有修改，应该上传以同步到远程
//   - false: 本地未修改，可根据版本决定是否上传
//

func (s *Server) SyntaxFlowRuleToOnline(req *ypb.SyntaxFlowRuleToOnlineRequest, stream ypb.Yak_SyntaxFlowRuleToOnlineServer) error {
	// 参数校验
	if req.Token == "" {
		return utils.Error("token is empty")
	}
	if req.Filter == nil {
		req.Filter = &ypb.SyntaxFlowRuleFilter{}
	}

	ctx := stream.Context()
	ret, err := yakit.AllSyntaxFlowRule(s.GetProfileDatabase(), req.GetFilter())
	if err != nil {
		return utils.Errorf("query failed: %s", err)
	}

	// 初始化进度跟踪变量
	var (
		successCount  int
		errorCount    int
		skippedCount  int
		conflictCount int
	)

	sendProgress(stream, 0, "准备上传规则......", string(INFO))

	client := yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())

	var ruleIds []string
	for _, r := range ret {
		ruleIds = append(ruleIds, r.RuleId)
	}
	remoteVersionMap, prefetchErr := fetchRemoteRuleVersionMap(stream.Context(), client, req.Token, ruleIds)
	if prefetchErr != nil {
		sendProgress(stream, 0, fmt.Sprintf("获取远端版本失败: %v，将继续尝试上传", prefetchErr), string(WARNING))
	}

	for i, k := range ret {
		progress := float64(i) / float64(len(ret))

		// 检查远程规则是否存在
		remoteRule, remoteExists := remoteVersionMap[k.RuleId]

		if remoteExists {
			// 远程规则存在，需要比较版本和检查 Dirty 状态
			remoteVersion := remoteRule.Version
			remoteIsNewer := sfdb.CheckNewerVersion(remoteVersion, k.Version)

			if remoteIsNewer {
				// 远程版本 > 本地版本
				if k.NeedUpdate {
					// 本地有修改，但远程版本更新 → 冲突
					conflictCount++
					sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 冲突-跳过（本地有修改但远程版本更新: 本地 %s ← 远程 %s）", k.RuleName, k.Version, remoteVersion), string(WARNING))

					data, _ := json.Marshal(&conflictInfo{
						Local:  k.Content,
						Remote: remoteRule.Content,
					})
					sendProgress(stream, progress, string(data), string(DATA))
					continue
				} else {
					// 本地无修改，远程版本更新 → 跳过上传（提示需要更新）
					skippedCount++
					sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 跳过上传（需要更新: 本地 %s ← 远程 %s）", k.RuleName, k.Version, remoteVersion), string(INFO))
					continue
				}
			} else {
				// 本地版本 >= 远程版本
				if k.NeedUpdate {
					// 本地有修改且版本更新 → 覆盖上传
					sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 覆盖上传（本地有修改且版本更新: 本地 %s → 远程 %s）", k.RuleName, k.Version, remoteVersion), string(INFO))
				} else {
					// 本地版本更新但没有修改标记 → 逻辑错误
					errorCount++
					sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 逻辑错误（本地版本 %s >= 远程 %s，但 NeedUpdate=false）", k.RuleName, k.Version, remoteVersion), string(ERROR))
					continue
				}
			}
		} else {
			// 远程规则不存在 → 上传规则（首次发布）
			sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 首次发布（版本 %s）", k.RuleName, k.Version), string(INFO))
		}

		// 执行上传
		err = uploadRule(ctx, client, req.Token, k)
		if err != nil {
			errorCount++
			sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 上传失败: %v", k.RuleName, err), string(ERROR))
			continue
		}

		successCount++
		sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 上传成功", k.RuleName), string(SUCCESS))
	}

	// 发送最终结果
	msg := fmt.Sprintf("完成: 成功 %d, 跳过 %d, 冲突 %d, 失败 %d", successCount, skippedCount, conflictCount, errorCount)
	msgType := SUCCESS
	if errorCount > 0 || skippedCount > 0 {
		msgType = WARNING
	}
	if successCount == 0 && (errorCount > 0 || skippedCount > 0) {
		msgType = ERROR
	}

	return stream.Send(&ypb.SyntaxFlowRuleOnlineProgress{
		Progress:    1,
		Message:     msg,
		MessageType: string(msgType),
	})
}

// fetchRemoteRuleVersion 拉取远端同ID的规则（若不存在则返回空字符串）
func fetchRemoteRuleVersion(ctx context.Context, client *yaklib.OnlineClient, token, ruleId string) (*yaklib.OnlineSyntaxFlowRule, error) {
	if ruleId == "" {
		return nil, utils.Error("ruleName is nil")
	}
	req := &ypb.DownloadSyntaxFlowRuleRequest{Filter: &ypb.SyntaxFlowRuleFilter{RuleIds: []string{ruleId}}}
	ch := client.DownloadOnlineSyntaxFlowRule(ctx, token, req)
	if ch == nil {
		return nil, utils.Error("download stream is nil")
	}
	for item := range ch.Chan {
		if item != nil && item.Rule != nil && item.Rule.RuleId == ruleId {
			return item.Rule, nil
		}
	}
	return nil, nil
}

// fetchRemoteRuleVersionMap 一次性拉取多个规则的远端版本
func fetchRemoteRuleVersionMap(ctx context.Context, client *yaklib.OnlineClient, token string, ruleIds []string) (map[string]*yaklib.OnlineSyntaxFlowRule, error) {
	versionMap := make(map[string]*yaklib.OnlineSyntaxFlowRule)
	if len(ruleIds) == 0 {
		return versionMap, nil
	}
	req := &ypb.DownloadSyntaxFlowRuleRequest{Filter: &ypb.SyntaxFlowRuleFilter{RuleIds: ruleIds}}
	ch := client.DownloadOnlineSyntaxFlowRule(ctx, token, req)
	if ch == nil {
		return nil, utils.Error("download stream is nil")
	}
	for item := range ch.Chan {
		if item != nil && item.Rule != nil {
			versionMap[item.Rule.RuleId] = item.Rule
		}
	}
	return versionMap, nil
}

func uploadRule(ctx context.Context, client *yaklib.OnlineClient, token string, rule *schema.SyntaxFlowRule) error {
	content, err := json.Marshal(rule)
	if err != nil {
		return fmt.Errorf("序列化失败: %w", err)
	}

	raw, err := json.Marshal(yaklib.UploadOnlineRequest{
		Content: content,
	})
	if err != nil {
		return fmt.Errorf("请求构造失败: %w", err)
	}

	return client.UploadToOnline(ctx, token, raw, "api/flow/rule/upload")
}

type ProgressStream interface {
	Send(*ypb.SyntaxFlowRuleOnlineProgress) error
}

// 统一发送进度
func sendProgress(stream ProgressStream, progress float64, message, messageType string) {
	stream.Send(&ypb.SyntaxFlowRuleOnlineProgress{
		Progress:    progress,
		Message:     message,
		MessageType: messageType,
	})
}

// DownloadSyntaxFlowRule 从在线服务下载规则到本地
//
// 下载规则时的处理逻辑：
//
//	┌────────────┬──────────┬──────────┬─────────┬────────────────────────────┐
//	│ 本地规则   │ 本地版本  │ 在线版本  │ Dirty   │ 处理动作                   │
//	├────────────┼──────────┼──────────┼─────────┼────────────────────────────┤
//	│ 不存在     │ -        │ v2.0     │ -       │ 下载规则                   │
//	│ 存在       │ v1.0     │ v2.0     │ false   │ 更新规则（在线版本更新）   │
//	│ 存在       │ v3.0     │ v2.0     │ false   │ 跳过更新（本地版本更新）   │
//	│ 存在       │ v1.0/v2.0│ v2.0     │ true    │ 冲突-跳过（本地有修改）    │
//	│ 存在       │ v2.0     │ v2.0     │ false   │ 更新规则（强制同步）       │
//	└────────────┴──────────┴──────────┴─────────┴────────────────────────────┘
//
// 版本比较规则：
//   - CheckNewerVersion(local, online) 返回 true 表示本地版本 > 在线版本
//   - 使用字符串字典序比较（注意：v10.0 < v2.0）
//   - 建议使用语义化版本号：1.0.0, 2.0.0 等
//
// Dirty 标记说明：
//   - true: 本地有修改，需要冲突处理，跳过更新
//   - false: 本地未修改，可以安全覆盖
func (s *Server) DownloadSyntaxFlowRule(req *ypb.DownloadSyntaxFlowRuleRequest, stream ypb.Yak_DownloadSyntaxFlowRuleServer) error {
	client := yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())
	var (
		ch    *yaklib.OnlineDownloadFlowRuleStream
		token string
	)
	if req.Filter == nil {
		req.Filter = &ypb.SyntaxFlowRuleFilter{}
	}
	if req.Token != "" {
		token = req.Token
	}
	ch = client.DownloadOnlineSyntaxFlowRule(stream.Context(), token, req)

	if ch == nil {
		return utils.Error("BUG: download stream error: empty")
	}

	// 初始化进度跟踪变量
	var (
		total           int64
		successCount    int
		errorCount      int
		skippedCount    int
		conflictCount   int
		count, progress float64
	)

	// 发送初始化进度

	sendProgress(stream, 0, "下载规则......", string(INFO))

	for resultIns := range ch.Chan {
		result := resultIns.Rule
		total = resultIns.Total
		if total > 0 {
			progress = count / float64(total)
		}
		count++

		// 查询本地规则
		localRule, err := sfdb.QueryRuleByRuleId(s.GetProfileDatabase(), result.RuleId)

		if err == nil && localRule != nil {
			// 本地规则存在 - 需要比较版本和检查 Dirty 状态
			localIsNewer := sfdb.CheckNewerVersion(localRule.Version, result.Version)

			if localIsNewer {
				// 本地版本 > 在线版本（如 v3.0 > v2.0）
				if localRule.NeedUpdate {
					// NeedUpdate=true 时理论上不应该出现本地版本更新的情况
					// 这可能表示用户手动修改了版本号
					errorCount++
					sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 本地版本更新且有修改 (本地 %s > 在线 %s)，跳过更新", result.RuleName, localRule.Version, result.Version), string(INFO))
				} else {
					// NeedUpdate=false 且本地版本更新 → 跳过更新
					skippedCount++
					sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 本地版本更新 (本地 %s > 在线 %s)，跳过更新", result.RuleName, localRule.Version, result.Version), string(INFO))
				}
				continue
			} else {
				// 在线版本 >= 本地版本
				if localRule.NeedUpdate {
					// 本地有修改 → 冲突-跳过
					conflictCount++
					conflictMsg := fmt.Sprintf("规则 [%s] 内容冲突-跳过（本地有修改: 本地 v%s ← 在线 v%s）",
						result.RuleName, localRule.Version, result.Version)
					sendProgress(stream, progress, conflictMsg, string(WARNING))

					data, _ := json.Marshal(&conflictInfo{
						Local:  localRule.Content,
						Remote: result.Content,
					})
					sendProgress(stream, progress, string(data), string(DATA))
					continue
				} else {
					// NeedUpdate=false → 可以安全更新
					sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 有新版本 (本地 %s → 在线 %s)，开始更新", result.RuleName, localRule.Version, result.Version), string(INFO))
				}
			}
		} else {
			// 本地规则不存在 → 下载规则
			sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 首次下载 (版本 %s)", result.RuleName, result.Version), string(INFO))
		}

		// 执行保存
		err = client.SaveSyntaxFlowRule(s.GetProfileDatabase(), result)
		if err != nil {
			errorCount++
			sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 保存失败: %s", result.RuleName, err), string(ERROR))
			continue
		}
		successCount++
		sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 保存成功", result.RuleName), string(SUCCESS))
	}
	// 发送最终结果
	msg := fmt.Sprintf("完成: 成功 %d, 跳过 %d, 冲突 %d, 失败 %d", successCount, skippedCount, conflictCount, errorCount)
	msgType := SUCCESS
	if errorCount > 0 || conflictCount > 0 {
		msgType = WARNING
	}
	if successCount == 0 && (errorCount > 0 || conflictCount > 0) {
		msgType = ERROR
	}

	return stream.Send(&ypb.SyntaxFlowRuleOnlineProgress{
		Progress:    1,
		Message:     msg,
		MessageType: string(msgType),
	})
}
