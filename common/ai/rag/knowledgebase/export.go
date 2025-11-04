package knowledgebase

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type ExportKnowledgeBaseOptions struct {
	KnowledgeBaseId                 int64
	ExportKnowledgeBaseEntryHandler func(entry schema.KnowledgeBaseEntry) (schema.KnowledgeBaseEntry, error)
	ExportRAGDocumentHandler        func(doc schema.VectorStoreDocument) (schema.VectorStoreDocument, error)
	OnProgressHandler               func(percent float64, message string, messageType string)
	ExtraDataReader                 io.Reader // 额外数据的 reader，会在导出时一起导出
	VectorStoreName                 string
}

func ExportKnowledgeBase(ctx context.Context, db *gorm.DB, opts *ExportKnowledgeBaseOptions) (io.Reader, error) {
	var buf bytes.Buffer
	buf.WriteString("YAKKNOWLEDGEBASE")

	// 进度回调辅助函数
	reportProgress := func(percent float64, message string, messageType string) {
		if opts.OnProgressHandler != nil {
			opts.OnProgressHandler(percent, message, messageType)
		}
	}

	reportProgress(0, "开始导出知识库", "info")

	// 写入知识库信息
	kbInfo, err := yakit.GetKnowledgeBase(db, opts.KnowledgeBaseId)
	if err != nil {
		return nil, utils.Wrap(err, "get knowledge base info failed")
	}
	if kbInfo == nil {
		return nil, utils.Errorf("knowledge base not found")
	}

	reportProgress(10, "正在写入知识库基本信息", "info")

	if err := pbWriteBytes(&buf, []byte(kbInfo.KnowledgeBaseName)); err != nil {
		return nil, utils.Wrap(err, "write knowledge base name")
	}
	if err := pbWriteBytes(&buf, []byte(kbInfo.KnowledgeBaseDescription)); err != nil {
		return nil, utils.Wrap(err, "write knowledge base description")
	}
	if err := pbWriteBytes(&buf, []byte(kbInfo.KnowledgeBaseType)); err != nil {
		return nil, utils.Wrap(err, "write knowledge base type")
	}

	var count int64
	if err := db.Model(&schema.KnowledgeBaseEntry{}).Where("knowledge_base_id = ?", kbInfo.ID).Count(&count).Error; err != nil {
		return nil, utils.Wrap(err, "write knowledge base entries count")
	}
	if err := pbWriteVarint(&buf, uint64(count)); err != nil {
		return nil, utils.Wrap(err, "write knowledge base entries count")
	}

	reportProgress(20, fmt.Sprintf("开始导出知识库条目，共 %d 条", count), "info")

	const pageSize = 100
	page := 1
	processedEntries := uint64(0)

	for {
		var entries []*schema.KnowledgeBaseEntry

		_, paginatedDB := bizhelper.Paging(
			db.Model(&schema.KnowledgeBaseEntry{}).Where("knowledge_base_id = ?", kbInfo.ID),
			page, pageSize, &entries,
		)

		if paginatedDB.Error != nil {
			return nil, utils.Errorf("failed to query entries page %d: %v", page, paginatedDB.Error)
		}

		// 如果没有更多数据，跳出循环
		if len(entries) == 0 {
			break
		}

		// 逐个写入知识库条目
		for _, entry := range entries {
			if opts.ExportKnowledgeBaseEntryHandler != nil {
				newEntry, err := opts.ExportKnowledgeBaseEntryHandler(*entry)
				if err != nil {
					return nil, utils.Wrap(err, "export knowledge base entry")
				}
				entry = &newEntry
			}
			if err := writeEntryToBinary(&buf, entry); err != nil {
				return nil, utils.Wrap(err, "write knowledge base entry")
			}
			processedEntries++

			// 每处理10个条目报告一次进度
			if processedEntries%10 == 0 || processedEntries == uint64(count) {
				progress := 20 + (float64(processedEntries)/float64(count))*50 // 20-70%用于条目导出
				reportProgress(progress, fmt.Sprintf("已导出 %d/%d 个知识库条目", processedEntries, count), "info")
			}
		}

		// 如果当前页数据少于pageSize，说明已经是最后一页
		if len(entries) < pageSize {
			break
		}

		page++
	}

	reportProgress(70, "知识库条目导出完成，开始导出向量库数据", "info")

	vectorStoreName := opts.VectorStoreName
	if vectorStoreName == "" {
		vectorStoreName = kbInfo.KnowledgeBaseName
	}
	// 写入向量库
	ragBinaryReader, err := vectorstore.ExportRAGToBinary(vectorStoreName,
		vectorstore.WithContext(ctx),
		vectorstore.WithImportExportDB(db),
		vectorstore.WithDocumentHandler(opts.ExportRAGDocumentHandler),
		vectorstore.WithProgressHandler(func(percent float64, message string, messageType string) {
			// 将RAG导出进度映射到70-90%范围
			ragProgress := 70 + (percent/100)*20
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
	if err := pbWriteBytes(&buf, ragBinary); err != nil {
		return nil, utils.Wrap(err, "write rag binary")
	}

	reportProgress(90, "向量库数据导出完成，开始导出额外数据", "info")

	// 写入额外数据
	if opts.ExtraDataReader != nil {
		extraData, err := io.ReadAll(opts.ExtraDataReader)
		if err != nil {
			return nil, utils.Wrap(err, "read extra data")
		}
		if err := pbWriteBytes(&buf, extraData); err != nil {
			return nil, utils.Wrap(err, "write extra data")
		}
		reportProgress(95, "额外数据导出完成", "info")
	} else {
		// 如果没有额外数据，写入空字节
		if err := pbWriteBytes(&buf, []byte{}); err != nil {
			return nil, utils.Wrap(err, "write empty extra data")
		}
	}

	reportProgress(100, "知识库导出完成", "success")
	return &buf, nil
}

func writeEntryToBinary(writer io.Writer, entry *schema.KnowledgeBaseEntry) error {
	if err := pbWriteBytes(writer, []byte(entry.RelatedEntityUUIDS)); err != nil {
		return utils.Wrap(err, "write related entity uuid s")
	}
	if err := pbWriteBytes(writer, []byte(entry.KnowledgeTitle)); err != nil {
		return utils.Wrap(err, "write knowledge title")
	}
	if err := pbWriteBytes(writer, []byte(entry.KnowledgeType)); err != nil {
		return utils.Wrap(err, "write knowledge type")
	}
	if err := pbWriteUint32(writer, uint32(entry.ImportanceScore)); err != nil {
		return utils.Wrap(err, "write importance score")
	}
	if err := pbWriteUint32(writer, uint32(len(entry.Keywords))); err != nil {
		return utils.Wrap(err, "write keywords length")
	}
	for _, val := range entry.Keywords {
		if err := pbWriteBytes(writer, []byte(val)); err != nil {
			return utils.Wrap(err, "write keywords")
		}
	}
	if err := pbWriteBytes(writer, []byte(entry.KnowledgeDetails)); err != nil {
		return utils.Wrap(err, "write knowledge details")
	}
	if err := pbWriteBytes(writer, []byte(entry.Summary)); err != nil {
		return utils.Wrap(err, "write summary")
	}
	if err := pbWriteUint32(writer, uint32(entry.SourcePage)); err != nil {
		return utils.Wrap(err, "write source page")
	}
	if err := pbWriteUint32(writer, uint32(len(entry.PotentialQuestions))); err != nil {
		return utils.Wrap(err, "write potential questions length")
	}
	for _, val := range entry.PotentialQuestions {
		if err := pbWriteBytes(writer, []byte(val)); err != nil {
			return utils.Wrap(err, "write potential questions")
		}
	}
	if err := pbWriteUint32(writer, uint32(len(entry.PotentialQuestionsVector))); err != nil {
		return utils.Wrap(err, "write potential questions vector")
	}
	for _, val := range entry.PotentialQuestionsVector {
		if err := pbWriteFloat32(writer, val); err != nil {
			return utils.Wrap(err, "write potential questions vector")
		}
	}

	return nil
}

type ImportKnowledgeBaseOptions struct {
	ImportKnowledgeBaseEntryHandler func(entry schema.KnowledgeBaseEntry) (schema.KnowledgeBaseEntry, error)
	ImportRAGDocumentHandler        func(doc schema.VectorStoreDocument) (schema.VectorStoreDocument, error)
	OverwriteExisting               bool
	NewKnowledgeBaseName            string
	OnProgressHandler               func(percent float64, message string, messageType string)
	ExtraDataHandler                func(extraData io.Reader) error // 额外数据处理回调函数
	RAGID                           string
}

// ImportKnowledgeBase 从二进制数据导入知识库
func ImportKnowledgeBase(ctx context.Context, db *gorm.DB, reader io.Reader, opts *ImportKnowledgeBaseOptions) error {
	// 进度回调辅助函数
	reportProgress := func(percent float64, message string, messageType string) {
		if opts.OnProgressHandler != nil {
			opts.OnProgressHandler(percent, message, messageType)
		}
	}

	reportProgress(0, "开始导入知识库", "info")

	// 读取魔数头
	magic := make([]byte, 16)
	if _, err := io.ReadFull(reader, magic); err != nil {
		return utils.Wrap(err, "read magic header")
	}
	if string(magic) != "YAKKNOWLEDGEBASE" {
		return utils.Error("invalid magic header")
	}

	reportProgress(5, "正在读取知识库信息", "info")

	// 读取知识库信息
	kbNameBytes, err := consumeBytes(reader)
	if err != nil {
		return utils.Wrap(err, "read knowledge base name")
	}
	originalKbName := string(kbNameBytes)

	// 确定最终使用的知识库名称
	finalKbName := originalKbName
	if opts.NewKnowledgeBaseName != "" {
		finalKbName = opts.NewKnowledgeBaseName
	}

	kbDescBytes, err := consumeBytes(reader)
	if err != nil {
		return utils.Wrap(err, "read knowledge base description")
	}
	kbDesc := string(kbDescBytes)

	kbTypeBytes, err := consumeBytes(reader)
	if err != nil {
		return utils.Wrap(err, "read knowledge base type")
	}
	kbType := string(kbTypeBytes)

	// 检查知识库是否已存在
	existingKB, err := yakit.GetKnowledgeBaseByName(db, finalKbName)
	isNotFound := err != nil && (gorm.IsRecordNotFoundError(err) || utils.StringContainsAnyOfSubString(err.Error(), []string{"record not found"}))

	if err != nil && !isNotFound {
		return utils.Wrap(err, "check existing knowledge base")
	}

	var kbInfo *schema.KnowledgeBaseInfo
	if existingKB != nil && !isNotFound {
		if !opts.OverwriteExisting {
			return utils.Errorf("knowledge base '%s' already exists", finalKbName)
		}
		// 更新现有知识库信息
		existingKB.KnowledgeBaseName = finalKbName
		existingKB.KnowledgeBaseDescription = kbDesc
		existingKB.KnowledgeBaseType = kbType
		existingKB.RAGID = opts.RAGID
		if err := yakit.UpdateKnowledgeBaseInfo(db, int64(existingKB.ID), existingKB); err != nil {
			return utils.Wrap(err, "update existing knowledge base")
		}
		kbInfo = existingKB

		// 删除现有条目
		if err := db.Where("knowledge_base_id = ?", kbInfo.ID).Unscoped().Delete(&schema.KnowledgeBaseEntry{}).Error; err != nil {
			return utils.Wrap(err, "delete existing entries")
		}
	} else {
		// 创建新知识库
		kbInfo = &schema.KnowledgeBaseInfo{
			KnowledgeBaseName:        finalKbName,
			KnowledgeBaseDescription: kbDesc,
			KnowledgeBaseType:        kbType,
			RAGID:                    opts.RAGID,
		}
		if err := yakit.CreateKnowledgeBase(db, kbInfo); err != nil {
			return utils.Wrap(err, "create knowledge base")
		}
	}

	// 读取条目数量
	entryCount, err := consumeVarint(reader)
	if err != nil {
		return utils.Wrap(err, "read entry count")
	}

	reportProgress(20, fmt.Sprintf("知识库信息处理完成，开始导入 %d 个条目", entryCount), "info")

	// 逐个读取并创建知识库条目
	for i := uint64(0); i < entryCount; i++ {
		entry, err := readEntryFromBinary(reader)
		if err != nil {
			return utils.Wrap(err, "read knowledge base entry")
		}
		entry.KnowledgeBaseID = int64(kbInfo.ID)

		if opts.ImportKnowledgeBaseEntryHandler != nil {
			newEntry, err := opts.ImportKnowledgeBaseEntryHandler(*entry)
			if err != nil {
				return utils.Wrap(err, "import knowledge base entry")
			}
			entry = &newEntry
		}
		if err := yakit.CreateKnowledgeBaseEntry(db, entry); err != nil {
			return utils.Wrap(err, "create knowledge base entry")
		}

		// 每处理10个条目或最后一个条目报告进度
		if (i+1)%10 == 0 || i+1 == entryCount {
			progress := 20 + (float64(i+1)/float64(entryCount))*50 // 20-70%用于条目导入
			reportProgress(progress, fmt.Sprintf("已导入 %d/%d 个知识库条目", i+1, entryCount), "info")
		}
	}

	reportProgress(70, "知识库条目导入完成，开始导入向量库数据", "info")

	// 读取并导入向量库数据
	ragBinaryBytes, err := consumeBytes(reader)
	if err != nil {
		return utils.Wrap(err, "read rag binary data")
	}

	if len(ragBinaryBytes) > 0 {
		reportProgress(75, "正在导入向量库数据", "info")
		ragReader := bytes.NewReader(ragBinaryBytes)

		opts := []vectorstore.RAGExportOptionFunc{
			vectorstore.WithContext(ctx),
			vectorstore.WithImportExportDB(db),
			vectorstore.WithOverwriteExisting(opts.OverwriteExisting),
			vectorstore.WithCollectionName(finalKbName),
			vectorstore.WithDocumentHandler(opts.ImportRAGDocumentHandler),
			vectorstore.WithRAGID(opts.RAGID),
			vectorstore.WithProgressHandler(func(percent float64, message string, messageType string) {
				ragProgress := 75 + (percent/100)*15
				reportProgress(ragProgress, message, messageType)
			}),
		}
		if err := vectorstore.ImportRAGFromReader(ragReader, opts...); err != nil {
			return utils.Wrap(err, "import rag data")
		}
		reportProgress(90, "向量库数据导入完成", "info")
	}

	reportProgress(92, "开始导入额外数据", "info")

	// 读取并处理额外数据
	extraDataBytes, err := consumeBytes(reader)
	if err != nil {
		return utils.Wrap(err, "read extra data")
	}

	if len(extraDataBytes) > 0 && opts.ExtraDataHandler != nil {
		extraDataReader := bytes.NewReader(extraDataBytes)
		if err := opts.ExtraDataHandler(extraDataReader); err != nil {
			return utils.Wrap(err, "handle extra data")
		}
		reportProgress(98, "额外数据导入完成", "info")
	}

	reportProgress(100, "知识库导入完成", "success")
	return nil
}

// readEntryFromBinary 从二进制数据读取知识库条目
func readEntryFromBinary(reader io.Reader) (*schema.KnowledgeBaseEntry, error) {
	entry := &schema.KnowledgeBaseEntry{}

	// 读取 RelatedEntityUUIDS
	relatedEntityBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read related entity uuids")
	}
	entry.RelatedEntityUUIDS = string(relatedEntityBytes)

	// 读取 KnowledgeTitle
	titleBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read knowledge title")
	}
	entry.KnowledgeTitle = string(titleBytes)

	// 读取 KnowledgeType
	typeBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read knowledge type")
	}
	entry.KnowledgeType = string(typeBytes)

	// 读取 ImportanceScore
	importanceScore, err := consumeUint32(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read importance score")
	}
	entry.ImportanceScore = int(importanceScore)

	// 读取 Keywords
	keywordsLength, err := consumeUint32(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read keywords length")
	}
	keywords := make([]string, keywordsLength)
	for i := uint32(0); i < keywordsLength; i++ {
		keywordBytes, err := consumeBytes(reader)
		if err != nil {
			return nil, utils.Wrap(err, "read keyword")
		}
		keywords[i] = string(keywordBytes)
	}
	entry.Keywords = schema.StringArray(keywords)

	// 读取 KnowledgeDetails
	detailsBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read knowledge details")
	}
	entry.KnowledgeDetails = string(detailsBytes)

	// 读取 Summary
	summaryBytes, err := consumeBytes(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read summary")
	}
	entry.Summary = string(summaryBytes)

	// 读取 SourcePage
	sourcePage, err := consumeUint32(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read source page")
	}
	entry.SourcePage = int(sourcePage)

	// 读取 PotentialQuestions
	questionsLength, err := consumeUint32(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read potential questions length")
	}
	questions := make([]string, questionsLength)
	for i := uint32(0); i < questionsLength; i++ {
		questionBytes, err := consumeBytes(reader)
		if err != nil {
			return nil, utils.Wrap(err, "read potential question")
		}
		questions[i] = string(questionBytes)
	}
	entry.PotentialQuestions = schema.StringArray(questions)

	// 读取 PotentialQuestionsVector
	vectorLength, err := consumeUint32(reader)
	if err != nil {
		return nil, utils.Wrap(err, "read potential questions vector length")
	}
	vector := make([]float32, vectorLength)
	for i := uint32(0); i < vectorLength; i++ {
		value, err := consumeFloat32(reader)
		if err != nil {
			return nil, utils.Wrap(err, "read potential questions vector value")
		}
		vector[i] = value
	}
	entry.PotentialQuestionsVector = schema.FloatArray(vector)
	return entry, nil
}
