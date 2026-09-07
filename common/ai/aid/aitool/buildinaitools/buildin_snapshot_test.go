package buildinaitools

import (
	"fmt"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestGetAllToolsReturnsSliceCopy(t *testing.T) {
	first := GetAllTools()
	if len(first) == 0 || first[0] == nil {
		t.Fatalf("GetAllTools returned no usable built-in tools: %#v", first)
	}

	wantFirst := first[0]
	first[0] = nil
	second := GetAllTools()
	if len(second) == 0 || second[0] != wantFirst {
		t.Fatalf("GetAllTools result aliases global slice: first tool = %#v, want %#v", second, wantFirst)
	}
}

func TestCreateScheduleToolIsEnabledByDefault(t *testing.T) {
	manager := NewToolManager()
	tools, err := manager.GetEnableTools()
	if err != nil {
		t.Fatalf("get enabled tools: %v", err)
	}
	for _, tool := range tools {
		if tool != nil && tool.Name == "create_ai_react_schedule" {
			return
		}
	}
	t.Fatal("create_ai_react_schedule must be visible to a normal ReAct chat")
}

func TestAllAIToolsSnapshotDoesNotAliasCallers(t *testing.T) {
	original := getAllAIToolsSnapshot()
	defer publishAllAITools(original)

	toolA := aitool.NewWithoutCallback("snapshot-a")
	toolB := aitool.NewWithoutCallback("snapshot-b")
	source := []*aitool.Tool{toolA}
	returned := publishAllAITools(source)

	source[0] = toolB
	returned[0] = toolB
	firstRead := getAllAIToolsSnapshot()
	if len(firstRead) != 1 || firstRead[0] != toolA {
		t.Fatalf("published snapshot changed through caller alias: %#v", firstRead)
	}

	firstRead[0] = toolB
	secondRead := getAllAIToolsSnapshot()
	if len(secondRead) != 1 || secondRead[0] != toolA {
		t.Fatalf("getter returned the global backing slice: %#v", secondRead)
	}
}

func TestAllAIToolsSnapshotConcurrentPublishAndRead(t *testing.T) {
	original := getAllAIToolsSnapshot()
	defer publishAllAITools(original)

	toolA := aitool.NewWithoutCallback("snapshot-concurrent-a")
	toolB := aitool.NewWithoutCallback("snapshot-concurrent-b")
	const iterations = 500
	publishAllAITools([]*aitool.Tool{toolA, toolA})

	var wg sync.WaitGroup
	start := make(chan struct{})
	for worker := 0; worker < 8; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < iterations; i++ {
				if worker%2 == 0 {
					tool := toolA
					if i%2 == 1 {
						tool = toolB
					}
					published := publishAllAITools([]*aitool.Tool{tool, tool})
					published[0] = nil
					continue
				}

				snapshot := getAllAIToolsSnapshot()
				if len(snapshot) != 2 || snapshot[0] == nil || snapshot[1] == nil || snapshot[0] != snapshot[1] {
					t.Errorf("reader observed a torn or aliased snapshot: %s", fmt.Sprint(snapshot))
					return
				}
				snapshot[0] = nil
			}
		}()
	}
	close(start)
	wg.Wait()
}
