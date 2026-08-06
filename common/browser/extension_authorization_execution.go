package browser

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func authorizationLogicalProfilesForCandidate(
	workspace ExtensionAuthorizationWorkspace,
	candidateID string,
) *ExtensionAuthorizationTransformProfileInput {
	if workspace.Baselines.Left == nil ||
		workspace.Baselines.Right == nil ||
		workspace.Baselines.Left.LogicalRequest == nil ||
		workspace.Baselines.Right.LogicalRequest == nil {
		return nil
	}
	for _, candidate := range workspace.BaselinePair.ResourceCandidates {
		if candidate.ID != candidateID || candidate.Source != "logical" {
			continue
		}
		return &ExtensionAuthorizationTransformProfileInput{
			Left:  workspace.Baselines.Left.LogicalRequest.ProfileID,
			Right: workspace.Baselines.Right.LogicalRequest.ProfileID,
		}
	}
	return nil
}

func validAuthorizationFingerprint(value string, prefix string) bool {
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+64 {
		return false
	}
	for _, character := range value[len(prefix):] {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func authorizationBaselineSelectedField(
	baseline *ExtensionAuthorizationBaseline,
	selector ExtensionAuthorizationPlanSelector,
) (ExtensionAuthorizationBaselineField, error) {
	if baseline == nil {
		return ExtensionAuthorizationBaselineField{}, errors.New("authorization baseline is missing")
	}
	fields := baseline.Request.Fields
	switch selector.Source {
	case "wire":
	case "logical":
		if baseline.LogicalRequest == nil {
			return ExtensionAuthorizationBaselineField{}, errors.New("authorization logical request binding is missing")
		}
		fields = baseline.LogicalRequest.Request.Fields
	default:
		return ExtensionAuthorizationBaselineField{}, errors.New("authorization resource selector source is invalid")
	}
	var selected *ExtensionAuthorizationBaselineField
	for index := range fields {
		field := &fields[index]
		if field.Location != selector.Location || field.Path != selector.Path {
			continue
		}
		if selected != nil {
			return ExtensionAuthorizationBaselineField{}, errors.New("authorization resource selector is ambiguous")
		}
		selected = field
	}
	if selected == nil {
		return ExtensionAuthorizationBaselineField{}, errors.New("authorization resource selector is not present in the baseline")
	}
	return *selected, nil
}

func validateAuthorizationResourceValue(
	value ExtensionAuthorizationResourceValue,
	baseline *ExtensionAuthorizationBaseline,
	selector ExtensionAuthorizationPlanSelector,
) error {
	if value.Version != 1 ||
		value.BaselineID != baseline.ID ||
		value.Source != selector.Source ||
		value.Location != selector.Location ||
		value.Path != selector.Path ||
		!authorizationPrimitiveResourceType(value.ValueType) ||
		value.ByteLength < 0 ||
		value.ByteLength > 8*1024 {
		return errors.New("browser authorization resource value identity is invalid")
	}
	if selector.Source == "logical" {
		if baseline.LogicalRequest == nil ||
			value.LogicalBindingFingerprint != baseline.LogicalRequest.BindingFingerprint {
			return errors.New("browser authorization logical resource binding is invalid")
		}
	} else if value.LogicalBindingFingerprint != "" {
		return errors.New("browser authorization wire resource contains a logical binding")
	}
	decoded, err := base64.StdEncoding.DecodeString(value.ValueBase64)
	if err != nil || len(decoded) != value.ByteLength {
		return errors.New("browser authorization resource value encoding is invalid")
	}
	field, err := authorizationBaselineSelectedField(baseline, selector)
	if err != nil {
		return err
	}
	if value.ValueType != field.ValueType ||
		value.ValueFingerprint != field.ValueFingerprint ||
		!validAuthorizationFingerprint(value.ValueFingerprint, "workspace-hmac-sha256:") {
		return errors.New("browser authorization resource value fingerprint is invalid")
	}
	return nil
}

func authorizationComparisonFingerprint(
	encodedKey string,
	value []byte,
) (string, error) {
	key, err := base64.RawURLEncoding.DecodeString(encodedKey)
	if err != nil || len(key) != 32 {
		return "", errors.New("authorization comparison key is invalid")
	}
	mac := hmac.New(sha256.New, key)
	if _, err := mac.Write(value); err != nil {
		return "", err
	}
	return "workspace-hmac-sha256:" + hex.EncodeToString(mac.Sum(nil)), nil
}

func parseAuthorizationIndexedSelector(
	prefix string,
	path string,
) (string, int, bool, error) {
	if !strings.HasPrefix(path, prefix+".") {
		return "", 0, false, errors.New("authorization selector path does not match its location")
	}
	raw := strings.TrimPrefix(path, prefix+".")
	if raw == "" {
		return "", 0, false, errors.New("authorization selector path is empty")
	}
	if !strings.HasSuffix(raw, "]") {
		return raw, 0, false, nil
	}
	open := strings.LastIndex(raw, "[")
	if open <= 0 {
		return "", 0, false, errors.New("authorization selector index is invalid")
	}
	index, err := strconv.Atoi(raw[open+1 : len(raw)-1])
	if err != nil || index < 0 {
		return "", 0, false, errors.New("authorization selector index is invalid")
	}
	return raw[:open], index, true, nil
}

type authorizationBodyPathSegment struct {
	name  string
	index int
	array bool
}

func parseAuthorizationBodyPath(path string) ([]authorizationBodyPathSegment, error) {
	if !strings.HasPrefix(path, "body.") && !strings.HasPrefix(path, "body[") {
		return nil, errors.New("compiled authorization Body selector path is invalid")
	}
	input := strings.TrimPrefix(path, "body")
	matches := authorizationBodyPathSegmentPattern.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 || len(matches) > 64 {
		return nil, errors.New("compiled authorization Body selector path is invalid or too deep")
	}
	segments := make([]authorizationBodyPathSegment, 0, len(matches))
	offset := 0
	for _, match := range matches {
		if len(match) != 8 || match[0] != offset {
			return nil, errors.New("compiled authorization Body selector path is invalid")
		}
		switch {
		case match[4] >= 0:
			name := input[match[4]:match[5]]
			switch strings.ToLower(name) {
			case "__proto__", "prototype", "constructor":
				return nil, errors.New("compiled authorization Body selector contains a reserved field")
			}
			segments = append(segments, authorizationBodyPathSegment{name: name})
		case match[6] >= 0:
			index, err := strconv.Atoi(input[match[6]:match[7]])
			if err != nil || index < 0 {
				return nil, errors.New("compiled authorization Body selector index is invalid")
			}
			segments = append(segments, authorizationBodyPathSegment{
				index: index,
				array: true,
			})
		default:
			return nil, errors.New("compiled authorization Body selector path is invalid")
		}
		offset = match[1]
	}
	if offset != len(input) {
		return nil, errors.New("compiled authorization Body selector path is invalid")
	}
	return segments, nil
}

func extractAuthorizationCompiledBodyResource(
	packet []byte,
	selector ExtensionAuthorizationPlanSelector,
) ([]byte, error) {
	headerEnd := bytes.Index(packet, []byte("\r\n\r\n"))
	if headerEnd < 0 {
		return nil, errors.New("compiled authorization request Header block is invalid")
	}
	body := packet[headerEnd+4:]
	contentType := strings.ToLower(lowhttp.GetHTTPPacketHeader(packet, "Content-Type"))
	if strings.Contains(contentType, "json") {
		segments, err := parseAuthorizationBodyPath(selector.Path)
		if err != nil {
			return nil, err
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.UseNumber()
		var value interface{}
		if err := decoder.Decode(&value); err != nil {
			return nil, errors.New("compiled authorization JSON Body is invalid")
		}
		var trailing interface{}
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return nil, errors.New("compiled authorization JSON Body has trailing data")
		}
		for _, segment := range segments {
			if segment.array {
				items, ok := value.([]interface{})
				if !ok || segment.index >= len(items) {
					return nil, errors.New("compiled authorization JSON Body resource is missing")
				}
				value = items[segment.index]
				continue
			}
			object, ok := value.(map[string]interface{})
			if !ok {
				return nil, errors.New("compiled authorization JSON Body resource is missing")
			}
			value, ok = object[segment.name]
			if !ok {
				return nil, errors.New("compiled authorization JSON Body resource is missing")
			}
		}
		switch typed := value.(type) {
		case string:
			return []byte(typed), nil
		case json.Number:
			if _, err := typed.Float64(); err != nil {
				return nil, errors.New("compiled authorization JSON Body resource number is invalid")
			}
			return []byte(typed.String()), nil
		case bool:
			return []byte(strconv.FormatBool(typed)), nil
		default:
			return nil, errors.New("compiled authorization JSON Body resource is not a primitive")
		}
	}
	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		name, index, indexed, err := parseAuthorizationIndexedSelector("body", selector.Path)
		if err != nil {
			return nil, err
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			return nil, errors.New("compiled authorization Form Body is invalid")
		}
		candidates, ok := values[name]
		if !ok || len(candidates) == 0 {
			return nil, errors.New("compiled authorization Form Body resource is missing")
		}
		if !indexed && len(candidates) != 1 {
			return nil, errors.New("compiled authorization Form Body resource is ambiguous")
		}
		if index >= len(candidates) {
			return nil, errors.New("compiled authorization Form Body resource is missing")
		}
		return []byte(candidates[index]), nil
	}
	return nil, errors.New("compiled authorization Body is not structured JSON or Form data")
}

func extractAuthorizationCompiledResource(
	packet []byte,
	requestURI string,
	selector ExtensionAuthorizationPlanSelector,
) ([]byte, error) {
	if selector.Source != "wire" {
		return nil, errors.New("compiled wire resource extraction requires a wire selector")
	}
	parsed, err := url.ParseRequestURI(requestURI)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Fragment != "" {
		return nil, errors.New("compiled authorization request URI is invalid")
	}
	switch selector.Location {
	case "path":
		const prefix = "path.segment["
		if !strings.HasPrefix(selector.Path, prefix) || !strings.HasSuffix(selector.Path, "]") {
			return nil, errors.New("compiled authorization path selector is invalid")
		}
		index, err := strconv.Atoi(selector.Path[len(prefix) : len(selector.Path)-1])
		if err != nil || index < 0 {
			return nil, errors.New("compiled authorization path selector is invalid")
		}
		segments := make([]string, 0)
		for _, segment := range strings.Split(parsed.EscapedPath(), "/") {
			if segment == "" {
				continue
			}
			decoded, err := url.PathUnescape(segment)
			if err != nil {
				return nil, errors.New("compiled authorization path resource is invalid")
			}
			segments = append(segments, decoded)
		}
		if index >= len(segments) {
			return nil, errors.New("compiled authorization path resource is missing")
		}
		return []byte(segments[index]), nil
	case "query":
		name, index, indexed, err := parseAuthorizationIndexedSelector("query", selector.Path)
		if err != nil {
			return nil, err
		}
		values, ok := parsed.Query()[name]
		if !ok || len(values) == 0 {
			return nil, errors.New("compiled authorization query resource is missing")
		}
		if !indexed && len(values) != 1 {
			return nil, errors.New("compiled authorization query resource is ambiguous")
		}
		if index >= len(values) {
			return nil, errors.New("compiled authorization query resource is missing")
		}
		return []byte(values[index]), nil
	case "header":
		name, index, indexed, err := parseAuthorizationIndexedSelector("header", selector.Path)
		if err != nil {
			return nil, err
		}
		headerEnd := bytes.Index(packet, []byte("\r\n\r\n"))
		if headerEnd < 0 {
			return nil, errors.New("compiled authorization request Header block is invalid")
		}
		lines := strings.Split(string(packet[:headerEnd]), "\r\n")
		values := make([]string, 0)
		for _, line := range lines[1:] {
			separator := strings.IndexByte(line, ':')
			if separator <= 0 || !strings.EqualFold(strings.TrimSpace(line[:separator]), name) {
				continue
			}
			values = append(values, strings.TrimSpace(line[separator+1:]))
		}
		if len(values) == 0 {
			return nil, errors.New("compiled authorization Header resource is missing")
		}
		if !indexed && len(values) != 1 {
			return nil, errors.New("compiled authorization Header resource is ambiguous")
		}
		if index >= len(values) {
			return nil, errors.New("compiled authorization Header resource is missing")
		}
		return []byte(values[index]), nil
	case "body":
		return extractAuthorizationCompiledBodyResource(packet, selector)
	default:
		return nil, errors.New("compiled authorization selector location is unsupported")
	}
}

func validateAuthorizationCompiledRequest(
	compiled extensionAuthorizationCompiledRequest,
	baseline *ExtensionAuthorizationBaseline,
	selector ExtensionAuthorizationPlanSelector,
	resource ExtensionAuthorizationResourceValue,
	comparisonKey string,
) ([]byte, error) {
	if compiled.Version != 1 ||
		compiled.BaselineID != baseline.ID ||
		compiled.Selector != selector ||
		compiled.Method != baseline.Request.Method ||
		compiled.URL != baseline.Request.URL ||
		compiled.ResourceValueFingerprint != resource.ValueFingerprint {
		return nil, errors.New("compiled browser authorization request identity is invalid")
	}
	if !validAuthorizationFingerprint(compiled.PacketFingerprint, "sha256:") {
		return nil, errors.New("compiled browser authorization request packet fingerprint is invalid")
	}
	origin, err := url.Parse(baseline.Origin)
	if err != nil || origin.Host == "" ||
		compiled.IsHTTPS != (strings.EqualFold(origin.Scheme, "https")) {
		return nil, errors.New("compiled browser authorization request origin is invalid")
	}
	packet, err := base64.StdEncoding.DecodeString(compiled.RawRequestBase64)
	if err != nil || len(packet) == 0 || len(packet) > 2*1024*1024 {
		return nil, errors.New("compiled browser authorization request packet is invalid")
	}
	packetSum := sha256.Sum256(packet)
	if compiled.PacketFingerprint != "sha256:"+hex.EncodeToString(packetSum[:]) {
		return nil, errors.New("compiled browser authorization request packet fingerprint does not match")
	}
	method, requestURI, protocol := lowhttp.GetHTTPPacketFirstLine(packet)
	if method != baseline.Request.Method ||
		(protocol != "HTTP/1.1" && protocol != "HTTP/2" && protocol != "HTTP/2.0") ||
		!strings.EqualFold(lowhttp.GetHTTPPacketHeader(packet, "Host"), origin.Host) {
		return nil, errors.New("compiled browser authorization request line or Host is invalid")
	}
	if selector.Source == "logical" {
		if baseline.LogicalRequest == nil ||
			compiled.LogicalBindingFingerprint != baseline.LogicalRequest.BindingFingerprint ||
			resource.LogicalBindingFingerprint == "" {
			return nil, errors.New("compiled browser authorization logical binding is invalid")
		}
	} else {
		if compiled.LogicalBindingFingerprint != "" {
			return nil, errors.New("compiled wire authorization request contains a logical binding")
		}
		value, err := extractAuthorizationCompiledResource(packet, requestURI, selector)
		if err != nil {
			return nil, err
		}
		fingerprint, err := authorizationComparisonFingerprint(comparisonKey, value)
		if err != nil || fingerprint != resource.ValueFingerprint {
			return nil, errors.New("compiled browser authorization resource fingerprint is invalid")
		}
	}
	return packet, nil
}

func validateAuthorizationBaselinePacket(
	compiled extensionAuthorizationBaselinePacket,
	baseline *ExtensionAuthorizationBaseline,
	comparisonKey string,
) ([]byte, error) {
	if baseline == nil ||
		compiled.Version != 1 ||
		compiled.BaselineID != baseline.ID ||
		compiled.Method != baseline.Request.Method ||
		compiled.URL != baseline.Request.URL ||
		!validAuthorizationFingerprint(compiled.PacketFingerprint, "sha256:") {
		return nil, errors.New("compiled browser authorization baseline packet identity is invalid")
	}
	origin, err := url.Parse(baseline.Origin)
	if err != nil || origin.Host == "" ||
		compiled.IsHTTPS != strings.EqualFold(origin.Scheme, "https") {
		return nil, errors.New("compiled browser authorization baseline packet origin is invalid")
	}
	packet, err := base64.StdEncoding.DecodeString(compiled.RawRequestBase64)
	if err != nil || len(packet) == 0 || len(packet) > 2*1024*1024 {
		return nil, errors.New("compiled browser authorization baseline packet is invalid")
	}
	sum := sha256.Sum256(packet)
	if compiled.PacketFingerprint != "sha256:"+hex.EncodeToString(sum[:]) {
		return nil, errors.New("compiled browser authorization baseline packet fingerprint does not match")
	}
	method, requestURI, protocol := lowhttp.GetHTTPPacketFirstLine(packet)
	if method != baseline.Request.Method ||
		(protocol != "HTTP/1.1" && protocol != "HTTP/2" && protocol != "HTTP/2.0") ||
		!strings.EqualFold(lowhttp.GetHTTPPacketHeader(packet, "Host"), origin.Host) {
		return nil, errors.New("compiled browser authorization baseline request line or Host is invalid")
	}
	for _, field := range baseline.Request.Fields {
		if !authorizationPrimitiveResourceType(field.ValueType) ||
			field.Path == "body" {
			continue
		}
		value, err := extractAuthorizationCompiledResource(
			packet,
			requestURI,
			ExtensionAuthorizationPlanSelector{
				Source:   "wire",
				Location: field.Location,
				Path:     field.Path,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("verify baseline packet field %s: %w", field.Path, err)
		}
		fingerprint, err := authorizationComparisonFingerprint(comparisonKey, value)
		if err != nil || fingerprint != field.ValueFingerprint {
			return nil, fmt.Errorf(
				"compiled browser authorization baseline field %s fingerprint is invalid",
				field.Path,
			)
		}
	}
	return packet, nil
}

func authorizationPrimitiveFromBytes(
	valueType string,
	value []byte,
) (interface{}, error) {
	switch valueType {
	case "string":
		return string(value), nil
	case "number":
		decoder := json.NewDecoder(bytes.NewReader(value))
		decoder.UseNumber()
		var output interface{}
		if err := decoder.Decode(&output); err != nil {
			return nil, errors.New("authorization number field is invalid")
		}
		number, ok := output.(json.Number)
		if !ok {
			return nil, errors.New("authorization number field changed type")
		}
		return number, nil
	case "boolean":
		switch string(value) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return nil, errors.New("authorization boolean field is invalid")
		}
	default:
		return nil, errors.New("authorization field type is not a supported primitive")
	}
}

func replaceAuthorizationJSONBodyField(
	packet []byte,
	path string,
	valueType string,
	value []byte,
) ([]byte, error) {
	_, body := lowhttp.SplitHTTPPacketFast(packet)
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var root interface{}
	if err := decoder.Decode(&root); err != nil {
		return nil, errors.New("authorization template JSON Body is invalid")
	}
	segments, err := parseAuthorizationBodyPath(path)
	if err != nil {
		return nil, err
	}
	replacement, err := authorizationPrimitiveFromBytes(valueType, value)
	if err != nil {
		return nil, err
	}
	current := root
	for _, segment := range segments[:len(segments)-1] {
		if segment.array {
			items, ok := current.([]interface{})
			if !ok || segment.index >= len(items) {
				return nil, errors.New("authorization template JSON Body field is missing")
			}
			current = items[segment.index]
			continue
		}
		object, ok := current.(map[string]interface{})
		if !ok {
			return nil, errors.New("authorization template JSON Body field is missing")
		}
		current, ok = object[segment.name]
		if !ok {
			return nil, errors.New("authorization template JSON Body field is missing")
		}
	}
	leaf := segments[len(segments)-1]
	if leaf.array {
		items, ok := current.([]interface{})
		if !ok || leaf.index >= len(items) {
			return nil, errors.New("authorization template JSON Body field is missing")
		}
		if fmt.Sprintf("%T", items[leaf.index]) != fmt.Sprintf("%T", replacement) {
			return nil, errors.New("authorization template JSON Body field type changed")
		}
		items[leaf.index] = replacement
	} else {
		object, ok := current.(map[string]interface{})
		if !ok {
			return nil, errors.New("authorization template JSON Body field is missing")
		}
		existing, ok := object[leaf.name]
		if !ok || fmt.Sprintf("%T", existing) != fmt.Sprintf("%T", replacement) {
			return nil, errors.New("authorization template JSON Body field type changed")
		}
		object[leaf.name] = replacement
	}
	rewritten, err := json.Marshal(root)
	if err != nil {
		return nil, errors.New("authorization template JSON Body cannot be serialized")
	}
	return lowhttp.ReplaceHTTPPacketBody(packet, rewritten, false), nil
}

func replaceAuthorizationEncodedEntry(
	encoded string,
	name string,
	index int,
	indexed bool,
	value string,
) (string, error) {
	segments := strings.Split(encoded, "&")
	matching := make([]int, 0)
	for segmentIndex, segment := range segments {
		key := segment
		if separator := strings.IndexByte(segment, '='); separator >= 0 {
			key = segment[:separator]
		}
		decoded, err := url.QueryUnescape(key)
		if err == nil && decoded == name {
			matching = append(matching, segmentIndex)
		}
	}
	if len(matching) == 0 || (!indexed && len(matching) != 1) || index >= len(matching) {
		return "", errors.New("authorization encoded resource field is missing or ambiguous")
	}
	target := matching[index]
	rawName := segments[target]
	if separator := strings.IndexByte(rawName, '='); separator >= 0 {
		rawName = rawName[:separator]
	}
	segments[target] = rawName + "=" + url.QueryEscape(value)
	return strings.Join(segments, "&"), nil
}

func replaceAuthorizationPacketField(
	packet []byte,
	selector ExtensionAuthorizationPlanSelector,
	valueType string,
	value []byte,
) ([]byte, error) {
	if selector.Source != "wire" {
		return nil, errors.New("authorization template replacement requires a wire selector")
	}
	switch selector.Location {
	case "header":
		if valueType != "string" || bytes.ContainsAny(value, "\r\n\x00") {
			return nil, errors.New("authorization Header replacement is invalid")
		}
		name, index, indexed, err := parseAuthorizationIndexedSelector("header", selector.Path)
		if err != nil {
			return nil, err
		}
		headerEnd := bytes.Index(packet, []byte("\r\n\r\n"))
		if headerEnd < 0 {
			return nil, errors.New("authorization template Header block is invalid")
		}
		lines := strings.Split(string(packet[:headerEnd]), "\r\n")
		matching := make([]int, 0)
		for lineIndex, line := range lines[1:] {
			separator := strings.IndexByte(line, ':')
			if separator > 0 && strings.EqualFold(strings.TrimSpace(line[:separator]), name) {
				matching = append(matching, lineIndex+1)
			}
		}
		if len(matching) == 0 || (!indexed && len(matching) != 1) || index >= len(matching) {
			return nil, errors.New("authorization template Header field is missing or ambiguous")
		}
		lineIndex := matching[index]
		separator := strings.IndexByte(lines[lineIndex], ':')
		lines[lineIndex] = lines[lineIndex][:separator] + ": " + string(value)
		head := []byte(strings.Join(lines, "\r\n") + "\r\n\r\n")
		output := make([]byte, 0, len(head)+len(packet)-(headerEnd+4))
		output = append(output, head...)
		output = append(output, packet[headerEnd+4:]...)
		return output, nil
	case "query":
		if valueType != "string" {
			return nil, errors.New("authorization Query replacement requires a string")
		}
		name, index, indexed, err := parseAuthorizationIndexedSelector("query", selector.Path)
		if err != nil {
			return nil, err
		}
		method, requestURI, protocol := lowhttp.GetHTTPPacketFirstLine(packet)
		parsed, err := url.ParseRequestURI(requestURI)
		if err != nil {
			return nil, errors.New("authorization template request URI is invalid")
		}
		query, err := replaceAuthorizationEncodedEntry(
			parsed.RawQuery,
			name,
			index,
			indexed,
			string(value),
		)
		if err != nil {
			return nil, err
		}
		parsed.RawQuery = query
		return lowhttp.ReplaceHTTPPacketFirstLine(
			packet,
			fmt.Sprintf("%s %s %s", method, parsed.RequestURI(), protocol),
		), nil
	case "body":
		contentType := strings.ToLower(lowhttp.GetHTTPPacketHeader(packet, "Content-Type"))
		if strings.Contains(contentType, "json") {
			return replaceAuthorizationJSONBodyField(packet, selector.Path, valueType, value)
		}
		if strings.Contains(contentType, "application/x-www-form-urlencoded") {
			if valueType != "string" {
				return nil, errors.New("authorization Form replacement requires a string")
			}
			name, index, indexed, err := parseAuthorizationIndexedSelector("body", selector.Path)
			if err != nil {
				return nil, err
			}
			_, body := lowhttp.SplitHTTPPacketFast(packet)
			rewritten, err := replaceAuthorizationEncodedEntry(
				string(body),
				name,
				index,
				indexed,
				string(value),
			)
			if err != nil {
				return nil, err
			}
			return lowhttp.ReplaceHTTPPacketBody(packet, []byte(rewritten), false), nil
		}
		return nil, errors.New("authorization template Body is not structured JSON or Form")
	default:
		return nil, errors.New("authorization template authentication fields cannot use Path")
	}
}

func transplantAuthorizationAuthentication(
	templatePacket []byte,
	templateBaseline *ExtensionAuthorizationBaseline,
	authPacket []byte,
	authBaseline *ExtensionAuthorizationBaseline,
	comparisonKey string,
) ([]byte, error) {
	if templateBaseline == nil || authBaseline == nil ||
		templateBaseline.Origin != authBaseline.Origin {
		return nil, errors.New("vertical authorization baselines do not share one origin")
	}
	_, authRequestURI, _ := lowhttp.GetHTTPPacketFirstLine(authPacket)
	sourceFields := make(map[string]ExtensionAuthorizationBaselineField)
	for _, field := range authBaseline.Request.Fields {
		if field.Category == "authentication" || field.Category == "csrf" {
			sourceFields[authorizationFieldKey("wire", field)] = field
		}
	}
	output := append([]byte(nil), templatePacket...)
	replaced := 0
	for _, target := range templateBaseline.Request.Fields {
		if target.Category != "authentication" && target.Category != "csrf" {
			continue
		}
		source, ok := sourceFields[authorizationFieldKey("wire", target)]
		if !ok || source.ValueType != target.ValueType {
			return nil, fmt.Errorf(
				"low-privilege authentication source is missing %s",
				target.Path,
			)
		}
		value, err := extractAuthorizationCompiledResource(
			authPacket,
			authRequestURI,
			ExtensionAuthorizationPlanSelector{
				Source:   "wire",
				Location: source.Location,
				Path:     source.Path,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("read low-privilege field %s: %w", source.Path, err)
		}
		output, err = replaceAuthorizationPacketField(
			output,
			ExtensionAuthorizationPlanSelector{
				Source:   "wire",
				Location: target.Location,
				Path:     target.Path,
			},
			source.ValueType,
			value,
		)
		if err != nil {
			return nil, fmt.Errorf("replace privileged template field %s: %w", target.Path, err)
		}
		replaced++
	}
	if replaced == 0 {
		return nil, errors.New("vertical authorization template has no replaceable authentication fields")
	}
	_, outputRequestURI, _ := lowhttp.GetHTTPPacketFirstLine(output)
	for _, source := range sourceFields {
		targetPresent := false
		for _, target := range templateBaseline.Request.Fields {
			if (target.Category == "authentication" || target.Category == "csrf") &&
				target.Location == source.Location &&
				target.Path == source.Path {
				targetPresent = true
				break
			}
		}
		if !targetPresent {
			continue
		}
		value, err := extractAuthorizationCompiledResource(
			output,
			outputRequestURI,
			ExtensionAuthorizationPlanSelector{
				Source:   "wire",
				Location: source.Location,
				Path:     source.Path,
			},
		)
		if err != nil {
			return nil, err
		}
		fingerprint, err := authorizationComparisonFingerprint(comparisonKey, value)
		if err != nil || fingerprint != source.ValueFingerprint {
			return nil, fmt.Errorf(
				"vertical authorization field %s does not use the low-privilege value",
				source.Path,
			)
		}
	}
	return output, nil
}

func authorizationPacketToTransform(
	packet []byte,
	origin string,
) (extensionAuthorizationTransformPacket, error) {
	method, requestURI, protocol := lowhttp.GetHTTPPacketFirstLine(packet)
	if method == "" ||
		(protocol != "HTTP/1.1" && protocol != "HTTP/2" && protocol != "HTTP/2.0") {
		return extensionAuthorizationTransformPacket{}, errors.New(
			"vertical authorization transform packet has an invalid request line",
		)
	}
	base, err := url.Parse(origin)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return extensionAuthorizationTransformPacket{}, errors.New(
			"vertical authorization transform origin is invalid",
		)
	}
	reference, err := url.Parse(requestURI)
	if err != nil {
		return extensionAuthorizationTransformPacket{}, errors.New(
			"vertical authorization transform request URI is invalid",
		)
	}
	resolved := base.ResolveReference(reference)
	if resolved.Scheme != base.Scheme ||
		resolved.Host != base.Host ||
		resolved.Fragment != "" {
		return extensionAuthorizationTransformPacket{}, errors.New(
			"vertical authorization transform request escaped its origin",
		)
	}
	headerEnd := bytes.Index(packet, []byte("\r\n\r\n"))
	if headerEnd < 0 {
		return extensionAuthorizationTransformPacket{}, errors.New(
			"vertical authorization transform Header block is invalid",
		)
	}
	lines := strings.Split(string(packet[:headerEnd]), "\r\n")
	headers := make([]extensionAuthorizationTransformHeader, 0, len(lines)-1)
	for _, line := range lines[1:] {
		if len(headers) >= 256 {
			return extensionAuthorizationTransformPacket{}, errors.New(
				"vertical authorization transform has too many Headers",
			)
		}
		separator := strings.IndexByte(line, ':')
		if separator <= 0 {
			return extensionAuthorizationTransformPacket{}, errors.New(
				"vertical authorization transform contains an invalid Header",
			)
		}
		headers = append(headers, extensionAuthorizationTransformHeader{
			Name:  strings.TrimSpace(line[:separator]),
			Value: strings.TrimSpace(line[separator+1:]),
		})
	}
	_, body := lowhttp.SplitHTTPPacketFast(packet)
	return extensionAuthorizationTransformPacket{
		Method:     method,
		URL:        resolved.String(),
		Headers:    headers,
		BodyBase64: base64.StdEncoding.EncodeToString(body),
	}, nil
}

func authorizationTransformAllowedFields(
	dynamicPaths []string,
) (map[string]struct{}, map[string]struct{}, error) {
	headers := make(map[string]struct{})
	query := make(map[string]struct{})
	for _, path := range dynamicPaths {
		switch {
		case strings.HasPrefix(path, "header."):
			name := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(path, "header.")))
			if name == "" || strings.ContainsAny(name, "\r\n:[]") {
				return nil, nil, errors.New(
					"vertical authorization transform Header path is invalid",
				)
			}
			headers[name] = struct{}{}
		case strings.HasPrefix(path, "query."):
			name := strings.TrimSpace(strings.TrimPrefix(path, "query."))
			if name == "" || strings.ContainsAny(name, "[]") {
				return nil, nil, errors.New(
					"vertical authorization transform Query path is invalid",
				)
			}
			query[name] = struct{}{}
		default:
			return nil, nil, errors.New(
				"vertical authorization transform currently accepts Header and Query outputs",
			)
		}
	}
	if len(headers)+len(query) == 0 {
		return nil, nil, errors.New(
			"vertical authorization transform has no bounded output",
		)
	}
	return headers, query, nil
}

func sameAuthorizationStringValues(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func applyVerticalAuthorizationTransformExecution(
	packet []byte,
	input extensionAuthorizationTransformPacket,
	execution extensionAuthorizationTransformExecution,
	binding ExtensionAuthorizationOperationTransformBinding,
) ([]byte, error) {
	if execution.ProfileID != binding.ProfileID ||
		execution.Direction != "request" ||
		execution.DurationMS < 0 ||
		len(execution.NodeDurations) > 64 ||
		len(execution.SetHeaders) > 64 ||
		len(execution.RemoveHeaders) > 64 {
		return nil, errors.New(
			"vertical authorization transform execution metadata is invalid",
		)
	}
	allowedHeaders, allowedQuery, err := authorizationTransformAllowedFields(
		binding.DynamicPaths,
	)
	if err != nil {
		return nil, err
	}
	body, err := base64.StdEncoding.DecodeString(execution.BodyBase64)
	if err != nil ||
		len(body) > 2*1024*1024 ||
		execution.BodyBase64 != input.BodyBase64 {
		return nil, errors.New(
			"vertical authorization transform changed the operation Body",
		)
	}
	originalURL, err := url.Parse(input.URL)
	if err != nil {
		return nil, errors.New("vertical authorization transform input URL is invalid")
	}
	transformedURL, err := url.Parse(execution.URL)
	if err != nil ||
		transformedURL.Scheme != originalURL.Scheme ||
		transformedURL.Host != originalURL.Host ||
		transformedURL.Path != originalURL.Path ||
		transformedURL.Fragment != "" {
		return nil, errors.New(
			"vertical authorization transform changed the operation route",
		)
	}
	originalQuery := originalURL.Query()
	transformedQuery := transformedURL.Query()
	queryNames := make(map[string]struct{}, len(originalQuery)+len(transformedQuery))
	for name := range originalQuery {
		queryNames[name] = struct{}{}
	}
	for name := range transformedQuery {
		queryNames[name] = struct{}{}
	}
	for name := range queryNames {
		if sameAuthorizationStringValues(originalQuery[name], transformedQuery[name]) {
			continue
		}
		if _, ok := allowedQuery[name]; !ok {
			return nil, fmt.Errorf(
				"vertical authorization transform changed undeclared Query field %s",
				name,
			)
		}
	}
	removed := make(map[string]struct{})
	for _, name := range execution.RemoveHeaders {
		normalized := strings.ToLower(strings.TrimSpace(name))
		if _, ok := allowedHeaders[normalized]; !ok {
			return nil, fmt.Errorf(
				"vertical authorization transform removed undeclared Header %s",
				name,
			)
		}
		removed[normalized] = struct{}{}
	}
	replacements := make(map[string]extensionAuthorizationTransformHeader)
	for _, header := range execution.SetHeaders {
		normalized := strings.ToLower(strings.TrimSpace(header.Name))
		if _, ok := allowedHeaders[normalized]; !ok ||
			normalized == "authorization" ||
			normalized == "cookie" ||
			normalized == "host" ||
			normalized == "proxy-authorization" ||
			strings.ContainsAny(header.Name, "\r\n:") ||
			strings.ContainsAny(header.Value, "\r\n\x00") ||
			len(header.Value) > 16*1024 {
			return nil, fmt.Errorf(
				"vertical authorization transform changed forbidden Header %s",
				header.Name,
			)
		}
		if _, exists := replacements[normalized]; exists {
			return nil, errors.New(
				"vertical authorization transform returned duplicate Header outputs",
			)
		}
		replacements[normalized] = extensionAuthorizationTransformHeader{
			Name:  strings.TrimSpace(header.Name),
			Value: header.Value,
		}
		delete(removed, normalized)
	}
	headers := make([]extensionAuthorizationTransformHeader, 0, len(input.Headers)+len(replacements))
	for _, header := range input.Headers {
		normalized := strings.ToLower(header.Name)
		if _, drop := removed[normalized]; drop {
			continue
		}
		if _, replace := replacements[normalized]; replace {
			continue
		}
		headers = append(headers, header)
	}
	replacementNames := make([]string, 0, len(replacements))
	for name := range replacements {
		replacementNames = append(replacementNames, name)
	}
	sort.Strings(replacementNames)
	for _, name := range replacementNames {
		headers = append(headers, replacements[name])
	}
	hostValid := false
	for _, header := range headers {
		if strings.EqualFold(header.Name, "Host") && header.Value == transformedURL.Host {
			hostValid = true
			break
		}
	}
	if !hostValid {
		return nil, errors.New(
			"vertical authorization transform changed or removed Host",
		)
	}
	_, _, protocol := lowhttp.GetHTTPPacketFirstLine(packet)
	head := []string{
		fmt.Sprintf(
			"%s %s %s",
			input.Method,
			transformedURL.RequestURI(),
			protocol,
		),
	}
	for _, header := range headers {
		head = append(head, header.Name+": "+header.Value)
	}
	encodedHead := []byte(strings.Join(append(head, "", ""), "\r\n"))
	output := make([]byte, 0, len(encodedHead)+len(body))
	output = append(output, encodedHead...)
	output = append(output, body...)
	if len(output) > 2*1024*1024 {
		return nil, errors.New(
			"vertical authorization transformed operation exceeds the packet limit",
		)
	}
	return output, nil
}

func validateVerticalAuthorizationProbePacket(
	packet []byte,
	templateBaseline *ExtensionAuthorizationBaseline,
	authBaseline *ExtensionAuthorizationBaseline,
	comparisonKey string,
) error {
	if templateBaseline == nil || authBaseline == nil {
		return errors.New("vertical authorization probe baselines are missing")
	}
	origin, err := url.Parse(templateBaseline.Origin)
	if err != nil || origin.Host == "" {
		return errors.New("vertical authorization probe origin is invalid")
	}
	method, requestURI, _ := lowhttp.GetHTTPPacketFirstLine(packet)
	if method != templateBaseline.Request.Method ||
		!strings.EqualFold(lowhttp.GetHTTPPacketHeader(packet, "Host"), origin.Host) {
		return errors.New(
			"vertical authorization probe changed method or Host",
		)
	}
	authFields := make(map[string]ExtensionAuthorizationBaselineField)
	for _, field := range authBaseline.Request.Fields {
		if field.Category == "authentication" || field.Category == "csrf" {
			authFields[authorizationFieldKey("wire", field)] = field
		}
	}
	for _, field := range templateBaseline.Request.Fields {
		if !authorizationPrimitiveResourceType(field.ValueType) || field.Path == "body" {
			continue
		}
		if field.Category == "signature" ||
			field.Category == "nonce" ||
			field.Category == "timestamp" {
			if _, err := extractAuthorizationCompiledResource(
				packet,
				requestURI,
				ExtensionAuthorizationPlanSelector{
					Source:   "wire",
					Location: field.Location,
					Path:     field.Path,
				},
			); err != nil {
				return fmt.Errorf(
					"vertical authorization dynamic field %s is missing: %w",
					field.Path,
					err,
				)
			}
			continue
		}
		expected := field
		if field.Category == "authentication" || field.Category == "csrf" {
			source, ok := authFields[authorizationFieldKey("wire", field)]
			if !ok || source.ValueType != field.ValueType {
				return fmt.Errorf(
					"vertical authorization low-privilege field %s is missing",
					field.Path,
				)
			}
			expected = source
		}
		value, err := extractAuthorizationCompiledResource(
			packet,
			requestURI,
			ExtensionAuthorizationPlanSelector{
				Source:   "wire",
				Location: field.Location,
				Path:     field.Path,
			},
		)
		if err != nil {
			return fmt.Errorf(
				"vertical authorization probe field %s is invalid: %w",
				field.Path,
				err,
			)
		}
		fingerprint, err := authorizationComparisonFingerprint(comparisonKey, value)
		if err != nil || fingerprint != expected.ValueFingerprint {
			return fmt.Errorf(
				"vertical authorization probe changed protected field %s",
				field.Path,
			)
		}
	}
	return nil
}

func authorizationJSONShape(
	value interface{},
	depth int,
	visited *int,
) interface{} {
	(*visited)++
	if *visited > 1000 || depth > 8 {
		return "bounded"
	}
	switch typed := value.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case json.Number, float64:
		return "number"
	case []interface{}:
		limit := len(typed)
		if limit > 20 {
			limit = 20
		}
		items := make([]interface{}, 0, limit)
		for _, item := range typed[:limit] {
			items = append(items, authorizationJSONShape(item, depth+1, visited))
		}
		return map[string]interface{}{
			"type":   "array",
			"length": len(typed),
			"items":  items,
		}
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(keys) > 100 {
			keys = keys[:100]
		}
		output := make(map[string]interface{}, len(keys))
		for _, key := range keys {
			output[key] = authorizationJSONShape(typed[key], depth+1, visited)
		}
		return output
	default:
		return fmt.Sprintf("%T", value)
	}
}

func authorizationResponseShapeFingerprint(
	body []byte,
	contentType string,
	truncated bool,
) string {
	if truncated || !strings.Contains(strings.ToLower(contentType), "json") {
		return ""
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value interface{}
	if err := decoder.Decode(&value); err != nil {
		return ""
	}
	visited := 0
	shape, err := json.Marshal(authorizationJSONShape(value, 0, &visited))
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(shape)
	return "sha256:" + hex.EncodeToString(sum[:])
}

const maxAuthorizationResponseAnalysisBytes = 256 << 10

type extensionAuthorizationResponseAnalysis struct {
	body            []byte
	contentEncoding string
	state           string
	representation  string
	decoded         bool
}

func authorizationTextResponseBody(body []byte, contentType string) bool {
	if !utf8.Valid(body) || bytes.IndexByte(body, 0) >= 0 {
		return false
	}
	normalizedType := strings.ToLower(contentType)
	if strings.HasPrefix(normalizedType, "text/") ||
		strings.Contains(normalizedType, "xml") ||
		strings.Contains(normalizedType, "javascript") ||
		strings.Contains(normalizedType, "graphql") {
		return true
	}
	if len(body) == 0 {
		return true
	}
	printable := 0
	for _, character := range string(body) {
		if character == '\r' || character == '\n' || character == '\t' ||
			character >= 0x20 {
			printable++
		}
	}
	return printable*100 >= len([]rune(string(body)))*95
}

func authorizationResponseRepresentation(body []byte, contentType string) string {
	normalizedType := strings.ToLower(contentType)
	switch {
	case strings.Contains(normalizedType, "json"):
		return "json"
	case strings.Contains(normalizedType, "html") || looksLikeAuthorizationHTML(body):
		return "html"
	case strings.Contains(normalizedType, "x-www-form-urlencoded"):
		return "form"
	case authorizationTextResponseBody(body, contentType):
		return "text"
	default:
		return "binary"
	}
}

func analyzeAuthorizationResponsePacket(
	packet []byte,
) extensionAuthorizationResponseAnalysis {
	contentEncoding := strings.TrimSpace(lowhttp.GetHTTPPacketHeader(packet, "Content-Encoding"))
	if len(contentEncoding) > 256 {
		contentEncoding = contentEncoding[:256]
	}
	transferEncoding := strings.TrimSpace(lowhttp.GetHTTPPacketHeader(packet, "Transfer-Encoding"))
	encodedOnWire := (contentEncoding != "" && !strings.EqualFold(contentEncoding, "identity")) ||
		strings.Contains(strings.ToLower(transferEncoding), "chunked")
	plainPacket, _, decoded := lowhttp.AutoUnzipPacketEncodingWithLimit(
		packet,
		maxAuthorizationResponseAnalysisBytes,
	)
	analysisPacket := packet
	state := "identity"
	if decoded {
		analysisPacket = plainPacket
		state = "decoded"
	} else if encodedOnWire {
		return extensionAuthorizationResponseAnalysis{
			contentEncoding: contentEncoding,
			state:           "encoded-unavailable",
			representation:  "encoded",
		}
	}
	_, body := lowhttp.SplitHTTPPacketFast(analysisPacket)
	if len(body) > maxAuthorizationResponseAnalysisBytes {
		return extensionAuthorizationResponseAnalysis{
			contentEncoding: contentEncoding,
			state:           "encoded-unavailable",
			representation:  "encoded",
			decoded:         decoded,
		}
	}
	contentType := lowhttp.GetHTTPPacketContentType(packet)
	return extensionAuthorizationResponseAnalysis{
		body:            append([]byte(nil), body...),
		contentEncoding: contentEncoding,
		state:           state,
		representation:  authorizationResponseRepresentation(body, contentType),
		decoded:         decoded,
	}
}

func authorizationResponseOutcome(status int) string {
	switch {
	case status >= 200 && status < 300:
		return "success"
	case status == 401 || status == 403 || status == 404:
		return "denied"
	case status >= 300 && status < 400:
		return "redirect"
	case status >= 400 && status < 500:
		return "client-error"
	case status >= 500:
		return "server-error"
	default:
		return "opaque"
	}
}

func executeAuthorizationCompiledRequest(
	ctx context.Context,
	compiled extensionAuthorizationCompiledRequest,
	packet []byte,
	baseline *ExtensionAuthorizationBaseline,
	selector ExtensionAuthorizationPlanSelector,
	comparisonKey string,
) (*ExtensionAuthorizationRequestExecution, error) {
	startedAt := time.Now()
	response, err := lowhttp.HTTPWithoutRedirect(
		lowhttp.WithPacketBytes(packet),
		lowhttp.WithHttps(compiled.IsHTTPS),
		lowhttp.WithContext(ctx),
		lowhttp.WithTimeout(30*time.Second),
		lowhttp.WithMaxContentLength(256*1024),
		lowhttp.WithSaveHTTPFlow(false),
		lowhttp.WithConnPool(false),
		lowhttp.WithRetryTimes(0),
		lowhttp.WithNoFixContentLength(true),
		lowhttp.WithNoReadMultiResponse(true),
	)
	if err != nil {
		return nil, err
	}
	if response == nil || len(response.RawPacket) == 0 {
		return nil, errors.New("Yak authorization request returned an empty response")
	}
	status := lowhttp.GetStatusCodeFromResponse(response.RawPacket)
	_, _, statusText := lowhttp.GetHTTPPacketFirstLine(response.RawPacket)
	_, wireBody := lowhttp.SplitHTTPPacketFast(response.RawPacket)
	truncated := response.TooLarge || len(wireBody) > maxAuthorizationResponseAnalysisBytes
	if len(wireBody) > maxAuthorizationResponseAnalysisBytes {
		wireBody = wireBody[:maxAuthorizationResponseAnalysisBytes]
	}
	analysis := analyzeAuthorizationResponsePacket(response.RawPacket)
	fingerprintBody := analysis.body
	if analysis.state == "encoded-unavailable" {
		fingerprintBody = wireBody
	}
	fingerprint, err := authorizationComparisonFingerprint(comparisonKey, fingerprintBody)
	if err != nil {
		return nil, err
	}
	contentType := lowhttp.GetHTTPPacketContentType(response.RawPacket)
	if len(contentType) > 512 {
		contentType = contentType[:512]
	}
	var declaredBytes *int
	if rawLength := lowhttp.GetHTTPPacketHeader(response.RawPacket, "Content-Length"); rawLength != "" {
		if value, err := strconv.Atoi(strings.TrimSpace(rawLength)); err == nil && value >= 0 {
			declaredBytes = &value
		}
	}
	if len(statusText) > 128 {
		statusText = statusText[:128]
	}
	elapsed := time.Since(startedAt)
	timing := extensionAuthorizationRequestTiming(response, elapsed)
	requestPacket, requestTruncated := boundedAuthorizationEvidencePacket(packet)
	responsePacket, responseTruncated := boundedAuthorizationEvidencePacket(response.RawPacket)
	return &ExtensionAuthorizationRequestExecution{
		Version:    1,
		BaselineID: baseline.ID,
		Selector:   selector,
		Method:     baseline.Request.Method,
		URL:        baseline.Request.URL,
		Status:     status,
		StatusText: statusText,
		Outcome:    authorizationResponseOutcome(status),
		Response: ExtensionAuthorizationResponseSummary{
			ContentType:            contentType,
			ContentEncoding:        analysis.contentEncoding,
			CapturedBytes:          len(wireBody),
			AnalysisBytes:          len(analysis.body),
			DeclaredBytes:          declaredBytes,
			Truncated:              truncated,
			Decoded:                analysis.decoded,
			AnalysisState:          analysis.state,
			AnalysisRepresentation: analysis.representation,
			ValueFingerprint:       fingerprint,
			ShapeFingerprint: authorizationResponseShapeFingerprint(
				analysis.body,
				contentType,
				truncated || analysis.state == "encoded-unavailable",
			),
		},
		DroppedHeaderNames: []string{},
		DurationMS:         timing.TotalMS,
		Timing:             timing,
		CompletedAt:        time.Now().UnixMilli(),
		responseBody:       append([]byte(nil), analysis.body...),
		requestPacket:      requestPacket,
		responsePacket:     responsePacket,
		requestTruncated:   requestTruncated,
		responseTruncated:  responseTruncated,
	}, nil
}

func validateAuthorizationRequestExecution(
	result ExtensionAuthorizationRequestExecution,
	baseline *ExtensionAuthorizationBaseline,
	selector ExtensionAuthorizationPlanSelector,
	allowSideEffects bool,
) error {
	if result.Version != 1 ||
		result.BaselineID != baseline.ID ||
		result.Selector != selector ||
		result.Method != baseline.Request.Method ||
		result.URL != baseline.Request.URL ||
		result.Status < 0 ||
		result.Status > 599 ||
		result.DurationMS < 0 ||
		result.CompletedAt <= 0 ||
		len(result.StatusText) > 128 ||
		len(result.Response.ContentType) > 512 ||
		len(result.Response.ContentEncoding) > 256 ||
		result.Response.CapturedBytes < 0 ||
		result.Response.CapturedBytes > 256*1024 ||
		result.Response.AnalysisBytes < 0 ||
		result.Response.AnalysisBytes > maxAuthorizationResponseAnalysisBytes ||
		len(result.DroppedHeaderNames) > 64 {
		return errors.New("browser authorization request execution metadata is invalid")
	}
	switch result.Method {
	case "GET", "HEAD", "OPTIONS":
	case "POST", "PUT", "PATCH", "DELETE":
		if !allowSideEffects {
			return errors.New("browser authorization request execution used a non-read-only method without review")
		}
	default:
		return errors.New("browser authorization request execution used an unsupported method")
	}
	switch result.Outcome {
	case "success", "denied", "redirect", "client-error", "server-error", "opaque":
	default:
		return errors.New("browser authorization request execution outcome is invalid")
	}
	if !validAuthorizationFingerprint(result.Response.ValueFingerprint, "workspace-hmac-sha256:") {
		return errors.New("browser authorization response fingerprint is invalid")
	}
	if result.Response.ShapeFingerprint != "" &&
		!validAuthorizationFingerprint(result.Response.ShapeFingerprint, "sha256:") {
		return errors.New("browser authorization response shape fingerprint is invalid")
	}
	if result.Response.DeclaredBytes != nil && *result.Response.DeclaredBytes < 0 {
		return errors.New("browser authorization response declared size is invalid")
	}
	if result.Response.AnalysisState != "" {
		switch result.Response.AnalysisState {
		case "identity", "decoded", "encoded-unavailable":
		default:
			return errors.New("browser authorization response analysis state is invalid")
		}
	}
	if result.Response.AnalysisRepresentation != "" {
		switch result.Response.AnalysisRepresentation {
		case "json", "html", "form", "text", "binary", "encoded":
		default:
			return errors.New("browser authorization response representation is invalid")
		}
	}
	for _, name := range result.DroppedHeaderNames {
		if name == "" || len(name) > 256 {
			return errors.New("browser authorization dropped header metadata is invalid")
		}
	}
	return nil
}

func authorizationBaselineForSide(
	workspace ExtensionAuthorizationWorkspace,
	side string,
) (*ExtensionAuthorizationBaseline, ExtensionAuthorizationIdentitySlot, error) {
	switch side {
	case "left":
		return workspace.Baselines.Left, workspace.Left, nil
	case "right":
		return workspace.Baselines.Right, workspace.Right, nil
	case "verification":
		return workspace.Baselines.Verification, workspace.Right, nil
	default:
		return nil, ExtensionAuthorizationIdentitySlot{}, errors.New("authorization plan contains an invalid identity side")
	}
}

func boundedAuthorizationError(err error) string {
	if err == nil {
		return ""
	}
	value := []rune(strings.TrimSpace(err.Error()))
	if len(value) > 512 {
		value = value[:512]
	}
	return string(value)
}

func authorizationTransformForSide(
	plan *ExtensionAuthorizationPlan,
	side string,
) *ExtensionAuthorizationTransformBinding {
	if plan == nil || plan.Transforms == nil {
		return nil
	}
	if side == "left" {
		return &plan.Transforms.Left
	}
	if side == "right" {
		return &plan.Transforms.Right
	}
	return nil
}

func (m *ExtensionBridgeManager) executeExtensionAuthorizationCase(
	ctx context.Context,
	workspace ExtensionAuthorizationWorkspace,
	planCase ExtensionAuthorizationPlanCase,
	resource ExtensionAuthorizationResourceValue,
) ExtensionAuthorizationCaseExecution {
	output := ExtensionAuthorizationCaseExecution{
		ID:                planCase.ID,
		Label:             planCase.Label,
		AuthContextSide:   planCase.AuthContextSide,
		ResourceValueSide: planCase.ResourceValueSide,
		State:             "failed",
	}
	baseline, slot, err := authorizationBaselineForSide(workspace, planCase.RequestBaselineSide)
	if err != nil || baseline == nil {
		if err == nil {
			err = errors.New("authorization request baseline is missing")
		}
		output.Error = boundedAuthorizationError(err)
		return output
	}
	method := "browser.authorization.baseline.compile"
	params := map[string]interface{}{
		"id": baseline.ID,
		"selector": map[string]interface{}{
			"source":   workspace.Plan.Selector.Source,
			"location": workspace.Plan.Selector.Location,
			"path":     workspace.Plan.Selector.Path,
		},
		"replacement":   resource,
		"comparisonKey": workspace.comparisonKey,
	}
	if transform := authorizationTransformForSide(
		workspace.Plan,
		planCase.RequestBaselineSide,
	); transform != nil {
		method = "browser.authorization.baseline.transform.compile"
		params["profileId"] = transform.ProfileID
		params["bindingFingerprint"] = transform.BindingFingerprint
	}
	raw, err := m.CallDevice(
		ctx,
		slot.DeviceID,
		method,
		params,
	)
	if err != nil {
		output.Error = boundedAuthorizationError(err)
		return output
	}
	var compiled extensionAuthorizationCompiledRequest
	if err := decodeAuthorizationResult(raw, &compiled); err != nil {
		output.Error = boundedAuthorizationError(err)
		return output
	}
	packet, err := validateAuthorizationCompiledRequest(
		compiled,
		baseline,
		workspace.Plan.Selector,
		resource,
		workspace.comparisonKey,
	)
	if err != nil {
		output.Error = boundedAuthorizationError(err)
		return output
	}
	result, err := executeAuthorizationCompiledRequest(
		ctx,
		compiled,
		packet,
		baseline,
		workspace.Plan.Selector,
		workspace.comparisonKey,
	)
	if err != nil {
		output.Error = boundedAuthorizationError(err)
		return output
	}
	if err := validateAuthorizationRequestExecution(
		*result,
		baseline,
		workspace.Plan.Selector,
		workspace.Plan.State == "review-required",
	); err != nil {
		output.Error = boundedAuthorizationError(err)
		return output
	}
	output.State = "completed"
	output.Result = result
	return output
}

func (m *ExtensionBridgeManager) executeExtensionAuthorizationCasePair(
	ctx context.Context,
	workspace ExtensionAuthorizationWorkspace,
	cases []ExtensionAuthorizationPlanCase,
	resources map[string]ExtensionAuthorizationResourceValue,
) []ExtensionAuthorizationCaseExecution {
	type indexedResult struct {
		index  int
		result ExtensionAuthorizationCaseExecution
	}
	results := make(chan indexedResult, len(cases))
	var wait sync.WaitGroup
	for index, planCase := range cases {
		index := index
		planCase := planCase
		wait.Add(1)
		go func() {
			defer wait.Done()
			resource, ok := resources[planCase.ResourceValueSide]
			if !ok {
				results <- indexedResult{
					index: index,
					result: ExtensionAuthorizationCaseExecution{
						ID:                planCase.ID,
						Label:             planCase.Label,
						AuthContextSide:   planCase.AuthContextSide,
						ResourceValueSide: planCase.ResourceValueSide,
						State:             "failed",
						Error:             "authorization resource value is missing",
					},
				}
				return
			}
			results <- indexedResult{
				index:  index,
				result: m.executeExtensionAuthorizationCase(ctx, workspace, planCase, resource),
			}
		}()
	}
	wait.Wait()
	close(results)
	output := make([]ExtensionAuthorizationCaseExecution, len(cases))
	for result := range results {
		output[result.index] = result.result
	}
	return output
}

func validateAuthorizationExecutionPlanReview(
	plan *ExtensionAuthorizationPlan,
	approveSideEffects bool,
) (bool, error) {
	if plan == nil {
		return false, errors.New("authorization execution plan is missing")
	}
	reviewApproved := plan.State == "review-required" && approveSideEffects
	if (plan.State != "ready" && !reviewApproved) ||
		plan.RequiresDynamicRebuild {
		return false, errors.New(
			"authorization execution plan is not eligible or still requires explicit side-effect review",
		)
	}
	switch plan.Mode {
	case "", "horizontal":
		if plan.RequestBudget != 4 ||
			len(plan.Cases) != 4 ||
			plan.Operation != nil {
			return false, errors.New(
				"horizontal authorization execution plan has an invalid fixed request budget",
			)
		}
	case "vertical":
		expectedBudget := 3
		if plan.Operation != nil &&
			strings.TrimSpace(plan.Operation.VerificationBaselineID) != "" {
			expectedBudget = 5
		}
		if plan.RequestBudget != expectedBudget ||
			len(plan.Cases) != expectedBudget ||
			plan.Operation == nil ||
			plan.Selector != (ExtensionAuthorizationPlanSelector{
				Source:   "operation",
				Location: "request",
				Path:     "right",
			}) {
			return false, errors.New(
				"vertical authorization execution plan has an invalid fixed request budget",
			)
		}
	default:
		return false, errors.New("authorization execution plan mode is unsupported")
	}
	return reviewApproved, nil
}

func validateVerticalAuthorizationExecutionPlan(
	workspace ExtensionAuthorizationWorkspace,
) (*ExtensionAuthorizationOperationCandidate, error) {
	if workspace.Mode != "vertical" ||
		workspace.Plan == nil ||
		workspace.Plan.Mode != "vertical" ||
		workspace.Baselines.Left == nil ||
		workspace.Baselines.Right == nil ||
		workspace.Plan.Operation == nil {
		return nil, errors.New("vertical authorization execution state is incomplete")
	}
	var selected *ExtensionAuthorizationOperationCandidate
	for index := range workspace.BaselinePair.OperationCandidates {
		candidate := &workspace.BaselinePair.OperationCandidates[index]
		if candidate.ID == workspace.Plan.CandidateID {
			selected = candidate
			break
		}
	}
	if selected == nil ||
		!selected.Eligible ||
		selected.TemplateSide != "right" ||
		selected.AuthContextSide != "left" ||
		selected.LowControlSide != "left" {
		return nil, errors.New("vertical authorization operation candidate is not eligible")
	}
	operation := workspace.Plan.Operation
	if operation.TemplateBaselineSide != selected.TemplateSide ||
		operation.AuthContextSide != selected.AuthContextSide ||
		operation.LowControlSide != selected.LowControlSide ||
		!sameAuthorizationDynamicPaths(
			operation.AuthenticationPaths,
			selected.AuthenticationPaths,
		) ||
		!sameAuthorizationDynamicPaths(operation.DynamicPaths, selected.DynamicPaths) {
		return nil, errors.New("vertical authorization operation binding changed")
	}
	if selected.RequiresDynamicRebuild {
		if operation.Transform == nil ||
			workspace.Plan.RequiresDynamicRebuild ||
			operation.Transform.BindingFingerprint !=
				authorizationOperationTransformFingerprint(*operation.Transform) ||
			!sameAuthorizationDynamicPaths(
				operation.Transform.DynamicPaths,
				selected.DynamicPaths,
			) {
			return nil, errors.New(
				"vertical authorization dynamic operation transform is not valid",
			)
		}
	} else if operation.Transform != nil {
		return nil, errors.New(
			"vertical authorization operation transform is unexpected",
		)
	}
	if workspace.Baselines.Verification == nil {
		if operation.VerificationBaselineID != "" {
			return nil, errors.New(
				"vertical authorization verification binding is missing",
			)
		}
	} else {
		if err := validateVerticalAuthorizationVerificationBaseline(
			workspace.Baselines.Verification,
		); err != nil {
			return nil, err
		}
		if operation.VerificationBaselineID != workspace.Baselines.Verification.ID {
			return nil, errors.New(
				"vertical authorization verification baseline changed",
			)
		}
	}
	expectedCases := []struct {
		id             string
		requestSide    string
		authentication string
		method         string
		path           string
	}{
		{
			id:             "low-control",
			requestSide:    "left",
			authentication: "left",
			method:         workspace.Baselines.Left.Request.Method,
			path:           workspace.Baselines.Left.Request.Path,
		},
		{
			id:             "privileged-baseline",
			requestSide:    "right",
			authentication: "right",
			method:         workspace.Baselines.Right.Request.Method,
			path:           workspace.Baselines.Right.Request.Path,
		},
	}
	if workspace.Baselines.Verification != nil {
		expectedCases = append(expectedCases, struct {
			id             string
			requestSide    string
			authentication string
			method         string
			path           string
		}{
			id:             "post-state-before",
			requestSide:    "verification",
			authentication: "right",
			method:         workspace.Baselines.Verification.Request.Method,
			path:           workspace.Baselines.Verification.Request.Path,
		})
	}
	expectedCases = append(expectedCases, struct {
		id             string
		requestSide    string
		authentication string
		method         string
		path           string
	}{
		id:             "low-privileged-probe",
		requestSide:    "right",
		authentication: "left",
		method:         workspace.Baselines.Right.Request.Method,
		path:           workspace.Baselines.Right.Request.Path,
	})
	if workspace.Baselines.Verification != nil {
		expectedCases = append(expectedCases, struct {
			id             string
			requestSide    string
			authentication string
			method         string
			path           string
		}{
			id:             "post-state-after",
			requestSide:    "verification",
			authentication: "right",
			method:         workspace.Baselines.Verification.Request.Method,
			path:           workspace.Baselines.Verification.Request.Path,
		})
	}
	if len(workspace.Plan.Cases) != len(expectedCases) {
		return nil, errors.New("vertical authorization case matrix changed")
	}
	for index, expected := range expectedCases {
		actual := workspace.Plan.Cases[index]
		if actual.ID != expected.id ||
			actual.RequestBaselineSide != expected.requestSide ||
			actual.AuthContextSide != expected.authentication ||
			actual.ResourceValueSide != "" ||
			actual.Method != expected.method ||
			actual.Path != expected.path {
			return nil, errors.New("vertical authorization case matrix changed")
		}
	}
	return selected, nil
}

func executeVerticalAuthorizationPacketCase(
	ctx context.Context,
	workspace ExtensionAuthorizationWorkspace,
	planCase ExtensionAuthorizationPlanCase,
	compiled extensionAuthorizationBaselinePacket,
	packet []byte,
	baseline *ExtensionAuthorizationBaseline,
	reviewApproved bool,
) ExtensionAuthorizationCaseExecution {
	output := ExtensionAuthorizationCaseExecution{
		ID:                planCase.ID,
		Label:             planCase.Label,
		AuthContextSide:   planCase.AuthContextSide,
		ResourceValueSide: planCase.ResourceValueSide,
		State:             "failed",
	}
	result, err := executeAuthorizationCompiledRequest(
		ctx,
		extensionAuthorizationCompiledRequest{IsHTTPS: compiled.IsHTTPS},
		packet,
		baseline,
		workspace.Plan.Selector,
		workspace.comparisonKey,
	)
	if err != nil {
		output.Error = boundedAuthorizationError(err)
		return output
	}
	if err := validateAuthorizationRequestExecution(
		*result,
		baseline,
		workspace.Plan.Selector,
		reviewApproved,
	); err != nil {
		output.Error = boundedAuthorizationError(err)
		return output
	}
	output.State = "completed"
	output.Result = result
	return output
}

func skippedVerticalAuthorizationCase(
	planCase ExtensionAuthorizationPlanCase,
	reason string,
) ExtensionAuthorizationCaseExecution {
	return ExtensionAuthorizationCaseExecution{
		ID:                planCase.ID,
		Label:             planCase.Label,
		AuthContextSide:   planCase.AuthContextSide,
		ResourceValueSide: planCase.ResourceValueSide,
		State:             "skipped",
		Error:             reason,
	}
}

func (m *ExtensionBridgeManager) rebuildVerticalAuthorizationOperation(
	ctx context.Context,
	workspace ExtensionAuthorizationWorkspace,
	packet []byte,
) ([]byte, error) {
	if workspace.Plan == nil ||
		workspace.Plan.Operation == nil ||
		workspace.Plan.Operation.Transform == nil {
		return packet, validateVerticalAuthorizationProbePacket(
			packet,
			workspace.Baselines.Right,
			workspace.Baselines.Left,
			workspace.comparisonKey,
		)
	}
	binding := workspace.Plan.Operation.Transform
	current, err := m.inspectVerticalAuthorizationOperationTransform(
		ctx,
		workspace,
		workspace.Plan.CandidateID,
		binding.ProfileID,
	)
	if err != nil {
		return nil, err
	}
	if current.BindingFingerprint != binding.BindingFingerprint ||
		current.ProfileUpdatedAt != binding.ProfileUpdatedAt {
		return nil, errors.New(
			"low-privilege operation Transform Profile changed after plan creation",
		)
	}
	transformPacket, err := authorizationPacketToTransform(
		packet,
		workspace.Left.Origin,
	)
	if err != nil {
		return nil, err
	}
	if err := validateVerticalAuthorizationProbePacket(
		packet,
		workspace.Baselines.Right,
		workspace.Baselines.Left,
		workspace.comparisonKey,
	); err != nil {
		return nil, err
	}
	raw, err := m.CallDevice(
		ctx,
		workspace.Left.DeviceID,
		"browser.transform.execute",
		map[string]interface{}{
			"profileId": binding.ProfileID,
			"direction": "request",
			"packet":    transformPacket,
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"rebuild vertical authorization dynamic fields: %w",
			err,
		)
	}
	var execution extensionAuthorizationTransformExecution
	if err := decodeAuthorizationResult(raw, &execution); err != nil {
		return nil, err
	}
	output, err := applyVerticalAuthorizationTransformExecution(
		packet,
		transformPacket,
		execution,
		*binding,
	)
	if err != nil {
		return nil, err
	}
	if err := validateVerticalAuthorizationProbePacket(
		output,
		workspace.Baselines.Right,
		workspace.Baselines.Left,
		workspace.comparisonKey,
	); err != nil {
		return nil, err
	}
	return output, nil
}

func (m *ExtensionBridgeManager) executeVerticalExtensionAuthorizationPlan(
	ctx context.Context,
	workspace ExtensionAuthorizationWorkspace,
	reviewApproved bool,
) (ExtensionAuthorizationWorkspace, error) {
	if _, err := validateVerticalAuthorizationExecutionPlan(workspace); err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	snapshot := m.Snapshot()
	for _, slot := range []ExtensionAuthorizationIdentitySlot{
		workspace.Left,
		workspace.Right,
	} {
		if _, _, err := authorizationDevice(
			snapshot,
			slot.DeviceID,
			"browser.authorization.baseline.packet.compile",
		); err != nil {
			return ExtensionAuthorizationWorkspace{}, err
		}
	}
	if workspace.Plan.Operation.Transform != nil {
		if _, _, err := authorizationDevice(
			snapshot,
			workspace.Left.DeviceID,
			"browser.transform.profile.list",
			"browser.transform.execute",
		); err != nil {
			return ExtensionAuthorizationWorkspace{}, err
		}
	}
	leftRaw, rightRaw, err := m.callAuthorizationPair(
		ctx,
		workspace.Left.DeviceID,
		"browser.authorization.baseline.packet.compile",
		map[string]interface{}{"id": workspace.Baselines.Left.ID},
		workspace.Right.DeviceID,
		"browser.authorization.baseline.packet.compile",
		map[string]interface{}{"id": workspace.Baselines.Right.ID},
	)
	if err != nil {
		return ExtensionAuthorizationWorkspace{}, fmt.Errorf(
			"compile vertical authorization baseline packets: %w",
			err,
		)
	}
	var leftCompiled, rightCompiled extensionAuthorizationBaselinePacket
	if err := decodeAuthorizationResult(leftRaw, &leftCompiled); err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	if err := decodeAuthorizationResult(rightRaw, &rightCompiled); err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	leftPacket, err := validateAuthorizationBaselinePacket(
		leftCompiled,
		workspace.Baselines.Left,
		workspace.comparisonKey,
	)
	if err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	rightPacket, err := validateAuthorizationBaselinePacket(
		rightCompiled,
		workspace.Baselines.Right,
		workspace.comparisonKey,
	)
	if err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	var verificationCompiled extensionAuthorizationBaselinePacket
	var verificationPacket []byte
	if workspace.Baselines.Verification != nil {
		verificationRaw, err := m.CallDevice(
			ctx,
			workspace.Right.DeviceID,
			"browser.authorization.baseline.packet.compile",
			map[string]interface{}{"id": workspace.Baselines.Verification.ID},
		)
		if err != nil {
			return ExtensionAuthorizationWorkspace{}, fmt.Errorf(
				"compile vertical authorization verification packet: %w",
				err,
			)
		}
		if err := decodeAuthorizationResult(
			verificationRaw,
			&verificationCompiled,
		); err != nil {
			return ExtensionAuthorizationWorkspace{}, err
		}
		verificationPacket, err = validateAuthorizationBaselinePacket(
			verificationCompiled,
			workspace.Baselines.Verification,
			workspace.comparisonKey,
		)
		if err != nil {
			return ExtensionAuthorizationWorkspace{}, err
		}
	}
	probePacket, err := transplantAuthorizationAuthentication(
		rightPacket,
		workspace.Baselines.Right,
		leftPacket,
		workspace.Baselines.Left,
		workspace.comparisonKey,
	)
	if err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	probePacket, err = m.rebuildVerticalAuthorizationOperation(
		ctx,
		workspace,
		probePacket,
	)
	if err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}

	startedAt := time.Now().UnixMilli()
	execution := &ExtensionAuthorizationExecution{
		Version:     1,
		ID:          "authorization-execution-" + uuid.NewString(),
		WorkspaceID: workspace.ID,
		PlanID:      workspace.Plan.ID,
		State:       "running",
		Verdict:     "inconclusive",
		Confidence:  "none",
		Cases:       make([]ExtensionAuthorizationCaseExecution, len(workspace.Plan.Cases)),
		Evidence:    make([]ExtensionAuthorizationCanaryEvidence, 0, 4),
		Reasons: []string{
			fmt.Sprintf(
				"纵向请求预算固定为 %d 项且不自动重试；任一前置控制失败都会停止后续请求",
				len(workspace.Plan.Cases),
			),
			"低权限探测保留高权限操作的 route、业务 Body 与非认证 Header，只移植低权限认证与 CSRF 原始值",
		},
		StartedAt: startedAt,
	}
	if reviewApproved {
		execution.Reasons = append(
			execution.Reasons,
			"用户或当前 Agent Review 策略已明确批准纵向矩阵中的非只读请求；每项仍只执行一次",
		)
	}
	if workspace.Baselines.Verification != nil {
		execution.Reasons = append(
			execution.Reasons,
			"状态前快照位于高权限操作对照之后；只有低权限探测成功后才读取状态后快照",
		)
	}

	execution.Cases[0] = executeVerticalAuthorizationPacketCase(
		ctx,
		workspace,
		workspace.Plan.Cases[0],
		leftCompiled,
		leftPacket,
		workspace.Baselines.Left,
		reviewApproved,
	)
	execution.RequestCount++
	lowControlEligible := execution.Cases[0].State == "completed" &&
		execution.Cases[0].Result != nil &&
		execution.Cases[0].Result.Outcome == "success"
	skipRemaining := func(start int, reason string) {
		for index := start; index < len(workspace.Plan.Cases); index++ {
			execution.Cases[index] = skippedVerticalAuthorizationCase(
				workspace.Plan.Cases[index],
				reason,
			)
		}
	}
	if !lowControlEligible {
		skipRemaining(1, "low-privilege control failed")
	} else {
		execution.Cases[1] = executeVerticalAuthorizationPacketCase(
			ctx,
			workspace,
			workspace.Plan.Cases[1],
			rightCompiled,
			rightPacket,
			workspace.Baselines.Right,
			reviewApproved,
		)
		execution.RequestCount++
		privilegedControlEligible := execution.Cases[1].State == "completed" &&
			execution.Cases[1].Result != nil &&
			execution.Cases[1].Result.Outcome == "success"
		if !privilegedControlEligible {
			skipRemaining(2, "privileged operation control failed")
		} else {
			probeIndex := 2
			if workspace.Baselines.Verification != nil {
				execution.Cases[2] = executeVerticalAuthorizationPacketCase(
					ctx,
					workspace,
					workspace.Plan.Cases[2],
					verificationCompiled,
					verificationPacket,
					workspace.Baselines.Verification,
					reviewApproved,
				)
				execution.RequestCount++
				beforeEligible := execution.Cases[2].State == "completed" &&
					execution.Cases[2].Result != nil &&
					execution.Cases[2].Result.Outcome == "success"
				if !beforeEligible {
					skipRemaining(3, "post-state control failed")
				} else {
					probeIndex = 3
				}
			}
			if execution.Cases[probeIndex].State == "" {
				execution.Cases[probeIndex] = executeVerticalAuthorizationPacketCase(
					ctx,
					workspace,
					workspace.Plan.Cases[probeIndex],
					rightCompiled,
					probePacket,
					workspace.Baselines.Right,
					reviewApproved,
				)
				execution.RequestCount++
				if workspace.Baselines.Verification != nil {
					probeEligible := execution.Cases[probeIndex].State == "completed" &&
						execution.Cases[probeIndex].Result != nil &&
						execution.Cases[probeIndex].Result.Outcome == "success"
					if !probeEligible {
						skipRemaining(probeIndex+1, "low-privilege probe did not succeed")
					} else {
						execution.Cases[probeIndex+1] = executeVerticalAuthorizationPacketCase(
							ctx,
							workspace,
							workspace.Plan.Cases[probeIndex+1],
							verificationCompiled,
							verificationPacket,
							workspace.Baselines.Verification,
							reviewApproved,
						)
						execution.RequestCount++
					}
				}
			}
		}
	}
	evaluateVerticalAuthorizationExecution(
		execution,
		workspace.comparisonKey,
		workspace.Plan.CanaryPaths...,
	)
	for index := range execution.Cases {
		if execution.Cases[index].Result != nil {
			execution.Cases[index].Result.responseBody = nil
		}
	}
	execution.EvidenceAvailable = authorizationExecutionHasEvidenceBundle(execution)
	execution.CompletedAt = time.Now().UnixMilli()
	workspace.Execution = execution
	if err := m.updateExtensionAuthorizationWorkspace(workspace); err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	return workspace, nil
}

func (m *ExtensionBridgeManager) ExecuteExtensionAuthorizationPlan(
	ctx context.Context,
	input ExtensionAuthorizationExecutionInput,
) (ExtensionAuthorizationWorkspace, error) {
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.PlanID = strings.TrimSpace(input.PlanID)
	if input.WorkspaceID == "" || input.PlanID == "" {
		return ExtensionAuthorizationWorkspace{}, errors.New("authorization execution workspaceId and planId are required")
	}
	workspace, err := m.GetExtensionAuthorizationWorkspace(ctx, input.WorkspaceID, true)
	if err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	if workspace.Plan == nil || workspace.Plan.ID != input.PlanID {
		return ExtensionAuthorizationWorkspace{}, errors.New("authorization execution plan does not match the current workspace")
	}
	reviewApproved, err := validateAuthorizationExecutionPlanReview(
		workspace.Plan,
		input.ApproveSideEffects,
	)
	if err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	if eligibleExtensionAuthorizationIsolationMode(workspace) == "" {
		return ExtensionAuthorizationWorkspace{}, errors.New("authorization execution isolation proof is no longer eligible")
	}
	if workspace.Mode == "vertical" || workspace.Plan.Mode == "vertical" {
		if workspace.Mode != "vertical" || workspace.Plan.Mode != "vertical" {
			return ExtensionAuthorizationWorkspace{}, errors.New(
				"authorization workspace and execution plan modes do not match",
			)
		}
		return m.executeVerticalExtensionAuthorizationPlan(
			ctx,
			workspace,
			reviewApproved,
		)
	}
	wireSelector := workspace.Plan.Selector.Source == "wire" &&
		(workspace.Plan.Selector.Location == "header" ||
			workspace.Plan.Selector.Location == "path" ||
			workspace.Plan.Selector.Location == "query" ||
			workspace.Plan.Selector.Location == "body")
	logicalSelector := workspace.Plan.Selector.Source == "logical" &&
		workspace.Plan.Selector.Location == "body" &&
		workspace.Plan.Transforms != nil
	if !wireSelector && !logicalSelector {
		return ExtensionAuthorizationWorkspace{}, errors.New("authorization execution selector is not supported")
	}
	snapshot := m.Snapshot()
	compileCapability := "browser.authorization.baseline.compile"
	if workspace.Plan.Transforms != nil {
		compileCapability = "browser.authorization.baseline.transform.compile"
	}
	for _, slot := range []ExtensionAuthorizationIdentitySlot{workspace.Left, workspace.Right} {
		if _, _, err := authorizationDevice(
			snapshot,
			slot.DeviceID,
			"browser.authorization.baseline.resource.get",
			compileCapability,
		); err != nil {
			return ExtensionAuthorizationWorkspace{}, err
		}
	}
	selector := map[string]interface{}{
		"source":   workspace.Plan.Selector.Source,
		"location": workspace.Plan.Selector.Location,
		"path":     workspace.Plan.Selector.Path,
	}
	leftRaw, rightRaw, err := m.callAuthorizationPair(
		ctx,
		workspace.Left.DeviceID,
		"browser.authorization.baseline.resource.get",
		map[string]interface{}{"id": workspace.Baselines.Left.ID, "selector": selector},
		workspace.Right.DeviceID,
		"browser.authorization.baseline.resource.get",
		map[string]interface{}{"id": workspace.Baselines.Right.ID, "selector": selector},
	)
	if err != nil {
		return ExtensionAuthorizationWorkspace{}, fmt.Errorf("read authorization resource values: %w", err)
	}
	var leftResource, rightResource ExtensionAuthorizationResourceValue
	if err := decodeAuthorizationResult(leftRaw, &leftResource); err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	if err := decodeAuthorizationResult(rightRaw, &rightResource); err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	if err := validateAuthorizationResourceValue(
		leftResource,
		workspace.Baselines.Left,
		workspace.Plan.Selector,
	); err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	if err := validateAuthorizationResourceValue(
		rightResource,
		workspace.Baselines.Right,
		workspace.Plan.Selector,
	); err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	startedAt := time.Now().UnixMilli()
	execution := &ExtensionAuthorizationExecution{
		Version:     1,
		ID:          "authorization-execution-" + uuid.NewString(),
		WorkspaceID: workspace.ID,
		PlanID:      workspace.Plan.ID,
		State:       "running",
		Verdict:     "inconclusive",
		Confidence:  "none",
		Cases:       make([]ExtensionAuthorizationCaseExecution, 0, 4),
		Evidence:    make([]ExtensionAuthorizationCanaryEvidence, 0, 4),
		Reasons: []string{
			"请求预算固定为四项且不自动重试；资源值仅在引擎内存与加密桥接调用中短时传递",
		},
		StartedAt: startedAt,
	}
	if reviewApproved {
		execution.Reasons = append(
			execution.Reasons,
			"用户或当前 Agent Review 策略已明确批准四项非只读请求；每项仍只执行一次",
		)
	}
	resources := map[string]ExtensionAuthorizationResourceValue{
		"left":  leftResource,
		"right": rightResource,
	}
	controls := m.executeExtensionAuthorizationCasePair(
		ctx,
		workspace,
		workspace.Plan.Cases[:2],
		resources,
	)
	execution.Cases = append(execution.Cases, controls...)
	execution.RequestCount += len(controls)
	controlsEligible := len(controls) == 2 &&
		controls[0].State == "completed" &&
		controls[1].State == "completed" &&
		controls[0].Result != nil &&
		controls[1].Result != nil &&
		controls[0].Result.Outcome == "success" &&
		controls[1].Result.Outcome == "success"
	if controlsEligible {
		cross := m.executeExtensionAuthorizationCasePair(
			ctx,
			workspace,
			workspace.Plan.Cases[2:],
			resources,
		)
		execution.Cases = append(execution.Cases, cross...)
		execution.RequestCount += len(cross)
	} else {
		for _, planCase := range workspace.Plan.Cases[2:] {
			execution.Cases = append(execution.Cases, ExtensionAuthorizationCaseExecution{
				ID:                planCase.ID,
				Label:             planCase.Label,
				AuthContextSide:   planCase.AuthContextSide,
				ResourceValueSide: planCase.ResourceValueSide,
				State:             "skipped",
				Error:             "normal control failed",
			})
		}
	}
	evaluateExtensionAuthorizationExecution(
		execution,
		workspace.comparisonKey,
		workspace.Plan.CanaryPaths...,
	)
	for index := range execution.Cases {
		if execution.Cases[index].Result != nil {
			execution.Cases[index].Result.responseBody = nil
		}
	}
	execution.EvidenceAvailable = authorizationExecutionHasEvidenceBundle(execution)
	execution.CompletedAt = time.Now().UnixMilli()
	workspace.Execution = execution
	if err := m.updateExtensionAuthorizationWorkspace(workspace); err != nil {
		return ExtensionAuthorizationWorkspace{}, err
	}
	return workspace, nil
}
