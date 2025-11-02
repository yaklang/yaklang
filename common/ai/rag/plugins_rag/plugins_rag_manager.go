package plugins_rag

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// PluginsRagManager 插件 RAG 管理器
type PluginsRagManager struct {
	db             *gorm.DB       // 数据库连接
	RagSystem      *rag.RAGSystem // RAG 系统
	collectionName string         // 集合名称
	mu             sync.RWMutex   // 互斥锁
	metadataFile   string         // 元数据文件
}

// PluginMetadata 插件元数据结构
type PluginMetadata struct {
	DocID           string         // 文档ID
	ScriptName      string         // 脚本名称
	Metadata        map[string]any // 元数据
	DocumentContent string         // 文档内容
}

// NewPluginsRagManager 创建一个新的插件 RAG 管理器
func NewPluginsRagManager(db *gorm.DB, ragSystem *rag.RAGSystem, collectionName string, metadataFile string) *PluginsRagManager {
	return &PluginsRagManager{
		db:             db,
		RagSystem:      ragSystem,
		collectionName: collectionName,
		metadataFile:   metadataFile,
	}
}

// NewSQLitePluginsRagManager 创建一个基于 SQLite 向量存储的插件 RAG 管理器
func NewSQLitePluginsRagManager(db *gorm.DB, collectionName string, modelName string, dimension int, metadataFile string, opts ...aispec.AIConfigOption) (*PluginsRagManager, error) {
	if collectionName == "" {
		collectionName = PLUGIN_RAG_COLLECTION_NAME
	}

	ragOptions := []any{}
	for _, opt := range opts {
		ragOptions = append(ragOptions, opt)
	}

	ragSystem, err := rag.CreateOrLoadCollection(db, collectionName, "用于储存 Yaklang 插件的 RAG 系统", ragOptions...)
	if err != nil {
		return nil, utils.Errorf("创建基于 SQLite 的 RAG 系统失败: %v", err)
	}

	// 创建插件 RAG 管理器
	return NewPluginsRagManager(db, ragSystem, collectionName, metadataFile), nil
}

// IndexAllPlugins 索引所有未被忽略的插件
func (m *PluginsRagManager) IndexAllPlugins() error {
	// 首先获取插件列表但不持有锁
	var scripts []schema.YakScript
	if err := m.db.Where("ignored = ?", false).Find(&scripts).Error; err != nil {
		return utils.Errorf("查询插件失败: %v", err)
	}

	log.Infof("开始索引 %d 个插件到 RAG 系统", len(scripts))

	// 使用生产者消费者模型处理索引
	scriptChan := make(chan schema.YakScript, 50)  // 插件通道
	metadataChan := make(chan *PluginMetadata, 50) // 元数据通道

	// 创建一个计数器来跟踪进度
	var processedCount int32
	var totalCount = int32(len(scripts))
	var progressMutex sync.Mutex

	// 设置定时器，每3秒输出一次进度
	progressTicker := time.NewTicker(3 * time.Second)
	defer progressTicker.Stop()

	// 创建一个通道用于结束进度报告
	progressDone := make(chan struct{})

	// 启动进度报告协程
	go func() {
		for {
			select {
			case <-progressTicker.C:
				progressMutex.Lock()
				current := processedCount
				progressMutex.Unlock()
				if totalCount > 0 {
					percentage := float32(current) / float32(totalCount) * 100
					log.Infof("索引进度: %.2f%% (%d/%d)", percentage, current, totalCount)
				}
			case <-progressDone:
				return
			}
		}
	}()

	// 启动消费者协程
	consumerCount := 3
	consumerWg := sync.WaitGroup{} // 消费者等待组
	for i := 0; i < consumerCount; i++ {
		consumerWg.Add(1)
		go func(consumerID int) {
			defer consumerWg.Done()
			m.indexPlugins(metadataChan, func(key string) {
				processedCount += 1
				progressMutex.Lock()
				progressMutex.Unlock()
			})
		}(i)
	}

	allMetadatas := []*PluginMetadata{}
	// 启动多个生产者协程
	producerCount := 1
	producerWg := sync.WaitGroup{} // 生产者等待组
	for i := 0; i < producerCount; i++ {
		producerWg.Add(1)
		go func(producerID int) {
			defer producerWg.Done()
			m.generateMetadata(producerID, scriptChan, func(meta *PluginMetadata) {
				progressMutex.Lock()
				defer progressMutex.Unlock()
				metadataChan <- meta
				allMetadatas = append(allMetadatas, meta)
				if m.metadataFile != "" {
					content, err := json.Marshal(allMetadatas)
					if err != nil {
						log.Errorf("序列化元数据失败: %v", err)
					}
					os.WriteFile(m.metadataFile, content, 0644)
				}
			})
		}(i)
	}

	for _, script := range scripts {
		scriptChan <- script
	}
	close(scriptChan) // 关闭脚本通道，表示没有更多脚本

	// 等待所有生产者完成
	producerWg.Wait()
	// 等待所有消费者完成
	consumerWg.Wait()
	close(metadataChan) // 关闭元数据通道
	close(progressDone)
	log.Infof("完成插件索引，共索引 %d 个插件", processedCount)
	return nil
}

// generateMetadata 生产者：生成插件元数据
func (m *PluginsRagManager) generateMetadata(producerID int, scriptChan <-chan schema.YakScript, onMetadataGenerated func(meta *PluginMetadata)) {
	for script := range scriptChan {
		// 处理单个插件，最多重试5次
		maxRetries := 5
		var success bool
		var lastErr error
		for retry := 0; retry < maxRetries && !success; retry++ {
			// 如果是重试，添加短暂延迟
			if retry > 0 {
				time.Sleep(time.Duration(retry) * 500 * time.Millisecond)
				log.Warnf("生产者 %d: 重试生成插件 %s 的元数据，第 %d 次尝试", producerID, script.ScriptName, retry+1)
			}

			// 将插件转换为 YakScript 格式
			yakScript := script.ToGRPCModel()

			// 序列化插件
			marshalScript, err := json.Marshal(yakScript)
			if err != nil {
				lastErr = utils.Errorf("序列化插件失败: %v", err)
				continue
			}

			// 生成元数据
			genMetadata, err := GenerateYakScriptMetadata(string(marshalScript))
			if err != nil {
				lastErr = utils.Errorf("生成插件元数据失败: %v", err)
				continue
			}

			// 准备插件元数据
			metaMap := map[string]any{}

			// 准备插件内容，组合多个字段以提高搜索质量
			documentContent := fmt.Sprintf(`脚本名称: %s
描述信息: %s
`,
				yakScript.ScriptName,
				genMetadata.Description,
			)

			// 生成文档 ID
			docID := yakScript.ScriptName

			// 创建元数据对象并发送到通道
			pluginMeta := &PluginMetadata{
				DocID:           docID,
				ScriptName:      script.ScriptName,
				Metadata:        metaMap,
				DocumentContent: documentContent,
			}

			onMetadataGenerated(pluginMeta)
			success = true
			log.Infof("生产者 %d: 成功生成插件 %s 的元数据", producerID, script.ScriptName)
		}

		if !success {
			log.Warnf("生产者 %d: 放弃生成插件 %s 的元数据，已尝试 %d 次: %v", producerID, script.ScriptName, maxRetries, lastErr)
			// 注意：不会停止整个处理过程，只是跳过这个插件
		}
	}
}

// indexPlugins 消费者：处理元数据并索引
func (m *PluginsRagManager) indexPlugins(metadataChan <-chan *PluginMetadata, onIndexFinished func(key string)) {
	for meta := range metadataChan {
		// 尝试索引该插件，最多重试5次
		if err := m.indexSinglePlugin(meta); err != nil {
			log.Warnf("索引插件 %s 失败: %v", meta.ScriptName, err)
			continue
		}
		onIndexFinished(meta.ScriptName)
		log.Infof("成功索引插件: %s", meta.ScriptName)
	}
}

// IndexPlugin 索引单个插件
func (m *PluginsRagManager) IndexPlugin(scriptName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 查询插件
	var script schema.YakScript
	if err := m.db.First(&script, "script_name = ?", scriptName).Error; err != nil {
		return utils.Errorf("查询插件失败: %v", err)
	}

	// 检查插件是否被忽略
	if script.Ignored {
		return utils.Errorf("插件 %s 已被忽略，不会被索引", script.ScriptName)
	}

	// 将插件转换为 YakScript 格式
	yakScript := script.ToGRPCModel()

	// 序列化插件
	marshalScript, err := json.Marshal(yakScript)
	if err != nil {
		return utils.Errorf("序列化插件失败: %v", err)
	}

	// 生成元数据
	genMetadata, err := GenerateYakScriptMetadata(string(marshalScript))
	if err != nil {
		return utils.Errorf("生成插件元数据失败: %v", err)
	}

	// 准备插件元数据
	metaMap := map[string]any{
		"id":          yakScript.Id,
		"type":        yakScript.Type,
		"script_name": yakScript.ScriptName,
		"author":      yakScript.Author,
		"tags":        yakScript.Tags,
		"level":       yakScript.Level,
		"description": genMetadata.Description,
		"keywords":    genMetadata.Keywords,
	}

	// 准备插件内容
	documentContent := fmt.Sprintf(`脚本名称: %s
类型: %s
作者: %s
标签: %s
等级: %s
描述信息: %s
关键词: %s
`,
		yakScript.ScriptName,
		yakScript.Type,
		yakScript.Author,
		yakScript.Tags,
		yakScript.Level,
		genMetadata.Description,
		strings.Join(genMetadata.Keywords, ","),
	)

	// 生成文档 ID
	docID := yakScript.ScriptName

	// 创建元数据对象
	pluginMeta := &PluginMetadata{
		DocID:           docID,
		ScriptName:      script.ScriptName,
		Metadata:        metaMap,
		DocumentContent: documentContent,
	}

	// 索引插件
	if err := m.indexSinglePlugin(pluginMeta); err != nil {
		return err
	}

	return nil
}

// indexSinglePlugin 索引单个插件（内部使用）
func (m *PluginsRagManager) indexSinglePlugin(meta *PluginMetadata) error {
	maxRetries := 5
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		err := m.indexSinglePluginOnce(meta)
		if err == nil {
			return nil // 成功索引，直接返回
		}

		lastErr = err
		log.Warnf("索引插件 %s 第 %d 次尝试失败: %v", meta.ScriptName, i+1, err)

		// 短暂延迟后重试
		time.Sleep(time.Second * time.Duration(i+1))
	}

	return utils.Errorf("索引插件 %s 失败，已重试 %d 次: %v", meta.ScriptName, maxRetries, lastErr)
}

func (m *PluginsRagManager) indexSinglePluginOnce(meta *PluginMetadata) error {
	// 添加到 RAG 系统
	err := m.RagSystem.Add(meta.DocID, meta.DocumentContent, vectorstore.WithDocumentRawMetadata(meta.Metadata))
	if err != nil {
		return utils.Errorf("添加插件文档到 RAG 系统失败: %v", err)
	}

	return nil
}

type PluginSearchResult struct {
	Script *ypb.YakScript
	Score  float64
}

func (m *PluginsRagManager) SearchPluginsIds(query string, page, limit int) (int, []string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results, err := m.RagSystem.QueryWithPage(query, page, limit)
	if err != nil {
		return 0, nil, utils.Errorf("搜索 RAG 系统失败: %v", err)
	}

	// 文档 id 和插件 id 相同
	var ids []string
	for _, result := range results {
		ids = append(ids, result.Document.ID)
	}
	total, err := m.RagSystem.CountDocuments()
	if err != nil {
		return 0, nil, utils.Errorf("获取文档总数失败: %v", err)
	}
	return total, ids, nil
}

// SearchPlugins 使用自然语言搜索插件
func (m *PluginsRagManager) SearchPlugins(query string, limit int) ([]*PluginSearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 搜索 RAG 系统
	results, err := m.RagSystem.QueryWithPage(query, 1, limit)
	if err != nil {
		return nil, utils.Errorf("搜索 RAG 系统失败: %v", err)
	}

	// 如果没有结果，返回空数组
	if len(results) == 0 {
		return []*PluginSearchResult{}, nil
	}

	// 提取插件 ID 并查询完整插件信息
	var scriptNames []string
	var idToScore = make(map[string]float64)
	for _, result := range results {
		scriptNames = append(scriptNames, result.Document.ID)
		idToScore[result.Document.ID] = result.Score
	}

	// 查询插件
	var scripts []schema.YakScript
	if err := m.db.Where("script_name IN (?)", scriptNames).Find(&scripts).Error; err != nil {
		return nil, utils.Errorf("查询插件详情失败: %v", err)
	}

	// 将插件转换为 YakScript 格式并按相关性排序
	scriptMap := make(map[string]*ypb.YakScript)
	for _, script := range scripts {
		scriptMap[script.ScriptName] = script.ToGRPCModel()
	}

	// 按相关性排序结果
	var sortedScripts []*PluginSearchResult
	for _, result := range results {
		scriptName := result.Document.ID
		if script, exists := scriptMap[scriptName]; exists {
			// 添加相似度得分
			sortedScripts = append(sortedScripts, &PluginSearchResult{
				Script: script,
				Score:  result.Score,
			})
		}
	}

	return sortedScripts, nil
}
