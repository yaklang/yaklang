package pcaputil

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type redisRequest struct {
	id        uint64
	ts        time.Time
	command   string
	remaining int
}
type binRedis struct {
	subscriptions          map[string]string
	values                 [2]uint64
	client                 int
	version                string
	pending                []redisRequest
	attributes             [2]map[string]any
	attrBytes              [2]int
	txn                    bool
	pubsub, desynchronized bool
	clock                  time.Time
}

func (r *binRedis) storage() int64 {
	n := 256 + int64(len(r.pending))*128 + int64(r.attrBytes[0]+r.attrBytes[1])
	for key := range r.subscriptions {
		n += 64 + int64(len(key))
	}
	return n
}

// DecodeRESPValue validates a single complete bounded RESP value. Map entries
// remain pairs: Redis keys are not limited to strings and duplicates are evidence.
func DecodeRESPValue(raw []byte, budget ParserBudget) (map[string]any, error) {
	d := DefaultParserBudget()
	if budget.MaxRecursionDepth == 0 {
		budget.MaxRecursionDepth = d.MaxRecursionDepth
	}
	if budget.MaxCollectionElements == 0 {
		budget.MaxCollectionElements = d.MaxCollectionElements
	}
	if budget.MaxMessageBytes == 0 {
		budget.MaxMessageBytes = d.MaxMessageBytes
	}
	if budget.MaxRecursionDepth < 1 || budget.MaxRecursionDepth > 64 || budget.MaxCollectionElements < 1 || budget.MaxCollectionElements > 4096 || budget.MaxMessageBytes < 1 || len(raw) > budget.MaxMessageBytes {
		return nil, protocolError(ErrResourceExceeded, "Redis value budget")
	}
	used := 0
	n, err := redisFrameLengthBudget(raw, 0, budget, &used)
	if err != nil {
		return nil, err
	}
	if n == 0 || n != len(raw) {
		return nil, fmt.Errorf("redis: incomplete or trailing value")
	}
	at := 0
	var read func() map[string]any
	read = func() map[string]any {
		kind := raw[at]
		nl := at + bytes.Index(raw[at:], []byte("\r\n"))
		line := string(raw[at+1 : nl])
		at = nl + 2
		v := map[string]any{"Type": redisTypeName(kind)}
		switch kind {
		case '+', '-', '(', ',':
			v["Value"] = line
		case ':':
			x, _ := strconv.ParseInt(line, 10, 64)
			v["Value"] = x
		case '#':
			v["Value"] = line == "t"
		case '_':
			v["Value"] = nil
		case '$', '!', '=':
			n, _ := strconv.Atoi(line)
			if n < 0 {
				v["Value"] = nil
				break
			}
			v["Value"] = bytes.Clone(raw[at : at+n])
			at += n + 2
		default:
			n, _ := strconv.Atoi(line)
			if n < 0 {
				v["Value"] = nil
				break
			}
			if kind == '%' || kind == '|' {
				n *= 2
			}
			items := make([]any, 0, n)
			for i := 0; i < n; i++ {
				items = append(items, read())
			}
			v["Items"] = items
		}
		return v
	}
	return read(), nil
}
func redisText(v map[string]any) string {
	switch x := v["Value"].(type) {
	case []byte:
		return string(x)
	case string:
		return x
	}
	return ""
}
func redisItems(v map[string]any) []any { x, _ := v["Items"].([]any); return x }
func redisCommand(v map[string]any) string {
	if v["Type"] != "Array" {
		return ""
	}
	xs := redisItems(v)
	if len(xs) == 0 {
		return ""
	}
	for _, x := range xs {
		if x.(map[string]any)["Type"] != "Bulk" {
			return ""
		}
	}
	return strings.ToUpper(redisText(xs[0].(map[string]any)))
}
func redisRoleCommand(c string) bool {
	switch c {
	case "HELLO", "AUTH", "PING", "SELECT", "GET", "SET", "MULTI", "CLIENT", "SUBSCRIBE", "PSUBSCRIBE":
		return true
	}
	return false
}
func redisSubscription(c string) bool {
	switch c {
	case "SUBSCRIBE", "PSUBSCRIBE", "SSUBSCRIBE", "UNSUBSCRIBE", "PUNSUBSCRIBE", "SUNSUBSCRIBE":
		return true
	}
	return false
}
func redactRedisCommand(v map[string]any, cmd string) {
	// The semantic display defaults to redacted authentication arguments. Raw is
	// still explicit capture evidence, never a promise to sanitize saved PCAPs.
	xs := redisItems(v)
	redact := cmd == "AUTH"
	for i := 1; i < len(xs); i++ {
		if cmd == "HELLO" && strings.EqualFold(redisText(xs[i].(map[string]any)), "AUTH") {
			for j := i + 1; j < len(xs) && j <= i+2; j++ {
				xs[j] = map[string]any{"Type": "Bulk", "Redacted": true}
			}
			break
		}
		if redact {
			xs[i] = map[string]any{"Type": "Bulk", "Redacted": true}
		}
	}
}

func (f *binFlow) consumeRedis(dir int, e *ProtocolEvent) (map[string]any, error) {
	r := f.redis
	v, err := DecodeRESPValue(e.Raw, f.a.budget)
	if err != nil {
		return nil, err
	}
	if e.Timestamp.After(r.clock) {
		r.clock = e.Timestamp
	}
	for len(r.pending) > 0 && r.clock.Sub(r.pending[0].ts) > 30*time.Second {
		r.pending[0] = redisRequest{}
		r.pending = r.pending[1:]
		if len(r.pending) == 0 {
			r.pending = nil
		}
		r.txn = false
		r.desynchronized = true
	}
	info := map[string]any{"RESP Type": redisTypeName(e.Raw[0]), "Version": redisVersion(e.Raw[0]), "Version Negotiated": false, "Context Level": "observed", "Value": v}
	r.values[dir]++
	info["Direction Value Index"] = r.values[dir]
	if r.version != "" {
		info["Version"] = r.version
		info["Version Negotiated"] = true
	}
	if e.Raw[0] == '|' {
		if r.attributes[dir] != nil {
			return nil, protocolError(ErrUnsupportedFeature, "Redis repeated attributes before value")
		}
		if err = f.reserveSession(r.storage() + int64(len(e.Raw))*8); err != nil {
			return nil, err
		}
		r.attributes[dir] = cloneSession(v)
		r.attrBytes[dir] = len(e.Raw) * 8
		info["Attribute"] = true
		return info, nil
	}
	if r.attributes[dir] != nil {
		info["Attributes"] = r.attributes[dir]
		r.attributes[dir] = nil
		r.attrBytes[dir] = 0
	}
	cmd := redisCommand(v)
	if len(cmd) > 128 {
		return nil, protocolError(ErrResourceExceeded, "Redis command name exceeds limit")
	}
	if r.client < 0 || r.client == dir {
		redactRedisCommand(v, cmd)
	}
	if r.desynchronized {
		info["Correlation Status"] = "expired-context"
		return info, nil
	}
	if r.client < 0 && redisRoleCommand(cmd) {
		r.client = dir
		info["Role Evidence"] = "command-shape"
	}
	if e.ID == 0 {
		e.ID = f.a.ids.Add(1)
	}
	if r.client == dir && cmd != "" {
		if len(r.pending) >= f.a.budget.MaxCollectionElements {
			return nil, protocolError(ErrResourceExceeded, "Redis pending requests")
		}
		if err = f.reserveSession(r.storage() + 128); err != nil {
			return nil, err
		}
		count := 1
		if redisSubscription(cmd) {
			count = len(redisItems(v)) - 1
			if count == 0 && !strings.Contains(cmd, "UNSUBSCRIBE") {
				count = 1
			}
		}
		r.pending = append(r.pending, redisRequest{e.ID, e.Timestamp, cmd, count})
		e.TransactionID = e.ID
		info["Command"] = cmd
		info["Outstanding"] = true

	} else if r.client >= 0 && dir != r.client {
		xs := redisItems(v)
		push := e.Raw[0] == '>'
		name := ""
		if len(xs) > 0 {
			name = strings.ToUpper(redisText(xs[0].(map[string]any)))
		}
		subscriptionAck := redisSubscription(name) && len(r.pending) > 0 && r.pending[0].command == name
		if r.pubsub && (name == "MESSAGE" || name == "PMESSAGE" || name == "SMESSAGE") || subscriptionAck {
			push = true
		}
		if push {
			info["Push"] = true
		}
		match := len(r.pending) > 0 && (!push || redisSubscription(name) && r.pending[0].command == name)
		if match {
			if subscriptionAck && len(xs) == 3 {
				category := ""
				if strings.HasPrefix(name, "P") {
					category = "p"
				}
				if strings.HasPrefix(name, "S") && name != "SUBSCRIBE" {
					category = "s"
				}
				if r.pending[0].remaining == 0 {
					n := 0
					for _, c := range r.subscriptions {
						if c == category {
							n++
						}
					}
					r.pending[0].remaining = max(1, n)
				}
				channel := redisText(xs[1].(map[string]any))
				key := category + ":" + channel
				if strings.Contains(name, "UNSUBSCRIBE") {
					delete(r.subscriptions, key)
				} else {
					_, exists := r.subscriptions[key]
					if !exists && len(r.subscriptions) >= f.a.budget.MaxCollectionElements {
						return nil, protocolError(ErrResourceExceeded, "Redis subscription count")
					}
					extra := int64(0)
					if !exists {
						extra = 64 + int64(len(key))
					}
					if err := f.reserveSession(r.storage() + extra); err != nil {
						return nil, err
					}
					if r.subscriptions == nil {
						r.subscriptions = map[string]string{}
					}
					r.subscriptions[key] = category
				}
				if n, ok := xs[2].(map[string]any)["Value"].(int64); ok {
					r.pubsub = n > 0
				}
			}
			q := r.pending[0]
			e.ResponseTo = q.id
			e.TransactionID = q.id
			info["Matched Request"] = true
			info["Command"] = q.command
			info["Latency NS"] = e.Timestamp.Sub(q.ts).Nanoseconds()
			// A command error is one final reply, even for multiple subscription targets.
			if v["Type"] == "Error" || v["Type"] == "BlobError" {
				r.pending[0].remaining = 1
			}
			r.pending[0].remaining--
			if r.pending[0].remaining <= 0 {
				r.pending[0] = redisRequest{}
				r.pending = r.pending[1:]
				if len(r.pending) == 0 {
					r.pending = nil
				}
			}
			success := v["Type"] != "Error" && v["Type"] != "BlobError"
			if q.command == "MULTI" && redisText(v) == "OK" {
				r.txn = true
			}
			if r.txn {
				info["Transaction Observed"] = true
			}
			if q.command == "EXEC" || q.command == "DISCARD" {
				info["Transaction Complete Observed"] = r.txn && success
				r.txn = false
			}
			if q.command == "HELLO" && success && v["Type"] == "Map" {
				for i := 0; i+1 < len(xs); i += 2 {
					if redisText(xs[i].(map[string]any)) == "proto" {
						if x, ok := xs[i+1].(map[string]any)["Value"].(int64); ok && (x == 2 || x == 3) {
							r.version = fmt.Sprintf("RESP%d", x)
							info["Version"] = r.version
							info["Version Negotiated"] = true
						}
					}
				}
			}
		} else if !push {
			info["Unmatched"] = true
		}
	} else if e.Raw[0] == '>' {
		info["Push"] = true
	}
	if err = f.reserveSession(r.storage()); err != nil {
		return nil, err
	}
	return info, nil
}
