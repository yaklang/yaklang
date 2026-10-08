package stream_parser

import "fmt"

// These four profiles are pinned to Apache Kafka 3.9.1 message schemas. In
// particular Produce v8 and versions newer than these profiles are not implied.
func KafkaFlexibleVersion(api, version int16) bool {
	return api == 18 && version == 3 || api == 3 && version == 9 || api == 0 && version == 9 || api == 1 && version == 12
}
func KafkaVersionSupported(api, version int16) bool {
	r, ok := kafkaAPIVersions[api]
	return ok && version >= r[0] && version <= r[1] || KafkaFlexibleVersion(api, version)
}
func (r *kafkaReader) unsigned(name string) uint32 {
	var value uint32
	for i := 0; i < 5 && r.err == nil; i++ {
		b := r.take(name, "uint8", 1)
		if b == nil {
			return 0
		}
		v := b[0]
		if i == 4 && v > 15 {
			r.err = fmt.Errorf("kafka: overflowing unsigned varint %s", name)
			return 0
		}
		value |= uint32(v&127) << uint(7*i)
		if v < 128 {
			return value
		}
	}
	return 0
}
func (r *kafkaReader) compactLength(name string, nullable, collection bool) int {
	n := r.unsigned(name + " Length")
	if r.err != nil {
		return 0
	}
	if n == 0 {
		if !nullable {
			r.err = fmt.Errorf("kafka: null %s", name)
		}
		return -1
	}
	n--
	if collection {
		if n > uint32(4096-r.elements) {
			r.err = fmt.Errorf("kafka: collection budget %s", name)
			return 0
		}
		r.elements += int(n)
	} else if n > uint32(len(r.wire)-r.at) {
		r.err = fmt.Errorf("kafka: truncated %s", name)
		return 0
	}
	return int(n)
}
func (r *kafkaReader) compactString(name string, nullable bool) any {
	n := r.compactLength(name, nullable, false)
	if n < 0 {
		return nil
	}
	return string(r.take(name, "string", n))
}
func (r *kafkaReader) compactInts(name string) []int32 {
	a := []int32{}
	for n := r.compactLength(name, false, true); n > 0 && r.err == nil; n-- {
		a = append(a, r.i32(name))
	}
	return a
}

// Every struct owns a separate tag section. Unknown payloads remain byte
// evidence; known fields decode inside their declared window and consume it.
func (r *kafkaReader) tags(m map[string]any, known func(uint32, *kafkaReader) bool) {
	if r.err != nil {
		return
	}
	n := r.unsigned("Tagged Fields Count")
	if n > uint32(4096-r.elements) {
		r.err = fmt.Errorf("kafka: tagged fields budget")
		return
	}
	r.elements += int(n)
	var previous uint32
	var tags []map[string]any
	for i := uint32(0); i < n && r.err == nil; i++ {
		tag := r.unsigned("Tag ID")
		size := r.unsigned("Tag Size")
		if i > 0 && tag <= previous {
			r.err = fmt.Errorf("kafka: duplicate or unordered tag")
			return
		}
		previous = tag
		if size > uint32(len(r.wire)-r.at) {
			r.err = fmt.Errorf("kafka: tag exceeds window")
			return
		}
		data := r.take("Tag Data", "raw", int(size))
		row := map[string]any{"Tag": tag, "Data": data, "Known": false}
		sub := &kafkaReader{wire: data, elements: r.elements}
		if known != nil && known(tag, sub) {
			row["Known"] = true
			r.elements = sub.elements
			if sub.err != nil {
				r.err = sub.err
				return
			}
			if sub.at != len(data) {
				r.err = fmt.Errorf("kafka: trailing known tag bytes")
				return
			}
		}
		tags = append(tags, row)
	}
	if len(tags) > 0 {
		m["Tagged Fields"] = tags
	}
}
func (r *kafkaReader) flexibleRecords(m map[string]any) {
	n := r.compactLength("Record Set", true, false)
	m["Record Set Null"] = n < 0
	if n < 0 || r.err != nil {
		return
	}
	data := r.take("Record Set", "raw", n)
	if r.recordBudget == nil {
		r.recordBudget = &kafkaRecordBudget{kafkaMaxBytes, 4096}
	}
	batches, err := kafkaRecords(data, 0, &r.recordBudget.bytes, &r.recordBudget.count)
	if err != nil {
		r.err = err
		return
	}
	m["Batches"] = batches
	if len(batches) > 0 {
		for k, v := range batches[0] {
			m[k] = v
		}
	}
}
func (r *kafkaReader) flexibleRequest(api int16, m map[string]any) {
	switch api {
	case 18:
		m["Client Software Name"] = r.compactString("Client Software Name", false)
		m["Client Software Version"] = r.compactString("Client Software Version", false)
	case 3:
		n := r.compactLength("Topics", true, true)
		m["All Topics"] = n < 0
		var names []any
		var topics []map[string]any
		for ; n > 0 && r.err == nil; n-- {
			t := map[string]any{"Topic Name": r.compactString("Topic Name", false)}
			r.tags(t, nil)
			names = append(names, t["Topic Name"])
			topics = append(topics, t)
		}
		m["Topics"], m["Topic Results"] = names, topics
		m["Allow Auto Topic Creation"] = r.boolean("Allow Auto Topic Creation")
		m["Include Cluster Authorized Operations"] = r.boolean("Include Cluster Authorized Operations")
		m["Include Topic Authorized Operations"] = r.boolean("Include Topic Authorized Operations")
	case 0, 1:
		if api == 0 {
			m["Transactional ID"] = r.compactString("Transactional ID", true)
			m["Acks"] = r.i16("Acks")
			if a := m["Acks"].(int16); a < -1 || a > 1 {
				r.err = fmt.Errorf("kafka: invalid acks")
				return
			}
			m["Timeout"] = r.i32("Timeout")
		} else {
			m["Replica ID"] = r.i32("Replica ID")
			m["Max Wait Time"] = r.i32("Max Wait Time")
			m["Min Bytes"] = r.i32("Min Bytes")
			m["Max Bytes"] = r.i32("Max Bytes")
			m["Isolation Level"] = r.i8("Isolation Level")
			if v := m["Isolation Level"].(int8); v < 0 || v > 1 {
				r.err = fmt.Errorf("kafka: isolation level")
				return
			}
			m["Session ID"] = r.i32("Session ID")
			m["Session Epoch"] = r.i32("Session Epoch")
		}
		var topics []map[string]any
		for n := r.compactLength("Topics", false, true); n > 0 && r.err == nil; n-- {
			t := map[string]any{"Topic Name": r.compactString("Topic Name", false)}
			var parts []map[string]any
			for c := r.compactLength("Partitions", false, true); c > 0 && r.err == nil; c-- {
				p := map[string]any{"Partition": r.i32("Partition")}
				if api == 0 {
					r.flexibleRecords(p)
				} else {
					p["Current Leader Epoch"] = r.i32("Current Leader Epoch")
					p["Fetch Offset"] = r.i64("Fetch Offset")
					p["Last Fetched Epoch"] = r.i32("Last Fetched Epoch")
					p["Log Start Offset"] = r.i64("Log Start Offset")
					p["Partition Max Bytes"] = r.i32("Partition Max Bytes")
				}
				r.tags(p, nil)
				parts = append(parts, p)
			}
			t["Partitions"] = parts
			r.tags(t, nil)
			topics = append(topics, t)
		}
		m["Topic Results"] = topics
		if api == 1 {
			var forgotten []map[string]any
			for n := r.compactLength("Forgotten Topics", false, true); n > 0 && r.err == nil; n-- {
				t := map[string]any{"Topic Name": r.compactString("Forgotten Topic Name", false), "Partitions": r.compactInts("Forgotten Partitions")}
				r.tags(t, nil)
				forgotten = append(forgotten, t)
			}
			m["Forgotten Topics"] = forgotten
			m["Rack ID"] = r.compactString("Rack ID", false)
		}
	}
	r.tags(m, func(tag uint32, s *kafkaReader) bool {
		if api == 1 && tag == 0 {
			m["Cluster ID"] = s.compactString("Cluster ID", true)
			return true
		}
		return false
	})
}
func (r *kafkaReader) flexibleResponse(api int16, m map[string]any) {
	switch api {
	case 18:
		m["Error Code"] = r.i16("Error Code")
		var entries []map[string]any
		var keys []int16
		for n := r.compactLength("API Keys", false, true); n > 0 && r.err == nil; n-- {
			e := map[string]any{"API Key": r.i16("API Key"), "Min Version": r.i16("Min Version"), "Max Version": r.i16("Max Version")}
			r.tags(e, nil)
			entries = append(entries, e)
			keys = append(keys, e["API Key"].(int16))
		}
		m["API Keys"], m["API Versions"] = keys, entries
		m["Throttle Time"] = r.i32("Throttle Time")
	case 3:
		m["Throttle Time"] = r.i32("Throttle Time")
		var brokers []map[string]any
		for n := r.compactLength("Brokers", false, true); n > 0 && r.err == nil; n-- {
			b := map[string]any{"Node ID": r.i32("Node ID"), "Host": r.compactString("Host", false), "Port": r.i32("Port"), "Rack": r.compactString("Rack", true)}
			r.tags(b, nil)
			brokers = append(brokers, b)
		}
		m["Brokers"] = brokers
		m["Cluster ID"] = r.compactString("Cluster ID", true)
		m["Controller ID"] = r.i32("Controller ID")
		var topics []map[string]any
		var names []any
		for n := r.compactLength("Topics", false, true); n > 0 && r.err == nil; n-- {
			t := map[string]any{"Error Code": r.i16("Topic Error"), "Topic Name": r.compactString("Topic Name", false), "Internal": r.boolean("Internal")}
			names = append(names, t["Topic Name"])
			var parts []map[string]any
			for c := r.compactLength("Partitions", false, true); c > 0 && r.err == nil; c-- {
				p := map[string]any{"Error Code": r.i16("Partition Error"), "Partition": r.i32("Partition"), "Leader": r.i32("Leader"), "Leader Epoch": r.i32("Leader Epoch"), "Replicas": r.compactInts("Replicas"), "ISR": r.compactInts("ISR"), "Offline Replicas": r.compactInts("Offline Replicas")}
				r.tags(p, nil)
				parts = append(parts, p)
			}
			t["Partitions"] = parts
			t["Authorized Operations"] = r.i32("Topic Authorized Operations")
			r.tags(t, nil)
			topics = append(topics, t)
		}
		m["Topics"], m["Topic Results"] = names, topics
		m["Cluster Authorized Operations"] = r.i32("Cluster Authorized Operations")
	case 0, 1:
		if api == 1 {
			m["Throttle Time"] = r.i32("Throttle Time")
			m["Error Code"] = r.i16("Error Code")
			m["Session ID"] = r.i32("Session ID")
		}
		var topics []map[string]any
		for n := r.compactLength("Topics", false, true); n > 0 && r.err == nil; n-- {
			t := map[string]any{"Topic Name": r.compactString("Topic Name", false)}
			var parts []map[string]any
			for c := r.compactLength("Partitions", false, true); c > 0 && r.err == nil; c-- {
				p := map[string]any{"Partition": r.i32("Partition"), "Error Code": r.i16("Error Code")}
				if api == 0 {
					p["Base Offset"] = r.i64("Base Offset")
					p["Log Append Time"] = r.i64("Log Append Time")
					p["Log Start Offset"] = r.i64("Log Start Offset")
					var errors []map[string]any
					for e := r.compactLength("Record Errors", false, true); e > 0 && r.err == nil; e-- {
						v := map[string]any{"Batch Index": r.i32("Batch Index"), "Error Message": r.compactString("Batch Error Message", true)}
						r.tags(v, nil)
						errors = append(errors, v)
					}
					p["Record Errors"] = errors
					p["Error Message"] = r.compactString("Error Message", true)
				} else {
					p["High Watermark"] = r.i64("High Watermark")
					p["Last Stable Offset"] = r.i64("Last Stable Offset")
					p["Log Start Offset"] = r.i64("Log Start Offset")
					n := r.compactLength("Aborted Transactions", true, true)
					p["Aborted Transactions Null"] = n < 0
					var aborted []map[string]any
					for ; n > 0 && r.err == nil; n-- {
						v := map[string]any{"Producer ID": r.i64("Producer ID"), "First Offset": r.i64("First Offset")}
						r.tags(v, nil)
						aborted = append(aborted, v)
					}
					p["Aborted Transactions"] = aborted
					p["Preferred Read Replica"] = r.i32("Preferred Read Replica")
					r.flexibleRecords(p)
				}
				r.tags(p, func(tag uint32, s *kafkaReader) bool {
					if api != 1 || tag > 2 {
						return false
					}
					v := map[string]any{}
					name := ""
					switch tag {
					case 0:
						name = "Diverging Epoch"
						v["Epoch"] = s.i32("Epoch")
						v["End Offset"] = s.i64("End Offset")
					case 1:
						name = "Current Leader"
						v["Leader ID"] = s.i32("Leader ID")
						v["Leader Epoch"] = s.i32("Leader Epoch")
					case 2:
						name = "Snapshot ID"
						v["End Offset"] = s.i64("End Offset")
						v["Epoch"] = s.i32("Epoch")
					}
					s.tags(v, nil)
					p[name] = v
					return true
				})
				parts = append(parts, p)
			}
			t["Partitions"] = parts
			r.tags(t, nil)
			topics = append(topics, t)
		}
		m["Topic Results"] = topics
		if api == 0 {
			m["Throttle Time"] = r.i32("Throttle Time")
		}
	}
	r.tags(m, func(tag uint32, s *kafkaReader) bool {
		if api != 18 || tag > 3 {
			return false
		}
		switch tag {
		case 0, 2:
			var features []map[string]any
			for n := s.compactLength("Features", false, true); n > 0 && s.err == nil; n-- {
				v := map[string]any{"Name": s.compactString("Feature Name", false)}
				if tag == 0 {
					v["Min Version"] = s.i16("Min Version")
					v["Max Version"] = s.i16("Max Version")
				} else {
					v["Max Version"] = s.i16("Max Version")
					v["Min Version"] = s.i16("Min Version")
				}
				s.tags(v, nil)
				features = append(features, v)
			}
			if tag == 0 {
				m["Supported Features"] = features
			} else {
				m["Finalized Features"] = features
			}
		case 1:
			m["Finalized Features Epoch"] = s.i64("Finalized Features Epoch")
		case 3:
			m["ZK Migration Ready"] = s.boolean("ZK Migration Ready")
		}
		return true
	})
}

// KafkaResponsePayloadVersion accepts the bytes after correlation ID. The
// flexible ApiVersions response deliberately retains header v0 (no tag buffer).
func KafkaResponsePayloadVersion(api, ver int16, body []byte) (map[string]any, error) {
	if !KafkaFlexibleVersion(api, ver) {
		return KafkaResponseBodyVersion(api, ver, body)
	}
	return kafkaFlexibleResponse(api, ver, body, true)
}
func kafkaFlexibleResponse(api, ver int16, body []byte, header bool) (map[string]any, error) {
	if len(body) > kafkaMaxBytes {
		return nil, fmt.Errorf("kafka: response byte budget")
	}
	r := &kafkaReader{wire: body}
	m := map[string]any{"API Name": kafkaAPIName(api), "API Version": ver, "Flexible": true, "Header Version": int16(0)}
	if api != 18 {
		m["Header Version"] = int16(1)
	}
	if header && api != 18 {
		h := map[string]any{}
		r.tags(h, nil)
		m["Response Header"] = h
	}
	r.flexibleResponse(api, m)
	if r.err != nil {
		return nil, r.err
	}
	if r.at != len(body) {
		return nil, fmt.Errorf("kafka: trailing flexible response bytes")
	}
	return m, nil
}
