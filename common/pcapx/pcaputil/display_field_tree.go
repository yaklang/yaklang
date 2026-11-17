package pcaputil

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	DisplayFieldTreeSchemaVersion = 2
	maxDisplayTreeNodes           = 4096
)

type DisplayRange struct {
	BitOffset int64 `json:"o"`
	BitLength int64 `json:"l"`
}

type DisplayNode struct {
	Name         string         `json:"n"`
	DisplayValue string         `json:"v,omitempty"`
	Children     []DisplayNode  `json:"c,omitempty"`
	Ranges       []DisplayRange `json:"r,omitempty"`
}

// DisplayFieldTree is persisted with one protocol message. All ranges are
// relative to that message's stored Raw bytes, never to a contributing frame.
type DisplayFieldTree struct {
	SchemaVersion int           `json:"schema_version"`
	Nodes         []DisplayNode `json:"nodes"`
	Truncated     bool          `json:"truncated,omitempty"`
}

type displayTreeBuilder struct {
	remaining int
	truncated bool
}

func (b *displayTreeBuilder) take() bool {
	if b.remaining <= 0 {
		b.truncated = true
		return false
	}
	b.remaining--
	return true
}

func displayByteRange(offset, length int) []DisplayRange {
	if offset < 0 || length <= 0 || offset > math.MaxInt64/8 || length > math.MaxInt64/8 {
		return nil
	}
	return []DisplayRange{{BitOffset: int64(offset) * 8, BitLength: int64(length) * 8}}
}

func appendDisplayLeaf(parent *DisplayNode, name string, offset, length int, b *displayTreeBuilder) {
	if !b.take() {
		return
	}
	parent.Children = append(parent.Children, DisplayNode{
		Name: name, Ranges: displayByteRange(offset, length),
	})
}

func appendDecodedRanges(parent *DisplayNode, snapshot DisplayFieldSnapshot, rawLength int, b *displayTreeBuilder) {
	if !b.take() {
		return
	}
	values := DisplayNode{Name: "Decoded fields"}
	for _, value := range snapshot.Values {
		if value.Origin != "pdu" || value.Range == nil {
			continue
		}
		ranges := displayByteRange(value.Range.Offset, value.Range.Length)
		if len(ranges) == 0 || int64(rawLength)*8 < ranges[0].BitOffset+ranges[0].BitLength {
			continue
		}
		if !b.take() {
			break
		}
		node := DisplayNode{Name: value.Name, Ranges: ranges}
		values.Children = append(values.Children, node)
	}
	if snapshot.Truncated || b.truncated {
		values.DisplayValue = "truncated"
	}
	if len(values.Children) > 0 {
		parent.Children = append(parent.Children, values)
	}
}

func httpFirstLine(raw []byte) (line []byte, end int) {
	if index := bytes.Index(raw, []byte("\r\n")); index >= 0 {
		return raw[:index], index
	}
	return nil, -1
}

func isHTTPDisplayProtocol(protocol string) bool {
	switch strings.ToLower(protocol) {
	case "http", "doh", "ipp":
		return true
	default:
		return false
	}
}

func appendHTTPStartLine(parent *DisplayNode, raw []byte, b *displayTreeBuilder) {
	line, end := httpFirstLine(raw)
	if end <= 0 {
		return
	}
	if bytes.HasPrefix(line, []byte("HTTP/")) {
		node := DisplayNode{Name: "Response Line", Ranges: displayByteRange(0, len(line))}
		firstSpace := bytes.IndexByte(line, ' ')
		if firstSpace > 0 {
			appendDisplayLeaf(&node, "Protocol", 0, firstSpace, b)
			statusStart := firstSpace + 1
			for statusStart < len(line) && line[statusStart] == ' ' {
				statusStart++
			}
			statusEnd := statusStart
			for statusEnd < len(line) && line[statusEnd] >= '0' && line[statusEnd] <= '9' {
				statusEnd++
			}
			if statusEnd > statusStart {
				appendDisplayLeaf(&node, "Status Code", statusStart, statusEnd-statusStart, b)
			}
			reasonStart := statusEnd
			for reasonStart < len(line) && line[reasonStart] == ' ' {
				reasonStart++
			}
			if reasonStart < len(line) {
				appendDisplayLeaf(&node, "Reason Phrase", reasonStart, len(line)-reasonStart, b)
			}
		}
		parent.Children = append(parent.Children, node)
		return
	}
	firstSpace := bytes.IndexByte(line, ' ')
	if firstSpace <= 0 {
		return
	}
	secondSpace := bytes.IndexByte(line[firstSpace+1:], ' ')
	if secondSpace < 0 {
		return
	}
	secondSpace += firstSpace + 1
	node := DisplayNode{Name: "Request Line", Ranges: displayByteRange(0, len(line))}
	appendDisplayLeaf(&node, "Method", 0, firstSpace, b)
	appendDisplayLeaf(&node, "Request URI", firstSpace+1, secondSpace-firstSpace-1, b)
	appendDisplayLeaf(&node, "Protocol", secondSpace+1, len(line)-secondSpace-1, b)
	parent.Children = append(parent.Children, node)
}

func appendHTTPHeadersAndBody(parent *DisplayNode, raw []byte, totalLength int, b *displayTreeBuilder) {
	line, firstLineEnd := httpFirstLine(raw)
	if firstLineEnd < 0 || len(line) == 0 {
		return
	}
	headerStart := firstLineEnd + 2
	headerEndRelative := bytes.Index(raw[headerStart:], []byte("\r\n\r\n"))
	headersEnd := len(raw)
	bodyStart := len(raw)
	if headerEndRelative >= 0 {
		headersEnd = headerStart + headerEndRelative
		bodyStart = headersEnd + 4
	}
	headers := DisplayNode{Name: "Headers"}
	offset := headerStart
	chunked := false
	for offset < headersEnd {
		next := bytes.Index(raw[offset:headersEnd], []byte("\r\n"))
		lineEnd := headersEnd
		if next >= 0 {
			lineEnd = offset + next
		}
		wireLine := raw[offset:lineEnd]
		colon := bytes.IndexByte(wireLine, ':')
		if colon > 0 {
			name := string(wireLine[:colon])
			valueStart := offset + colon + 1
			for valueStart < lineEnd && (raw[valueStart] == ' ' || raw[valueStart] == '\t') {
				valueStart++
			}
			valueEnd := lineEnd
			for valueEnd > valueStart && (raw[valueEnd-1] == ' ' || raw[valueEnd-1] == '\t') {
				valueEnd--
			}
			value := string(raw[valueStart:valueEnd])
			if strings.EqualFold(name, "Transfer-Encoding") && strings.Contains(strings.ToLower(value), "chunked") {
				chunked = true
			}
			appendDisplayLeaf(&headers, name, valueStart, valueEnd-valueStart, b)
		}
		if next < 0 {
			break
		}
		offset = lineEnd + 2
	}
	if len(headers.Children) > 0 {
		headers.DisplayValue = strconv.Itoa(len(headers.Children)) + " fields"
		parent.Children = append(parent.Children, headers)
	}
	sourceLength := totalLength
	if sourceLength < len(raw) {
		sourceLength = len(raw)
	}
	if bodyStart < sourceLength {
		parent.Children = append(parent.Children, DisplayNode{
			Name: "Body (wire)", DisplayValue: strconv.Itoa(sourceLength-bodyStart) + " bytes",
			Ranges: displayByteRange(bodyStart, sourceLength-bodyStart),
		})
		if chunked {
			appendHTTPChunkedBody(parent, raw, bodyStart, b)
		}
	}
}

func appendHTTPChunkedBody(parent *DisplayNode, raw []byte, bodyStart int, b *displayTreeBuilder) {
	offset := bodyStart
	for index := 0; offset < len(raw); index++ {
		lineEndRelative := bytes.Index(raw[offset:], []byte("\r\n"))
		if lineEndRelative < 0 {
			return
		}
		lineEnd := offset + lineEndRelative
		sizeField := strings.SplitN(string(raw[offset:lineEnd]), ";", 2)[0]
		sizeToken := strings.TrimSpace(sizeField)
		size, err := strconv.ParseUint(sizeToken, 16, 32)
		if err != nil {
			return
		}
		if size == 0 {
			return
		}
		if size > uint64(len(raw)) {
			return
		}
		dataStart := lineEnd + 2
		dataEnd := dataStart + int(size)
		if dataEnd < dataStart || dataEnd+2 > len(raw) || !bytes.Equal(raw[dataEnd:dataEnd+2], []byte("\r\n")) {
			return
		}
		if !b.take() {
			return
		}
		chunk := DisplayNode{Name: fmt.Sprintf("Chunk [%d]", index), DisplayValue: fmt.Sprintf("%d bytes", size)}
		sizeOffset := offset + strings.Index(string(raw[offset:lineEnd]), sizeField) + strings.Index(sizeField, sizeToken)
		appendDisplayLeaf(&chunk, "Size", sizeOffset, len(sizeToken), b)
		appendDisplayLeaf(&chunk, "Content", dataStart, int(size), b)
		parent.Children = append(parent.Children, chunk)
		offset = dataEnd + 2
	}
}

// BuildHTTPDisplayNodes reconstructs HTTP/1 wire spans directly from the
// message prefix. totalLength is the complete stored message length, allowing
// a fixed body range to extend beyond the initially loaded prefix.
func BuildHTTPDisplayNodes(protocol string, raw []byte, totalLength int) ([]DisplayNode, bool) {
	if !isHTTPDisplayProtocol(protocol) || len(raw) == 0 {
		return nil, false
	}
	builder := &displayTreeBuilder{remaining: maxDisplayTreeNodes}
	root := DisplayNode{Name: strings.ToUpper(protocol)}
	appendHTTPStartLine(&root, raw, builder)
	appendHTTPHeadersAndBody(&root, raw, totalLength, builder)
	if len(root.Children) == 0 {
		return nil, builder.truncated
	}
	return []DisplayNode{root}, builder.truncated
}

// BuildDisplayFieldTree materializes a bounded, persisted field tree. Only
// parser-confirmed wire spans are emitted. HTTP/1 spans are reconstructed from
// message bytes in the detail request instead of being duplicated on disk.
func BuildDisplayFieldTree(event *ProtocolEvent) DisplayFieldTree {
	tree := DisplayFieldTree{SchemaVersion: DisplayFieldTreeSchemaVersion}
	if event == nil || isHTTPDisplayProtocol(event.Protocol) {
		return tree
	}
	builder := &displayTreeBuilder{remaining: maxDisplayTreeNodes}
	protocol := strings.ToUpper(event.Protocol)
	if protocol == "" {
		protocol = "Application"
	}
	root := DisplayNode{Name: protocol}
	appendDecodedRanges(&root, event.DisplayFieldValues(), len(event.Raw), builder)
	if len(root.Children) > 0 {
		tree.Nodes = append(tree.Nodes, root)
	}
	tree.Truncated = builder.truncated
	return tree
}
