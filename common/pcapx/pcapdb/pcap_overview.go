package pcapdb

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/yaklang/gorm"
)

const (
	maxOverviewProtocols  = 256
	maxOverviewStatuses   = 32
	maxOverviewTransports = 32
)

type ProtocolOverviewCount struct {
	Protocol string `json:"protocol"`
	Count    int64  `json:"count"`
}

type ProtocolStatusOverviewCount struct {
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

type TransportOverviewCount struct {
	Transport string `json:"transport"`
	Count     int64  `json:"count"`
}

// CaptureOverview is an exact, bounded inventory of the ready capture. The
// manifest remains the source of total counters; this structure only exposes
// the distributions needed to choose a drill-down query.
type CaptureOverview struct {
	FirstTimestampNS    *int64                        `json:"first_timestamp_ns,omitempty"`
	LastTimestampNS     *int64                        `json:"last_timestamp_ns,omitempty"`
	ProtocolKindCount   int64                         `json:"protocol_kind_count"`
	Protocols           []ProtocolOverviewCount       `json:"protocols"`
	Statuses            []ProtocolStatusOverviewCount `json:"statuses"`
	Transports          []TransportOverviewCount      `json:"transports"`
	LinkTypes           []int                         `json:"link_types"`
	SectionCount        int64                         `json:"section_count"`
	InterfaceCount      int64                         `json:"interface_count"`
	ProtocolsTruncated  bool                          `json:"protocols_truncated"`
	StatusesTruncated   bool                          `json:"statuses_truncated"`
	TransportsTruncated bool                          `json:"transports_truncated"`
}

type overviewCountRow struct {
	Name  string
	Count int64
}

func parseOverviewQuery(options []QueryOption) (*queryConfig, error) {
	c, err := parseQuery(options, false)
	if err != nil {
		return nil, err
	}
	if c.transport != "" || c.protocol != "" || c.sourceIP != "" || c.destinationIP != "" ||
		c.sourceMAC != "" || c.sourcePort != nil || c.destinationPort != nil ||
		c.flow != nil || c.session != nil || c.stream != nil || c.start != nil ||
		len(c.fields) > 0 {
		return nil, fmt.Errorf("pcapdb: capture overview accepts only context/output options")
	}
	return c, nil
}

func overviewCounts(ctx context.Context, tx *gorm.DB, column string, limit int) ([]overviewCountRow, bool, error) {
	rows, err := indexWithContext(ctx, tx).Table("protocol_messages").
		Select(column + " AS name,COUNT(*) AS count").
		Where("deleted_at IS NULL").
		Group(column).
		Order("count DESC," + column + " ASC").
		Limit(limit + 1).
		Rows()
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]overviewCountRow, 0, limit+1)
	for rows.Next() {
		var item overviewCountRow
		if err = rows.Scan(&item.Name, &item.Count); err != nil {
			return nil, false, err
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	truncated := len(items) > limit
	if truncated {
		items = items[:limit]
	}
	return items, truncated, nil
}

func overviewLabel(value, unknown string) string {
	if value == "" {
		return unknown
	}
	return value
}

// CaptureOverview computes exact protocol/status/transport distributions from
// the indexed protocol messages. It never returns message rows or raw payloads.
func (d *Database) CaptureOverview(options ...QueryOption) (*CaptureOverview, error) {
	c, err := parseOverviewQuery(options)
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.operationContext(d.manager)
	defer cancel()
	if err = d.lockRead(ctx); err != nil {
		return nil, err
	}
	defer d.mu.RUnlock()
	if d.closed {
		return nil, ErrClosed
	}
	tx, meta, err := d.readSnapshot(ctx, false, false)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result := &CaptureOverview{
		Protocols:  make([]ProtocolOverviewCount, 0),
		Statuses:   make([]ProtocolStatusOverviewCount, 0),
		Transports: make([]TransportOverviewCount, 0),
		LinkTypes:  make([]int, 0),
	}
	var first, last sql.NullInt64
	if err = indexWithContext(ctx, tx).Model(&Packet{}).
		Select("MIN(timestamp_ns),MAX(timestamp_ns)").
		Row().Scan(&first, &last); err != nil {
		return nil, err
	}
	if first.Valid {
		result.FirstTimestampNS = &first.Int64
	}
	if last.Valid {
		result.LastTimestampNS = &last.Int64
	}
	linkRows, err := indexWithContext(ctx, tx).Table("capture_interfaces").
		Select("DISTINCT link_type").
		Where("deleted_at IS NULL").
		Order("link_type").
		Rows()
	if err != nil {
		return nil, err
	}
	for linkRows.Next() {
		var linkType int
		if err = linkRows.Scan(&linkType); err != nil {
			linkRows.Close()
			return nil, err
		}
		result.LinkTypes = append(result.LinkTypes, linkType)
	}
	if err = linkRows.Err(); err != nil {
		linkRows.Close()
		return nil, err
	}
	if err = linkRows.Close(); err != nil {
		return nil, err
	}
	if err = indexWithContext(ctx, tx).Model(&PCAPCaptureInterface{}).
		Count(&result.InterfaceCount).Error; err != nil {
		return nil, err
	}
	if err = indexWithContext(ctx, tx).Table("capture_interfaces").
		Select("COUNT(DISTINCT section)").
		Where("deleted_at IS NULL").
		Row().Scan(&result.SectionCount); err != nil {
		return nil, err
	}
	if !meta.ProtocolsIndexed {
		return result, nil
	}
	protocols, protocolsTruncated, err := overviewCounts(ctx, tx, "protocol", maxOverviewProtocols)
	if err != nil {
		return nil, err
	}
	for _, item := range protocols {
		result.Protocols = append(result.Protocols, ProtocolOverviewCount{
			Protocol: overviewLabel(item.Name, "(unparsed)"),
			Count:    item.Count,
		})
	}
	result.ProtocolsTruncated = protocolsTruncated
	statuses, statusesTruncated, err := overviewCounts(ctx, tx, "status", maxOverviewStatuses)
	if err != nil {
		return nil, err
	}
	for _, item := range statuses {
		result.Statuses = append(result.Statuses, ProtocolStatusOverviewCount{
			Status: overviewLabel(item.Name, "(unknown)"),
			Count:  item.Count,
		})
	}
	result.StatusesTruncated = statusesTruncated
	transports, transportsTruncated, err := overviewCounts(ctx, tx, "transport", maxOverviewTransports)
	if err != nil {
		return nil, err
	}
	for _, item := range transports {
		result.Transports = append(result.Transports, TransportOverviewCount{
			Transport: overviewLabel(item.Name, "(unknown)"),
			Count:     item.Count,
		})
	}
	result.TransportsTruncated = transportsTruncated
	if err = indexWithContext(ctx, tx).Table("protocol_messages").
		Where("deleted_at IS NULL").
		Select("COUNT(DISTINCT protocol)").
		Row().Scan(&result.ProtocolKindCount); err != nil {
		return nil, err
	}
	return result, nil
}
