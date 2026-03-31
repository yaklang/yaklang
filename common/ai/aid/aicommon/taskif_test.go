package aicommon

import "testing"

func TestAIStatefulTaskBase_TaskSemanticAccessors(t *testing.T) {
	task := NewStatefulTaskBase("task-1", "input", nil, nil, true)

	task.SetTaskRetrievalInfo(&AITaskRetrievalInfo{
		Tags:      []string{"java", "rewrite"},
		Questions: []string{"哪些方法需要重写？"},
		Target:    "提升可读性",
	})

	info := task.GetTaskRetrievalInfo()
	if info == nil {
		t.Fatal("unexpected nil retrieval info")
	}
	if len(info.Tags) != 2 || info.Tags[0] != "java" || info.Tags[1] != "rewrite" {
		t.Fatalf("unexpected tags: %#v", info.Tags)
	}
	if len(info.Questions) != 1 || info.Questions[0] != "哪些方法需要重写？" {
		t.Fatalf("unexpected questions: %#v", info.Questions)
	}
	if info.Target != "提升可读性" {
		t.Fatalf("unexpected target: %#v", info.Target)
	}

	info.Tags[0] = "mutated"
	info.Questions[0] = "mutated"
	info.Target = "mutated"

	got := task.GetTaskRetrievalInfo()
	if got.Tags[0] != "java" {
		t.Fatalf("tags should be returned as a copy")
	}
	if got.Questions[0] != "哪些方法需要重写？" {
		t.Fatalf("questions should be returned as a copy")
	}
	if got.Target != "提升可读性" {
		t.Fatalf("target should be returned as a copy")
	}
}
