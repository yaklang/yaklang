package stream_parser

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// decodeRedfishServiceRootTarget implements the request-only service-root
// profile in DMTF DSP0266 1.23.1 sections 6.6/6.7, 7.2.3 and 7.3.1-7.3.3.
// It never infers a service's implementation/version/capability from a URI.
// Bounds and restrictions below are an implementation profile, not global
// Redfish limits: origin-form, <=8192 bytes, <=256 nonempty query pairs,
// single occurrences of recognized parameters, uint32 expansion levels and
// ASCII property-path selection. Non-dollar unknown parameters remain raw;
// collection-only and other dollar parameters are outside this entry's scope.
func decodeRedfishServiceRootTarget(target string) (map[string]any, error) {
	fail := func(reason string) (map[string]any, error) {
		return nil, fmt.Errorf("redfish service-root: %s", reason)
	}
	if len(target) == 0 || len(target) > 8192 {
		return fail("target outside 1..8192-byte implementation profile")
	}
	path, query, queryPresent := strings.Cut(target, "?")
	if path != "/redfish/v1/" && path != "/redfish/v1" {
		return fail("requires origin-form /redfish/v1[/] service-root route")
	}
	isHex := func(c byte) bool { return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' }
	for i := 0; i < len(query); i++ {
		c := query[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-._~!$&'()*+,;=:@/?", rune(c)) {
			continue
		}
		if c == '%' && i+2 < len(query) && isHex(query[i+1]) && isHex(query[i+2]) {
			i += 2
			continue
		}
		return fail("invalid query URI octet or percent escape")
	}
	info := map[string]any{
		"Service Root Path": "/redfish/v1/", "Request Path": path, "Trailing Slash Present": strings.HasSuffix(path, "/"),
		"Major Version": uint64(1), "Full Protocol Version Observed": false, "ServiceRoot Schema Parsed": false,
		"Redirect Observed": false, "Service Capabilities Observed": false, "Query Semantics Evaluated": false,
		"Raw Query": query, "Query Present": queryPresent,
		"Query Name Matching": "case-insensitive implementation profile", "Standard Query Duplicate Policy": "single occurrence implementation profile",
		"Parameter Span Coordinate System": "request-target-relative-bits", "Major Version Target Bit Span": [2]uint64{80, 88},
		"Excerpt": false, "Include Origin Of Condition": false, "Expand Present": false, "Select Present": false,
	}
	pairs := []map[string]any{}
	values := map[string][]string{}
	unknown := map[string][]string{}
	seen := map[string]bool{}
	cursor := len(path) + 1
	for _, raw := range strings.Split(query, "&") {
		if raw == "" {
			cursor++
			continue
		}
		if len(pairs) == 256 {
			return fail("query exceeds 256-pair implementation profile")
		}
		key, value, equal := strings.Cut(raw, "=")
		// Unlike HTML form parsing, Redfish/OData URI query values use URI
		// percent decoding; a literal plus is not silently changed to space.
		name, err := url.PathUnescape(key)
		if err != nil {
			return fail("invalid query name escape")
		}
		decoded, err := url.PathUnescape(value)
		if err != nil {
			return fail("invalid query value escape")
		}
		lower := strings.ToLower(name)
		values[name] = append(values[name], decoded)
		valueStart := cursor + len(key)
		if equal {
			valueStart++
		}
		pair := map[string]any{
			"Name": name, "Value": decoded, "Raw Name": key, "Raw Value": value, "Equals Present": equal,
			"Name Target Bit Span":  [2]uint64{uint64(cursor) * 8, uint64(cursor+len(key)) * 8},
			"Value Target Bit Span": [2]uint64{uint64(valueStart) * 8, uint64(cursor+len(raw)) * 8},
		}
		switch lower {
		case "excerpt", "includeoriginofcondition", "$expand", "$select":
			if seen[lower] {
				return fail("repeated standard query parameter outside single-occurrence profile")
			}
			seen[lower] = true
			pair["Recognized"] = true
		case "$filter", "$top", "$skip", "only":
			return fail("collection-only query parameter outside service-root profile")
		default:
			if strings.HasPrefix(name, "$") {
				return fail("unsupported dollar-prefixed query parameter outside profile")
			}
			pair["Recognized"] = false
			unknown[name] = append(unknown[name], decoded)
		}
		switch lower {
		case "excerpt", "includeoriginofcondition":
			if decoded != "" {
				return fail("flag query parameter must not contain a value")
			}
			if lower == "excerpt" {
				info["Excerpt"] = true
			} else {
				info["Include Origin Of Condition"] = true
			}
		case "$expand":
			if len(decoded) == 0 || !strings.ContainsRune("*.~", rune(decoded[0])) {
				return fail("unsupported $expand mode outside Redfish option profile")
			}
			level, explicit := uint64(1), false
			if len(decoded) > 1 {
				if !strings.HasPrefix(decoded[1:], "($levels=") || !strings.HasSuffix(decoded, ")") {
					return fail("unsupported $expand levels expression")
				}
				digits := decoded[10 : len(decoded)-1]
				if digits == "" || strings.Trim(digits, "0123456789") != "" {
					return fail("$levels requires unsigned decimal integer")
				}
				var err error
				level, err = strconv.ParseUint(digits, 10, 32)
				if err != nil {
					return fail("$levels exceeds uint32 implementation profile")
				}
				explicit = true
			}
			info["Expand Present"], info["Expand Mode"], info["Expand Levels"], info["Expand Levels Present"] = true, decoded[:1], level, explicit
		case "$select":
			paths := strings.Split(decoded, ",")
			for _, path := range paths {
				for _, property := range strings.Split(path, "/") {
					if property == "" {
						return fail("$select requires nonempty property paths")
					}
					for i, b := range []byte(property) {
						if b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b == '_' || b == '@' || (i > 0 && (b >= '0' && b <= '9' || b == '.')) {
							continue
						}
						return fail("$select outside ASCII property-path profile (no indexes or wildcards)")
					}
				}
			}
			info["Select Present"], info["Selected Property Paths"] = true, paths
		}
		pairs = append(pairs, pair)
		cursor += len(raw) + 1
	}
	info["Query Pairs"], info["Query Values"], info["Unknown Query Values"], info["Query Pair Count"] = pairs, values, unknown, len(pairs)
	return info, nil
}
