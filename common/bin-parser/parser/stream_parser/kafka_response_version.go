package stream_parser

import "fmt"

// KafkaResponseBodyVersion uses the version from the observed request. A
// response never carries that version itself. Flexible versions are separate.
func KafkaResponseBodyVersion(api, ver int16, body []byte) (map[string]any, error) {
	rng, ok := kafkaAPIVersions[api]
	if !ok || ver < rng[0] || ver > rng[1] {
		return nil, fmt.Errorf("%w: API/version %d/%d", ErrKafkaUnsupported, api, ver)
	}
	r := &kafkaReader{wire: body}
	m := map[string]any{"API Name": kafkaAPIName(api), "API Version": ver}
	switch api {
	case 18:
		m["Error Code"] = r.i16("Error Code")
		var keys []int16
		var entries []map[string]any
		for n := r.count("API Keys Count", false); n > 0 && r.err == nil; n-- {
			k := r.i16("API Key")
			keys = append(keys, k)
			entries = append(entries, map[string]any{"API Key": k, "Min Version": r.i16("Min Version"), "Max Version": r.i16("Max Version")})
		}
		m["API Keys"] = keys
		m["API Versions"] = entries
		if ver >= 1 {
			m["Throttle Time"] = r.i32("Throttle Time")
		}
	case 3:
		if ver >= 3 {
			m["Throttle Time"] = r.i32("Throttle Time")
		}
		var brokers []map[string]any
		for n := r.count("Broker Count", false); n > 0 && r.err == nil; n-- {
			b := map[string]any{"Node ID": r.i32("Node ID"), "Host": r.requiredString("Host"), "Port": r.i32("Port")}
			if ver >= 1 {
				b["Rack"] = r.str("Rack")
			}
			brokers = append(brokers, b)
		}
		m["Brokers"] = brokers
		if ver >= 2 {
			m["Cluster ID"] = r.str("Cluster ID")
		}
		if ver >= 1 {
			m["Controller ID"] = r.i32("Controller ID")
		}
		var names []string
		var topics []map[string]any
		for n := r.count("Topic Count", false); n > 0 && r.err == nil; n-- {
			t := map[string]any{"Error Code": r.i16("Topic Error"), "Topic Name": r.requiredString("Topic Name")}
			names = append(names, t["Topic Name"].(string))
			if ver >= 1 {
				t["Internal"] = r.boolean("Internal")
			}
			var parts []map[string]any
			for c := r.count("Partition Count", false); c > 0 && r.err == nil; c-- {
				p := map[string]any{"Error Code": r.i16("Partition Error"), "Partition": r.i32("Partition"), "Leader": r.i32("Leader")}
				if ver >= 7 {
					p["Leader Epoch"] = r.i32("Leader Epoch")
				}
				p["Replicas"] = r.intArray("Replicas")
				p["ISR"] = r.intArray("ISR")
				if ver >= 5 {
					p["Offline Replicas"] = r.intArray("Offline Replicas")
				}
				parts = append(parts, p)
			}
			t["Partitions"] = parts
			if ver >= 8 {
				t["Authorized Operations"] = r.i32("Topic Authorized Operations")
			}
			topics = append(topics, t)
		}
		m["Topics"] = names
		m["Topic Results"] = topics
		if ver >= 8 {
			m["Cluster Authorized Operations"] = r.i32("Cluster Authorized Operations")
		}
	case 0, 1:
		if api == 1 && ver >= 1 {
			m["Throttle Time"] = r.i32("Throttle Time")
		}
		if api == 1 && ver >= 7 {
			m["Error Code"] = r.i16("Error Code")
			m["Session ID"] = r.i32("Session ID")
		}
		var topics []map[string]any
		for n := r.count("Topic Count", false); n > 0 && r.err == nil; n-- {
			t := map[string]any{"Topic Name": r.requiredString("Topic Name")}
			var parts []map[string]any
			for c := r.count("Partition Count", false); c > 0 && r.err == nil; c-- {
				p := map[string]any{"Partition": r.i32("Partition"), "Error Code": r.i16("Error Code")}
				if api == 0 {
					p["Base Offset"] = r.i64("Base Offset")
					if ver >= 2 {
						p["Log Append Time"] = r.i64("Log Append Time")
					}
					if ver >= 5 {
						p["Log Start Offset"] = r.i64("Log Start Offset")
					}
				} else {
					p["High Watermark"] = r.i64("High Watermark")
					if ver >= 4 {
						p["Last Stable Offset"] = r.i64("Last Stable Offset")
					}
					if ver >= 5 {
						p["Log Start Offset"] = r.i64("Log Start Offset")
					}
					if ver >= 4 {
						var aborted []map[string]any
						for a := r.count("Aborted Transactions", true); a > 0 && r.err == nil; a-- {
							aborted = append(aborted, map[string]any{"Producer ID": r.i64("Producer ID"), "First Offset": r.i64("First Offset")})
						}
						p["Aborted Transactions"] = aborted
					}
					if ver >= 11 {
						p["Preferred Read Replica"] = r.i32("Preferred Read Replica")
					}
					set := r.bytes("Record Set")
					if r.err == nil {
						if r.recordBudget == nil {
							r.recordBudget = &kafkaRecordBudget{kafkaMaxBytes, 4096}
						}
						b, err := kafkaRecords(set, 0, &r.recordBudget.bytes, &r.recordBudget.count)
						if err != nil {
							return nil, err
						}
						p["Batches"] = b
						if len(b) > 0 {
							for k, v := range b[0] {
								p[k] = v
							}
						}
					}
				}
				if len(topics) == 0 && len(parts) == 0 {
					m["Topic Name"] = t["Topic Name"]
					for k, v := range p {
						m[k] = v
					}
				}
				parts = append(parts, p)
			}
			t["Partitions"] = parts
			topics = append(topics, t)
		}
		m["Topic Results"] = topics
		if api == 0 && ver >= 1 {
			m["Throttle Time"] = r.i32("Throttle Time")
		}
	}
	if r.err != nil {
		return nil, r.err
	}
	if r.at != len(body) {
		return nil, fmt.Errorf("kafka: trailing response bytes")
	}
	return m, nil
}
func (r *kafkaReader) count(name string, nullable bool) int {
	n := r.i32(name)
	if n < 0 && !(nullable && n == -1) || n > int32(4096-r.elements) {
		r.err = fmt.Errorf("kafka: invalid collection %s", name)
		return 0
	}
	if n > 0 {
		r.elements += int(n)
	}
	return int(n)
}
func (r *kafkaReader) intArray(name string) []int32 {
	var a []int32
	for n := r.count(name, false); n > 0 && r.err == nil; n-- {
		a = append(a, r.i32(name))
	}
	return a
}
func (r *kafkaReader) requiredString(name string) string {
	n := int(r.i16(name + " Length"))
	if n < 0 {
		r.err = fmt.Errorf("kafka: null %s", name)
		return ""
	}
	return string(r.take(name, "string", n))
}
