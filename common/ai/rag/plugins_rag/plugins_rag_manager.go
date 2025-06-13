package plugins_rag

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// PluginsRagManager 插件 RAG 管理器
type PluginsRagManager struct {
	db             *gorm.DB        // 数据库连接
	ragSystem      *rag.RAGSystem  // RAG 系统
	collectionName string          // 集合名称
	mu             sync.RWMutex    // 互斥锁
	indexedPlugins map[string]bool // 已索引的插件 ID
}

// PluginMetadata 插件元数据结构
type PluginMetadata struct {
	DocID           string         // 文档ID
	ScriptName      string         // 脚本名称
	Metadata        map[string]any // 元数据
	DocumentContent string         // 文档内容
}

// NewPluginsRagManager 创建一个新的插件 RAG 管理器
func NewPluginsRagManager(db *gorm.DB, ragSystem *rag.RAGSystem, collectionName string) *PluginsRagManager {
	return &PluginsRagManager{
		db:             db,
		ragSystem:      ragSystem,
		collectionName: collectionName,
		indexedPlugins: make(map[string]bool),
	}
}

// IndexAllPlugins 索引所有未被忽略的插件
func (m *PluginsRagManager) IndexAllPlugins() error {
	// 首先获取插件列表但不持有锁
	var scripts []schema.YakScript
	if err := m.db.Where("ignored = ?", false).Find(&scripts).Error; err != nil {
		return utils.Errorf("查询插件失败: %v", err)
	}

	log.Infof("开始索引 %d 个插件到 RAG 系统", len(scripts))

	// 复制已索引插件列表以避免并发问题
	m.mu.RLock()
	indexedPlugins := make(map[string]bool)
	for k, v := range m.indexedPlugins {
		indexedPlugins[k] = v
	}
	m.mu.RUnlock()

	// 使用生产者消费者模型处理索引
	scriptChan := make(chan schema.YakScript, 50)  // 插件通道
	metadataChan := make(chan *PluginMetadata, 50) // 元数据通道
	errorChan := make(chan error, 1)               // 错误通道
	doneChan := make(chan struct{})                // 完成通道
	resultChan := make(chan map[string]bool)       // 结果通道
	producerWg := sync.WaitGroup{}                 // 生产者等待组

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
					percentage := float64(current) / float64(totalCount) * 100
					log.Infof("索引进度: %.2f%% (%d/%d)", percentage, current, totalCount)
				}
			case <-progressDone:
				return
			}
		}
	}()

	// 启动消费者协程
	go m.indexConsumer(metadataChan, errorChan, doneChan, resultChan)

	// 启动多个生产者协程
	producerCount := 3
	for i := 0; i < producerCount; i++ {
		producerWg.Add(1)
		go func(producerID int) {
			defer producerWg.Done()
			m.metadataProducer(producerID, scriptChan, indexedPlugins, metadataChan, errorChan)
		}(i)
	}

	// 将插件发送到脚本通道
	go func() {
		for _, script := range scripts {
			// 跳过已经索引的插件
			if indexedPlugins[script.ScriptName] {
				progressMutex.Lock()
				processedCount++
				progressMutex.Unlock()
				continue
			}
			scriptChan <- script
		}
		close(scriptChan) // 关闭脚本通道，表示没有更多脚本

		// 等待所有生产者完成
		producerWg.Wait()
		close(metadataChan) // 关闭元数据通道
	}()

	// 等待处理完成或错误
	var newIndexed map[string]bool
	select {
	case err := <-errorChan:
		close(progressDone)
		return err
	case newIndexed = <-resultChan:
		// 更新已索引插件列表
		m.mu.Lock()
		for id := range newIndexed {
			m.indexedPlugins[id] = true
			progressMutex.Lock()
			processedCount++
			progressMutex.Unlock()
		}
		m.mu.Unlock()
		close(progressDone)
		log.Infof("完成插件索引，共索引 %d 个插件", len(m.indexedPlugins))
		return nil
	}
}

// metadataProducer 生产者：生成插件元数据
func (m *PluginsRagManager) metadataProducer(producerID int, scriptChan <-chan schema.YakScript, indexedPlugins map[string]bool, metadataChan chan<- *PluginMetadata, errorChan chan<- error) {
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
			metaMap := map[string]any{
				"id": yakScript.Id,
			}

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

			metadataChan <- pluginMeta
			success = true
			log.Infof("生产者 %d: 成功生成插件 %s 的元数据", producerID, script.ScriptName)
		}

		if !success {
			log.Warnf("生产者 %d: 放弃生成插件 %s 的元数据，已尝试 %d 次: %v", producerID, script.ScriptName, maxRetries, lastErr)
			// 注意：不会停止整个处理过程，只是跳过这个插件
		}
	}
}

// indexConsumer 消费者：处理元数据并索引
func (m *PluginsRagManager) indexConsumer(metadataChan <-chan *PluginMetadata, errorChan chan<- error, doneChan chan<- struct{}, resultChan chan<- map[string]bool) {
	var wg sync.WaitGroup
	workerCount := 3 // 设置工作协程数量

	// 使用并发安全的map来收集已索引的插件
	var resultMu sync.Mutex
	successfullyIndexed := make(map[string]bool)

	// 创建工作协程池
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for meta := range metadataChan {
				// 尝试索引该插件，最多重试5次
				if err := m.indexSinglePlugin(meta); err != nil {
					log.Warnf("工作协程 %d: 索引插件 %s 失败: %v", workerID, meta.ScriptName, err)
					continue
				}

				// 记录成功索引的插件ID
				resultMu.Lock()
				successfullyIndexed[meta.ScriptName] = true
				resultMu.Unlock()

				log.Infof("工作协程 %d: 成功索引插件: %s", workerID, meta.ScriptName)
			}
		}(i)
	}

	// 等待所有工作协程完成
	wg.Wait()

	// 发送结果并关闭通道
	resultChan <- successfullyIndexed
	close(doneChan)
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

	// 标记为已索引
	m.indexedPlugins[scriptName] = true

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
	// 创建文档
	doc := rag.Document{
		ID:       meta.DocID,
		Content:  meta.DocumentContent,
		Metadata: meta.Metadata,
	}

	// 添加到 RAG 系统
	err := m.ragSystem.AddDocuments(doc)
	if err != nil {
		return utils.Errorf("添加插件文档到 RAG 系统失败: %v", err)
	}

	return nil
}

type PluginSearchResult struct {
	Script *ypb.YakScript
	Score  float64
}

func (m *PluginsRagManager) SearchPluginsIds(query string, limit int) ([]int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 搜索 RAG 系统
	results, err := m.ragSystem.Query(query, limit)
	if err != nil {
		return nil, utils.Errorf("搜索 RAG 系统失败: %v", err)
	}
	ids := make([]int64, 0)
	for _, result := range results {
		if id, ok := result.Document.Metadata["id"].(float64); ok {
			ids = append(ids, int64(id))
		}
	}
	return ids, nil
}

// SearchPlugins 使用自然语言搜索插件
func (m *PluginsRagManager) SearchPlugins(query string, limit int) ([]*PluginSearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 检查是否有插件被索引
	// if len(m.indexedPlugins) == 0 {
	// 	return nil, utils.Errorf("尚未索引任何插件，请先调用 IndexAllPlugins")
	// }

	// 搜索 RAG 系统
	results, err := m.ragSystem.Query(query, limit)
	if err != nil {
		return nil, utils.Errorf("搜索 RAG 系统失败: %v", err)
	}

	// 如果没有结果，返回空数组
	if len(results) == 0 {
		return []*PluginSearchResult{}, nil
	}

	// 提取插件 ID 并查询完整插件信息
	var scriptIDs []int64
	var idToScore = make(map[int64]float64)
	for _, result := range results {
		if id, ok := result.Document.Metadata["id"].(float64); ok {
			scriptIDs = append(scriptIDs, int64(id))
			idToScore[int64(id)] = result.Score
		}
	}

	// 查询插件
	var scripts []schema.YakScript
	if err := m.db.Where("id IN (?)", scriptIDs).Find(&scripts).Error; err != nil {
		return nil, utils.Errorf("查询插件详情失败: %v", err)
	}

	// 将插件转换为 YakScript 格式并按相关性排序
	scriptMap := make(map[int64]*ypb.YakScript)
	for _, script := range scripts {
		scriptMap[int64(script.ID)] = script.ToGRPCModel()
	}

	// 按相关性排序结果
	var sortedScripts []*PluginSearchResult
	for _, result := range results {
		if id, ok := result.Document.Metadata["id"].(float64); ok {
			if script, exists := scriptMap[int64(id)]; exists {
				// 添加相似度得分
				sortedScripts = append(sortedScripts, &PluginSearchResult{
					Script: script,
					Score:  result.Score,
				})
			}
		}
	}

	return sortedScripts, nil
}

// RemovePlugin 从 RAG 系统中移除插件
func (m *PluginsRagManager) RemovePlugin(scriptName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 生成文档 ID
	docID := scriptName

	// 从 RAG 系统中删除
	err := m.ragSystem.DeleteDocuments(docID)
	if err != nil {
		return utils.Errorf("从 RAG 系统中删除插件失败: %v", err)
	}

	// 从已索引集合中移除
	delete(m.indexedPlugins, scriptName)

	return nil
}

// GetIndexedPluginsCount 获取已索引的插件数量
func (m *PluginsRagManager) GetIndexedPluginsCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.indexedPlugins)
}

// Clear 清空所有索引的插件
func (m *PluginsRagManager) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 删除所有文档
	for id := range m.indexedPlugins {
		docID := id
		if err := m.ragSystem.DeleteDocuments(docID); err != nil {
			log.Warnf("删除插件文档 %s 失败: %v", docID, err)
		}
	}

	// 清空已索引的插件集合
	m.indexedPlugins = make(map[string]bool)

	return nil
}
