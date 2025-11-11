package lsp

import (
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

// DocumentState 表示单个文档的状态
type DocumentState struct {
	URI     string
	Version int
	Content string
	Editor  *memedit.MemEditor

	// 缓存层次
	SyntaxCache *SyntaxCache // AST + 语法错误
	SSACache    *SSACache    // SSA Program + 语义信息

	LastEditTime time.Time
	EditCount    int // 连续编辑计数（用于判断输入爆发）

	mu sync.RWMutex
}

// SyntaxCache 存储 AST 相关缓存
type SyntaxCache struct {
	AST        ssa.FrontAST
	AntlrCache *ssa.AntlrCache
	Hash       string // 结构哈希
	CreatedAt  time.Time
}

// SSACache 存储 SSA Program 缓存
type SSACache struct {
	Program   *ssaapi.Program
	Hash      string // 语义哈希
	CreatedAt time.Time
	Stale     bool // 标记为过期但仍可用
}

// DocumentManager 管理所有打开的文档
type DocumentManager struct {
	documents map[string]*DocumentState
	mu        sync.RWMutex

	// 配置
	maxCacheAge  time.Duration // 最大缓存年龄
	maxDocuments int           // 最大文档数
}

// NewDocumentManager 创建文档管理器
func NewDocumentManager() *DocumentManager {
	return &DocumentManager{
		documents:    make(map[string]*DocumentState),
		maxCacheAge:  5 * time.Minute,
		maxDocuments: 50, // 限制同时打开的文档数
	}
}

// GetDocument 获取文档状态
func (dm *DocumentManager) GetDocument(uri string) (*DocumentState, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	doc, ok := dm.documents[uri]
	return doc, ok
}

// OpenDocument 打开新文档
func (dm *DocumentManager) OpenDocument(uri string, version int, content string) *DocumentState {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	// 检查文档数量限制
	if len(dm.documents) >= dm.maxDocuments {
		dm.evictOldestDocument()
	}

	doc := &DocumentState{
		URI:          uri,
		Version:      version,
		Content:      content,
		Editor:       memedit.NewMemEditor(content),
		LastEditTime: time.Now(),
		EditCount:    0,
	}

	dm.documents[uri] = doc
	log.Debugf("[LSP DocMgr] opened document: %s (version: %d, size: %d bytes)", uri, version, len(content))
	return doc
}

// UpdateDocument 更新文档内容（全量更新）
func (dm *DocumentManager) UpdateDocument(uri string, version int, content string) *DocumentState {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	doc, exists := dm.documents[uri]
	if !exists {
		// 文档不存在，创建新的
		doc = &DocumentState{
			URI:     uri,
			Version: version,
			Content: content,
			Editor:  memedit.NewMemEditor(content),
		}
		dm.documents[uri] = doc
	} else {
		doc.mu.Lock()
		defer doc.mu.Unlock()

		// 更新版本和内容
		doc.Version = version
		doc.Content = content
		doc.Editor = memedit.NewMemEditor(content)

		// 更新编辑统计
		now := time.Now()
		if now.Sub(doc.LastEditTime) < 500*time.Millisecond {
			// 连续编辑（输入爆发）
			doc.EditCount++
		} else {
			// 停顿后的新编辑
			doc.EditCount = 1
		}
		doc.LastEditTime = now

		// 标记缓存为过期
		if doc.SSACache != nil {
			doc.SSACache.Stale = true
		}

		log.Debugf("[LSP DocMgr] updated document: %s (version: %d, editCount: %d)", uri, version, doc.EditCount)
	}

	return doc
}

// CloseDocument 关闭文档
func (dm *DocumentManager) CloseDocument(uri string) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if doc, exists := dm.documents[uri]; exists {
		log.Debugf("[LSP DocMgr] closed document: %s (version: %d)", uri, doc.Version)
		delete(dm.documents, uri)
	}
}

// SetSyntaxCache 设置语法缓存
func (ds *DocumentState) SetSyntaxCache(ast ssa.FrontAST, antlrCache *ssa.AntlrCache, hash string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	ds.SyntaxCache = &SyntaxCache{
		AST:        ast,
		AntlrCache: antlrCache,
		Hash:       hash,
		CreatedAt:  time.Now(),
	}
}

// SetSSACache 设置 SSA 缓存
func (ds *DocumentState) SetSSACache(program *ssaapi.Program, hash string) {
	ds.mu.Lock()
	defer ds.mu.Unlock()

	ds.SSACache = &SSACache{
		Program:   program,
		Hash:      hash,
		CreatedAt: time.Now(),
		Stale:     false,
	}
}

// GetSSACache 获取 SSA 缓存
func (ds *DocumentState) GetSSACache() *SSACache {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.SSACache
}

// GetSyntaxCache 获取语法缓存
func (ds *DocumentState) GetSyntaxCache() *SyntaxCache {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.SyntaxCache
}

// IsTyping 判断是否处于输入爆发状态
func (ds *DocumentState) IsTyping() bool {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.EditCount > 2 && time.Since(ds.LastEditTime) < 500*time.Millisecond
}

// GetContent 获取文档内容
func (ds *DocumentState) GetContent() string {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.Content
}

// GetVersion 获取文档版本
func (ds *DocumentState) GetVersion() int {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.Version
}

// GetEditor 获取编辑器
func (ds *DocumentState) GetEditor() *memedit.MemEditor {
	ds.mu.RLock()
	defer ds.mu.RUnlock()
	return ds.Editor
}

// evictOldestDocument 驱逐最老的文档（内部方法，调用者需持有锁）
func (dm *DocumentManager) evictOldestDocument() {
	var oldestURI string
	var oldestTime time.Time

	for uri, doc := range dm.documents {
		doc.mu.RLock()
		editTime := doc.LastEditTime
		doc.mu.RUnlock()

		if oldestURI == "" || editTime.Before(oldestTime) {
			oldestURI = uri
			oldestTime = editTime
		}
	}

	if oldestURI != "" {
		log.Debugf("[LSP DocMgr] evicting oldest document: %s", oldestURI)
		delete(dm.documents, oldestURI)
	}
}

// GetAllDocuments 获取所有文档（用于调试）
func (dm *DocumentManager) GetAllDocuments() []string {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	uris := make([]string, 0, len(dm.documents))
	for uri := range dm.documents {
		uris = append(uris, uri)
	}
	return uris
}

// CleanupStaleCache 清理过期的缓存
func (dm *DocumentManager) CleanupStaleCache() {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	now := time.Now()
	for uri, doc := range dm.documents {
		doc.mu.Lock()
		if doc.SSACache != nil && now.Sub(doc.SSACache.CreatedAt) > dm.maxCacheAge {
			log.Debugf("[LSP DocMgr] cleaning stale SSA cache: %s", uri)
			doc.SSACache = nil
		}
		if doc.SyntaxCache != nil && now.Sub(doc.SyntaxCache.CreatedAt) > dm.maxCacheAge {
			log.Debugf("[LSP DocMgr] cleaning stale syntax cache: %s", uri)
			doc.SyntaxCache = nil
		}
		doc.mu.Unlock()
	}
}
