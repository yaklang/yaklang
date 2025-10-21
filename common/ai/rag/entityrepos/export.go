package entityrepos

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type ExportEntityRepositoryOptions struct {
	RepositoryID              int64
	ExportEntityHandler       func(entity schema.ERModelEntity) (schema.ERModelEntity, error)
	ExportRelationshipHandler func(relationship schema.ERModelRelationship) (schema.ERModelRelationship, error)
	ExportRAGDocumentHandler  func(doc schema.VectorStoreDocument) (schema.VectorStoreDocument, error)
	OnProgressHandler         func(percent float64, message string, messageType string)
	SkipVectorStore           bool // 是否跳过向量库的导出
}

func ExportEntityRepository(ctx context.Context, db *gorm.DB, opts *ExportEntityRepositoryOptions) (io.Reader, error) {
	var buf bytes.Buffer
	writer := utils.NewProtoWriter(&buf)

	// 进度回调辅助函数
	reportProgress := func(percent float64, message string, messageType string) {
		if opts.OnProgressHandler != nil {
			opts.OnProgressHandler(percent, message, messageType)
		}
	}

	reportProgress(0, "开始导出实体仓库", "info")

	// 写入魔数头
	if err := writer.WriteMagicHeader("YAKENTITYREPOS__"); err != nil {
		return nil, utils.Wrap(err, "write magic header")
	}

	// 写入实体仓库信息
	var reposInfo schema.EntityRepository
	if err := db.Model(&schema.EntityRepository{}).Where("id = ?", opts.RepositoryID).First(&reposInfo).Error; err != nil {
		return nil, utils.Wrap(err, "get entity repository info failed")
	}

	reportProgress(5, "正在写入实体仓库基本信息", "info")

	if err := writer.WriteString(reposInfo.EntityBaseName); err != nil {
		return nil, utils.Wrap(err, "write entity repository name")
	}
	if err := writer.WriteString(reposInfo.Description); err != nil {
		return nil, utils.Wrap(err, "write entity repository description")
	}

	// 统计实体数量
	var entityCount int64
	if err := db.Model(&schema.ERModelEntity{}).Where("repository_uuid = ?", reposInfo.Uuid).Count(&entityCount).Error; err != nil {
		return nil, utils.Wrap(err, "count entities")
	}
	if err := writer.WriteVarint(uint64(entityCount)); err != nil {
		return nil, utils.Wrap(err, "write entity count")
	}

	reportProgress(10, fmt.Sprintf("开始导出实体，共 %d 个", entityCount), "info")

	// 分页导出实体
	const pageSize = 100
	page := 1
	processedEntities := uint64(0)

	for {
		var entities []*schema.ERModelEntity

		_, paginatedDB := bizhelper.Paging(
			db.Model(&schema.ERModelEntity{}).Where("repository_uuid = ?", reposInfo.Uuid),
			page, pageSize, &entities,
		)

		if paginatedDB.Error != nil {
			return nil, utils.Errorf("failed to query entities page %d: %v", page, paginatedDB.Error)
		}

		if len(entities) == 0 {
			break
		}

		// 逐个写入实体
		for _, entity := range entities {
			if opts.ExportEntityHandler != nil {
				newEntity, err := opts.ExportEntityHandler(*entity)
				if err != nil {
					return nil, utils.Wrap(err, "export entity")
				}
				entity = &newEntity
			}
			if err := writeEntityToBinary(writer, entity); err != nil {
				return nil, utils.Wrap(err, "write entity")
			}
			processedEntities++

			// 每处理10个实体报告一次进度
			if processedEntities%10 == 0 || processedEntities == uint64(entityCount) {
				progress := 10 + (float64(processedEntities)/float64(entityCount))*30 // 10-40%用于实体导出
				reportProgress(progress, fmt.Sprintf("已导出 %d/%d 个实体", processedEntities, entityCount), "info")
			}
		}

		if len(entities) < pageSize {
			break
		}

		page++
	}

	reportProgress(40, "实体导出完成，开始导出关系", "info")

	// 统计关系数量
	var relationshipCount int64
	if err := db.Model(&schema.ERModelRelationship{}).Where("repository_uuid = ?", reposInfo.Uuid).Count(&relationshipCount).Error; err != nil {
		return nil, utils.Wrap(err, "count relationships")
	}
	if err := writer.WriteVarint(uint64(relationshipCount)); err != nil {
		return nil, utils.Wrap(err, "write relationship count")
	}

	reportProgress(45, fmt.Sprintf("开始导出关系，共 %d 个", relationshipCount), "info")

	// 分页导出关系
	page = 1
	processedRelationships := uint64(0)

	for {
		var relationships []*schema.ERModelRelationship

		_, paginatedDB := bizhelper.Paging(
			db.Model(&schema.ERModelRelationship{}).Where("repository_uuid = ?", reposInfo.Uuid),
			page, pageSize, &relationships,
		)

		if paginatedDB.Error != nil {
			return nil, utils.Errorf("failed to query relationships page %d: %v", page, paginatedDB.Error)
		}

		if len(relationships) == 0 {
			break
		}

		// 逐个写入关系
		for _, relationship := range relationships {
			if opts.ExportRelationshipHandler != nil {
				newRelationship, err := opts.ExportRelationshipHandler(*relationship)
				if err != nil {
					return nil, utils.Wrap(err, "export relationship")
				}
				relationship = &newRelationship
			}
			if err := writeRelationshipToBinary(writer, relationship); err != nil {
				return nil, utils.Wrap(err, "write relationship")
			}
			processedRelationships++

			// 每处理10个关系报告一次进度
			if processedRelationships%10 == 0 || processedRelationships == uint64(relationshipCount) {
				progress := 45 + (float64(processedRelationships)/float64(relationshipCount))*20 // 45-65%用于关系导出
				reportProgress(progress, fmt.Sprintf("已导出 %d/%d 个关系", processedRelationships, relationshipCount), "info")
			}
		}

		if len(relationships) < pageSize {
			break
		}

		page++
	}

	reportProgress(65, "关系导出完成", "info")

	// 根据 SkipVectorStore 选项决定是否导出向量库
	if opts.SkipVectorStore {
		reportProgress(70, "跳过向量库数据导出", "info")
		// 写入空的向量库数据
		if err := writer.WriteBytes([]byte{}); err != nil {
			return nil, utils.Wrap(err, "write empty rag binary")
		}
		reportProgress(100, "实体仓库导出完成（已跳过向量库）", "success")
	} else {
		reportProgress(65, "开始导出向量库数据", "info")
		// 写入向量库
		ragBinaryReader, err := rag.ExportRAGToBinary(ctx, db, reposInfo.EntityBaseName,
			rag.WithExportDocumentHandler(opts.ExportRAGDocumentHandler),
			rag.WithExportProgressHandler(func(percent float64, message string, messageType string) {
				// 将RAG导出进度映射到65-95%范围
				ragProgress := 65 + (percent/100)*30
				reportProgress(ragProgress, message, messageType)
			}),
		)
		if err != nil {
			return nil, utils.Wrap(err, "export rag to binary")
		}
		ragBinary, err := io.ReadAll(ragBinaryReader)
		if err != nil {
			return nil, utils.Wrap(err, "read rag binary")
		}
		if err := writer.WriteBytes(ragBinary); err != nil {
			return nil, utils.Wrap(err, "write rag binary")
		}

		reportProgress(100, "实体仓库导出完成", "success")
	}

	return &buf, nil
}

func writeEntityToBinary(pw *utils.ProtoWriter, entity *schema.ERModelEntity) error {
	if err := pw.WriteString(entity.EntityName); err != nil {
		return utils.Wrap(err, "write entity name")
	}
	if err := pw.WriteString(entity.Uuid); err != nil {
		return utils.Wrap(err, "write entity uuid")
	}
	if err := pw.WriteString(entity.Description); err != nil {
		return utils.Wrap(err, "write description")
	}
	if err := pw.WriteString(entity.EntityType); err != nil {
		return utils.Wrap(err, "write entity type")
	}
	if err := pw.WriteString(entity.EntityTypeVerbose); err != nil {
		return utils.Wrap(err, "write entity type verbose")
	}

	// 序列化属性
	attrBytes, err := json.Marshal(entity.Attributes)
	if err != nil {
		return utils.Wrap(err, "marshal attributes")
	}
	if err := pw.WriteBytes(attrBytes); err != nil {
		return utils.Wrap(err, "write attributes")
	}

	return nil
}

func writeRelationshipToBinary(pw *utils.ProtoWriter, relationship *schema.ERModelRelationship) error {
	if err := pw.WriteString(relationship.Uuid); err != nil {
		return utils.Wrap(err, "write relationship uuid")
	}
	if err := pw.WriteString(relationship.SourceEntityIndex); err != nil {
		return utils.Wrap(err, "write source entity index")
	}
	if err := pw.WriteString(relationship.TargetEntityIndex); err != nil {
		return utils.Wrap(err, "write target entity index")
	}
	if err := pw.WriteString(relationship.RelationshipType); err != nil {
		return utils.Wrap(err, "write relationship type")
	}
	if err := pw.WriteString(relationship.RelationshipTypeVerbose); err != nil {
		return utils.Wrap(err, "write relationship type verbose")
	}

	// 序列化属性
	attrBytes, err := json.Marshal(relationship.Attributes)
	if err != nil {
		return utils.Wrap(err, "marshal attributes")
	}
	if err := pw.WriteBytes(attrBytes); err != nil {
		return utils.Wrap(err, "write attributes")
	}

	return nil
}

type ImportEntityRepositoryOptions struct {
	ImportEntityHandler       func(entity schema.ERModelEntity) (schema.ERModelEntity, error)
	ImportRelationshipHandler func(relationship schema.ERModelRelationship) (schema.ERModelRelationship, error)
	ImportRAGDocumentHandler  func(doc schema.VectorStoreDocument) (schema.VectorStoreDocument, error)
	OverwriteExisting         bool
	NewRepositoryName         string
	OnProgressHandler         func(percent float64, message string, messageType string)
}

// ImportEntityRepository 从二进制数据导入实体仓库
func ImportEntityRepository(ctx context.Context, db *gorm.DB, reader io.Reader, opts *ImportEntityRepositoryOptions) error {
	pr := utils.NewProtoReader(reader)

	// 进度回调辅助函数
	reportProgress := func(percent float64, message string, messageType string) {
		if opts.OnProgressHandler != nil {
			opts.OnProgressHandler(percent, message, messageType)
		}
	}

	reportProgress(0, "开始导入实体仓库", "info")

	// 读取并验证魔数头
	if err := pr.ReadMagicHeader("YAKENTITYREPOS__"); err != nil {
		return err
	}

	reportProgress(5, "正在读取实体仓库信息", "info")

	// 读取实体仓库信息
	originalReposName, err := pr.ReadString()
	if err != nil {
		return utils.Wrap(err, "read entity repository name")
	}

	// 确定最终使用的仓库名称
	finalReposName := originalReposName
	if opts.NewRepositoryName != "" {
		finalReposName = opts.NewRepositoryName
	}

	reposDesc, err := pr.ReadString()
	if err != nil {
		return utils.Wrap(err, "read entity repository description")
	}

	// 检查实体仓库是否已存在
	var existingRepos schema.EntityRepository
	err = db.Model(&schema.EntityRepository{}).Where("entity_base_name = ?", finalReposName).First(&existingRepos).Error
	isNotFound := err != nil && (gorm.IsRecordNotFoundError(err) || utils.StringContainsAnyOfSubString(err.Error(), []string{"record not found"}))

	if err != nil && !isNotFound {
		return utils.Wrap(err, "check existing entity repository")
	}

	var reposInfo *schema.EntityRepository
	if !isNotFound && existingRepos.ID > 0 {
		if !opts.OverwriteExisting {
			return utils.Errorf("entity repository '%s' already exists", finalReposName)
		}
		// 更新现有实体仓库信息
		existingRepos.EntityBaseName = finalReposName
		existingRepos.Description = reposDesc
		if err := db.Save(&existingRepos).Error; err != nil {
			return utils.Wrap(err, "update existing entity repository")
		}
		reposInfo = &existingRepos

		// 删除现有实体和关系
		if err := db.Where("repository_uuid = ?", reposInfo.Uuid).Unscoped().Delete(&schema.ERModelEntity{}).Error; err != nil {
			return utils.Wrap(err, "delete existing entities")
		}
		if err := db.Where("repository_uuid = ?", reposInfo.Uuid).Unscoped().Delete(&schema.ERModelRelationship{}).Error; err != nil {
			return utils.Wrap(err, "delete existing relationships")
		}
	} else {
		// 创建新实体仓库
		reposInfo = &schema.EntityRepository{
			EntityBaseName: finalReposName,
			Description:    reposDesc,
		}
		if err := yakit.CreateEntityBaseInfo(db, reposInfo); err != nil {
			return utils.Wrap(err, "create entity repository")
		}
	}

	// 读取实体数量
	entityCount, err := pr.ReadVarint()
	if err != nil {
		return utils.Wrap(err, "read entity count")
	}

	reportProgress(10, fmt.Sprintf("实体仓库信息处理完成，开始导入 %d 个实体", entityCount), "info")

	// 存储UUID映射关系（旧UUID -> 新UUID）
	uuidMap := make(map[string]string)

	// 逐个读取并创建实体
	for i := uint64(0); i < entityCount; i++ {
		entity, err := readEntityFromBinary(pr)
		if err != nil {
			return utils.Wrap(err, "read entity")
		}

		oldUuid := entity.Uuid
		entity.RepositoryUUID = reposInfo.Uuid
		entity.RuntimeID = ""
		entity.ID = 0 // 重置ID让数据库分配新的

		if opts.ImportEntityHandler != nil {
			newEntity, err := opts.ImportEntityHandler(*entity)
			if err != nil {
				return utils.Wrap(err, "import entity")
			}
			entity = &newEntity
		}

		if err := yakit.CreateEntity(db, entity); err != nil {
			return utils.Wrap(err, "create entity")
		}

		// 记录UUID映射
		uuidMap[oldUuid] = entity.Uuid

		// 每处理10个实体或最后一个实体报告进度
		if (i+1)%10 == 0 || i+1 == entityCount {
			progress := 10 + (float64(i+1)/float64(entityCount))*30 // 10-40%用于实体导入
			reportProgress(progress, fmt.Sprintf("已导入 %d/%d 个实体", i+1, entityCount), "info")
		}
	}

	reportProgress(40, "实体导入完成，开始导入关系", "info")

	// 读取关系数量
	relationshipCount, err := pr.ReadVarint()
	if err != nil {
		return utils.Wrap(err, "read relationship count")
	}

	reportProgress(45, fmt.Sprintf("开始导入 %d 个关系", relationshipCount), "info")

	// 逐个读取并创建关系
	for i := uint64(0); i < relationshipCount; i++ {
		relationship, err := readRelationshipFromBinary(pr)
		if err != nil {
			return utils.Wrap(err, "read relationship")
		}

		// 使用UUID映射更新实体索引
		if newSourceUuid, ok := uuidMap[relationship.SourceEntityIndex]; ok {
			relationship.SourceEntityIndex = newSourceUuid
		}
		if newTargetUuid, ok := uuidMap[relationship.TargetEntityIndex]; ok {
			relationship.TargetEntityIndex = newTargetUuid
		}

		relationship.RepositoryUUID = reposInfo.Uuid
		relationship.RuntimeID = ""
		relationship.ID = 0 // 重置ID让数据库分配新的

		if opts.ImportRelationshipHandler != nil {
			newRelationship, err := opts.ImportRelationshipHandler(*relationship)
			if err != nil {
				return utils.Wrap(err, "import relationship")
			}
			relationship = &newRelationship
		}

		if err := db.Create(relationship).Error; err != nil {
			return utils.Wrap(err, "create relationship")
		}

		// 每处理10个关系或最后一个关系报告进度
		if (i+1)%10 == 0 || i+1 == relationshipCount {
			progress := 45 + (float64(i+1)/float64(relationshipCount))*20 // 45-65%用于关系导入
			reportProgress(progress, fmt.Sprintf("已导入 %d/%d 个关系", i+1, relationshipCount), "info")
		}
	}

	reportProgress(65, "关系导入完成，开始处理向量库数据", "info")

	// 读取向量库数据
	ragBinaryBytes, err := pr.ReadBytes()
	if err != nil {
		return utils.Wrap(err, "read rag binary data")
	}

	// 只有在向量库数据不为空时才导入
	if len(ragBinaryBytes) > 0 {
		reportProgress(70, "正在导入向量库数据", "info")
		ragReader := bytes.NewReader(ragBinaryBytes)

		// 使用现有的ImportRAGFromReader函数
		importConfig := &rag.RAGImportConfig{
			OverwriteExisting: opts.OverwriteExisting,
			CollectionName:    finalReposName, // 使用最终的仓库名称作为集合名称
			DocumentHandler:   opts.ImportRAGDocumentHandler,
			OnProgressHandler: func(percent float64, message string, messageType string) {
				// 将RAG导入进度映射到70-95%范围
				ragProgress := 70 + (percent/100)*25
				reportProgress(ragProgress, message, messageType)
			},
		}
		if err := rag.ImportRAGFromReader(ctx, db, ragReader, importConfig); err != nil {
			return utils.Wrap(err, "import rag data")
		}
		reportProgress(95, "向量库数据导入完成", "info")
	} else {
		// 向量库数据为空，跳过导入
		reportProgress(70, "向量库数据为空，跳过导入", "info")
	}

	reportProgress(100, "实体仓库导入完成", "success")
	return nil
}

// readEntityFromBinary 从二进制数据读取实体
func readEntityFromBinary(pr *utils.ProtoReader) (*schema.ERModelEntity, error) {
	entity := &schema.ERModelEntity{}

	// 读取 EntityName
	entityName, err := pr.ReadString()
	if err != nil {
		return nil, utils.Wrap(err, "read entity name")
	}
	entity.EntityName = entityName

	// 读取 Uuid
	uuid, err := pr.ReadString()
	if err != nil {
		return nil, utils.Wrap(err, "read entity uuid")
	}
	entity.Uuid = uuid

	// 读取 Description
	description, err := pr.ReadString()
	if err != nil {
		return nil, utils.Wrap(err, "read description")
	}
	entity.Description = description

	// 读取 EntityType
	entityType, err := pr.ReadString()
	if err != nil {
		return nil, utils.Wrap(err, "read entity type")
	}
	entity.EntityType = entityType

	// 读取 EntityTypeVerbose
	typeVerbose, err := pr.ReadString()
	if err != nil {
		return nil, utils.Wrap(err, "read entity type verbose")
	}
	entity.EntityTypeVerbose = typeVerbose

	// 读取 Attributes
	attrBytes, err := pr.ReadBytes()
	if err != nil {
		return nil, utils.Wrap(err, "read attributes")
	}
	if len(attrBytes) > 0 {
		if err := json.Unmarshal(attrBytes, &entity.Attributes); err != nil {
			return nil, utils.Wrap(err, "unmarshal attributes")
		}
	}

	return entity, nil
}

// readRelationshipFromBinary 从二进制数据读取关系
func readRelationshipFromBinary(pr *utils.ProtoReader) (*schema.ERModelRelationship, error) {
	relationship := &schema.ERModelRelationship{}

	// 读取 Uuid
	uuid, err := pr.ReadString()
	if err != nil {
		return nil, utils.Wrap(err, "read relationship uuid")
	}
	relationship.Uuid = uuid

	// 读取 SourceEntityIndex
	sourceIndex, err := pr.ReadString()
	if err != nil {
		return nil, utils.Wrap(err, "read source entity index")
	}
	relationship.SourceEntityIndex = sourceIndex

	// 读取 TargetEntityIndex
	targetIndex, err := pr.ReadString()
	if err != nil {
		return nil, utils.Wrap(err, "read target entity index")
	}
	relationship.TargetEntityIndex = targetIndex

	// 读取 RelationshipType
	relType, err := pr.ReadString()
	if err != nil {
		return nil, utils.Wrap(err, "read relationship type")
	}
	relationship.RelationshipType = relType

	// 读取 RelationshipTypeVerbose
	typeVerbose, err := pr.ReadString()
	if err != nil {
		return nil, utils.Wrap(err, "read relationship type verbose")
	}
	relationship.RelationshipTypeVerbose = typeVerbose

	// 读取 Attributes
	attrBytes, err := pr.ReadBytes()
	if err != nil {
		return nil, utils.Wrap(err, "read attributes")
	}
	if len(attrBytes) > 0 {
		if err := json.Unmarshal(attrBytes, &relationship.Attributes); err != nil {
			return nil, utils.Wrap(err, "unmarshal attributes")
		}
	}

	return relationship, nil
}
