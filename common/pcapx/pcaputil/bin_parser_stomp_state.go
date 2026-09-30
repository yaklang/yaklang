package pcaputil

import "strings"

func stompVersionBit(v string) uint8 {
	switch v {
	case "1.0":
		return 1
	case "1.1":
		return 2
	case "1.2":
		return 4
	}
	return 0
}
func (s *binSTOMP) parseVersion() string {
	if s.version != "" {
		return s.version
	}
	switch s.offered {
	case 1:
		return "1.0"
	case 2:
		return "1.1"
	case 4:
		return "1.2"
	}
	return ""
}
func stompMidstreamEvidence(f stompFrame) bool {
	h := f.headers
	switch f.command {
	case "CONNECTED":
		return stompVersionBit(h["version"]) != 0 || h["session"] != ""
	case "MESSAGE":
		return h["destination"] != "" && h["message-id"] != ""
	case "SEND", "SUBSCRIBE":
		return h["destination"] != ""
	case "RECEIPT":
		return h["receipt-id"] != ""
	case "ACK", "NACK":
		return h["id"] != "" || h["message-id"] != ""
	case "UNSUBSCRIBE":
		return h["id"] != "" || h["destination"] != ""
	case "BEGIN", "COMMIT", "ABORT":
		return h["transaction"] != ""
	case "ERROR":
		return h["message"] != "" || h["receipt-id"] != ""
	}
	return false
}

func (s *binSTOMP) validate(f stompFrame) (string, error) {
	context := "observed"
	if !s.sawConnect || s.version == "" {
		context = "partial"
	}
	if f.heartbeat {
		return context, nil
	}
	h := f.headers
	require := func(keys ...string) error {
		for _, k := range keys {
			if _, ok := h[k]; !ok {
				return protocolError(ErrMalformedMessage, "stomp: %s requires %s header", f.command, k)
			}
		}
		return nil
	}
	if f.command == "CONNECT" || f.command == "STOMP" {
		if s.sawConnect {
			return context, protocolError(ErrMalformedMessage, "stomp: repeated connection setup")
		}
		offered, present := h["accept-version"]
		if !present {
			offered = "1.0"
		}
		var mask uint8
		for _, v := range strings.Split(offered, ",") {
			mask |= stompVersionBit(v)
		}
		if mask == 0 {
			return context, protocolError(ErrUnsupportedVersion, "stomp: unsupported offered versions")
		}
		if mask&6 != 0 || f.command == "STOMP" {
			if err := require("accept-version", "host"); err != nil {
				return context, err
			}
		}
		if f.command == "STOMP" && mask == 1 {
			return context, protocolError(ErrUnsupportedFeature, "stomp: STOMP command requires 1.1 or 1.2")
		}
		s.offered, s.sawConnect = mask, true
		return "observed", nil
	}
	if f.command == "CONNECTED" {
		if s.sawConnected {
			return context, protocolError(ErrMalformedMessage, "stomp: repeated CONNECTED response")
		}
		version, present := h["version"]
		if !present {
			version = "1.0"
		}
		bit := stompVersionBit(version)
		if bit == 0 {
			return context, protocolError(ErrUnsupportedVersion, "stomp: unsupported negotiated version %q", version)
		}
		if s.sawConnect && s.offered&bit == 0 {
			return context, protocolError(ErrMalformedMessage, "stomp: negotiated version was not offered")
		}
		s.version = version
		s.sawConnected = true
		if s.sawConnect {
			context = "observed"
		}
		return context, nil
	}
	v := s.parseVersion()
	switch f.command {
	case "SEND":
		return context, require("destination")
	case "SUBSCRIBE":
		if err := require("destination"); err != nil {
			return context, err
		}
		if v != "" && v != "1.0" {
			if err := require("id"); err != nil {
				return context, err
			}
		}
		ack, ok := h["ack"]
		if ok && ack != "auto" && ack != "client" && (ack != "client-individual" || v == "1.0") {
			return context, protocolError(ErrMalformedMessage, "stomp: invalid subscription ack mode")
		}
	case "UNSUBSCRIBE":
		if v == "" && h["id"] == "" && h["destination"] == "" {
			return context, protocolError(ErrMalformedMessage, "stomp: UNSUBSCRIBE requires id or destination header")
		}
		if v == "1.0" {
			if _, id := h["id"]; !id {
				return context, require("destination")
			}
		}
		if v != "" && v != "1.0" {
			return context, require("id")
		}
	case "ACK", "NACK":
		if v == "" && h["id"] == "" && h["message-id"] == "" {
			return context, protocolError(ErrMalformedMessage, "stomp: acknowledgement requires id or message-id header")
		}
		if f.command == "NACK" && v == "1.0" {
			return context, protocolError(ErrUnsupportedFeature, "stomp: NACK requires 1.1 or 1.2")
		}
		switch v {
		case "1.2":
			return context, require("id")
		case "1.1":
			return context, require("message-id", "subscription")
		case "1.0":
			return context, require("message-id")
		}
	case "BEGIN", "COMMIT", "ABORT":
		return context, require("transaction")
	case "MESSAGE":
		if err := require("destination", "message-id"); err != nil {
			return context, err
		}
		if v != "" && v != "1.0" {
			if err := require("subscription"); err != nil {
				return context, err
			}
		}
		if v == "1.2" {
			if ack, ok := s.state.subscriptions[h["subscription"]]; ok && ack != "auto" {
				return context, require("ack")
			}
		}
	case "RECEIPT":
		return context, require("receipt-id")
	}
	return context, nil
}

type stompAck struct {
	subscription, transaction string
	sequence                  uint64
	cumulative                bool
	transactionBound          bool // an empty transaction identifier is distinct from no transaction
}
type stompState struct {
	subscriptions map[string]string
	transactions  map[string]bool
	messages      map[string]stompAck
	receipts      map[string]string
	sequence      uint64
	slots         int
}

// Charge retained correlation maps before growing them. reserveSession keeps
// their high-water mark until flow close, including map buckets after deletion.
func (s *binSTOMP) reserveState(extra, additional int) error {
	st := &s.state
	count := len(st.subscriptions) + len(st.transactions) + len(st.messages) + len(st.receipts) + additional
	limit := 4096
	if s.flow != nil && s.flow.a.budget.MaxCollectionElements > 0 {
		limit = s.flow.a.budget.MaxCollectionElements
	}
	if count > limit {
		return protocolError(ErrResourceExceeded, "stomp: correlation entry budget")
	}
	slots := max(st.slots, count)
	size := int64(2048 + slots*512 + extra*2)
	for k, v := range st.subscriptions {
		size += int64(len(k) + len(v))
	}
	for k := range st.transactions {
		size += int64(len(k))
	}
	for k, v := range st.messages {
		size += int64(len(k) + len(v.subscription) + len(v.transaction))
	}
	for k, v := range st.receipts {
		size += int64(len(k) + len(v))
	}
	if s.flow != nil {
		if err := s.flow.reserveSession(size); err != nil {
			return err
		}
	}
	st.slots = slots
	if st.subscriptions == nil {
		st.subscriptions = map[string]string{}
		st.transactions = map[string]bool{}
		st.messages = map[string]stompAck{}
		st.receipts = map[string]string{}
	}
	return nil
}
func stompMissingContext(info map[string]any) {
	info["Context Level"] = "partial"
	info["Association Status"] = "missing-context"
}
func (s *binSTOMP) ackKey(h map[string]string, message bool) string {
	if s.parseVersion() == "1.2" {
		if message {
			return h["ack"]
		}
		return h["id"]
	}
	if s.parseVersion() == "1.1" {
		return h["subscription"] + "\x00" + h["message-id"]
	}
	if s.parseVersion() == "" {
		if h["ack"] != "" {
			return h["ack"]
		}
		if h["id"] != "" {
			return h["id"]
		}
	}
	return h["message-id"]
}
func (s *binSTOMP) removeAcknowledged(key string, ack stompAck) {
	for k, m := range s.state.messages {
		if k == key || (ack.cumulative && m.subscription == ack.subscription && m.sequence <= ack.sequence) {
			delete(s.state.messages, k)
		}
	}
}
func (s *binSTOMP) observe(f stompFrame, info map[string]any) error {
	h, st := f.headers, &s.state
	transaction, hasTransaction := h["transaction"]
	if hasTransaction && f.command != "BEGIN" {
		if st.transactions[transaction] {
			info["Transaction Status"] = "observed"
		} else {
			info["Transaction Status"] = "missing-context"
			stompMissingContext(info)
		}
	}
	switch f.command {
	case "SUBSCRIBE":
		key := h["id"]
		if key == "" {
			key = "destination:" + h["destination"]
		}
		if _, ok := st.subscriptions[key]; ok {
			return protocolError(ErrMalformedMessage, "stomp: duplicate active subscription id")
		}
		ack := h["ack"]
		if ack == "" {
			ack = "auto"
		}
		if err := s.reserveState(len(key)+len(ack), 1); err != nil {
			return err
		}
		st.subscriptions[strings.Clone(key)] = strings.Clone(ack)
	case "UNSUBSCRIBE":
		key := h["id"]
		if key == "" {
			key = "destination:" + h["destination"]
		}
		if _, ok := st.subscriptions[key]; !ok {
			stompMissingContext(info)
		} else {
			delete(st.subscriptions, key)
			info["Association Status"] = "matched"
		}
	case "BEGIN":
		if st.transactions[transaction] {
			return protocolError(ErrMalformedMessage, "stomp: duplicate active transaction id")
		}
		if err := s.reserveState(len(transaction), 1); err != nil {
			return err
		}
		st.transactions[strings.Clone(transaction)] = true
	case "COMMIT", "ABORT":
		for key, ack := range st.messages {
			if !ack.transactionBound || ack.transaction != transaction {
				continue
			}
			if f.command == "COMMIT" {
				s.removeAcknowledged(key, ack)
			} else {
				ack.transaction = ""
				ack.transactionBound = false
				st.messages[key] = ack
			}
		}
		delete(st.transactions, transaction)
	case "MESSAGE":
		sub := h["subscription"]
		if sub == "" {
			sub = "destination:" + h["destination"]
		}
		ackMode, known := st.subscriptions[sub]
		if !known {
			stompMissingContext(info)
		}
		key := s.ackKey(h, true)
		if key != "" && (!known || ackMode != "auto") {
			additional := 1
			if _, exists := st.messages[key]; exists {
				additional = 0
			}
			if err := s.reserveState(len(key)+len(sub), additional); err != nil {
				return err
			}
			st.sequence++
			st.messages[strings.Clone(key)] = stompAck{subscription: strings.Clone(sub), sequence: st.sequence, cumulative: ackMode == "client"}
		}
	case "ACK", "NACK":
		key := s.ackKey(h, false)
		ack, known := st.messages[key]
		if !known {
			stompMissingContext(info)
		} else {
			info["Association Status"] = "matched"
			info["Subscription"] = ack.subscription
			if hasTransaction {
				if err := s.reserveState(len(transaction), 0); err != nil {
					return err
				}
				ack.transaction = strings.Clone(transaction)
				ack.transactionBound = true
				st.messages[key] = ack
			} else {
				s.removeAcknowledged(key, ack)
			}
		}
	case "RECEIPT", "ERROR":
		if receipt, ok := h["receipt-id"]; ok {
			if command, known := st.receipts[receipt]; known {
				info["Association Status"] = "matched"
				info["Receipt Command"] = command
				delete(st.receipts, receipt)
			} else {
				stompMissingContext(info)
			}
		}
	}
	if receipt, ok := h["receipt"]; ok && stompClientCommands[f.command] && f.command != "CONNECT" && f.command != "STOMP" {
		additional := 1
		if _, exists := st.receipts[receipt]; exists {
			additional = 0
		}
		if err := s.reserveState(len(receipt)+len(f.command), additional); err != nil {
			return err
		}
		st.receipts[strings.Clone(receipt)] = f.command
	}
	return nil
}
