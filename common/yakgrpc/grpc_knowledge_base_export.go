package yakgrpc

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/ai/rag/entityrepos"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ExportKnowledgeBase 导出知识库到指定路径
func (s *Server) ExportKnowledgeBase(req *ypb.ExportKnowledgeBaseRequest, stream ypb.Yak_ExportKnowledgeBaseServer) error {
	db := consts.GetGormProfileDatabase()
	ctx := stream.Context()

	kb, err := yakit.GetKnowledgeBase(db, req.GetKnowledgeBaseId())
	if err != nil {
		return utils.Wrap(err, "获取知识库失败")
	}

	var entityRepository schema.EntityRepository
	err = db.Model(&schema.EntityRepository{}).Where("entity_base_name = ?", kb.KnowledgeBaseName).First(&entityRepository).Error
	if err != nil {
		return utils.Wrap(err, "获取实体仓库失败")
	}

	entityReposReader, err := entityrepos.ExportEntityRepository(ctx, db, &entityrepos.ExportEntityRepositoryOptions{
		RepositoryID: int64(entityRepository.ID),
		OnProgressHandler: func(percent float64, message string, messageType string) {
			if err := stream.Send(&ypb.GeneralProgress{
				Percent:     percent,
				Message:     message,
				MessageType: messageType,
			}); err != nil {
			}
		},
	})
	if err != nil {
		return utils.Wrap(err, "导出实体仓库失败")
	}

	// 调用导出函数，使用真实的进度回调
	reader, err := knowledgebase.ExportKnowledgeBase(ctx, db, &knowledgebase.ExportKnowledgeBaseOptions{
		KnowledgeBaseId: req.GetKnowledgeBaseId(),
		ExtraDataReader: entityReposReader,
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
		ExtraDataHandler: func(extraData io.Reader) error {
			return entityrepos.ImportEntityRepository(ctx, db, extraData, &entityrepos.ImportEntityRepositoryOptions{
				OverwriteExisting: true,
				NewRepositoryName: newKnowledgeBaseName,
			})
		},
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
