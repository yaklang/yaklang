package yakgrpc

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ExportKnowledgeBase 导出知识库到指定路径
func (s *Server) ExportKnowledgeBase(req *ypb.ExportKnowledgeBaseRequest, stream ypb.Yak_ExportKnowledgeBaseServer) error {
	db := consts.GetGormProfileDatabase()
	ctx := stream.Context()

	// 调用导出函数，使用真实的进度回调
	reader, err := knowledgebase.ExportKnowledgeBase(ctx, db, &knowledgebase.ExportKnowledgeBaseOptions{
		KnowledgeBaseId: req.GetKnowledgeBaseId(),
		OnProgressHandler: func(percent float64, message string, messageType string) {
			// 将进度映射到0-90%范围，为文件写入保留10%
			mappedPercent := percent * 0.9
			if err := stream.Send(&ypb.GeneralProgress{
				Percent:     mappedPercent,
				Message:     message,
				MessageType: messageType,
			}); err != nil {
				// 记录错误但不中断导出过程
				fmt.Printf("发送进度消息失败: %v\n", err)
			}
		},
	})
	if err != nil {
		return utils.Wrap(err, "导出知识库失败")
	}

	// 发送文件写入开始消息
	if err := stream.Send(&ypb.GeneralProgress{
		Percent:     90,
		Message:     "正在写入文件...",
		MessageType: "info",
	}); err != nil {
		return err
	}

	// 确保目标目录存在
	targetDir := filepath.Dir(req.GetTargetPath())
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return utils.Wrap(err, "创建目标目录失败")
	}

	// 创建目标文件
	file, err := os.Create(req.GetTargetPath())
	if err != nil {
		return utils.Wrap(err, "创建目标文件失败")
	}
	defer file.Close()

	// 将导出数据写入文件
	written, err := io.Copy(file, reader)
	if err != nil {
		return utils.Wrap(err, "写入文件失败")
	}

	// 发送完成消息
	if err := stream.Send(&ypb.GeneralProgress{
		Percent:     100,
		Message:     fmt.Sprintf("导出完成，文件大小: %d 字节，保存到: %s", written, req.GetTargetPath()),
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

	// 检查输入文件是否存在
	if _, err := os.Stat(req.GetInputPath()); os.IsNotExist(err) {
		return utils.Errorf("输入文件不存在: %s", req.GetInputPath())
	}

	// 打开输入文件
	file, err := os.Open(req.GetInputPath())
	if err != nil {
		return utils.Wrap(err, "打开输入文件失败")
	}
	defer file.Close()

	// 发送文件读取完成消息
	if err := stream.Send(&ypb.GeneralProgress{
		Percent:     5,
		Message:     "文件读取完成，开始导入知识库...",
		MessageType: "info",
	}); err != nil {
		return err
	}

	// 获取新知识库名称
	newKnowledgeBaseName := req.NewKnowledgeBaseName

	// 调用导入函数，使用真实的进度回调
	err = knowledgebase.ImportKnowledgeBase(ctx, db, file, &knowledgebase.ImportKnowledgeBaseOptions{
		NewKnowledgeBaseName: newKnowledgeBaseName,
		OverwriteExisting:    true,
		OnProgressHandler: func(percent float64, message string, messageType string) {
			// 将进度映射到5-100%范围，前5%用于文件读取
			mappedPercent := 5 + (percent * 0.95)
			if err := stream.Send(&ypb.GeneralProgress{
				Percent:     mappedPercent,
				Message:     message,
				MessageType: messageType,
			}); err != nil {
				// 记录错误但不中断导入过程
				fmt.Printf("发送进度消息失败: %v\n", err)
			}
		},
	})
	if err != nil {
		return utils.Wrap(err, "导入知识库失败")
	}

	return nil
}

// ImportKnowledgeBaseWithName 导入知识库并指定新名称（扩展版本）
// 这个函数处理了 proto 定义问题，提供了正确的字符串类型参数
func (s *Server) ImportKnowledgeBaseWithName(ctx context.Context, inputPath string, newName string, overwriteExisting bool, progressCallback func(percent float64, message string, messageType string)) error {
	db := consts.GetGormProfileDatabase()

	// 检查输入文件是否存在
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return utils.Errorf("输入文件不存在: %s", inputPath)
	}

	// 打开输入文件
	file, err := os.Open(inputPath)
	if err != nil {
		return utils.Wrap(err, "打开输入文件失败")
	}
	defer file.Close()

	if progressCallback != nil {
		progressCallback(5, "文件读取完成，开始导入知识库...", "info")
	}

	// 调用导入函数，使用真实的进度回调
	err = knowledgebase.ImportKnowledgeBase(ctx, db, file, &knowledgebase.ImportKnowledgeBaseOptions{
		NewKnowledgeBaseName: newName,
		OverwriteExisting:    overwriteExisting,
		OnProgressHandler: func(percent float64, message string, messageType string) {
			if progressCallback != nil {
				// 将进度映射到5-100%范围，前5%用于文件读取
				mappedPercent := 5 + (percent * 0.95)
				progressCallback(mappedPercent, message, messageType)
			}
		},
	})
	if err != nil {
		return utils.Wrap(err, "导入知识库失败")
	}

	return nil
}

// ExportKnowledgeBaseToFile 导出知识库到文件（同步版本）
func (s *Server) ExportKnowledgeBaseToFile(ctx context.Context, knowledgeBaseId int64, targetPath string) error {
	db := consts.GetGormProfileDatabase()

	// 调用导出函数，不使用进度回调（同步版本）
	reader, err := knowledgebase.ExportKnowledgeBase(ctx, db, &knowledgebase.ExportKnowledgeBaseOptions{
		KnowledgeBaseId: knowledgeBaseId,
	})
	if err != nil {
		return utils.Wrap(err, "导出知识库失败")
	}

	// 确保目标目录存在
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return utils.Wrap(err, "创建目标目录失败")
	}

	// 创建目标文件
	file, err := os.Create(targetPath)
	if err != nil {
		return utils.Wrap(err, "创建目标文件失败")
	}
	defer file.Close()

	// 将导出数据写入文件
	_, err = io.Copy(file, reader)
	if err != nil {
		return utils.Wrap(err, "写入文件失败")
	}

	return nil
}

// ExportKnowledgeBaseToFileWithProgress 导出知识库到文件（带进度回调的版本）
func (s *Server) ExportKnowledgeBaseToFileWithProgress(ctx context.Context, knowledgeBaseId int64, targetPath string, progressCallback func(percent float64, message string, messageType string)) error {
	db := consts.GetGormProfileDatabase()

	// 调用导出函数，使用进度回调
	reader, err := knowledgebase.ExportKnowledgeBase(ctx, db, &knowledgebase.ExportKnowledgeBaseOptions{
		KnowledgeBaseId: knowledgeBaseId,
		OnProgressHandler: func(percent float64, message string, messageType string) {
			if progressCallback != nil {
				// 将进度映射到0-90%范围，为文件写入保留10%
				mappedPercent := percent * 0.9
				progressCallback(mappedPercent, message, messageType)
			}
		},
	})
	if err != nil {
		return utils.Wrap(err, "导出知识库失败")
	}

	if progressCallback != nil {
		progressCallback(90, "正在写入文件...", "info")
	}

	// 确保目标目录存在
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return utils.Wrap(err, "创建目标目录失败")
	}

	// 创建目标文件
	file, err := os.Create(targetPath)
	if err != nil {
		return utils.Wrap(err, "创建目标文件失败")
	}
	defer file.Close()

	// 将导出数据写入文件
	written, err := io.Copy(file, reader)
	if err != nil {
		return utils.Wrap(err, "写入文件失败")
	}

	if progressCallback != nil {
		progressCallback(100, fmt.Sprintf("导出完成，文件大小: %d 字节，保存到: %s", written, targetPath), "success")
	}

	return nil
}
