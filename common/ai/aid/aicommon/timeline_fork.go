package aicommon

import (
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

type TimelineFork struct {
	Parent    *Timeline
	Branch    *Timeline
	TaskIndex string
	TaskName  string
	BaseMaxID int64
	CreatedAt time.Time
}

type TimelineMergeResult struct {
	TaskIndex             string
	ActiveItemsMerged     int
	CompressedHeadsMerged int
	RebasedIDs            int
}

func (m *Timeline) ForkForTask(taskIndex, taskName string, config AICallerConfigIf, ai AICaller) (*TimelineFork, error) {
	if m == nil {
		return nil, nil
	}

	baseMaxID := m.GetMaxID()
	raw, err := MarshalTimeline(m)
	if err != nil {
		return nil, err
	}
	branch, err := UnmarshalTimeline(raw)
	if err != nil {
		return nil, err
	}
	branch.SoftBindConfig(config, ai)
	branch.forkProtectedMaxID = baseMaxID
	branch.compressing = utils.NewOnce()
	branch.markBranchTimeline(true)

	return &TimelineFork{
		Parent:    m,
		Branch:    branch,
		TaskIndex: taskIndex,
		TaskName:  taskName,
		BaseMaxID: baseMaxID,
		CreatedAt: time.Now(),
	}, nil
}

func (f *TimelineFork) MergeBack() (*TimelineMergeResult, error) {
	if f == nil || f.Parent == nil || f.Branch == nil {
		return nil, nil
	}

	type activeSnapshot struct {
		id   int64
		item *TimelineItem
	}
	type compressedHeadSnapshot struct {
		head *TimelineCompressedHead
		ref  *TimelineArchiveRef
	}

	var activeItems []activeSnapshot
	var compressedHead *compressedHeadSnapshot
	f.Branch.mu.RLock()
	for _, id := range f.Branch.idToTimelineItem.Keys() {
		if id <= f.BaseMaxID {
			continue
		}
		item, ok := f.Branch.idToTimelineItem.Get(id)
		if !ok || item == nil || item.deleted {
			continue
		}
		activeItems = append(activeItems, activeSnapshot{id: id, item: item})
	}
	// Forks are created from a snapshot of the parent timeline, so a compressed head
	// whose covered end is still inside BaseMaxID belongs to the inherited prefix.
	// Only a head covering IDs produced by the branch is merged back to the parent.
	if head := f.Branch.compressedHead; head != nil && head.CoveredEndItemID > f.BaseMaxID && strings.TrimSpace(head.Text) != "" {
		s := &compressedHeadSnapshot{head: cloneTimelineCompressedHead(head)}
		if ref, ok := f.Branch.archiveRefs.Get(head.CoveredEndItemID); ok && ref != nil {
			refCopy := *ref
			s.ref = &refCopy
		}
		compressedHead = s
	}
	f.Branch.mu.RUnlock()

	parent := f.Parent
	parent.mu.Lock()
	defer parent.mu.Unlock()

	result := &TimelineMergeResult{TaskIndex: f.TaskIndex}

	allocateID := func() int64 {
		if parent.config != nil {
			return parent.config.AcquireId()
		}
		return parent.getMaxIDLocked() + 1
	}
	nextTS := int64(time.Now().UnixMilli())
	if keys := parent.tsToTimelineItem.Keys(); len(keys) > 0 {
		lastTS := keys[len(keys)-1]
		if lastTS >= nextTS {
			nextTS = lastTS + 1
		}
	}

	// Merge strategy (deterministic order first):
	// 1) active entries and branch-local compressed heads are merged by runtime stage task order;
	// 2) each merged entry is rebased to parent-generated IDs;
	// 3) timestamps are regenerated as a monotonic merge sequence.
	// This intentionally prefers deterministic replay/stable prompt order over preserving
	// branch-local wall-clock execution time.
	for _, active := range activeItems {
		newID := allocateID()
		if newID != active.id {
			result.RebasedIDs++
		}
		setTimelineItemID(active.item, newID)
		ts := nextTS
		nextTS++
		active.item.createdAt = time.Unix(0, ts*int64(time.Millisecond))
		parent.idToTimelineItem.OrderInsert(newID, active.item, lessInt64)
		parent.idToTs.Set(newID, ts)
		parent.tsToTimelineItem.OrderInsert(ts, active.item, lessInt64)
		result.ActiveItemsMerged++
	}

	if compressedHead != nil && compressedHead.head != nil {
		targetID := allocateID()
		if targetID != compressedHead.head.CoveredEndItemID {
			result.RebasedIDs++
		}
		compressedHead.head.CoveredEndItemID = targetID
		parent.updateCompressedHead(compressedHead.head)
		if compressedHead.ref != nil {
			compressedHead.ref.ReducerKeyID = targetID
			parent.archiveRefs.Set(targetID, compressedHead.ref)
		}
		result.CompressedHeadsMerged++
	}

	parent.dumpSizeCheckLocked()
	return result, nil
}

func (f *TimelineFork) Diff() (string, error) {
	if f == nil || f.Branch == nil {
		return "", nil
	}

	f.Branch.mu.RLock()
	defer f.Branch.mu.RUnlock()
	var ids []int64
	for _, id := range f.Branch.idToTimelineItem.Keys() {
		if id > f.BaseMaxID {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return "", nil
	}
	sub := f.Branch.createSubTimelineLocked(ids...)
	if sub == nil {
		return "", nil
	}
	return sub.Dump(), nil
}

func setTimelineItemID(item *TimelineItem, id int64) {
	if item == nil || item.value == nil {
		return
	}
	switch v := item.value.(type) {
	case *aitool.ToolResult:
		v.ID = id
	case *UserInteraction:
		v.ID = id
	case *TextTimelineItem:
		v.ID = id
	}
}

func lessInt64(a, b int64) bool {
	return a < b
}
