package aimem

import (
	"context"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	midtermForkSessionInfix     = "@fork:"
	midtermForkParentTagPrefix  = "midterm_fork_parent:"
	midtermForkBranchTagPrefix  = "midterm_fork_branch:"
	midtermForkTaskIndexTagPref = "midterm_fork_task:"
)

// MidtermMemoryFork isolates midterm archive graph writes on a branch while sharing
// fork-time entity nodes with the parent store.
type MidtermMemoryFork struct {
	ParentSessionID     string
	BranchSessionID     string
	TaskIndex           string
	TaskName            string
	PersistentSessionID string
	ParentStore         *AIMemoryTriage
	BranchStore         *AIMemoryTriage
	BaseMemoryIDs       map[string]struct{}
	CreatedAt           time.Time
}

// MidtermMemoryMergeResult summarizes nodes adopted into the parent store.
type MidtermMemoryMergeResult struct {
	TaskIndex       string
	NodesMerged     int
	NodesSkipped    int
	ParentSessionID string
	BranchSessionID string
}

// BuildMidtermForkSessionID derives an isolated branch session id from parent midterm session.
func BuildMidtermForkSessionID(parentSessionID, taskIndex string) string {
	parentSessionID = strings.TrimSpace(parentSessionID)
	taskIndex = strings.TrimSpace(taskIndex)
	if parentSessionID == "" || taskIndex == "" {
		return ""
	}
	taskIndex = strings.ReplaceAll(taskIndex, "/", "-")
	return parentSessionID + midtermForkSessionInfix + taskIndex
}

// ForkMidtermArchiveStore clones the parent HNSW graph into a branch session without
// duplicating ai_memory_entities rows. Branch archives only touch BranchStore.
func ForkMidtermArchiveStore(parent *AIMemoryTriage, taskIndex, taskName, persistentSessionID string) (*MidtermMemoryFork, error) {
	if parent == nil {
		return nil, nil
	}
	parentSessionID := strings.TrimSpace(parent.GetSessionID())
	if parentSessionID == "" {
		return nil, utils.Error("parent midterm session id is empty")
	}
	branchSessionID := BuildMidtermForkSessionID(parentSessionID, taskIndex)
	if branchSessionID == "" {
		return nil, utils.Errorf("invalid midterm fork session for task %q", taskIndex)
	}

	db := parent.SafeGetDB()
	if db == nil {
		return nil, utils.Error("database connection is nil")
	}
	if err := cloneAIMemoryCollectionGraph(db, parentSessionID, branchSessionID); err != nil {
		return nil, err
	}

	branchStore, err := NewAIMemoryForQuery(branchSessionID, WithDatabase(db))
	if err != nil {
		return nil, utils.Errorf("create branch midterm store failed: %v", err)
	}

	baseIDs, err := listHNSWMemoryIDs(branchStore.hnswBackend)
	if err != nil {
		return nil, err
	}
	baseSet := make(map[string]struct{}, len(baseIDs))
	for _, id := range baseIDs {
		if id = strings.TrimSpace(id); id != "" {
			baseSet[id] = struct{}{}
		}
	}

	return &MidtermMemoryFork{
		ParentSessionID:     parentSessionID,
		BranchSessionID:     branchSessionID,
		TaskIndex:           strings.TrimSpace(taskIndex),
		TaskName:            strings.TrimSpace(taskName),
		PersistentSessionID: strings.TrimSpace(persistentSessionID),
		ParentStore:         parent,
		BranchStore:         branchStore,
		BaseMemoryIDs:       baseSet,
		CreatedAt:           time.Now(),
	}, nil
}

var _ aicommon.TimelineArchiveStore = (*MidtermMemoryFork)(nil)

func (f *MidtermMemoryFork) ArchiveCompressedBatch(ctx context.Context, batch *aicommon.TimelineArchiveBatch) (*aicommon.TimelineArchiveRef, error) {
	if f == nil || f.BranchStore == nil {
		return nil, utils.Error("midterm memory fork is nil")
	}
	if batch != nil && f.PersistentSessionID != "" {
		batch.PersistentSessionID = f.PersistentSessionID
		if batch.Tags == nil {
			batch.Tags = []string{}
		}
		batch.Tags = append(batch.Tags,
			midtermForkBranchTagPrefix+f.BranchSessionID,
			midtermForkParentTagPrefix+f.ParentSessionID,
		)
		if f.TaskIndex != "" {
			batch.Tags = append(batch.Tags, midtermForkTaskIndexTagPref+f.TaskIndex)
		}
	}
	return f.BranchStore.ArchiveCompressedBatch(ctx, batch)
}

func (f *MidtermMemoryFork) SearchArchivedBatches(ctx context.Context, query *aicommon.TimelineArchiveSearchQuery) (*aicommon.TimelineArchiveSearchResult, error) {
	if f == nil {
		return nil, nil
	}
	if query == nil {
		query = &aicommon.TimelineArchiveSearchQuery{}
	}

	var (
		branchResult *aicommon.TimelineArchiveSearchResult
		parentResult *aicommon.TimelineArchiveSearchResult
		err          error
	)
	if f.BranchStore != nil {
		branchResult, err = f.BranchStore.SearchArchivedBatches(ctx, query)
		if err != nil {
			return nil, err
		}
	}
	if f.ParentStore != nil && len(f.BaseMemoryIDs) > 0 {
		parentResult, err = f.ParentStore.SearchArchivedBatches(ctx, query)
		if err != nil {
			return nil, err
		}
	}
	return mergeMidtermForkSearchResults(branchResult, parentResult, f.BaseMemoryIDs), nil
}

// MergeBack adopts branch-only nodes into the parent store (graph + session rows).
func (f *MidtermMemoryFork) MergeBack() (*MidtermMemoryMergeResult, error) {
	if f == nil || f.ParentStore == nil || f.BranchStore == nil {
		return nil, nil
	}
	result := &MidtermMemoryMergeResult{
		TaskIndex:       f.TaskIndex,
		ParentSessionID: f.ParentSessionID,
		BranchSessionID: f.BranchSessionID,
	}

	db := f.ParentStore.SafeGetDB()
	if db == nil {
		return nil, utils.Error("database connection is nil")
	}

	var branchEntities []schema.AIMemoryEntity
	if err := db.Where("session_id = ?", f.BranchSessionID).Find(&branchEntities).Error; err != nil {
		return nil, utils.Errorf("query branch midterm entities failed: %v", err)
	}

	for _, dbEntity := range branchEntities {
		memoryID := strings.TrimSpace(dbEntity.MemoryID)
		if memoryID == "" {
			result.NodesSkipped++
			continue
		}
		if f.ParentStore.hnswBackend != nil && f.ParentStore.hnswBackend.HasMemoryID(memoryID) {
			result.NodesSkipped++
			continue
		}
		entity := memoryEntityFromDBEntity(dbEntity)
		if err := f.ParentStore.adoptForkedMemoryEntity(entity); err != nil {
			return nil, err
		}
		result.NodesMerged++
	}

	if result.NodesMerged > 0 {
		log.Infof("midterm fork merged task=%s branch=%s -> parent=%s merged=%d skipped=%d",
			f.TaskIndex, f.BranchSessionID, f.ParentSessionID, result.NodesMerged, result.NodesSkipped)
	}
	return result, nil
}

func mergeMidtermForkSearchResults(branch, parent *aicommon.TimelineArchiveSearchResult, baseIDs map[string]struct{}) *aicommon.TimelineArchiveSearchResult {
	merged := &aicommon.TimelineArchiveSearchResult{}
	seenArchive := make(map[string]struct{})
	seenMemory := make(map[string]struct{})
	var summaries []string

	appendResult := func(src *aicommon.TimelineArchiveSearchResult, restrictToBase bool) {
		if src == nil {
			return
		}
		if summary := strings.TrimSpace(src.SearchSummary); summary != "" {
			summaries = append(summaries, summary)
		}
		if src.TotalContent != "" && merged.TotalContent == "" {
			merged.TotalContent = src.TotalContent
			merged.ContentBytes = src.ContentBytes
		}
		for _, ref := range src.ArchiveRefs {
			if ref == nil || strings.TrimSpace(ref.ArchiveID) == "" {
				continue
			}
			if _, ok := seenArchive[ref.ArchiveID]; ok {
				continue
			}
			seenArchive[ref.ArchiveID] = struct{}{}
			merged.ArchiveRefs = append(merged.ArchiveRefs, ref)
		}
		for _, memory := range src.SelectedMemory {
			if memory == nil || strings.TrimSpace(memory.Id) == "" {
				continue
			}
			if restrictToBase {
				if _, ok := baseIDs[memory.Id]; !ok {
					continue
				}
			}
			if _, ok := seenMemory[memory.Id]; ok {
				continue
			}
			seenMemory[memory.Id] = struct{}{}
			merged.SelectedMemory = append(merged.SelectedMemory, memory)
		}
	}

	appendResult(branch, false)
	appendResult(parent, true)
	if len(summaries) > 0 {
		merged.SearchSummary = strings.Join(summaries, " -> ")
	}
	return merged
}

func cloneAIMemoryCollectionGraph(db *gorm.DB, parentSessionID, branchSessionID string) error {
	if db == nil {
		return utils.Error("database connection is nil")
	}
	var parentCol schema.AIMemoryCollection
	err := db.Where("session_id = ?", parentSessionID).First(&parentCol).Error
	if err == gorm.ErrRecordNotFound {
		parentCol = schema.AIMemoryCollection{
			SessionID:   parentSessionID,
			M:           16,
			Ml:          0.25,
			EfSearch:    20,
			EfConstruct: 200,
			Dimension:   7,
		}
		if err := db.Create(&parentCol).Error; err != nil {
			return utils.Errorf("create parent midterm collection failed: %v", err)
		}
	} else if err != nil {
		return utils.Errorf("load parent midterm collection failed: %v", err)
	}

	branchCol := schema.AIMemoryCollection{
		SessionID:   branchSessionID,
		GraphBinary: append([]byte(nil), parentCol.GraphBinary...),
		M:           parentCol.M,
		Ml:          parentCol.Ml,
		EfSearch:    parentCol.EfSearch,
		EfConstruct: parentCol.EfConstruct,
		Dimension:   parentCol.Dimension,
	}
	if branchCol.Dimension == 0 {
		branchCol.Dimension = 7
	}

	var existing schema.AIMemoryCollection
	err = db.Where("session_id = ?", branchSessionID).First(&existing).Error
	switch {
	case err == gorm.ErrRecordNotFound:
		return db.Create(&branchCol).Error
	case err != nil:
		return utils.Errorf("load branch midterm collection failed: %v", err)
	default:
		return db.Model(&existing).Updates(map[string]interface{}{
			"graph_binary":  branchCol.GraphBinary,
			"m":             branchCol.M,
			"ml":            branchCol.Ml,
			"ef_search":     branchCol.EfSearch,
			"ef_construct":  branchCol.EfConstruct,
			"dimension":     branchCol.Dimension,
		}).Error
	}
}

func listHNSWMemoryIDs(backend *AIMemoryHNSWBackend) ([]string, error) {
	if backend == nil {
		return nil, nil
	}
	return backend.ListMemoryIDs(), nil
}

// adoptForkedMemoryEntity moves a branch-only entity row into the parent session and indexes it.
func (r *AIMemoryTriage) adoptForkedMemoryEntity(entity *aicommon.MemoryEntity) error {
	if r == nil || entity == nil || strings.TrimSpace(entity.Id) == "" {
		return utils.Error("invalid memory entity for fork merge")
	}
	db := r.SafeGetDB()
	if db == nil {
		return utils.Error("database connection is nil")
	}

	var existing schema.AIMemoryEntity
	err := db.Where("memory_id = ?", entity.Id).First(&existing).Error
	if err != nil {
		return utils.Errorf("load fork memory entity failed: %v", err)
	}

	existing.SessionID = r.sessionID
	existing.Content = entity.Content
	existing.Tags = schema.StringArray(entity.Tags)
	existing.PotentialQuestions = schema.StringArray(entity.PotentialQuestions)
	existing.C_Score = entity.C_Score
	existing.O_Score = entity.O_Score
	existing.R_Score = entity.R_Score
	existing.E_Score = entity.E_Score
	existing.P_Score = entity.P_Score
	existing.A_Score = entity.A_Score
	existing.T_Score = entity.T_Score
	existing.CorePactVector = schema.FloatArray(entity.CorePactVector)

	if err := db.Save(&existing).Error; err != nil {
		return utils.Errorf("adopt fork memory entity failed: %v", err)
	}

	entity = memoryEntityFromDBEntity(existing)
	if r.hnswBackend != nil {
		if err := r.hnswBackend.Add(entity); err != nil {
			return utils.Errorf("add adopted memory to parent graph failed: %v", err)
		}
	}

	if r.rag != nil {
		for _, question := range existing.PotentialQuestions {
			question = strings.TrimSpace(question)
			if question == "" {
				continue
			}
			docID := existing.QuestionHashID(question)
			if err := r.rag.Add(docID, question,
				rag.WithDocumentMetadataKeyValue("memory_id", entity.Id),
				rag.WithDocumentMetadataKeyValue("question", question),
				rag.WithDocumentMetadataKeyValue("session_id", r.sessionID),
			); err != nil {
				log.Warnf("index adopted fork memory to RAG failed: %v", err)
			}
		}
	}
	return nil
}
