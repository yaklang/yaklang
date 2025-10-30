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

	for i, k := range ret {
		progress := float64(i) / float64(len(ret))

		err := uploadRule(ctx, client, req.Token, k)
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
		err := client.SaveSyntaxFlowRule(s.GetProfileDatabase(), result)
		if err != nil {
			errorCount++
			sendProgress(stream, progress, fmt.Sprintf("save [%s] to local db failed: %s", result.RuleName, err), "error")
			continue
		}
		successCount++
		sendProgress(stream, progress, fmt.Sprintf("save [%s] to local db success: %s", result.RuleName, err), "success")
	}
	// 发送最终结果
	msg := fmt.Sprintf("完成: 成功 %d, 失败 %d", successCount, errorCount)
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

// DownloadSyntaxFlowRuleFromOSS 从OSS下载规则并保存到本地数据库
// func (s *Server) DownloadSyntaxFlowRuleFromOSS(req *ypb.DownloadSyntaxFlowRuleFromOSSRequest, stream ypb.Yak_DownloadSyntaxFlowRuleFromOSSServer) error {
// 	// 参数校验
// 	if req.OssConfig == nil {
// 		return utils.Error("OSS config is empty")
// 	}
// 	config := req.OssConfig

// 	// 创建 OSS 客户端
// 	var ossClient yaklib.OSSClient
// 	var err error

// 	switch config.Type {
// 	case "aliyun":
// 		ossClient, err = yaklib.NewAliyunOSSClient(config.Endpoint, config.AccessKeyId, config.AccessKeySecret)
// 		if err != nil {
// 			return utils.Wrapf(err, "create aliyun OSS client failed")
// 		}
// 	default:
// 		return utils.Errorf("unsupported OSS type: %s", config.Type)
// 	}
// 	defer ossClient.Close()

// 	// OSS 配置
// 	bucket := config.Bucket
// 	if bucket == "" {
// 		bucket = "yaklang-rules"
// 	}
// 	prefix := config.Prefix
// 	if prefix == "" {
// 		prefix = "syntaxflow/"
// 	}

// 	// 发送初始化进度
// 	sendProgress(stream, 0, fmt.Sprintf("准备从OSS下载规则 (bucket: %s, prefix: %s)", bucket, prefix), "info")

// 	// 从 OSS 下载规则文件
// 	ctx := stream.Context()
// 	ossStream := yaklib.DownloadOSSSyntaxFlowRuleFiles(ctx, ossClient, bucket, prefix)

// 	// 解析并保存规则
// 	var (
// 		total           int64
// 		successCount    int
// 		errorCount      int
// 		count, progress float64
// 	)

// 	for item := range ossStream.Chan {
// 		if item.Error != nil {
// 			errorCount++
// 			sendProgress(stream, progress, fmt.Sprintf("下载规则文件失败: %v", item.Error), "error")
// 			continue
// 		}

// 		if total == 0 {
// 			total = ossStream.Total
// 		}
// 		if total > 0 {
// 			progress = count / float64(total)
// 		}
// 		count++

// 		// 解析规则内容
// 		rule, err := sfdb.CheckSyntaxFlowRuleContent(item.Content)
// 		if err != nil {
// 			errorCount++
// 			sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 解析失败: %v", item.RuleName, err), "error")
// 			continue
// 		}

// 		// 设置规则名称
// 		rule.RuleName = item.RuleName

// 		// 保存到数据库
// 		_, err = sfdb.CreateOrUpdateRuleWithGroup(rule)
// 		if err != nil {
// 			errorCount++
// 			sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 保存失败: %v", item.RuleName, err), "error")
// 			continue
// 		}

// 		successCount++
// 		sendProgress(stream, progress, fmt.Sprintf("规则 [%s] 保存成功", item.RuleName), "success")
// 	}

// 	// 发送最终结果
// 	msg := fmt.Sprintf("完成: 成功 %d, 失败 %d", successCount, errorCount)
// 	msgType := "success"
// 	if errorCount > 0 {
// 		msgType = "warning"
// 	}
// 	if successCount == 0 && errorCount > 0 {
// 		msgType = "error"
// 	}

// 	return stream.Send(&ypb.SyntaxFlowRuleOnlineProgress{
// 		Progress:    1,
// 		Message:     msg,
// 		MessageType: msgType,
// 	})
// }
