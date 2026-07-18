package aicommon

import (
	"strings"
	"time"

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
		ts   int64
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
		var ts int64
		if branchTS, ok := f.Branch.idToTs.Get(id); ok {
			ts = branchTS
		}
		activeItems = append(activeItems, activeSnapshot{id: id, ts: ts, item: item})
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

	nextTS := int64(time.Now().UnixMilli())
	if keys := parent.tsToTimelineItem.Keys(); len(keys) > 0 {
		lastTS := keys[len(keys)-1]
		if lastTS >= nextTS {
			nextTS = lastTS + 1
		}
	}

	// Merge strategy:
	// 1) branch entries already carry globally allocated IDs from the shared SeqIdProvider;
	// 2) preserve those IDs on merge (no rebasing);
	// 3) regenerate timestamps as a monotonic merge sequence when branch timestamps
	//    would break parent ordering.
	for _, active := range activeItems {
		if _, exists := parent.idToTimelineItem.Get(active.id); exists {
			return nil, utils.Errorf("timeline fork merge: id %d already exists in parent timeline", active.id)
		}
		ts := nextTS
		if active.ts > 0 && active.ts >= nextTS {
			ts = active.ts
		}
		nextTS = ts + 1
		active.item.createdAt = time.Unix(0, ts*int64(time.Millisecond))
		parent.idToTimelineItem.OrderInsert(active.id, active.item, lessInt64)
		parent.idToTs.Set(active.id, ts)
		parent.tsToTimelineItem.OrderInsert(ts, active.item, lessInt64)
		if !isPromotableTimelineItem(active.item) {
			result.ActiveItemsMerged++
		}
	}

	if compressedHead != nil && compressedHead.head != nil {
		// CoveredEndItemID references the last compressed item in the branch; it was
		// allocated by the shared global ID provider and must not be remapped here.
		coveredID := compressedHead.head.CoveredEndItemID
		parent.updateCompressedHead(compressedHead.head)
		if compressedHead.ref != nil {
			compressedHead.ref.ReducerKeyID = coveredID
			parent.archiveRefs.Set(coveredID, compressedHead.ref)
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

func lessInt64(a, b int64) bool {
	return a < b
}
