package aibalance

import (
	"testing"
)

func TestEntrypoint(t *testing.T) {
	// 创建新的 Entrypoint
	ep := NewEntrypoint()

	// 测试添加 Provider
	provider1 := &Provider{
		ModelName:   "test-model",
		TypeName:    "type1",
		DomainOrURL: "http://example.com",
	}
	provider2 := &Provider{
		ModelName:   "test-model",
		TypeName:    "type2",
		DomainOrURL: "http://example2.com",
	}
	modelName := "test-model"

	// 添加 Provider
	ep.AddProvider(modelName, provider1)
	ep.AddProvider(modelName, provider2)

	// 测试获取 Provider
	entry, ok := ep.ModelEntries.Get(modelName)
	if !ok {
		t.Fatalf("期望找到模型 %s，但未找到", modelName)
	}

	if len(entry.Providers) != 2 {
		t.Fatalf("期望有 2 个 Provider，但实际有 %d 个", len(entry.Providers))
	}

	// 测试 PeekProvider
	peekedProvider := ep.PeekProvider(modelName)
	if peekedProvider == nil {
		t.Fatal("PeekProvider 返回了 nil")
	}

	// 测试创建新的 ModelEntry
	newModelName := "new-model"
	newEntry := ep.CreateModelEntry(newModelName)
	if newEntry.ModelName != newModelName {
		t.Fatalf("期望模型名称为 %s，但实际为 %s", newModelName, newEntry.ModelName)
	}

	// 测试添加新的 ModelEntry
	ep.ModelEntries.Set(newModelName, newEntry)
	if _, ok := ep.ModelEntries.Get(newModelName); !ok {
		t.Fatalf("期望找到新添加的模型 %s，但未找到", newModelName)
	}
}
