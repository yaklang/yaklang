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

// 只需要配置关键信息，其他由AI生成
func (s *Server) CreateSyntaxFlowRuleAuto(ctx context.Context, req *ypb.CreateSyntaxFlowRuleAutoRequest) (*ypb.CreateSyntaxFlowRuleResponse, error) {
	if req == nil || req.GetSyntaxFlowInput() == nil {
		return nil, utils.Error("create syntax flow rule failed: request is nil")
	}

	input := req.GetSyntaxFlowInput()
	rule, err := yakit.ParseSyntaxFlowAutoInput(input)
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
		successCount int
		errorCount   int
	)

	sendProgress(stream, 0, "准备上传规则......", "info")

	client := yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())

	var ruleIds []string
	for _, r := range ret {
		ruleIds = append(ruleIds, r.RuleId)
	}
	remoteVersionMap, prefetchErr := fetchRemoteRuleVersionMap(stream.Context(), client, req.Token, ruleIds)
	if prefetchErr != nil {
		sendProgress(stream, 0, fmt.Sprintf("获取远端版本失败: %v，将继续尝试上传", err), "warning")
	}

	for i, k := range ret {
		progress := float64(i) / float64(len(ret))

		if remoteVersion, ok := remoteVersionMap[k.RuleName]; ok {
			if shouldSkipUpload(k.Version, remoteVersion) {
				sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 远端版本为最新 (远端: %s，本地: %s)，跳过上传", k.RuleName, remoteVersion, k.Version), "info")
				continue
			}

			if remoteVersion != "" {
				sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 发现远端旧版本 (远端: %s → 本地: %s)，开始覆盖上传", k.RuleName, remoteVersion, k.Version), "info")
			}
		}

		err = uploadRule(ctx, client, req.Token, k)
		if err != nil {
			errorCount++
			sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 上传失败: %v", k.RuleName, err), "error")
			continue
		}

		successCount++
		sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 上传成功", k.RuleName), "success")
	}

	// 发送最终结果
	msg := fmt.Sprintf("筛选数据为空或全为内置规则暂无法上传，请重新筛选数据上传")
	if len(ret) > 0 {
		msg = fmt.Sprintf("完成: 成功 %d, 失败 %d", successCount, errorCount)
	}
	msgType := "success"
	if errorCount > 0 {
		msgType = "warning"
	}
	if successCount == 0 && errorCount > 0 {
		msgType = "error"
	}

	return stream.Send(&ypb.SyntaxFlowRuleOnlineProgress{
		Progress:    1,
		Message:     msg,
		MessageType: msgType,
	})
}

// fetchRemoteRuleVersion 拉取远端同名规则的版本（若不存在则返回空字符串）
func fetchRemoteRuleVersion(ctx context.Context, client *yaklib.OnlineClient, token, ruleName string) (string, error) {
	if ruleName == "" {
		return "", nil
	}
	req := &ypb.DownloadSyntaxFlowRuleRequest{Filter: &ypb.SyntaxFlowRuleFilter{RuleNames: []string{ruleName}}}
	ch := client.DownloadOnlineSyntaxFlowRule(ctx, token, req)
	if ch == nil {
		return "", utils.Error("download stream is nil")
	}
	for item := range ch.Chan {
		if item != nil && item.Rule != nil {
			return item.Rule.Version, nil
		}
	}
	return "", nil
}

// fetchRemoteRuleVersionMap 一次性拉取多个规则的远端版本
func fetchRemoteRuleVersionMap(ctx context.Context, client *yaklib.OnlineClient, token string, ruleNames []string) (map[string]string, error) {
	versionMap := make(map[string]string)
	if len(ruleNames) == 0 {
		return versionMap, nil
	}
	req := &ypb.DownloadSyntaxFlowRuleRequest{Filter: &ypb.SyntaxFlowRuleFilter{RuleNames: ruleNames}}
	ch := client.DownloadOnlineSyntaxFlowRule(ctx, token, req)
	if ch == nil {
		return nil, utils.Error("download stream is nil")
	}
	for item := range ch.Chan {
		if item != nil && item.Rule != nil {
			versionMap[item.Rule.RuleId] = item.Rule.Version
		}
	}
	return versionMap, nil
}

func shouldSkipUpload(localVersion, remoteVersion string) bool {
	if remoteVersion == "" {
		return false
	}
	if localVersion == "" {
		return true
	}
	return localVersion <= remoteVersion
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
		count, progress float64
	)

	// 发送初始化进度

	sendProgress(stream, 0, "下载规则......", "info")

	for resultIns := range ch.Chan {
		result := resultIns.Rule
		total = resultIns.Total
		if total > 0 {
			progress = count / float64(total)
		}
		count++

		localRule, err := sfdb.QueryRuleByRuleId(s.GetProfileDatabase(), result.RuleId)
		if err == nil && localRule != nil {
			if shouldSkipUpdate(localRule.Version, result.Version) {
				skippedCount++
				sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 已是最新版本 (本地: %s, 在线: %s)，跳过更新", result.RuleName, localRule.Version, result.Version), "info")
				continue
			}
			sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 有新版本 (本地: %s → 在线: %s)，开始更新", result.RuleName, localRule.Version, result.Version), "info")
		}

		err = client.SaveSyntaxFlowRule(s.GetProfileDatabase(), result)
		if err != nil {
			errorCount++
			sendProgress(stream, progress, fmt.Sprintf("save [%s] to local db failed: %s", result.RuleName, err), "error")
			continue
		}
		successCount++
		sendProgress(stream, progress, fmt.Sprintf("save [%s] to local db success", result.RuleName), "success")
	}
	// 发送最终结果
	msg := fmt.Sprintf("完成: 成功 %d, 跳过 %d, 失败 %d", successCount, skippedCount, errorCount)
	msgType := "success"
	if errorCount > 0 {
		msgType = "warning"
	}
	if successCount == 0 && errorCount > 0 {
		msgType = "error"
	}

	return stream.Send(&ypb.SyntaxFlowRuleOnlineProgress{
		Progress:    1,
		Message:     msg,
		MessageType: msgType,
	})
}

func shouldSkipUpdate(localVersion, onlineVersion string) bool {
	if onlineVersion == "" {
		return true
	}

	if localVersion == "" {
		return false
	}
	return localVersion >= onlineVersion
}
