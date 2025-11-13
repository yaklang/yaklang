package lsp

import (
	"testing"
	"time"
)

func TestDocumentManager_BasicOperations(t *testing.T) {
	dm := NewDocumentManager()

	// 测试打开文档
	uri := "file:///test.yak"
	content := "println(\"hello world\")"
	doc := dm.OpenDocument(uri, 1, content)

	if doc == nil {
		t.Fatal("OpenDocument returned nil")
	}

	if doc.GetContent() != content {
		t.Errorf("expected content %q, got %q", content, doc.GetContent())
	}

	if doc.GetVersion() != 1 {
		t.Errorf("expected version 1, got %d", doc.GetVersion())
	}

	// 测试获取文档
	retrieved, ok := dm.GetDocument(uri)
	if !ok {
		t.Fatal("GetDocument failed to retrieve document")
	}

	if retrieved.URI != uri {
		t.Errorf("expected URI %q, got %q", uri, retrieved.URI)
	}

	// 测试更新文档
	newContent := "println(\"hello yaklang\")"
	dm.UpdateDocument(uri, 2, newContent)

	retrieved, _ = dm.GetDocument(uri)
	if retrieved.GetContent() != newContent {
		t.Errorf("expected updated content %q, got %q", newContent, retrieved.GetContent())
	}

	if retrieved.GetVersion() != 2 {
		t.Errorf("expected version 2, got %d", retrieved.GetVersion())
	}

	// 测试关闭文档
	dm.CloseDocument(uri)
	_, ok = dm.GetDocument(uri)
	if ok {
		t.Error("document should be closed but still exists")
	}
}

func TestDocumentManager_EditCountTracking(t *testing.T) {
	dm := NewDocumentManager()

	uri := "file:///test.yak"
	_ = dm.OpenDocument(uri, 1, "initial")

	// 快速连续编辑
	dm.UpdateDocument(uri, 2, "edit1")
	time.Sleep(100 * time.Millisecond)
	dm.UpdateDocument(uri, 3, "edit2")
	time.Sleep(100 * time.Millisecond)
	dm.UpdateDocument(uri, 4, "edit3")

	retrieved, _ := dm.GetDocument(uri)
	if retrieved.EditCount < 2 {
		t.Errorf("expected EditCount >= 2, got %d", retrieved.EditCount)
	}

	// 测试 IsTyping
	if !retrieved.IsTyping() {
		t.Error("expected IsTyping to be true during burst editing")
	}

	// 等待停顿
	time.Sleep(600 * time.Millisecond)

	// 再次编辑应该重置计数
	dm.UpdateDocument(uri, 5, "edit4")
	retrieved, _ = dm.GetDocument(uri)
	if retrieved.EditCount != 1 {
		t.Errorf("expected EditCount to reset to 1, got %d", retrieved.EditCount)
	}
}

func TestDocumentManager_CacheOperations(t *testing.T) {
	dm := NewDocumentManager()

	uri := "file:///test.yak"
	doc := dm.OpenDocument(uri, 1, "test content")

	// 测试设置 SSA 缓存
	hash := "test_hash_123"
	doc.SetSSACache(nil, hash) // 使用 nil program 进行测试

	cache := doc.GetSSACache()
	if cache == nil {
		t.Fatal("SSA cache should be set")
	}

	if cache.Hash != hash {
		t.Errorf("expected hash %q, got %q", hash, cache.Hash)
	}

	if cache.Stale {
		t.Error("newly set cache should not be stale")
	}

	// 测试缓存过期标记
	doc.mu.Lock()
	if doc.SSACache != nil {
		doc.SSACache.Stale = true
	}
	doc.mu.Unlock()

	cache = doc.GetSSACache()
	if !cache.Stale {
		t.Error("cache should be marked as stale")
	}
}

func TestDocumentManager_MaxDocuments(t *testing.T) {
	dm := NewDocumentManager()
	dm.maxDocuments = 3 // 设置最大文档数为 3

	// 打开 4 个文档
	for i := 1; i <= 4; i++ {
		uri := "file:///test" + string(rune('0'+i)) + ".yak"
		dm.OpenDocument(uri, 1, "content")
		time.Sleep(10 * time.Millisecond) // 确保时间戳不同
	}

	// 应该只有 3 个文档
	docs := dm.GetAllDocuments()
	if len(docs) > 3 {
		t.Errorf("expected at most 3 documents, got %d", len(docs))
	}

	// 第一个文档应该被驱逐
	_, ok := dm.GetDocument("file:///test1.yak")
	if ok {
		t.Error("oldest document should have been evicted")
	}
}

func TestDocumentManager_ConcurrentAccess(t *testing.T) {
	dm := NewDocumentManager()

	uri := "file:///concurrent.yak"
	dm.OpenDocument(uri, 1, "initial")

	// 并发读写
	done := make(chan bool)

	// 并发读取
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				dm.GetDocument(uri)
			}
			done <- true
		}()
	}

	// 并发写入
	for i := 0; i < 5; i++ {
		go func(version int) {
			for j := 0; j < 50; j++ {
				dm.UpdateDocument(uri, version+j, "content")
			}
			done <- true
		}(i * 50)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 15; i++ {
		<-done
	}

	// 验证文档仍然存在
	_, ok := dm.GetDocument(uri)
	if !ok {
		t.Error("document should still exist after concurrent access")
	}
}
