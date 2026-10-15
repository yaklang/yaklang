package pcapdb

import (
	"context"
	"errors"
	"fmt"
	"github.com/yaklang/gorm"
	"sort"
	"strings"
)

type ProtocolFieldIndex struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// Index definitions are authoritative in the child sqlite_schema, including
// indexes created by an older process. No field registry is added to the profile.
func protocolFieldIndexes(ctx context.Context, db *gorm.DB) ([]ProtocolFieldIndex, error) {
	var rows []struct {
		Name string
		SQL  string
	}
	err := indexWithContext(ctx, db).Table("sqlite_schema").Select("name,sql").Where("type = ? AND tbl_name = ? AND substr(name,1,?) = ?", "index", "protocol_messages", len(protocolFieldIndexPrefix), protocolFieldIndexPrefix).Order("name").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	indexes := make([]ProtocolFieldIndex, 0, len(rows))
	for _, row := range rows {
		const marker = "json_extract(fields, '"
		start := strings.Index(row.SQL, marker)
		if start < 0 {
			return nil, fmt.Errorf("pcapdb: invalid field index definition: %s", row.Name)
		}
		tail := row.SQL[start+len(marker):]
		var path strings.Builder
		end := false
		for i := 0; i < len(tail); i++ {
			if tail[i] == '\'' {
				if i+1 < len(tail) && tail[i+1] == '\'' {
					path.WriteByte('\'')
					i++
					continue
				}
				end = true
				break
			}
			path.WriteByte(tail[i])
		}
		value := path.String()
		if !end || validateProtocolFieldPath(value) != nil || protocolFieldIndexName(value) != row.Name {
			return nil, fmt.Errorf("pcapdb: invalid field index path: %s", row.Name)
		}
		indexes = append(indexes, ProtocolFieldIndex{Name: row.Name, Path: value})
	}
	sort.Slice(indexes, func(i, j int) bool { return indexes[i].Path < indexes[j].Path })
	return indexes, nil
}

func (d *Database) ProtocolFieldIndexes(options ...QueryOption) ([]ProtocolFieldIndex, error) {
	c, err := parseDetailQuery(options)
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.operationContext(d.manager)
	defer cancel()
	if err := d.lockRead(ctx); err != nil {
		return nil, err
	}
	defer d.mu.RUnlock()
	if d.closed {
		return nil, ErrClosed
	}
	tx, _, err := d.readSnapshot(ctx, true, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	return protocolFieldIndexes(ctx, tx)
}

// EnsureProtocolFieldIndexes adds only schema indexes under the existing writer
// lock. It needs neither the source capture nor an analysis replay. Ready rows,
// hashes and checkpoint counts stay intact on success, cancellation and failure.
func (m *InstanceManager) EnsureProtocolFieldIndexes(ctx context.Context, identifier string, paths ...string) ([]ProtocolFieldIndex, error) {
	if err := m.begin(); err != nil {
		return nil, err
	}
	defer m.ops.Done()
	if len(paths) == 0 || len(paths) > maxProtocolFieldIndexes {
		return nil, fmt.Errorf("pcapdb: supply 1..%d field paths", maxProtocolFieldIndexes)
	}
	for _, path := range paths {
		if err := validateProtocolFieldPath(path); err != nil {
			return nil, err
		}
	}
	ctx, cancel := m.operationContext(ctx)
	defer cancel()
	catalog, err := m.resolveContext(ctx, identifier)
	if err != nil {
		return nil, err
	}
	lock, err := acquireFileLock(ctx, datasetLockPath(catalog), true)
	if err != nil {
		return nil, err
	}
	defer releaseFileLock(lock)
	writer, err := openIndex(catalog.DatabasePath, true, false)
	if err != nil {
		return nil, err
	}
	defer writer.Close()
	meta, err := readManifest(ctx, writer, catalog.DatasetID)
	if err != nil {
		return nil, err
	}
	if meta.State != StateReady || !meta.ProtocolsIndexed {
		return nil, ErrNotReady
	}
	if err = ensureProtocolFieldIndexes(ctx, writer, paths); err != nil {
		return nil, err
	}
	if err = checkpointIndex(ctx, writer); err != nil {
		return nil, err
	}
	if err = m.refreshReader(ctx, meta); err != nil {
		return nil, err
	}
	return protocolFieldIndexes(ctx, writer)
}

type ProtocolField struct {
	Path             string   `json:"path"`
	Types            []string `json:"types"`
	Occurrences      int      `json:"occurrences"`
	ExampleMessageID uint     `json:"example_message_id"`
	Searchable       bool     `json:"searchable"`
	Indexed          bool     `json:"indexed"`
}

const discoveryNodes = 4096

// DiscoverProtocolFields samples bounded message pages directly from JSONB.
// Counts apply to this page only. NextCursor is a message ID, not a field ID.
// A huge/deep message or an output-truncated field set is explicitly reported,
// never presented as an exhaustive schema. No values or full trees are loaded.
func (d *Database) DiscoverProtocolFields(options ...QueryOption) (*ResultPage[ProtocolField], error) {
	page := emptyPage[ProtocolField](d.ID, 0, "message_id")
	page.Sampled = true
	c, err := parseQuery(options, true)
	if err != nil {
		return failPage(page, err)
	}
	page.NextCursor = c.after
	page.requestCursor = c.after
	if c.includeFields {
		return failPage(page, errors.New("pcapdb: field discovery does not project complete JSON trees"))
	}
	ctx, cancel := c.operationContext(d.manager)
	defer cancel()
	if err := d.lockRead(ctx); err != nil {
		return failPage(page, err)
	}
	defer d.mu.RUnlock()
	if d.closed {
		return failPage(page, ErrClosed)
	}
	tx, _, err := d.readSnapshot(ctx, true, c.session != nil || c.stream != nil)
	if err != nil {
		return failPage(page, err)
	}
	defer tx.Rollback()
	scoped := indexWithContext(ctx, tx)
	indexes, err := protocolFieldIndexes(ctx, tx)
	if err != nil {
		return failPage(page, err)
	}
	indexed := make(map[string]bool, len(indexes))
	for _, index := range indexes {
		indexed[index.Path] = true
	}
	query := c.apply(scoped).Model(&PCAPProtocolMessage{}).Select("id,length(fields) AS size").Limit(c.limit + 1)
	if c.session != nil {
		query = query.Where("session_id = ?", *c.session)
	}
	if c.stream != nil {
		query = query.Where("stream_id = ?", *c.stream)
	}
	var candidates []struct {
		ID   uint
		Size int
	}
	if err = query.Scan(&candidates).Error; err != nil {
		return failPage(page, err)
	}
	found := make(map[string]*ProtocolField)
	nodes, sampledBytes := 0, 0
	for i, message := range candidates {
		if err = ctx.Err(); err != nil {
			return failPage(page, err)
		}
		if i >= c.limit {
			page.HasMore = true
			break
		}
		if message.Size > 8<<20 {
			page.Scanned++
			page.Skipped++
			page.Truncated = true
			page.NextCursor = int64(message.ID)
			continue
		}
		if sampledBytes+message.Size > 16<<20 {
			page.HasMore = true
			break
		}
		sampledBytes += message.Size
		var fields []struct {
			Path string
			Type string
		}
		// CROSS JOIN expands one primary-key message only. substr caps unusual keys;
		// invalid/overlong paths are marked non-searchable rather than used in SQL.
		err = scoped.Table("protocol_messages AS m").Joins("CROSS JOIN json_tree(m.fields) AS j").Select("substr(j.fullkey,1,1025) AS path,j.type").Where("m.id = ? AND m.deleted_at IS NULL AND j.fullkey != '$'", message.ID).Limit(discoveryNodes + 1).Scan(&fields).Error
		if err != nil {
			return failPage(page, err)
		}
		if len(fields) > discoveryNodes {
			page.Scanned++
			page.Skipped++
			page.Truncated = true
			page.NextCursor = int64(message.ID)
			continue
		}
		if nodes+len(fields) > discoveryNodes {
			page.HasMore = true
			break
		}
		nodes += len(fields)
		page.Scanned++
		page.NextCursor = int64(message.ID)
		for _, field := range fields {
			key := field.Path
			if len(key) > 1024 {
				page.Truncated = true
			}
			entry := found[key]
			if entry == nil {
				entry = &ProtocolField{Path: key, Types: []string{}, ExampleMessageID: message.ID, Searchable: validateProtocolFieldPath(key) == nil, Indexed: indexed[key]}
				found[key] = entry
			}
			entry.Occurrences++
			exists := false
			for _, kind := range entry.Types {
				if kind == field.Type {
					exists = true
					break
				}
			}
			if !exists {
				entry.Types = append(entry.Types, field.Type)
				sort.Strings(entry.Types)
			}
		}
	}
	page.State = StateReady
	keys := make([]string, 0, len(found))
	for key := range found {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	// Field output is sampled independently from message pagination. If its
	// budget is exhausted, report Truncated without pretending a field cursor can
	// resume it. A smaller message limit gives a more complete local schema.
	envelope, _ := jsonMarshalSize(page)
	used := envelope + 32
	for _, key := range keys {
		item := *found[key]
		size, err := jsonMarshalSize(item)
		if err != nil {
			return failPage(page, err)
		}
		if used+size+1 > c.resultBytes {
			page.Truncated = true
			break
		}
		page.Items = append(page.Items, item)
		used += size + 1
	}
	return page, nil
}
