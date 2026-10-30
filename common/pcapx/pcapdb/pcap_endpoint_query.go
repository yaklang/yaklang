package pcapdb

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/yaklang/gorm"
)

// MACSummary is a bounded, source-observed MAC inventory. SourceIPCount exposes
// when one link-layer address was observed with many network addresses, which
// commonly identifies a gateway/NAT boundary but is not itself that inference.
type MACSummary struct {
	ID               uint   `json:"id"`
	Section          int64  `json:"section"`
	Interface        int    `json:"interface"`
	LinkType         int    `json:"link_type"`
	VLANStack        string `json:"vlan_stack,omitempty"`
	SourceMAC        string `json:"source_mac"`
	SourceIPCount    int64  `json:"source_ip_count"`
	PacketCount      int64  `json:"packet_count"`
	FirstTimestampNS *int64 `json:"first_timestamp_ns,omitempty"`
	LastTimestampNS  *int64 `json:"last_timestamp_ns,omitempty"`
	FirstPacketID    uint   `json:"first_packet_id"`
	LastPacketID     uint   `json:"last_packet_id"`
}

// EndpointBindingSummary is one time-bounded source MAC/IP observation edge.
// It does not claim ownership after routing or NAT.
type EndpointBindingSummary struct {
	ID               uint   `json:"id"`
	Section          int64  `json:"section"`
	Interface        int    `json:"interface"`
	LinkType         int    `json:"link_type"`
	VLANStack        string `json:"vlan_stack,omitempty"`
	SourceMAC        string `json:"source_mac"`
	SourceIP         string `json:"source_ip"`
	PacketCount      int64  `json:"packet_count"`
	FirstTimestampNS *int64 `json:"first_timestamp_ns,omitempty"`
	LastTimestampNS  *int64 `json:"last_timestamp_ns,omitempty"`
	FirstPacketID    uint   `json:"first_packet_id"`
	LastPacketID     uint   `json:"last_packet_id"`
}

func parseEndpointQuery(options []QueryOption) (*queryConfig, error) {
	c, err := parseQuery(options, false)
	if err != nil {
		return nil, err
	}
	if c.transport != "" || c.protocol != "" || c.destinationIP != "" ||
		c.sourcePort != nil || c.destinationPort != nil || c.flow != nil ||
		c.session != nil || c.stream != nil || len(c.fields) > 0 {
		return nil, fmt.Errorf("pcapdb: endpoint queries accept only time, source IP, source MAC, cursor and limit filters")
	}
	if uint64(c.after) > uint64(^uint(0)>>1) {
		return nil, fmt.Errorf("pcapdb: endpoint cursor exceeds platform integer range")
	}
	return c, nil
}

func endpointPacketQuery(ctx context.Context, tx *gorm.DB, c *queryConfig) *gorm.DB {
	query := indexWithContext(ctx, tx).Table("packets").
		Where("deleted_at IS NULL AND src_mac <> '' AND src_ip <> ''")
	if c.sourceIP != "" {
		query = query.Where("src_ip = ?", c.sourceIP)
	}
	if c.sourceMAC != "" {
		query = query.Where("src_mac = ?", c.sourceMAC)
	}
	if c.start != nil {
		query = query.Where("timestamp_ns >= ? AND timestamp_ns < ?", *c.start, *c.end)
	}
	return query
}

// QueryMACSummariesPage groups source-observed MACs before expanding their IPs.
// It lets callers triage a small number of link-layer endpoints without paging
// every MAC/IP pair or decoding packet BLOBs.
func (d *Database) QueryMACSummariesPage(options ...QueryOption) (*ResultPage[MACSummary], error) {
	c, err := parseEndpointQuery(options)
	page := emptyPage[MACSummary](d.ID, 0, "mac_index")
	if err == nil {
		page = emptyPage[MACSummary](d.ID, c.after, "mac_index")
	}
	if err != nil {
		return failPage(page, err)
	}
	ctx, cancel := c.operationContext(d.manager)
	defer cancel()
	if err = d.lockRead(ctx); err != nil {
		return failPage(page, err)
	}
	defer d.mu.RUnlock()
	if d.closed {
		return failPage(page, ErrClosed)
	}
	tx, _, err := d.readSnapshot(ctx, false, false)
	if err != nil {
		return failPage(page, err)
	}
	defer tx.Rollback()
	rows, err := endpointPacketQuery(ctx, tx, c).
		Select("section,interface_id,link_type,vlan_stack,src_mac,COUNT(*) AS packet_count,COUNT(DISTINCT src_ip) AS source_ip_count,MIN(timestamp_ns) AS first_timestamp_ns,MAX(timestamp_ns) AS last_timestamp_ns,MIN(id) AS first_packet_id,MAX(id) AS last_packet_id").
		Group("section,interface_id,link_type,vlan_stack,src_mac").
		Order("section,interface_id,link_type,vlan_stack,src_mac").
		Offset(int(c.after)).Limit(c.limit + 1).Rows()
	if err != nil {
		return failPage(page, err)
	}
	defer rows.Close()
	items := make([]MACSummary, 0, c.limit+1)
	ordinal := c.after
	for rows.Next() {
		var item MACSummary
		var firstTS, lastTS sql.NullInt64
		if err = rows.Scan(&item.Section, &item.Interface, &item.LinkType, &item.VLANStack, &item.SourceMAC, &item.PacketCount, &item.SourceIPCount, &firstTS, &lastTS, &item.FirstPacketID, &item.LastPacketID); err != nil {
			return failPage(page, err)
		}
		if firstTS.Valid {
			item.FirstTimestampNS = &firstTS.Int64
		}
		if lastTS.Valid {
			item.LastTimestampNS = &lastTS.Int64
		}
		ordinal++
		item.ID = uint(ordinal)
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return failPage(page, err)
	}
	page.State = StateReady
	page.HasMore = len(items) > c.limit
	if page.HasMore {
		items = items[:c.limit]
	}
	return pageItems(page, items, c.limit, c.resultBytes, func(item MACSummary) int64 { return int64(item.ID) })
}

// QueryEndpointBindingsPage returns source-observed MAC/IP edges within the
// selected capture domains and time range. Destination pairs are intentionally
// omitted because their MAC is often the next hop rather than the IP owner.
func (d *Database) QueryEndpointBindingsPage(options ...QueryOption) (*ResultPage[EndpointBindingSummary], error) {
	c, err := parseEndpointQuery(options)
	page := emptyPage[EndpointBindingSummary](d.ID, 0, "binding_index")
	if err == nil {
		page = emptyPage[EndpointBindingSummary](d.ID, c.after, "binding_index")
	}
	if err != nil {
		return failPage(page, err)
	}
	ctx, cancel := c.operationContext(d.manager)
	defer cancel()
	if err = d.lockRead(ctx); err != nil {
		return failPage(page, err)
	}
	defer d.mu.RUnlock()
	if d.closed {
		return failPage(page, ErrClosed)
	}
	tx, _, err := d.readSnapshot(ctx, false, false)
	if err != nil {
		return failPage(page, err)
	}
	defer tx.Rollback()
	rows, err := endpointPacketQuery(ctx, tx, c).
		Select("section,interface_id,link_type,vlan_stack,src_mac,src_ip,COUNT(*) AS packet_count,MIN(timestamp_ns) AS first_timestamp_ns,MAX(timestamp_ns) AS last_timestamp_ns,MIN(id) AS first_packet_id,MAX(id) AS last_packet_id").
		Group("section,interface_id,link_type,vlan_stack,src_mac,src_ip").
		Order("section,interface_id,link_type,vlan_stack,src_mac,src_ip").
		Offset(int(c.after)).Limit(c.limit + 1).Rows()
	if err != nil {
		return failPage(page, err)
	}
	defer rows.Close()
	items := make([]EndpointBindingSummary, 0, c.limit+1)
	ordinal := c.after
	for rows.Next() {
		var item EndpointBindingSummary
		var firstTS, lastTS sql.NullInt64
		if err = rows.Scan(&item.Section, &item.Interface, &item.LinkType, &item.VLANStack, &item.SourceMAC, &item.SourceIP, &item.PacketCount, &firstTS, &lastTS, &item.FirstPacketID, &item.LastPacketID); err != nil {
			return failPage(page, err)
		}
		if firstTS.Valid {
			item.FirstTimestampNS = &firstTS.Int64
		}
		if lastTS.Valid {
			item.LastTimestampNS = &lastTS.Int64
		}
		ordinal++
		item.ID = uint(ordinal)
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return failPage(page, err)
	}
	page.State = StateReady
	page.HasMore = len(items) > c.limit
	if page.HasMore {
		items = items[:c.limit]
	}
	return pageItems(page, items, c.limit, c.resultBytes, func(item EndpointBindingSummary) int64 { return int64(item.ID) })
}
