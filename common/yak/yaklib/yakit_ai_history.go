package yaklib

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type aiQueryConfig struct {
	ctx                                        context.Context
	projectDB, profileDB                       *gorm.DB
	snapshotSession                            string
	snapshot                                   func() (string, error)
	sessionID, mode, view                      string
	projectID, itemID                          int64
	limit, offset, contentOffset, contentLimit int
}
type AIQueryOption func(*aiQueryConfig)

func WithAITimelineView(view string) AIQueryOption { return func(c *aiQueryConfig) { c.view = view } }

func WithAIQueryContext(ctx context.Context) AIQueryOption {
	return func(c *aiQueryConfig) { c.ctx = ctx }
}
func WithAIQueryDatabases(project, profile *gorm.DB) AIQueryOption {
	return func(c *aiQueryConfig) { c.projectDB, c.profileDB = project, profile }
}
func WithAITimelineSnapshot(session string, snapshot func() (string, error)) AIQueryOption {
	return func(c *aiQueryConfig) { c.snapshotSession, c.snapshot = session, snapshot }
}
func WithAISession(id string) AIQueryOption      { return func(c *aiQueryConfig) { c.sessionID = id } }
func WithAIProject(id int64) AIQueryOption       { return func(c *aiQueryConfig) { c.projectID = id } }
func WithAISearchMode(mode string) AIQueryOption { return func(c *aiQueryConfig) { c.mode = mode } }
func WithAIQueryLimit(n int) AIQueryOption       { return func(c *aiQueryConfig) { c.limit = n } }
func WithAIQueryOffset(n int) AIQueryOption      { return func(c *aiQueryConfig) { c.offset = n } }
func WithAITimelineItem(id int64) AIQueryOption  { return func(c *aiQueryConfig) { c.itemID = id } }
func WithAIContentOffset(n int) AIQueryOption    { return func(c *aiQueryConfig) { c.contentOffset = n } }
func WithAIContentLimit(n int) AIQueryOption     { return func(c *aiQueryConfig) { c.contentLimit = n } }
func aiQueryOptions(opts []AIQueryOption) (*aiQueryConfig, error) {
	c := &aiQueryConfig{ctx: context.Background(), mode: "bm25", view: "content", limit: 10, contentLimit: 2048}
	for _, opt := range opts {
		opt(c)
	}
	if c.ctx == nil {
		return nil, fmt.Errorf("query context is nil")
	}
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	if c.limit < 1 || c.limit > 50 || c.offset < 0 || c.offset > 100000 || c.projectID < 0 || c.itemID < 0 || c.contentOffset < 0 || c.contentLimit < 1 || c.contentLimit > 8192 || c.limit*c.contentLimit > 65536 {
		return nil, fmt.Errorf("invalid query bounds: limit 1..50, offset 0..100000, content_limit 1..8192, total content budget <=65536; IDs/offsets must be nonnegative")
	}
	if c.projectDB == nil {
		c.projectDB = consts.GetGormProjectDatabase()
	}
	if c.profileDB == nil {
		c.profileDB = consts.GetGormProfileDatabase()
	}
	return c, nil
}

func init() {
	for k, v := range map[string]any{
		"GrepAITimelineHistory": GrepAITimelineHistory, "QueryYakProjects": QueryYakProjects, "QueryHTTPHistory": QueryHTTPHistory,
		"CurrentAISession": func() string { return "" }, "aiSession": WithAISession, "aiProject": WithAIProject, "aiSearchMode": WithAISearchMode, "aiQueryLimit": WithAIQueryLimit, "aiQueryOffset": WithAIQueryOffset, "aiTimelineItem": WithAITimelineItem, "aiTimelineView": WithAITimelineView, "aiContentOffset": WithAIContentOffset, "aiContentLimit": WithAIContentLimit,
	} {
		YakitExports[k] = v
	}
}

// GrepAITimelineHistory searches durable original Timeline entries. sessionID
// must be supplied by the host (AI tools bind it automatically).
func GrepAITimelineHistory(query string, opts ...AIQueryOption) (map[string]any, error) {
	c, err := aiQueryOptions(opts)
	if err != nil {
		return nil, err
	}
	if c.projectDB == nil {
		return nil, fmt.Errorf("project database is unavailable")
	}
	if c.view != "content" && c.view != "value_json" {
		return nil, fmt.Errorf("timeline view must be content or value_json")
	}
	// Backfill still-existing entries for pre-upgrade sessions. Already discarded
	// originals cannot be reconstructed from a summary.
	if c.sessionID == "" {
		return nil, fmt.Errorf("ai session ID is required")
	}
	if c.projectDB.HasTable(&schema.AIAgentRuntime{}) {
		runtime, err := yakit.GetLatestAIAgentRuntimeByPersistentSession(c.projectDB, c.sessionID)
		if err != nil && !gorm.IsRecordNotFoundError(err) {
			return nil, err
		}
		if runtime != nil && runtime.QuotedTimeline != "" {
			if err := yakit.EnsureAITimelineHistory(c.projectDB, false); err != nil {
				return nil, err
			}
			if err := c.projectDB.Transaction(func(tx *gorm.DB) error {
				return yakit.ArchiveAITimelineSnapshot(tx, c.sessionID, runtime.GetTimeline())
			}); err != nil {
				return nil, err
			}
		}
	}
	if c.snapshot != nil && c.snapshotSession == c.sessionID {
		raw, err := c.snapshot()
		if err != nil {
			return nil, err
		}
		if raw != "" {
			if err := yakit.EnsureAITimelineHistory(c.projectDB, false); err != nil {
				return nil, err
			}
			if err := c.projectDB.Transaction(func(tx *gorm.DB) error { return yakit.ArchiveAITimelineSnapshot(tx, c.sessionID, raw) }); err != nil {
				return nil, err
			}
		}
	}
	rows, err := yakit.GrepAITimelineHistory(c.ctx, c.projectDB, query, yakit.AITimelineHistoryQuery{SessionID: c.sessionID, Mode: c.mode, Limit: c.limit, Offset: c.offset, ItemID: c.itemID})
	if err != nil {
		return nil, err
	}
	hits := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		source := row.Content
		if c.view == "value_json" {
			source = row.ValueJSON
		}
		content, next, truncated := aiContentSlice(source, c.contentOffset, c.contentLimit)
		hits = append(hits, map[string]any{"item_id": row.ID, "timeline_item_id": row.ItemID, "type": row.Type, "timestamp": row.Timestamp, "content": content, "view": c.view, "content_offset": c.contentOffset, "next_content_offset": next, "truncated": truncated})
	}
	return map[string]any{"session_id": c.sessionID, "search_mode": c.mode, "literal_fallback": c.mode == "bm25" && strings.TrimSpace(query) != "" && !yakit.AITimelineBM25UsesFTS(query), "hits": hits, "offset": c.offset, "next_offset": c.offset + len(rows)}, nil
}

func aiContentSlice(s string, offset, limit int) (string, int, bool) {
	runes := []rune(s)
	if offset >= len(runes) {
		return "", len(runes), false
	}
	end := offset + limit
	if end > len(runes) {
		end = len(runes)
	}
	return string(runes[offset:end]), end, end < len(runes)
}

// QueryYakProjects lists real project databases across folders from the profile
// registry located through the existing YAKIT_HOME/default home infrastructure.
func QueryYakProjects(keyword string, opts ...AIQueryOption) (map[string]any, error) {
	// Project listings have no content chunks. Do not let the default 2048
	// character content budget reject the advertised 50 metadata rows.
	c, err := aiQueryOptions(append([]AIQueryOption{WithAIContentLimit(1)}, opts...))
	if err != nil {
		return nil, err
	}
	if c.profileDB == nil {
		return nil, fmt.Errorf("profile database is unavailable")
	}
	q := c.profileDB.Model(&schema.Project{}).Where("(type = ? OR type IS NULL OR type = '') AND database_path <> ''", "project")
	if keyword != "" {
		q = q.Where("project_name LIKE ?", "%"+keyword+"%")
	}
	var total int
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []*schema.Project
	if err := q.Order("updated_at DESC, id DESC").Limit(c.limit).Offset(c.offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	hits := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		hits = append(hits, map[string]any{"project_id": row.ID, "name": row.ProjectName, "description": row.Description, "database_path": row.DatabasePath, "is_current": row.IsCurrentProject, "folder_id": row.FolderID, "child_folder_id": row.ChildFolderID, "updated_at": row.UpdatedAt})
	}
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"yakit_home": consts.GetDefaultYakitBaseDir(), "projects": hits, "total": total, "offset": c.offset, "next_offset": c.offset + len(rows)}, nil
}

func aiHTTPDatabase(c *aiQueryConfig) (*gorm.DB, func(), error) {
	if c.projectID == 0 {
		if c.projectDB == nil {
			return nil, nil, fmt.Errorf("project database is unavailable")
		}
		return c.projectDB, func() {}, nil
	}
	if c.profileDB == nil {
		return nil, nil, fmt.Errorf("profile database is unavailable")
	}
	project, err := yakit.GetProjectByID(c.profileDB, c.projectID)
	if err != nil {
		return nil, nil, err
	}
	if project.Type != "" && project.Type != "project" {
		return nil, nil, fmt.Errorf("ID %d is not a database project", c.projectID)
	}
	if project.DatabasePath == "" {
		return nil, nil, fmt.Errorf("project has no database path")
	}
	path := project.DatabasePath
	if !filepath.IsAbs(path) {
		path = filepath.Join(consts.GetDefaultYakitBaseDir(), path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("project database is not a regular file")
	}
	uri := (&url.URL{Scheme: "file", Path: path}).String() + "?mode=ro&_query_only=1&_busy_timeout=10000"
	db, err := gorm.Open("sqlite3", uri)
	if err != nil {
		return nil, nil, err
	}
	db.DB().SetMaxOpenConns(1)
	return db, func() { db.Close() }, nil
}

// QueryHTTPHistory reuses Yakit's full HTTP filter vocabulary. The filter uses
// QueryHTTPFlowRequest field names; unknown fields fail instead of being ignored.
// Pagination and packet projection are always bounded by query options.
func QueryHTTPHistory(filterJSON string, opts ...AIQueryOption) (map[string]any, error) {
	c, err := aiQueryOptions(opts)
	if err != nil {
		return nil, err
	}
	req := &ypb.QueryHTTPFlowRequest{}
	if strings.TrimSpace(filterJSON) != "" {
		if !strings.HasPrefix(strings.TrimSpace(filterJSON), "{") {
			return nil, fmt.Errorf("HTTP history filter must be a JSON object")
		}
		if len(filterJSON) > 65536 {
			return nil, fmt.Errorf("filter JSON exceeds 64 KiB")
		}
		dec := json.NewDecoder(strings.NewReader(filterJSON))
		dec.DisallowUnknownFields()
		if err := dec.Decode(req); err != nil {
			return nil, fmt.Errorf("invalid HTTP history filter: %w", err)
		}
		var trailing any
		if err := dec.Decode(&trailing); err != io.EOF {
			return nil, fmt.Errorf("HTTP history filter must contain exactly one JSON object")
		}
	}
	req.Pagination = &ypb.Paging{Page: 1, Limit: int64(c.limit), OrderBy: "id", Order: "desc"}
	if len(req.IncludeId) > 0 && c.limit*c.contentLimit*2 > 65536 {
		return nil, fmt.Errorf("HTTP packet content budget must be <=65536 characters")
	}
	req.Full = false
	req.ExcludeRequestRaw, req.ExcludeResponseRaw = true, true
	db, closeDB, err := aiHTTPDatabase(c)
	if err != nil {
		return nil, err
	}
	defer closeDB()
	query := yakit.FilterHTTPFlow(db, req)
	var total int
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []*schema.HTTPFlow
	// Select only metadata until the caller requests packet content explicitly.
	// Raw packets in the DB are quoted strings; slicing happens after decoding.
	fields := "id, created_at, url, method, status_code, content_type, body_length, tags, source_type, runtime_id, from_plugin"
	if len(req.IncludeId) > 0 {
		fields += ", request, response"
	}
	if err := query.Select(fields).Order("id DESC").Offset(c.offset).Limit(c.limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	hits := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		hit := map[string]any{"flow_id": row.ID, "url": row.Url, "method": row.Method, "status_code": row.StatusCode, "content_type": row.ContentType, "body_length": row.BodyLength, "tags": row.Tags, "source_type": row.SourceType, "runtime_id": row.RuntimeId, "from_plugin": row.FromPlugin, "created_at": row.CreatedAt}
		if len(req.IncludeId) > 0 {
			request, nextReq, reqTruncated := aiContentSlice(string(row.GetRequest()), c.contentOffset, c.contentLimit)
			response, nextRsp, rspTruncated := aiContentSlice(string(row.GetResponse()), c.contentOffset, c.contentLimit)
			hit["request"], hit["response"] = request, response
			hit["request_next_offset"], hit["response_next_offset"] = nextReq, nextRsp
			hit["request_truncated"], hit["response_truncated"] = reqTruncated, rspTruncated
		}
		hits = append(hits, hit)
	}
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	return map[string]any{"project_id": c.projectID, "hits": hits, "total": total, "offset": c.offset, "next_offset": c.offset + len(rows)}, nil
}
