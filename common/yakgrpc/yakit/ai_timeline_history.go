package yakit

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
)

var timelineHistorySchemaMu sync.Mutex
var timelineHistoryFTS = &bizhelper.SQLiteFTS5Config{BaseModel: &schema.AITimelineHistory{}, FTSTable: "ai_timeline_histories_fts", Columns: []string{"content", "value_json"}, ContentTable: "ai_timeline_histories", Tokenize: "trigram"}

func EnsureAITimelineHistory(db *gorm.DB, withFTS bool) error {
	timelineHistorySchemaMu.Lock()
	defer timelineHistorySchemaMu.Unlock()
	if db == nil {
		return fmt.Errorf("timeline history database is nil")
	}
	if !db.HasTable(&schema.AITimelineHistory{}) {
		if err := db.AutoMigrate(&schema.AITimelineHistory{}).Error; err != nil {
			return err
		}
	}
	if withFTS && !db.HasTable(timelineHistoryFTS.FTSTable) {
		return bizhelper.SQLiteFTS5Setup(db, timelineHistoryFTS)
	}
	return nil
}

// ArchiveAITimelineSnapshot is used inside the same transaction as the runtime
// checkpoint. Keeping full original value JSON also covers tool results which
// have already acquired a smaller prompt projection.
func ArchiveAITimelineSnapshot(db *gorm.DB, sessionID, raw string) error {
	if sessionID == "" {
		return fmt.Errorf("timeline history requires a session ID")
	}
	var snapshot struct {
		Items map[string]struct {
			HistoryID string          `json:"history_id"`
			Deleted   bool            `json:"deleted"`
			CreatedAt time.Time       `json:"created_at"`
			Type      string          `json:"type"`
			Value     json.RawMessage `json:"value"`
		} `json:"id_to_timeline_item"`
	}
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return err
	}
	ids := make([]int64, 0, len(snapshot.Items))
	for key := range snapshot.Items {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	rows := make([]schema.AITimelineHistory, 0, len(ids))
	for _, id := range ids {
		item := snapshot.Items[strconv.FormatInt(id, 10)]
		if item.Deleted {
			continue
		}
		// Decode string values and text entries for readable literal/regexp matching.
		// Other structured values remain intact JSON, including unshrunk tool output.
		content := string(item.Value)
		var text struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(item.Value, &text) == nil && text.Text != "" {
			content = text.Text
		}
		var plain string
		if json.Unmarshal(item.Value, &plain) == nil {
			content = plain
		}
		historyID := item.HistoryID
		if historyID == "" {
			historyID = schema.AITimelineLegacyHistoryID(item.Type, item.CreatedAt, item.Value)
		}
		row := schema.AITimelineHistory{SessionID: sessionID, HistoryID: historyID, ItemID: id, Timestamp: item.CreatedAt, Type: item.Type, Content: content, ValueJSON: string(item.Value), ContentHash: fmt.Sprintf("%x", sha256.Sum256([]byte(item.Type+"/"+content+"/"+string(item.Value))))}
		rows = append(rows, row)
	}
	// Fetch only identities and hashes in bounded batches. Repeated checkpoints
	// should not issue one SELECT per item or reread large archived tool packets.
	for start := 0; start < len(rows); start += 100 {
		batch := rows[start:min(start+100, len(rows))]
		keys := make([]string, 0, len(batch))
		for _, row := range batch {
			keys = append(keys, row.HistoryID)
		}
		var stored []schema.AITimelineHistory
		if err := db.Select("id,history_id,content_hash").Where("session_id = ? AND history_id IN (?)", sessionID, keys).Find(&stored).Error; err != nil {
			return err
		}
		existing := make(map[string]schema.AITimelineHistory, len(stored))
		for _, row := range stored {
			existing[row.HistoryID] = row
		}
		for _, row := range batch {
			if old, ok := existing[row.HistoryID]; ok {
				if old.ContentHash == row.ContentHash {
					continue
				}
				if err := db.Model(&old).Updates(map[string]any{"content": row.Content, "value_json": row.ValueJSON, "content_hash": row.ContentHash, "type": row.Type, "timestamp": row.Timestamp}).Error; err != nil {
					return err
				}
			} else if err := db.Create(&row).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

type AITimelineHistoryQuery struct {
	SessionID     string
	Mode          string
	Limit, Offset int
	ItemID        int64
}

// AITimelineBM25UsesFTS reports whether trigram search can represent at least
// one query term. One/two-character queries need a literal fallback.
func AITimelineBM25UsesFTS(query string) bool {
	for _, term := range strings.Fields(query) {
		if len([]rune(term)) >= 3 {
			return true
		}
	}
	return false
}

// GrepAITimelineHistory searches originals, not frontend stream/event records.
// Regexp scans keyset pages to avoid loading the whole conversation into memory.
func GrepAITimelineHistory(ctx context.Context, db *gorm.DB, query string, c AITimelineHistoryQuery) ([]*schema.AITimelineHistory, error) {
	if ctx == nil {
		return nil, fmt.Errorf("timeline search context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(c.SessionID) == "" {
		return nil, fmt.Errorf("ai session ID is required")
	}
	if c.Limit < 1 || c.Limit > 50 || c.Offset < 0 || c.Offset > 100000 {
		return nil, fmt.Errorf("limit must be 1..50; offset must be 0..100000")
	}
	if len([]rune(query)) > 1024 {
		return nil, fmt.Errorf("query must be at most 1024 characters")
	}
	if c.Mode == "bm25" {
		query = strings.TrimSpace(query)
	}
	if c.Mode != "bm25" && c.Mode != "regexp" && c.Mode != "literal" {
		return nil, fmt.Errorf("search mode must be bm25, regexp or literal")
	}
	if err := EnsureAITimelineHistory(db, c.Mode == "bm25" && AITimelineBM25UsesFTS(query)); err != nil {
		return nil, err
	}
	base := db.Model(&schema.AITimelineHistory{}).Where("session_id = ?", c.SessionID)
	if c.ItemID > 0 {
		base = base.Where("id = ?", c.ItemID)
	}
	if c.Mode == "bm25" && AITimelineBM25UsesFTS(query) {
		rows, err := bizhelper.SQLiteFTS5BM25Match[*schema.AITimelineHistory](base, timelineHistoryFTS, strings.Fields(query), c.Limit, c.Offset)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return rows, err
	}
	hits := []*schema.AITimelineHistory{}
	if c.Mode == "literal" || c.Mode == "bm25" || query == "" {
		if query != "" {
			terms := []string{query}
			if c.Mode == "bm25" {
				terms = strings.Fields(query)
			}
			clauses := []string{}
			args := []any{}
			for _, term := range terms {
				pattern := "%" + strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(term) + "%"
				clauses = append(clauses, "(content LIKE ? ESCAPE '\\' OR value_json LIKE ? ESCAPE '\\')")
				args = append(args, pattern, pattern)
			}
			base = base.Where("("+strings.Join(clauses, " OR ")+")", args...)
		}

		err := base.Order("id DESC").Limit(c.Limit).Offset(c.Offset).Find(&hits).Error
		if err == nil {
			err = ctx.Err()
		}
		return hits, err
	}
	re, err := regexp.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("invalid timeline regexp: %w", err)
	}
	var before int64
	skipped := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page := base.Order("id DESC").Limit(100)
		if before > 0 {
			page = page.Where("id < ?", before)
		}
		var rows []*schema.AITimelineHistory
		if err := page.Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			before = int64(row.ID)
			if !re.MatchString(row.Content) && !re.MatchString(row.ValueJSON) {
				continue
			}
			if skipped < c.Offset {
				skipped++
				continue
			}
			hits = append(hits, row)
			if len(hits) == c.Limit {
				return hits, nil
			}
		}
		if len(rows) < 100 {
			return hits, nil
		}
	}
}
