package stream_parser

import (
	"encoding/hex"
	"fmt"
	"net"
	"unicode/utf8"
)

func isisTLVs(r *controlReader, pdu uint64, limit *int) {
	var rows []map[string]any
	defer func() {
		delete(r.info, "TLV Type")
		delete(r.info, "TLV Length")
		r.info["TLVs"] = rows
	}()
	optional := 0
	for r.at < len(r.w) && r.err == nil {
		if !pimCharge(r, limit) {
			return
		}
		start := r.at
		typ := r.num("TLV Type", 1, false)
		n := r.num("TLV Length", 1, false)
		r.need(n <= uint64(len(r.w)-r.at), "IS-IS TLV exceeds declared PDU")
		if r.err != nil {
			return
		}
		known := true
		row := r.child("TLV", r.at+int(n), func(c *controlReader) {
			known = isisTLVValue(c, pdu, typ, limit)
			c.info["Wire Field Ranges"] = isisFieldRanges(c.fields)
		})
		if r.err != nil {
			// The envelope was bounded before child decoding. Its type, literal
			// length and captured value are evidence; failed child fields are not.
			rows = append(rows, map[string]any{"Type": typ, "Length": n, "TLV Name": "Malformed TLV value", "Decode Failure": r.err.Error(),
				"Opaque Value":      append([]byte(nil), r.w[start+2:start+2+int(n)]...),
				"Wire Field Ranges": map[string]any{"Type": []any{uint64(start), uint64(start + 1)}, "Length": []any{uint64(start + 1), uint64(start + 2)}, "Opaque Value": []any{uint64(start + 2), uint64(start + 2 + int(n))}}})
			return
		}
		row["Type"], row["Length"] = typ, n
		spans := row["Wire Field Ranges"].(map[string]any)
		spans["Type"], spans["Length"] = []any{uint64(start), uint64(start + 1)}, []any{uint64(start + 1), uint64(start + 2)}
		if !known || row["Unselected Extensions"] == true {
			r.info["Unselected Extensions"] = true
		}
		if typ == 12 {
			optional++
			r.need(optional == 1 && pdu != 18 && pdu != 20, "RFC3358 optional checksum is unique and limited to IIH/CSNP/PSNP")
			checksum, _ := row["Checksum Value"].(uint64)
			valid := checksum == 0 || isisFletcherValid(r.w)
			row["Checksum Accepted"], row["Checksum Verified"] = valid, checksum != 0 && valid
			if checksum == 0 {
				row["Checksum Observation"] = "zero accepted by RFC3358; no integrity evidence"
			} else {
				row["Checksum Observation"] = "complete PDU Fletcher check"
			}
			if !valid {
				r.info["Optional Checksum Failure"] = true
			}
		}
		rows = append(rows, row)
	}
	delete(r.info, "TLV Type")
	delete(r.info, "TLV Length")
	r.info["TLVs"] = rows
}

func isisTLVValue(r *controlReader, pdu, typ uint64, limit *int) bool {
	remaining := len(r.w) - r.at
	if !isisTLVContextSelected(pdu, typ) {
		r.info["TLV Name"] = "Unselected TLV in this PDU context"
		r.rest("Opaque Value")
		return false
	}
	switch typ {
	case 1:
		r.info["TLV Name"] = "Area Addresses"
		var addresses []map[string]any
		for r.at < len(r.w) && r.err == nil {
			if !pimCharge(r, limit) {
				return true
			}
			start := r.at
			n := r.num("Area Address Length", 1, false)
			r.need(n >= 1 && n <= 20, "IS-IS area address length1..20")
			b := r.take("Area Address Bytes", int(n))
			if r.err == nil {
				encoded := fmt.Sprintf("%02x", n)
				for _, v := range b {
					encoded += fmt.Sprintf(":%02x", v)
				}
				addresses = append(addresses, map[string]any{"Address": isisAreaName(b), "Length": n, "Length-prefixed Address Octets": encoded, "Bytes": append([]byte(nil), b...), "Wire Field Ranges": map[string]any{
					"Length": []any{uint64(start), uint64(start + 1)}, "Length-prefixed Address Octets": []any{uint64(start), uint64(r.at)}, "Bytes": []any{uint64(start + 1), uint64(r.at)}, "Address": []any{uint64(start + 1), uint64(r.at)}}})
			}
		}
		r.info["Area Addresses"] = addresses
		delete(r.info, "Area Address Length")
		delete(r.info, "Area Address Bytes")
	case 2, 3:
		isisLegacyNeighbors(r, typ, pdu, limit)
	case 6:
		r.info["TLV Name"] = "Advertised IS Neighbor SNPAs"
		r.need(remaining%6 == 0, "IS neighbor list requires complete six-octet SNPAs")
		var rows []map[string]any
		for r.at < len(r.w) && r.err == nil {
			if !pimCharge(r, limit) {
				return true
			}
			row := r.child("IS Neighbor", r.at+6, func(c *controlReader) {
				b := c.take("SNPA", 6)
				if c.err == nil {
					c.info["SNPA"] = net.HardwareAddr(b).String()
				}
				c.info["Wire Field Ranges"] = isisFieldRanges(c.fields)
			})
			rows = append(rows, row)
		}
		r.info["IS Neighbors"] = rows
	case 8:
		r.info["TLV Name"] = "Padding"
		r.rest("Padding Bytes")
	case 9:
		r.info["TLV Name"] = "Advertised LSP Entries"
		r.need(remaining%16 == 0, "IS-IS SNP entries require complete sixteen-octet records")
		var rows []map[string]any
		for r.at < len(r.w) && r.err == nil {
			if !pimCharge(r, limit) {
				return true
			}
			row := r.child("LSP Entry", r.at+16, func(c *controlReader) {
				c.num("Remaining Lifetime Seconds", 2, false)
				isisID(c, "LSP ID", 8)
				c.num("Sequence Number", 4, false)
				c.num("Reported Checksum", 2, false)
				c.info["Observation Scope"] = "advertised LSP metadata; referenced LSP bytes and database membership not proved"
				c.info["Wire Field Ranges"] = isisFieldRanges(c.fields)
			})
			rows = append(rows, row)
		}
		r.info["LSP Entries"] = rows
	case 10:
		r.info["TLV Name"] = "Authentication (unverified)"
		r.need(remaining >= 1, "IS-IS authentication type missing")
		typ := r.num("Authentication Type", 1, false)
		if typ == 54 {
			r.need(remaining == 17, "IS-IS HMAC-MD5 authentication requires sixteen value octets")
		}
		r.rest("Authentication Value")
		r.info["Authentication Verified"], r.info["Contains Sensitive Value"] = false, typ == 1
		if typ != 1 && typ != 54 {
			r.info["Unselected Extensions"] = true
		}
	case 12:
		r.info["TLV Name"] = "Optional PDU Checksum"
		r.need(remaining == 2, "IS-IS optional checksum requires two octets")
		r.num("Checksum Value", 2, false)
	case 14:
		r.info["TLV Name"] = "Advertised LSP Buffer Size"
		r.need(remaining == 2, "IS-IS buffer size requires two octets")
		r.num("Buffer Size", 2, false)
	case 22:
		isisExtendedNeighbors(r, limit)
	case 128, 130:
		isisLegacyIPReachability(r, typ, limit)
	case 129:
		r.info["TLV Name"] = "Advertised Protocols Supported"
		var rows []map[string]any
		for r.at < len(r.w) && r.err == nil {
			if !pimCharge(r, limit) {
				return true
			}
			start := r.at
			v := r.num("NLPID", 1, false)
			name := map[uint64]string{0x81: "CLNP", 0xcc: "IPv4", 0x8e: "IPv6"}[v]
			if name == "" {
				name = "Unselected NLPID"
			}
			rows = append(rows, map[string]any{"NLPID": v, "Name": name, "Wire Field Ranges": map[string]any{"NLPID": []any{uint64(start), uint64(r.at)}}})
		}
		delete(r.info, "NLPID")
		r.info["Protocols Supported"] = rows
	case 132, 232:
		n := 4
		if typ == 232 {
			n = 16
		}
		version := 4
		if n == 16 {
			version = 6
		}
		r.info["TLV Name"] = fmt.Sprintf("Advertised IPv%d Interface Addresses", version)
		r.need(remaining%n == 0, "IS-IS interface address list contains a partial address")
		var rows []map[string]any
		for r.at < len(r.w) && r.err == nil {
			if !pimCharge(r, limit) {
				return true
			}
			row := r.child("Interface Address", r.at+n, func(c *controlReader) {
				routingAddress(c, "Address", n)
				c.info["Wire Field Ranges"] = isisFieldRanges(c.fields)
			})
			rows = append(rows, row)
		}
		r.info["Interface Addresses"] = rows
	case 134:
		r.info["TLV Name"] = "Advertised TE Router ID"
		r.need(remaining == 4, "IS-IS TE router ID requires four octets")
		routingAddress(r, "TE Router ID", 4)
	case 135, 236:
		isisExtendedIPReachability(r, typ == 236, limit)
	case 137:
		r.info["TLV Name"] = "Advertised Dynamic Hostname"
		r.need(remaining > 0, "IS-IS hostname cannot be empty")
		b := r.rest("Hostname Bytes")
		if utf8.Valid(b) {
			r.info["Hostname"] = string(b)
		} else {
			r.info["Hostname Escaped"] = fmt.Sprintf("%q", b)
			r.info["Unselected Extensions"] = true
			r.info["Conformance Warning"] = "hostname text is invalid UTF8; original octets and escaped display retained"
		}
	case 211:
		if pdu != 15 && pdu != 16 && pdu != 17 {
			break
		}
		r.info["TLV Name"] = "Advertised Restart Signaling"
		r.need(remaining == 3 || remaining == 9, "IS-IS restart signaling length must be3 or9")
		flags := r.num("Restart Flags", 1, false)
		r.info["Restart Requested"], r.info["Restart Acknowledged"], r.info["Suppress Adjacency"] = flags&1 != 0, flags&2 != 0, flags&4 != 0
		r.num("Remaining Holding Time Seconds", 2, false)
		if remaining == 9 {
			isisID(r, "Restarting Neighbor ID", 6)
		}
	case 240:
		if pdu != 17 {
			break
		}
		r.info["TLV Name"] = "Advertised Point-to-Point Adjacency State"
		r.need(remaining == 1 || remaining == 5 || remaining == 11 || remaining == 15, "IS-IS point-to-point state length outside selected profile")
		v := r.num("Adjacency State", 1, false)
		name := "Unknown (uninterpreted)"
		if v <= 2 {
			name = []string{"Up (advertised)", "Initializing (advertised)", "Down (advertised)"}[v]
		}
		r.info["Adjacency State Name"] = name
		if remaining >= 5 {
			r.num("Extended Local Circuit ID", 4, false)
		}
		if remaining >= 11 {
			isisID(r, "Neighbor System ID", 6)
		}
		if remaining == 15 {
			r.num("Neighbor Extended Local Circuit ID", 4, false)
		}
	default:
		break
	}
	if _, known := r.info["TLV Name"]; !known {
		r.info["TLV Name"] = "Unselected TLV or PDU context"
		r.rest("Opaque Value")
		return false
	}
	return true
}

func isisTLVContextSelected(pdu, typ uint64) bool {
	hello := pdu == 15 || pdu == 16 || pdu == 17
	lsp := pdu == 18 || pdu == 20
	snp := pdu == 24 || pdu == 25 || pdu == 26 || pdu == 27
	switch typ {
	case 1, 129, 132, 232:
		return hello || lsp
	case 2, 3, 14, 22, 128, 130, 134, 135, 137, 236:
		return lsp
	case 6:
		return pdu == 15 || pdu == 16
	case 9:
		return snp
	case 211:
		return hello
	case 240:
		return pdu == 17
	default:
		return true
	}
}

func isisAreaName(w []byte) string {
	if len(w) == 0 {
		return ""
	}
	s := hex.EncodeToString(w[:1])
	for at := 1; at < len(w); at += 2 {
		end := at + 2
		if end > len(w) {
			end = len(w)
		}
		s += "." + hex.EncodeToString(w[at:end])
	}
	return s
}
