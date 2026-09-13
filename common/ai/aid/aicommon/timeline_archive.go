package aicommon

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type TimelineArchiveReason string

const (
	TimelineArchiveReasonBatchCompress     TimelineArchiveReason = "batch_compress"
	TimelineArchiveReasonEmergencyCompress TimelineArchiveReason = "emergency_compress"
)

type TimelineArchiveBatch struct {
	ArchiveID           string
	PersistentSessionID string
	Reason              TimelineArchiveReason
	Summary             string
	MergedContent       string
	SourceChunks        []string
	ReducerKeyID        int64
	SourceStartID       int64
	SourceEndID         int64
	SourceStartAt       time.Time
	SourceEndAt         time.Time
	ItemCount           int
	RepresentativeSnips []string
	Tags                []string
}

// TimelineArchiveRef preserves archive metadata in existing serialized timelines.
// New timeline compression does not create archive references.
type TimelineArchiveRef struct {
	ArchiveID      string                `json:"archive_id"`
	Reason         TimelineArchiveReason `json:"reason"`
	SummaryPreview string                `json:"summary_preview"`
	ReducerKeyID   int64                 `json:"reducer_key_id"`
	SourceStartID  int64                 `json:"source_start_id"`
	SourceEndID    int64                 `json:"source_end_id"`
	ItemCount      int                   `json:"item_count"`
	CreatedAt      time.Time             `json:"created_at"`
}

func (r *TimelineArchiveRef) String() string {
	if r == nil {
		return ""
	}
	parts := []string{
		fmt.Sprintf("archive_id=%s", r.ArchiveID),
		fmt.Sprintf("reason=%s", r.Reason),
		fmt.Sprintf("range=%d-%d", r.SourceStartID, r.SourceEndID),
		fmt.Sprintf("items=%d", r.ItemCount),
	}
	if preview := strings.TrimSpace(r.SummaryPreview); preview != "" {
		parts = append(parts, fmt.Sprintf("summary=%s", preview))
	}
	return strings.Join(parts, " ")
}

type TimelineArchiveSearchQuery struct {
	Query                 string
	BytesLimit            int
	DisableSemanticSearch bool
}

type TimelineArchiveSearchResult struct {
	ArchiveRefs    []*TimelineArchiveRef
	TotalContent   string
	ContentBytes   int
	SearchSummary  string
	SelectedMemory []*MemoryEntity
}

// Deprecated: retained for legacy configuration compatibility. The runtime no longer
// invokes this interface; historical archive data can still be inspected and deleted.
type TimelineArchiveStore interface {
	ArchiveCompressedBatch(ctx context.Context, batch *TimelineArchiveBatch) (*TimelineArchiveRef, error)
	SearchArchivedBatches(ctx context.Context, query *TimelineArchiveSearchQuery) (*TimelineArchiveSearchResult, error)
}
