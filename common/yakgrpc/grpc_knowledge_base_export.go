package yakgrpc

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ExportKnowledgeBase 导出知识库到指定路径
func (s *Server) ExportKnowledgeBase(req *ypb.ExportKnowledgeBaseRequest, stream ypb.Yak_ExportKnowledgeBaseServer) error {
	db := consts.GetGormProfileDatabase()
	ctx := stream.Context()

	ragName := req.GetName()

	kb, err := yakit.GetKnowledgeBase(db, req.GetKnowledgeBaseId())
	if err != nil {
		return utils.Wrap(err, "获取知识库失败")
	}

	ragName = kb.KnowledgeBaseName

	// 确保目标目录存在
	targetDir := filepath.Dir(req.GetTargetPath())
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return utils.Wrap(err, "创建目标目录失败")
	}

	// 导出RAG
	err = rag.ExportRAG(ragName, req.GetTargetPath(),
		rag.WithRAGCtx(ctx),
		rag.WithExportOnProgressHandler(func(percent float64, message string, messageType string) {
			// 将进度映射到0-90%范围，为文件写入保留10%
			mappedPercent := percent
			if err := stream.Send(&ypb.GeneralProgress{
				Percent:     mappedPercent,
				Message:     message,
				MessageType: messageType,
			}); err != nil {
				// 记录错误但不中断导出过程
				fmt.Printf("发送进度消息失败: %v\n", err)
			}
		}))
	if err != nil {
		return utils.Wrap(err, "导出知识库失败")
	}

	// 发送完成消息
	if err := stream.Send(&ypb.GeneralProgress{
		Percent:     100,
		Message:     fmt.Sprintf("导出完成，保存到: %s", req.GetTargetPath()),
		MessageType: "success",
	}); err != nil {
		return err
	}

	return nil
}

// ImportKnowledgeBase 从指定路径导入知识库
func (s *Server) ImportKnowledgeBase(req *ypb.ImportKnowledgeBaseRequest, stream ypb.Yak_ImportKnowledgeBaseServer) error {
	db := consts.GetGormProfileDatabase()
	ctx := stream.Context()

	// 发送文件读取开始消息
	if err := stream.Send(&ypb.GeneralProgress{
		Percent:     0,
		Message:     "正在读取文件...",
		MessageType: "info",
	}); err != nil {
		return err
	}

	err := rag.ImportRAG(req.GetInputPath(),
		rag.WithRAGCtx(ctx),
		rag.WithExportOverwriteExisting(true),
		rag.WithName(req.NewKnowledgeBaseName),
		rag.WithExportOnProgressHandler(func(percent float64, message string, messageType string) {
			mappedPercent := 5 + (percent * 0.95)
			if err := stream.Send(&ypb.GeneralProgress{
				Percent:     mappedPercent,
				Message:     message,
				MessageType: messageType,
			}); err != nil {
				// 记录错误但不中断导入过程
				fmt.Printf("发送进度消息失败: %v\n", err)
			}
		}),
		rag.WithDB(db),
	)

	if err != nil {
		return utils.Wrap(err, "导入知识库失败")
	}

	return nil
}
