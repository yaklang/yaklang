package stream_parser

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// decodeDockerContainerListTarget decodes the bounded request-parameter codec,
// not a daemon response or an execution result. Primary layout: Docker Engine
// v1.41 OpenAPI /containers/json GET. Receiver compatibility is pinned to Moby
// eeddea2f9026b9b3e6a14b8bdb40bafec81ef10a (v20.10.0):
// api/server/router/container/container_routes.go:getContainersJSON,
// api/server/httputils/form.go:BoolValue, api/types/filters/parse.go:FromJSON.
// In particular, first query values win; booleans have receiver-compatible
// spelling; filter arrays and boolean sets (including false/null) are retained.
// The 8192-byte target and 256 nonempty query-pair limits, origin-form route,
// ampersand separators, and signed 64-bit integers are implementation bounds.
// Filter value evaluation, object lookup, platform checks and list results are
// intentionally not inferred from a captured request.
func decodeDockerContainerListTarget(target string) (map[string]any, error) {
	const path = "/v1.41/containers/json"
	fail := func(reason string) (map[string]any, error) {
		return nil, fmt.Errorf("docker container-list: %s", reason)
	}
	if len(target) == 0 || len(target) > 8192 {
		return fail("target outside 1..8192-byte implementation profile")
	}
	parts := strings.SplitN(target, "?", 2)
	if parts[0] != path {
		return fail("requires origin-form /v1.41/containers/json route")
	}
	rawQuery := ""
	if len(parts) == 2 {
		rawQuery = parts[1]
	}
	isHex := func(c byte) bool { return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' }
	for i := 0; i < len(rawQuery); i++ {
		c := rawQuery[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("-._~!$&'()*+,=:@/?", rune(c)) {
			continue
		}
		if c == '%' && i+2 < len(rawQuery) && isHex(rawQuery[i+1]) && isHex(rawQuery[i+2]) {
			i += 2
			continue
		}
		return fail("query outside URI-octet/percent-escape/ampersand-separator profile")
	}
	values := make(map[string][]string)
	pairs := make([]map[string]any, 0)
	cursor := len(path) + 1
	for _, raw := range strings.Split(rawQuery, "&") {
		if raw == "" {
			cursor++
			continue
		}
		if len(pairs) == 256 {
			return fail("query exceeds 256-pair implementation profile")
		}
		key, value, hasEquals := strings.Cut(raw, "=")
		name, err := url.QueryUnescape(key)
		if err != nil {
			return fail("invalid query name escape")
		}
		decoded, err := url.QueryUnescape(value)
		if err != nil {
			return fail("invalid query value escape")
		}
		valueStart := cursor + len(key)
		if hasEquals {
			valueStart++
		}
		pairs = append(pairs, map[string]any{
			"Name": name, "Value": decoded, "Raw Name": key, "Raw Value": value,
			"Equals Present":        hasEquals,
			"Name Target Bit Span":  [2]uint64{uint64(cursor) * 8, uint64(cursor+len(key)) * 8},
			"Value Target Bit Span": [2]uint64{uint64(valueStart) * 8, uint64(cursor+len(raw)) * 8},
		})
		values[name] = append(values[name], decoded)
		cursor += len(raw) + 1
	}
	first := func(name string) string {
		if list := values[name]; len(list) != 0 {
			return list[0]
		}
		return ""
	}
	present := func(name string) bool { _, ok := values[name]; return ok }
	boolean := func(name string) bool {
		s := strings.ToLower(strings.TrimSpace(first(name)))
		return !(s == "" || s == "0" || s == "no" || s == "false" || s == "none")
	}
	var limit int64
	if value := first("limit"); value != "" {
		var err error
		limit, err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fail("limit is not a signed 64-bit decimal integer")
		}
	}
	filters := map[string]map[string]bool{}
	filterEncoding := "absent-or-empty"
	if text := first("filters"); text != "" {
		filterEncoding = "boolean-set"
		if err := json.Unmarshal([]byte(text), &filters); err != nil {
			legacy := map[string][]string{}
			if err := json.Unmarshal([]byte(text), &legacy); err != nil {
				return fail("filters must be a JSON object of string arrays or boolean sets (or null)")
			}
			filters = make(map[string]map[string]bool, len(legacy))
			for key, list := range legacy {
				set := make(map[string]bool, len(list))
				for _, value := range list {
					set[value] = true
				}
				filters[key] = set
			}
			filterEncoding = "string-array"
		}
		// Args.UnmarshalJSON in the pinned receiver has a value receiver.
		// A top-level null therefore leaves NewArgs' original empty map in
		// place; nested null values still become nil sets. Keep raw text in
		// Query Values while reproducing that distinction in decoded Filters.
		if filters == nil {
			filters = map[string]map[string]bool{}
		}
	}
	// These are the v1.41 documented names and Moby's acceptedPsFilterTags.
	// Unknown names are retained and reported, not conflated with malformed
	// JSON. The captured request can therefore describe a daemon-side error.
	knownFilters := map[string]bool{"ancestor": true, "before": true, "exited": true, "id": true, "isolation": true, "label": true, "name": true, "status": true, "health": true, "since": true, "volume": true, "network": true, "is-task": true, "publish": true, "expose": true}
	unknownFilters := []string{}
	for key := range filters {
		if !knownFilters[key] {
			unknownFilters = append(unknownFilters, key)
		}
	}
	sort.Strings(unknownFilters)
	unknownQuery := map[string][]string{}
	for key, list := range values {
		if key != "all" && key != "size" && key != "limit" && key != "filters" && key != "since" && key != "before" {
			unknownQuery[key] = list
		}
	}
	return map[string]any{
		"API Version": "1.41", "Route": "/containers/json", "Operation": "ContainerList",
		"Parameter Span Coordinate System": "request-target-relative-bits",
		"API Version Target Bit Span":      [2]uint64{16, 48}, "Route Target Bit Span": [2]uint64{48, uint64(len(path)) * 8},
		"Raw Query": rawQuery, "Query Present": len(parts) == 2, "Query Pairs": pairs,
		"Query Values": values, "Query Pair Count": len(pairs), "Unknown Query Values": unknownQuery,
		"All": boolean("all"), "All Present": present("all"), "Size": boolean("size"), "Size Present": present("size"),
		"Limit": limit, "Limit Present": present("limit"), "Filters": filters, "Filters Present": present("filters"),
		"Filter Encoding": filterEncoding, "Unknown Filter Names": unknownFilters,
		"Filter Names Recognized": len(unknownFilters) == 0,
		"Legacy Since":            first("since"), "Legacy Before": first("before"),
		"Legacy Since Present": present("since"), "Legacy Before Present": present("before"),
		"Query Duplicate Policy": "first value", "Boolean Codec": "Moby v20.10.0 BoolValue",
		"Filter Values Evaluated": false, "Backend Acceptance Proven": false,
	}, nil
}
