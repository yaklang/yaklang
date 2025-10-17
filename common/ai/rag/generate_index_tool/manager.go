package generate_index_tool

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// IndexManager 通用索引管理器
type IndexManager struct {
	db             *gorm.DB
	ragSystem      *rag.RAGSystem
	collectionName string
	options        *IndexOptions
	mu             sync.RWMutex
}

// NewIndexManager 创建索引管理器
func NewIndexManager(db *gorm.DB, ragSystem *rag.RAGSystem, collectionName string, options *IndexOptions) *IndexManager {
	if options == nil {
		options = DefaultIndexOptions()
	}

	// 设置默认值
	if options.CacheManager == nil {
		options.CacheManager = NewFileCacheManager(options.CacheDir)
	}
	if options.ContentProcessor == nil {
		// 默认使用简单处理器，不使用AI
		options.ContentProcessor = NewSimpleContentProcessor()
	}
	if options.BatchSize <= 0 {
		options.BatchSize = 50
	}
	if options.ConcurrentWorkers <= 0 {
		options.ConcurrentWorkers = 3
	}

	return &IndexManager{
		db:             db,
		ragSystem:      ragSystem,
		collectionName: collectionName,
		options:        options,
	}
}

// DefaultIndexOptions 默认索引选项
func DefaultIndexOptions() *IndexOptions {
	return &IndexOptions{
		CacheDir:          "",
		ForceBypassCache:  false,
		IncludeMetadata:   true,
		BatchSize:         50,
		ConcurrentWorkers: 3,
	}
}

// IndexItems 索引数据项列表
func (m *IndexManager) IndexItems(ctx context.Context, items []IndexableItem) (*IndexResult, error) {
	startTime := time.Now()

	log.Infof("Starting to index %d items to RAG system", len(items))

	result := &IndexResult{
		FailedItems: []FailedItem{},
	}

	// 第一步：生成原始内容
	rawContents, err := m.generateRawContents(ctx, items, result)
	if err != nil {
		return result, err
	}

	// 第二步和第三步：流式处理（AI处理后立即生成向量）
	err = m.processAndIndexStreaming(ctx, items, rawContents, result)
	if err != nil {
		return result, err
	}

	result.Duration = time.Since(startTime).String()
	log.Infof("Indexing completed, success: %d, failed: %d, skipped: %d, duration: %s",
		result.SuccessCount, len(result.FailedItems), result.SkippedCount, result.Duration)

	return result, nil
}

// generateRawContents 第一步：生成原始内容
func (m *IndexManager) generateRawContents(ctx context.Context, items []IndexableItem, result *IndexResult) (map[string]string, error) {
	// 加载缓存
	var rawCache map[string]string
	var err error

	if !m.options.ForceBypassCache {
		rawCache, err = m.options.CacheManager.LoadRawCache()
		if err != nil {
			log.Warnf("Failed to load raw content cache: %v", err)
			rawCache = make(map[string]string)
		}
	} else {
		rawCache = make(map[string]string)
	}

	// 找出需要处理的项目
	var needProcessItems []IndexableItem
	for _, item := range items {
		key := item.GetKey()
		if _, exists := rawCache[key]; !exists || m.options.ForceBypassCache {
			needProcessItems = append(needProcessItems, item)
		}
	}

	if len(needProcessItems) == 0 {
		log.Info("All items are already in raw content cache, skipping generation step")
		return rawCache, nil
	}

	log.Infof("Number of items needing raw content generation: %d", len(needProcessItems))

	// 并发处理
	var processedCount int32
	var mu sync.Mutex

	// 创建工作通道
	itemChan := make(chan IndexableItem, m.options.BatchSize)
	var wg sync.WaitGroup

	// 启动进度打印协程
	progressTicker := time.NewTicker(1 * time.Second)
	defer progressTicker.Stop()

	progressDone := make(chan bool)
	go func() {
		for {
			select {
			case <-progressTicker.C:
				current := atomic.LoadInt32(&processedCount)
				percentage := float64(current) / float64(len(needProcessItems)) * 100
				log.Infof("Raw content generation progress: %d/%d (%.1f%%)", current, len(needProcessItems), percentage)
			case <-progressDone:
				return
			}
		}
	}()

	// 启动工作协程
	for i := 0; i < m.options.ConcurrentWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for item := range itemChan {
				content, err := item.GetContent()
				if err != nil {
					mu.Lock()
					result.FailedItems = append(result.FailedItems, FailedItem{
						Key:   item.GetKey(),
						Error: fmt.Sprintf("Failed to generate raw content: %v", err),
					})
					mu.Unlock()
					continue
				}

				mu.Lock()
				rawCache[item.GetKey()] = content
				// 立即保存缓存
				if err := m.options.CacheManager.SaveRawCache(rawCache); err != nil {
					log.Warnf("Failed to save raw content cache: %v", err)
				}
				mu.Unlock()

				current := atomic.AddInt32(&processedCount, 1)
				if m.options.ProgressCallback != nil {
					m.options.ProgressCallback(int(current), len(needProcessItems),
						fmt.Sprintf("Generating raw content: %s", item.GetDisplayName()))
				}
			}
		}(i)
	}

	// 发送任务
	for _, item := range needProcessItems {
		itemChan <- item
	}
	close(itemChan)

	// 等待完成
	wg.Wait()

	// 停止进度打印协程
	close(progressDone)

	// 打印最终进度
	finalCount := atomic.LoadInt32(&processedCount)
	log.Infof("Raw content generation completed: %d/%d (100.0%%)", finalCount, len(needProcessItems))

	// 保存缓存
	if err := m.options.CacheManager.SaveRawCache(rawCache); err != nil {
		log.Warnf("Failed to save raw content cache: %v", err)
	}

	log.Infof("Raw content generation completed, processed %d items", finalCount)
	return rawCache, nil
}

// processAndIndexStreaming 第二步和第三步：流式处理（AI处理后立即生成向量）
func (m *IndexManager) processAndIndexStreaming(ctx context.Context, items []IndexableItem, rawContents map[string]string, result *IndexResult) error {

	// 加载处理后内容缓存
	var processedCache map[string]string
	var err error

	if !m.options.ForceBypassCache {
		processedCache, err = m.options.CacheManager.LoadProcessedCache()
		if err != nil {
			log.Warnf("Failed to load processed content cache: %v", err)
			processedCache = make(map[string]string)
		}
	} else {
		processedCache = make(map[string]string)
	}

	// 创建项目映射
	itemMap := make(map[string]IndexableItem)
	for _, item := range items {
		itemMap[item.GetKey()] = item
	}

	// 找出需要处理的内容
	var needProcessKeys []string
	for key := range rawContents {
		if _, exists := processedCache[key]; !exists || m.options.ForceBypassCache {
			needProcessKeys = append(needProcessKeys, key)
		}
	}

	if len(needProcessKeys) == 0 {
		log.Info("All content is already in processed cache, skipping AI processing and vector generation steps")
		return nil
	}

	log.Infof("Number of contents needing streaming processing: %d", len(needProcessKeys))

	// 并发处理
	var successCount int32
	var mu sync.Mutex

	// 创建工作通道
	keyChan := make(chan string, m.options.BatchSize)
	var wg sync.WaitGroup

	// 启动工作协程
	for i := 0; i < m.options.ConcurrentWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for key := range keyChan {
				item := itemMap[key]
				rawContent := rawContents[key]

				// 第二步：AI处理内容
				processedContent, err := m.options.ContentProcessor.ProcessContent(ctx, rawContent)
				if err != nil {
					mu.Lock()
					result.FailedItems = append(result.FailedItems, FailedItem{
						Key:   key,
						Error: fmt.Sprintf("Failed to process content with AI: %v", err),
					})
					mu.Unlock()
					continue
				}

				// 立即保存处理后内容到缓存
				mu.Lock()
				processedCache[key] = processedContent
				if err := m.options.CacheManager.SaveProcessedCache(processedCache); err != nil {
					log.Warnf("Failed to save processed content cache: %v", err)
				}
				mu.Unlock()

				// 第三步：立即生成向量并索引到RAG系统
				var ragOptions []rag.DocumentOption
				if m.options.IncludeMetadata {
					metadata := item.GetMetadata()
					if metadata != nil {
						ragOptions = append(ragOptions, rag.WithDocumentRawMetadata(metadata))
					}
				}

				// 添加到RAG系统
				err = m.ragSystem.Add(key, processedContent, ragOptions...)
				if err != nil {
					mu.Lock()
					result.FailedItems = append(result.FailedItems, FailedItem{
						Key:   key,
						Error: fmt.Sprintf("Failed to add to RAG system: %v", err),
					})
					mu.Unlock()
					continue
				}

				current := atomic.AddInt32(&successCount, 1)
				if m.options.ProgressCallback != nil {
					m.options.ProgressCallback(int(current), len(needProcessKeys),
						fmt.Sprintf("Streaming processing completed: %s", item.GetDisplayName()))
				}
			}
		}(i)
	}

	// 发送任务
	for _, key := range needProcessKeys {
		if _, exists := itemMap[key]; exists {
			keyChan <- key
		}
	}
	close(keyChan)

	// 等待完成
	wg.Wait()

	result.SuccessCount = int(successCount)
	log.Infof("Streaming processing completed, successfully processed %d items", successCount)

	return nil
}

// processContents 第二步：AI处理内容
func (m *IndexManager) processContents(ctx context.Context, rawContents map[string]string, result *IndexResult) (map[string]string, error) {
	log.Info("Step 2: AI content processing")

	// 加载缓存
	var processedCache map[string]string
	var err error

	if !m.options.ForceBypassCache {
		processedCache, err = m.options.CacheManager.LoadProcessedCache()
		if err != nil {
			log.Warnf("Failed to load processed content cache: %v", err)
			processedCache = make(map[string]string)
		}
	} else {
		processedCache = make(map[string]string)
	}

	// 找出需要处理的内容
	var needProcessKeys []string
	for key := range rawContents {
		if _, exists := processedCache[key]; !exists || m.options.ForceBypassCache {
			needProcessKeys = append(needProcessKeys, key)
		}
	}

	if len(needProcessKeys) == 0 {
		log.Info("All content is already in processed cache, skipping AI processing step")
		return processedCache, nil
	}

	log.Infof("Number of contents needing AI processing: %d", len(needProcessKeys))

	// 并发处理
	var processedCount int32
	var mu sync.Mutex

	// 创建工作通道
	keyChan := make(chan string, m.options.BatchSize)
	var wg sync.WaitGroup

	// 启动进度打印协程
	progressTicker := time.NewTicker(1 * time.Second)
	defer progressTicker.Stop()

	progressDone := make(chan bool)
	go func() {
		for {
			select {
			case <-progressTicker.C:
				current := atomic.LoadInt32(&processedCount)
				percentage := float64(current) / float64(len(needProcessKeys)) * 100
				log.Infof("AI content processing progress: %d/%d (%.1f%%)", current, len(needProcessKeys), percentage)
			case <-progressDone:
				return
			}
		}
	}()

	// 启动工作协程
	for i := 0; i < m.options.ConcurrentWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for key := range keyChan {
				rawContent := rawContents[key]
				processedContent, err := m.options.ContentProcessor.ProcessContent(ctx, rawContent)
				if err != nil {
					mu.Lock()
					result.FailedItems = append(result.FailedItems, FailedItem{
						Key:   key,
						Error: fmt.Sprintf("Failed to process content with AI: %v", err),
					})
					mu.Unlock()
					continue
				}

				mu.Lock()
				processedCache[key] = processedContent
				mu.Unlock()

				current := atomic.AddInt32(&processedCount, 1)
				if m.options.ProgressCallback != nil {
					m.options.ProgressCallback(int(current), len(needProcessKeys),
						fmt.Sprintf("AI content processing: %s", key))
				}
			}
		}(i)
	}

	// 发送任务
	for _, key := range needProcessKeys {
		keyChan <- key
	}
	close(keyChan)

	// 等待完成
	wg.Wait()

	// 停止进度打印协程
	close(progressDone)

	// 打印最终进度
	finalCount := atomic.LoadInt32(&processedCount)
	log.Infof("AI content processing completed: %d/%d (100.0%%)", finalCount, len(needProcessKeys))

	// 保存缓存
	if err := m.options.CacheManager.SaveProcessedCache(processedCache); err != nil {
		log.Warnf("Failed to save processed content cache: %v", err)
	}

	log.Infof("AI content processing completed, processed %d items", finalCount)
	return processedCache, nil
}

// indexToRAG 第三步：生成向量并索引到RAG系统
func (m *IndexManager) indexToRAG(ctx context.Context, items []IndexableItem, processedContents map[string]string, result *IndexResult) error {
	log.Info("Step 3: Generate vectors and index to RAG system")

	// 创建项目映射
	itemMap := make(map[string]IndexableItem)
	for _, item := range items {
		itemMap[item.GetKey()] = item
	}

	var successCount int32
	var mu sync.Mutex

	// 创建工作通道
	keyChan := make(chan string, m.options.BatchSize)
	var wg sync.WaitGroup

	// 启动进度打印协程
	progressTicker := time.NewTicker(1 * time.Second)
	defer progressTicker.Stop()

	progressDone := make(chan bool)
	go func() {
		for {
			select {
			case <-progressTicker.C:
				current := atomic.LoadInt32(&successCount)
				percentage := float64(current) / float64(len(processedContents)) * 100
				log.Infof("RAG indexing progress: %d/%d (%.1f%%)", current, len(processedContents), percentage)
			case <-progressDone:
				return
			}
		}
	}()

	// 启动工作协程
	for i := 0; i < m.options.ConcurrentWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for key := range keyChan {
				item := itemMap[key]
				processedContent := processedContents[key]

				// 准备RAG选项
				var ragOptions []rag.DocumentOption
				if m.options.IncludeMetadata {
					metadata := item.GetMetadata()
					if metadata != nil {
						ragOptions = append(ragOptions, rag.WithDocumentRawMetadata(metadata))
					}
				}

				// 添加到RAG系统
				err := m.ragSystem.Add(key, processedContent, ragOptions...)
				if err != nil {
					mu.Lock()
					result.FailedItems = append(result.FailedItems, FailedItem{
						Key:   key,
						Error: fmt.Sprintf("Failed to add to RAG system: %v", err),
					})
					mu.Unlock()
					continue
				}

				current := atomic.AddInt32(&successCount, 1)
				if m.options.ProgressCallback != nil {
					m.options.ProgressCallback(int(current), len(processedContents),
						fmt.Sprintf("Indexing to RAG: %s", item.GetDisplayName()))
				}
			}
		}(i)
	}

	// 发送任务
	for key := range processedContents {
		if _, exists := itemMap[key]; exists {
			keyChan <- key
		}
	}
	close(keyChan)

	// 等待完成
	wg.Wait()

	// 停止进度打印协程
	close(progressDone)

	// 打印最终进度
	finalCount := atomic.LoadInt32(&successCount)
	log.Infof("RAG indexing completed: %d/%d (100.0%%)", finalCount, len(processedContents))

	result.SuccessCount = int(finalCount)
	log.Infof("RAG indexing completed, successfully indexed %d items", finalCount)

	return nil
}

// SearchItems 搜索项目
func (m *IndexManager) SearchItems(query string, page, limit int) ([]rag.SearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results, err := m.ragSystem.QueryWithPage(query, page, limit)
	if err != nil {
		return nil, utils.Errorf("Failed to search RAG system: %v", err)
	}

	return results, nil
}

// GetTotalCount 获取总文档数量
func (m *IndexManager) GetTotalCount() (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.ragSystem.CountDocuments()
}

// ClearCache 清空缓存
func (m *IndexManager) ClearCache() error {
	return m.options.CacheManager.Clear()
}
