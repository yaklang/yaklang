package sharkcli

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

const maxStatisticsEntries = 65536

type trafficCount struct {
	Packets uint64    `json:"packets"`
	Bytes   uint64    `json:"bytes"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
}

func (c *trafficCount) add(row packetSummary) {
	c.Packets++
	c.Bytes += uint64(row.CapturedLength)
	if c.Start.IsZero() || row.Time.Before(c.Start) {
		c.Start = row.Time
	}
	if c.End.IsZero() || row.Time.After(c.End) {
		c.End = row.Time
	}
}

type conversationCount struct {
	trafficCount
	Transport      string                 `json:"transport"`
	Source         string                 `json:"source"`
	Destination    string                 `json:"destination"`
	TCPFlowID      uint64                 `json:"tcp_flow_id"`
	Domain         pcaputil.CaptureDomain `json:"capture_domain"`
	ForwardPackets uint64                 `json:"forward_packets"`
	ReversePackets uint64                 `json:"reverse_packets"`
	ForwardBytes   uint64                 `json:"forward_bytes"`
	ReverseBytes   uint64                 `json:"reverse_bytes"`
	DirectionRatio float64                `json:"forward_packet_fraction"`
}
type endpointCount struct {
	Endpoint        string `json:"endpoint"`
	Packets         uint64 `json:"packets"`
	Bytes           uint64 `json:"bytes"`
	SentPackets     uint64 `json:"sent_packets"`
	ReceivedPackets uint64 `json:"received_packets"`
	SentBytes       uint64 `json:"sent_bytes"`
	ReceivedBytes   uint64 `json:"received_bytes"`
}
type protocolCount struct {
	FrameFraction float64           `json:"frame_fraction"`
	PDUFraction   float64           `json:"pdu_fraction"`
	Protocol      string            `json:"protocol"`
	Frames        uint64            `json:"frames"`
	PDUs          uint64            `json:"pdus"`
	Bytes         uint64            `json:"bytes"`
	PDUBytes      uint64            `json:"pdu_bytes"`
	Statuses      map[string]uint64 `json:"statuses"`
}
type ioCount struct {
	Bucket  time.Time `json:"bucket"`
	Packets uint64    `json:"packets"`
	Bytes   uint64    `json:"bytes"`
}
type trafficStatistics struct {
	frames        uint64
	pdus          uint64
	kinds         []string
	interval      time.Duration
	conversations map[statisticsConversationKey]*conversationCount
	endpoints     map[string]*endpointCount
	protocols     map[string]*protocolCount
	buckets       map[int64]*ioCount
}

type statisticsConversationKey struct {
	domain    pcaputil.CaptureDomain
	transport string
	flow      uint64
	low, high string
}

func newTrafficStatistics(kinds []string, interval time.Duration) (*trafficStatistics, error) {
	s := &trafficStatistics{interval: interval}
	if interval <= 0 {
		return nil, fmt.Errorf("stats-interval must be positive")
	}
	seen := map[string]bool{}
	for _, kind := range kinds {
		if seen[kind] {
			continue
		}
		seen[kind] = true
		switch kind {
		case "conv":
			s.conversations = make(map[statisticsConversationKey]*conversationCount)
		case "endpoints":
			s.endpoints = make(map[string]*endpointCount)
		case "proto":
			s.protocols = make(map[string]*protocolCount)
		case "io":
			s.buckets = make(map[int64]*ioCount)
		default:
			return nil, fmt.Errorf("unknown stats kind %q (conv, io, proto, endpoints)", kind)
		}
		s.kinds = append(s.kinds, kind)
	}
	return s, nil
}
func (s *trafficStatistics) protocol(name string) *protocolCount {
	name = strings.ToLower(name)
	if strings.HasPrefix(name, "http/") {
		name = "http"
	}
	if p := s.protocols[name]; p != nil {
		return p
	}
	p := &protocolCount{Protocol: name, Statuses: make(map[string]uint64)}
	s.protocols[name] = p
	return p
}
func (s *trafficStatistics) addPDUs(pdus []map[string]any) {
	if s.protocols == nil {
		return
	}
	for _, pdu := range pdus {
		name, _ := pdu["protocol"].(string)
		if name == "" {
			name = "unknown"
		}
		p := s.protocol(name)
		p.PDUs++
		s.pdus++
		status, _ := pdu["status"].(string)
		p.Statuses[status]++
		if length, ok := pdu["length"].(int); ok && length > 0 {
			p.PDUBytes += uint64(length)
		}
	}
}
func (s *trafficStatistics) add(row packetSummary) error {
	if s == nil {
		return nil
	}
	s.frames++
	if s.conversations != nil && (row.Transport == "tcp" && row.TCPFlowID != 0 || row.Transport == "udp") {
		source, destination := net.JoinHostPort(row.Source, strconv.Itoa(int(row.SourcePort))), net.JoinHostPort(row.Destination, strconv.Itoa(int(row.DestinationPort)))
		a, b := source, destination
		if a > b {
			a, b = b, a
		}
		key := statisticsConversationKey{row.CaptureDomain, row.Transport, row.TCPFlowID, a, b}
		c := s.conversations[key]
		if c == nil {
			c = &conversationCount{Transport: row.Transport, Source: source, Destination: destination, TCPFlowID: row.TCPFlowID, Domain: row.CaptureDomain}
			s.conversations[key] = c
		}
		c.add(row)
		if source == c.Source {
			c.ForwardPackets++
			c.ForwardBytes += uint64(row.CapturedLength)
		} else {
			c.ReversePackets++
			c.ReverseBytes += uint64(row.CapturedLength)
		}
		c.DirectionRatio = float64(c.ForwardPackets) / float64(c.Packets)
	}
	if s.buckets != nil {
		n := row.Time.UnixNano() / int64(s.interval)
		if row.Time.UnixNano() < 0 && row.Time.UnixNano()%int64(s.interval) != 0 {
			n--
		}
		c := s.buckets[n]
		if c == nil {
			c = &ioCount{Bucket: time.Unix(0, n*int64(s.interval)).UTC()}
			s.buckets[n] = c
		}
		c.Packets++
		c.Bytes += uint64(row.CapturedLength)
	}
	if s.endpoints != nil {
		for i, endpoint := range []string{row.Source, row.Destination} {
			if endpoint == "" {
				continue
			}
			c := s.endpoints[endpoint]
			if c == nil {
				c = &endpointCount{Endpoint: endpoint}
				s.endpoints[endpoint] = c
			}
			c.Packets++
			c.Bytes += uint64(row.CapturedLength)
			if i == 0 {
				c.SentPackets++
				c.SentBytes += uint64(row.CapturedLength)
			} else {
				c.ReceivedPackets++
				c.ReceivedBytes += uint64(row.CapturedLength)
			}
		}
	}
	if s.protocols != nil {
		seen := map[string]bool{}
		names := append(append([]string(nil), row.Layers...), row.ProtocolStack...)
		for _, pdu := range row.PDUs {
			if name, ok := pdu["protocol"].(string); ok {
				names = append(names, name)
			}
		}
		for _, name := range names {
			p := s.protocol(name)
			name = p.Protocol
			if seen[name] {
				continue
			}
			seen[name] = true
			p.Frames++
			p.Bytes += uint64(row.CapturedLength)
		}
		if len(seen) == 0 {
			p := s.protocol(row.Protocol)
			p.Frames++
			p.Bytes += uint64(row.CapturedLength)
		}
		s.addPDUs(row.PDUs)
		s.addPDUs(row.PublishedPDUs)
	}
	if len(s.conversations)+len(s.buckets)+len(s.endpoints) > maxStatisticsEntries {
		return fmt.Errorf("statistics cardinality budget exceeded (%d entries)", maxStatisticsEntries)
	}
	return nil
}
func (s *trafficStatistics) write(out io.Writer, asJSON bool) error {
	if s == nil {
		return nil
	}
	for _, p := range s.protocols {
		if s.frames > 0 {
			p.FrameFraction = float64(p.Frames) / float64(s.frames)
		}
		if s.pdus > 0 {
			p.PDUFraction = float64(p.PDUs) / float64(s.pdus)
		}
	}
	for _, kind := range s.kinds {
		var rows any
		switch kind {
		case "conv":
			values := make([]*conversationCount, 0, len(s.conversations))
			for _, v := range s.conversations {
				values = append(values, v)
			}
			sort.Slice(values, func(i, j int) bool {
				if values[i].Bytes != values[j].Bytes {
					return values[i].Bytes > values[j].Bytes
				}
				if values[i].TCPFlowID != values[j].TCPFlowID {
					return values[i].TCPFlowID < values[j].TCPFlowID
				}
				return values[i].Source+values[i].Destination < values[j].Source+values[j].Destination
			})
			rows = values
		case "endpoints":
			values := make([]*endpointCount, 0, len(s.endpoints))
			for _, v := range s.endpoints {
				values = append(values, v)
			}
			sort.Slice(values, func(i, j int) bool {
				if values[i].Bytes != values[j].Bytes {
					return values[i].Bytes > values[j].Bytes
				}
				return values[i].Endpoint < values[j].Endpoint
			})
			rows = values
		case "proto":
			values := make([]*protocolCount, 0, len(s.protocols))
			for _, v := range s.protocols {
				values = append(values, v)
			}
			sort.Slice(values, func(i, j int) bool { return values[i].Protocol < values[j].Protocol })
			rows = values
		case "io":
			values := make([]*ioCount, 0, len(s.buckets))
			for _, v := range s.buckets {
				values = append(values, v)
			}
			sort.Slice(values, func(i, j int) bool { return values[i].Bucket.Before(values[j].Bucket) })
			rows = values
		}
		if asJSON {
			if err := json.NewEncoder(out).Encode(map[string]any{"record_type": "statistics", "physical": false, "kind": kind, "interval": s.interval.String(), "filter_scope": "matched_output_rows; counts include pdus and published_pdus", "rows": rows}); err != nil {
				return err
			}
		} else {
			if _, err := fmt.Fprintf(out, "Statistics %s (display filter applied first):\n", kind); err != nil {
				return err
			}
			if err := writeStatisticsTable(out, rows); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeStatisticsTable(out io.Writer, rows any) error {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	switch values := rows.(type) {
	case []*conversationCount:
		fmt.Fprintln(w, "TRANSPORT\tSOURCE\tDESTINATION\tTCP FLOW\tPACKETS\tBYTES\tSTART\tEND\tFORWARD/REVERSE")
		for _, v := range values {
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%d\t%s\t%s\t%d/%d\n", v.Transport, v.Source, v.Destination, v.TCPFlowID, v.Packets, v.Bytes, v.Start.Format(time.RFC3339Nano), v.End.Format(time.RFC3339Nano), v.ForwardPackets, v.ReversePackets)
		}
	case []*endpointCount:
		fmt.Fprintln(w, "ENDPOINT\tPACKETS\tBYTES\tSENT PACKETS\tRECEIVED PACKETS\tSENT BYTES\tRECEIVED BYTES")
		for _, v := range values {
			fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%d\t%d\t%d\n", v.Endpoint, v.Packets, v.Bytes, v.SentPackets, v.ReceivedPackets, v.SentBytes, v.ReceivedBytes)
		}
	case []*protocolCount:
		fmt.Fprintln(w, "PROTOCOL\tFRAMES\tFRAME %\tPDUS\tPDU %\tFRAME BYTES\tPDU BYTES\tSTATES")
		for _, v := range values {
			states, _ := json.Marshal(v.Statuses)
			fmt.Fprintf(w, "%s\t%d\t%.2f\t%d\t%.2f\t%d\t%d\t%s\n", v.Protocol, v.Frames, 100*v.FrameFraction, v.PDUs, 100*v.PDUFraction, v.Bytes, v.PDUBytes, states)
		}
	case []*ioCount:
		fmt.Fprintln(w, "BUCKET\tPACKETS\tBYTES")
		for _, v := range values {
			fmt.Fprintf(w, "%s\t%d\t%d\n", v.Bucket.Format(time.RFC3339Nano), v.Packets, v.Bytes)
		}
	}
	return w.Flush()
}
