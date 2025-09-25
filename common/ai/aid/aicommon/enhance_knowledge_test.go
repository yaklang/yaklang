package aicommon

import (
	"context"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
)

type testKnowledge struct {
	BasicEnhanceKnowledge
	method string
	uuid   string
	title  string
}

func (t *testKnowledge) GetUUID() string        { return t.uuid }
func (t *testKnowledge) GetTitle() string       { return t.title }
func (t *testKnowledge) GetScoreMethod() string { return t.method }

func newTestKnowledge(method, uuid, title, content, source string, score float64) *testKnowledge {
	return &testKnowledge{
		BasicEnhanceKnowledge: *NewBasicEnhanceKnowledge(content, source, score),
		uuid:                  uuid,
		title:                 title,
		method:                method,
	}
}

func TestEnhanceKnowledgeManager_AppendAndGet(t *testing.T) {
	manager := NewEnhanceKnowledgeManager(func(ctx context.Context, query string) (<-chan EnhanceKnowledge, error) {
		ch := make(chan EnhanceKnowledge, 1)
		close(ch)
		return ch, nil
	})

	uuid1 := uuid.NewString()
	uuid2 := uuid.NewString()

	taskID := "task-1"
	k1 := newTestKnowledge(uuid1, "mock", "Title1", "Content1", "Source1", 0.9)
	k2 := newTestKnowledge(uuid2, "mock", "Title2", "Content2", "Source2", 0.8)

	manager.AppendKnowledge(taskID, k1)
	manager.AppendKnowledge(taskID, k2)

	got := manager.GetKnowledgeByTaskID(taskID)
	if len(got) != 2 {
		t.Fatalf("expected 2 knowledge, got %d", len(got))
	}
	if got[0].GetUUID() != uuid1 && got[1].GetUUID() != uuid1 {
		t.Errorf("uuid-1 not found in results")
	}
	if got[0].GetUUID() != uuid2 && got[1].GetUUID() != uuid2 {
		t.Errorf("uuid-2 not found in results")
	}
}

func TestEnhanceKnowledgeManager_DumpKnowledgeByTaskID(t *testing.T) {
	manager := NewEnhanceKnowledgeManager(func(ctx context.Context, query string) (<-chan EnhanceKnowledge, error) {
		ch := make(chan EnhanceKnowledge, 1)
		close(ch)
		return ch, nil
	})

	taskID := "task-2"
	k := newTestKnowledge("mock", "uuid-3", "DumpTitle", "DumpContent", "DumpSource", 0.7)
	manager.AppendKnowledge(taskID, k)

	dump := manager.DumpTaskAboutKnowledge(taskID)
	if dump == "" || dump == "\n" {
		t.Errorf("expected non-empty dump, got: %q", dump)
	}
	if want := "DumpTitle"; !contains(dump, want) {
		t.Errorf("expected dump to contain %q, got: %q", want, dump)
	}
}

func TestEnhanceKnowledgeManager_FetchKnowledge(t *testing.T) {
	manager, token := NewMockEKManagerAndToken()
	ctx := context.Background()
	ch, err := manager.FetchKnowledge(ctx, "query")
	if err != nil {
		t.Fatalf("FetchKnowledge error: %v", err)
	}
	var found bool
	for k := range ch {
		if k.GetContent() == token {
			found = true
		}
	}
	if !found {
		t.Errorf("expected to find token %q in fetched knowledge", token)
	}
}

func TestKnowledgeCollection_RRFRank_FlattenDifference(t *testing.T) {
	kc := NewKnowledgeCollection()
	uuidA := uuid.NewString()
	uuidB := uuid.NewString()
	uuidC := uuid.NewString()
	uuidD := uuid.NewString()

	// uuidA: methodA极高分，methodB极低分
	kc.Append(newTestKnowledge("methodA", uuidA, "A", "CA", "SA", 0.99))
	kc.Append(newTestKnowledge("methodB", uuidA, "A", "CA", "SA", 0.01))
	// uuidB: methodA极低分，methodB极高分
	kc.Append(newTestKnowledge("methodA", uuidB, "B", "CB", "SB", 0.01))
	kc.Append(newTestKnowledge("methodB", uuidB, "B", "CB", "SB", 0.99))
	// uuidC: 两个方法都是中等分
	kc.Append(newTestKnowledge("methodA", uuidC, "C", "CC", "SC", 0.5))
	kc.Append(newTestKnowledge("methodB", uuidC, "C", "CC", "SC", 0.5))
	// uuidD: 两个方法都是低等分
	kc.Append(newTestKnowledge("methodA", uuidD, "D", "CD", "SD", 0.4))
	kc.Append(newTestKnowledge("methodB", uuidD, "D", "CD", "SD", 0.4))

	got := kc.GetKnowledgeList()
	if len(got) != 4 {
		t.Fatalf("expected 3 unique knowledge, got %d", len(got))
	}

	// 理论上C > A ~ B > D ，但RRF拉平A和B的差距，使其接近
	for i, k := range got {
		switch k.GetUUID() {
		case uuidA:
			require.True(t, i <= 2 && i >= 1, "uuidA should be in top 3, and not top 1,but got position %d", i)
		case uuidB:
			require.True(t, i <= 2 && i >= 1, "uuidB should be in top 3, and not top 1,but got position %d", i)
		case uuidC:
			require.Equal(t, 0, i, "uuidC should be top 1, but got position %d", i)
		case uuidD:
			require.Equal(t, 3, i, "uuidD should be last, but got position %d", i)
		default:
			t.Errorf("unexpected uuid %s in result", k.GetUUID())
		}
	}

}

func TestKnowledgeCollection_UselessFilter(t *testing.T) {
	kc := NewKnowledgeCollection()
	uuid1 := uuid.NewString()
	uuid2 := uuid.NewString()
	k1 := newTestKnowledge("mock", uuid1, "T1", "C1", "S1", 0.9)
	k2 := newTestKnowledge("mock", uuid2, "T2", "C2", "S2", 0.8)
	kc.Append(k1)
	kc.Append(k2)

	// 默认都能查到
	got := kc.GetKnowledgeList()
	require.Len(t, got, 2)

	// 设置uuid1为useless
	kc.SetUseless(uuid1)
	got = kc.GetKnowledgeList()
	require.Len(t, got, 1)
	require.Equal(t, uuid2, got[0].GetUUID())

	// 取消useless
	kc.UnsetUseless(uuid1)
	got = kc.GetKnowledgeList()
	require.Len(t, got, 2)
}

func TestEnhanceKnowledgeManager_UselessFilter(t *testing.T) {
	manager := NewEnhanceKnowledgeManager(func(ctx context.Context, query string) (<-chan EnhanceKnowledge, error) {
		ch := make(chan EnhanceKnowledge, 1)
		close(ch)
		return ch, nil
	})

	taskID := "task-useless"
	uuid1 := uuid.NewString()
	uuid2 := uuid.NewString()
	k1 := newTestKnowledge("mock", uuid1, "T1", "C1", "S1", 0.9)
	k2 := newTestKnowledge("mock", uuid2, "T2", "C2", "S2", 0.8)
	manager.AppendKnowledge(taskID, k1)
	manager.AppendKnowledge(taskID, k2)

	got := manager.GetKnowledgeByTaskID(taskID)
	require.Len(t, got, 2)

	// 设置useless
	manager.SetKnowledgeUseless(taskID, uuid1)
	got = manager.GetKnowledgeByTaskID(taskID)
	require.Len(t, got, 1)
	require.Equal(t, uuid2, got[0].GetUUID())

	// 取消useless
	manager.UnsetKnowledge(taskID, uuid1)
	got = manager.GetKnowledgeByTaskID(taskID)
	require.Len(t, got, 2)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > 0 && (contains(s[1:], substr) || contains(s[:len(s)-1], substr))))
}
