package rag

import (
	"io"
	"os"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/entityrepos"
	"github.com/yaklang/yaklang/common/ai/rag/knowledgebase"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func ExportRAG(name string, filePath string, opts ...RAGSystemConfigOption) error {
	// 加载配置
	config := NewRAGSystemConfig(opts...)

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return utils.Wrap(err, "open file failed")
	}
	defer file.Close()

	// 查找集合名
	collection, err := yakit.GetRAGCollectionInfoByName(config.db, name)
	if err != nil {
		return utils.Wrap(err, "get collection failed")
	}
	if collection == nil {
		return utils.Errorf("collection[%s] not found", name)
	}

	var hasEntityRepository bool
	var hasKnowledgeBase bool

	// 查找实体库
	var entityRepository schema.EntityRepository
	err = config.db.Model(&schema.EntityRepository{}).Where("rag_id = ?", collection.RAGID).First(&entityRepository).Error
	if gorm.IsRecordNotFoundError(err) {
		hasEntityRepository = false
	} else {
		hasEntityRepository = true
	}

	// 查找知识库
	var knowledgeBase schema.KnowledgeBaseInfo
	err = config.db.Model(&schema.KnowledgeBaseInfo{}).Where("rag_id = ?", collection.RAGID).First(&knowledgeBase).Error
	if gorm.IsRecordNotFoundError(err) {
		hasKnowledgeBase = false
	} else {
		hasKnowledgeBase = true
	}

	if !hasKnowledgeBase {
		return utils.Errorf("knowledge base not found")
	}

	var entityReposReader io.Reader
	if hasEntityRepository {
		entityReposReader, err = entityrepos.ExportEntityRepository(config.ctx, config.db, &entityrepos.ExportEntityRepositoryOptions{
			RepositoryID:    int64(entityRepository.ID),
			SkipVectorStore: true,
		})
		if err != nil {
			return utils.Wrap(err, "export entity repository failed")
		}
	} else {
		entityReposReader = nil
	}

	reader, err := knowledgebase.ExportKnowledgeBase(config.ctx, config.db, &knowledgebase.ExportKnowledgeBaseOptions{
		KnowledgeBaseId:   int64(knowledgeBase.ID),
		OnProgressHandler: config.progressHandler,
		ExtraDataReader:   entityReposReader,
		VectorStoreName:   collection.Name,
	})
	if err != nil {
		return utils.Wrap(err, "export knowledge base failed")
	}

	_, err = io.Copy(file, reader)
	if err != nil {
		return utils.Wrap(err, "copy knowledge base to file failed")
	}

	if err := file.Sync(); err != nil {
		return utils.Wrap(err, "sync file failed")
	}

	return nil
}

func ImportRAG(filePath string, optFuncs ...RAGSystemConfigOption) error {
	file, err := os.Open(filePath)
	if err != nil {
		return utils.Wrap(err, "open file failed")
	}
	defer file.Close()

	config := NewRAGSystemConfig(optFuncs...)
	if config.ragID == "" {
		config.ragID = uuid.NewString()
	}
	err = knowledgebase.ImportKnowledgeBase(config.ctx, config.db, file, &knowledgebase.ImportKnowledgeBaseOptions{
		OverwriteExisting:    config.overwriteExisting,
		NewKnowledgeBaseName: config.Name,
		OnProgressHandler:    config.progressHandler,
		RAGID:                config.ragID,
		ExtraDataHandler: func(extraData io.Reader) error {
			return entityrepos.ImportEntityRepository(config.ctx, config.db, extraData, &entityrepos.ImportEntityRepositoryOptions{
				OverwriteExisting: config.overwriteExisting,
				NewRepositoryName: config.Name,
				RAGID:             config.ragID,
			})
		},
	})
	if err != nil {
		return utils.Errorf("import knowledge base failed: %v", err)
	}
	return nil
}
