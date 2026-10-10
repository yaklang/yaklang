package pcapdb

import "context"

// CatalogSummary is an overview, not a copy of potentially large source aliases
// or parser trees. DatasetID is the stable reference for subsequent operations.
type CatalogSummary struct {
	ID               uint   `json:"id"`
	DatasetID        string `json:"dataset_id"`
	State            State  `json:"state"`
	SourcePath       string `json:"source_path"`
	DatabasePath     string `json:"database_path"`
	Format           string `json:"format"`
	SourceSize       int64  `json:"source_size"`
	PacketCount      int64  `json:"packet_count"`
	ProtocolCount    int64  `json:"protocol_count"`
	SessionCount     int64  `json:"session_count"`
	StreamCount      int64  `json:"stream_count"`
	ProtocolsIndexed bool   `json:"protocols_indexed"`
	StreamsIndexed   bool   `json:"streams_indexed"`
	LastError        string `json:"last_error,omitempty"`
	PreviewTruncated bool   `json:"preview_truncated,omitempty"`
}

func (m *InstanceManager) ListPage(ctx context.Context, options ...QueryOption) (*ResultPage[CatalogSummary], error) {
	page := emptyPage[CatalogSummary]("", 0, "catalog_id")
	c, err := parseDetailQueryWithoutCursor(options)
	if err != nil {
		return failPage(page, err)
	}
	page.NextCursor = c.after
	page.requestCursor = c.after
	if err = m.begin(); err != nil {
		return failPage(page, err)
	}
	defer m.ops.Done()
	c.parent = ctx
	queryCtx, cancel := c.operationContext(m)
	defer cancel()
	var rows []PCAPFileDBMetadata
	projection := "id,dataset_id,state,substr(source_path,1,513) AS source_path,substr(database_path,1,513) AS database_path,format,source_size,packet_count,protocol_count,session_count,stream_count,protocols_indexed,streams_indexed,substr(last_error,1,513) AS last_error"
	err = indexWithContext(queryCtx, m.profile).Select(projection).Where("id > ?", c.after).Order("id").Limit(c.limit + 1).Find(&rows).Error
	if err != nil {
		return failPage(page, err)
	}
	items := make([]CatalogSummary, 0, len(rows))
	for _, row := range rows {
		items = append(items, CatalogSummary{ID: row.ID, DatasetID: row.DatasetID, State: row.State, SourcePath: shortText(row.SourcePath, 512), DatabasePath: shortText(row.DatabasePath, 512), Format: row.Format, SourceSize: row.SourceSize, PacketCount: row.PacketCount, ProtocolCount: row.ProtocolCount, SessionCount: row.SessionCount, StreamCount: row.StreamCount, ProtocolsIndexed: row.ProtocolsIndexed, StreamsIndexed: row.StreamsIndexed, LastError: shortText(row.LastError, 512), PreviewTruncated: len(row.SourcePath) > 512 || len(row.DatabasePath) > 512 || len(row.LastError) > 512})
	}
	page.State = StateReady
	return pageItems(page, items, c.limit, c.resultBytes, func(p CatalogSummary) int64 { return int64(p.ID) })
}

func parseDetailQueryWithoutCursor(options []QueryOption) (*queryConfig, error) {
	c, err := parseQuery(options, false)
	if err != nil {
		return nil, err
	}
	saved := c.after
	check := append(append([]QueryOption(nil), options...), func(c *queryConfig) error { c.after = 0; return nil })
	if _, err = parseDetailQuery(check); err != nil {
		return nil, err
	}
	c.after = saved
	return c, nil
}

func (m *InstanceManager) ListContext(ctx context.Context) ([]PCAPFileDBMetadata, error) {
	if err := m.begin(); err != nil {
		return nil, err
	}
	defer m.ops.Done()
	ctx, cancel := m.operationContext(ctx)
	defer cancel()
	var rows []PCAPFileDBMetadata
	err := indexWithContext(ctx, m.profile).Order("created_at DESC, dataset_id ASC").Find(&rows).Error
	return rows, err
}
func (m *InstanceManager) CountContext(ctx context.Context) (int64, error) {
	if err := m.begin(); err != nil {
		return 0, err
	}
	defer m.ops.Done()
	ctx, cancel := m.operationContext(ctx)
	defer cancel()
	var count int64
	err := indexWithContext(ctx, m.profile).Model(&PCAPFileDBMetadata{}).Count(&count).Error
	return count, err
}
