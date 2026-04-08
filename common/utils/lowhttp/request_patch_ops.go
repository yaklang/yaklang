package lowhttp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
)

func PatchHTTPPacketJSONField(packet []byte, operation, fieldName, fieldValue string) ([]byte, error) {
	_, body := SplitHTTPPacketFast(packet)
	payload := strings.TrimSpace(string(body))

	var jsonMap map[string]interface{}
	if payload == "" {
		jsonMap = map[string]interface{}{}
	} else {
		if err := json.Unmarshal([]byte(payload), &jsonMap); err != nil {
			return nil, fmt.Errorf("body.json patch requires a top-level JSON object body: %w", err)
		}
		if jsonMap == nil {
			jsonMap = map[string]interface{}{}
		}
	}

	switch operation {
	case "add", "replace":
		var existingValue interface{}
		if current, ok := jsonMap[fieldName]; ok {
			existingValue = current
		}
		jsonMap[fieldName] = parseJSONValueWithExistingType(fieldValue, existingValue)
	case "remove":
		delete(jsonMap, fieldName)
	default:
		return nil, fmt.Errorf("unsupported json body patch operation: %s", operation)
	}

	packet = ReplaceHTTPPacketJsonBody(packet, jsonMap)
	packet = ReplaceHTTPPacketHeader(packet, "Content-Type", "application/json")
	return packet, nil
}

func TransformHTTPPacketBodyFormat(packet []byte, targetFormat, xmlRoot string) ([]byte, error) {
	targetFormat = strings.TrimSpace(strings.ToLower(targetFormat))
	if targetFormat == "" {
		return nil, fmt.Errorf("target format is empty")
	}

	_, body := SplitHTTPPacketFast(packet)
	bodyText := strings.TrimSpace(string(body))
	switch targetFormat {
	case "xml", "application/xml", "text/xml":
		var payload interface{}
		if err := json.Unmarshal([]byte(bodyText), &payload); err != nil {
			return nil, fmt.Errorf("body.format xml currently requires a JSON body: %w", err)
		}
		if xmlRoot == "" {
			xmlRoot = "root"
		}
		xmlBody, err := convertValueToXML(xmlRoot, payload)
		if err != nil {
			return nil, err
		}
		packet = ReplaceHTTPPacketBodyFast(packet, xmlBody)
		packet = ReplaceHTTPPacketHeader(packet, "Content-Type", "application/xml")
		return packet, nil
	case "json", "application/json":
		values, err := extractHTTPRequestBodyParams(packet)
		if err != nil {
			return nil, err
		}
		packet = ReplaceHTTPPacketJsonBody(packet, values)
		packet = ReplaceHTTPPacketHeader(packet, "Content-Type", "application/json")
		return packet, nil
	case "form", "application/x-www-form-urlencoded":
		var jsonMap map[string]interface{}
		if err := json.Unmarshal([]byte(bodyText), &jsonMap); err != nil {
			return nil, fmt.Errorf("body.format form currently requires a JSON object body: %w", err)
		}
		formPacket := packet
		for key, value := range jsonMap {
			formPacket = ReplaceHTTPPacketPostParam(formPacket, key, utils.InterfaceToString(value))
		}
		formPacket = ReplaceHTTPPacketHeader(formPacket, "Content-Type", "application/x-www-form-urlencoded")
		return formPacket, nil
	default:
		return nil, fmt.Errorf("unsupported target body format: %s", targetFormat)
	}
}

func ReplaceHTTPPacketBasicAuthByPatch(packet []byte, fieldName, fieldValue string) ([]byte, error) {
	username, password, err := parseBasicCredentialPatch(fieldName, fieldValue)
	if err != nil {
		return nil, err
	}
	return ReplaceHTTPPacketBasicAuth(packet, username, password), nil
}

func RewriteHTTPPacketBearerJWTClaims(packet []byte, claimsJSON string) ([]byte, error) {
	authValue := strings.TrimSpace(GetHTTPPacketHeader(packet, "Authorization"))
	if !strings.HasPrefix(strings.ToLower(authValue), "bearer ") {
		return nil, fmt.Errorf("Authorization header is not a Bearer token")
	}
	token := strings.TrimSpace(authValue[len("Bearer "):])
	if token == "" {
		return nil, fmt.Errorf("Bearer token is empty")
	}

	claimsPatch := make(map[string]interface{})
	if err := json.Unmarshal([]byte(claimsJSON), &claimsPatch); err != nil {
		return nil, fmt.Errorf("auth.bearer.jwt field_value must be a JSON object: %w", err)
	}
	rewrittenToken, err := rewriteJWTClaimsWithoutVerification(token, claimsPatch)
	if err != nil {
		return nil, err
	}
	return ReplaceHTTPPacketHeader(packet, "Authorization", "Bearer "+rewrittenToken), nil
}

func RepairHTTPRequestPacket(packet []byte, profile string) ([]byte, error) {
	if profile == "" {
		profile = "basic"
	}
	if profile != "basic" && profile != "browser_like" {
		return nil, fmt.Errorf("repair_profile must be one of: basic, browser_like")
	}

	repaired := FixHTTPRequest(packet)
	req, err := ParseBytesToHttpRequest(repaired)
	if err == nil && req != nil {
		if strings.TrimSpace(req.Host) == "" && req.URL != nil && strings.TrimSpace(req.URL.Host) != "" {
			repaired = ReplaceHTTPPacketHost(repaired, req.URL.Host)
		}
	}

	repaired = AppendHTTPPacketHeaderIfNotExist(repaired, "User-Agent", consts.DefaultUserAgent)
	repaired = AppendHTTPPacketHeaderIfNotExist(repaired, "Accept", "*/*")
	repaired = AppendHTTPPacketHeaderIfNotExist(repaired, "Connection", "close")
	if profile == "browser_like" {
		repaired = AppendHTTPPacketHeaderIfNotExist(repaired, "Accept-Language", "en-US,en;q=0.9")
	}

	if contentType := strings.TrimSpace(GetHTTPPacketHeader(repaired, "Content-Type")); contentType == "" {
		_, body := SplitHTTPPacketFast(repaired)
		if len(body) > 0 {
			bodyText := strings.TrimSpace(string(body))
			switch {
			case strings.HasPrefix(bodyText, "{") || strings.HasPrefix(bodyText, "["):
				repaired = ReplaceHTTPPacketHeader(repaired, "Content-Type", "application/json")
			case strings.HasPrefix(bodyText, "<") && strings.HasSuffix(bodyText, ">"):
				repaired = ReplaceHTTPPacketHeader(repaired, "Content-Type", "application/xml")
			default:
				repaired = ReplaceHTTPPacketHeader(repaired, "Content-Type", "application/x-www-form-urlencoded")
			}
		}
	}
	return FixHTTPRequest(repaired), nil
}

func parseFlexibleJSONValue(raw string) interface{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var parsed interface{}
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		return parsed
	}
	return raw
}

func parseJSONValueWithExistingType(raw string, existing interface{}) interface{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if existing == nil {
		return parseFlexibleJSONValue(raw)
	}

	switch existing.(type) {
	case string:
		if strings.HasPrefix(raw, `"`) && strings.HasSuffix(raw, `"`) {
			return parseFlexibleJSONValue(raw)
		}
		return raw
	case bool:
		return parseFlexibleJSONValue(raw)
	case float64, float32, int, int32, int64, uint, uint32, uint64:
		return parseFlexibleJSONValue(raw)
	case map[string]interface{}, []interface{}:
		return parseFlexibleJSONValue(raw)
	default:
		return parseFlexibleJSONValue(raw)
	}
}

func extractHTTPRequestBodyParams(packet []byte) (map[string]interface{}, error) {
	contentType := GetHTTPPacketHeader(packet, "Content-Type")
	_, body := SplitHTTPPacketFast(packet)
	params, useRaw, err := GetParamsFromBody(contentType, body)
	if err != nil || useRaw {
		return nil, fmt.Errorf("unable to extract structured body params for format transform")
	}
	result := make(map[string]interface{}, len(params.Items))
	for _, item := range params.Items {
		if len(item.Values) == 1 {
			result[item.Key] = item.Values[0]
			continue
		}
		values := make([]string, len(item.Values))
		copy(values, item.Values)
		result[item.Key] = values
	}
	return result, nil
}

func convertValueToXML(root string, payload interface{}) ([]byte, error) {
	var out strings.Builder
	out.WriteString(xml.Header)
	writeXMLNode(&out, root, payload)
	return []byte(out.String()), nil
}

func writeXMLNode(out *strings.Builder, name string, value interface{}) {
	name = sanitizeXMLName(name)
	out.WriteString("<")
	out.WriteString(name)
	out.WriteString(">")

	switch typed := value.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			writeXMLNode(out, key, typed[key])
		}
	case []interface{}:
		for _, item := range typed {
			writeXMLNode(out, "item", item)
		}
	case []string:
		for _, item := range typed {
			writeXMLNode(out, "item", item)
		}
	default:
		var escaped bytes.Buffer
		_ = xml.EscapeText(&escaped, []byte(utils.InterfaceToString(value)))
		out.Write(escaped.Bytes())
	}

	out.WriteString("</")
	out.WriteString(name)
	out.WriteString(">")
}

func sanitizeXMLName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "item"
	}
	replacer := strings.NewReplacer(" ", "_", "-", "_", ".", "_", ":", "_", "/", "_")
	return replacer.Replace(name)
}

func parseBasicCredentialPatch(fieldName, fieldValue string) (string, string, error) {
	raw := strings.TrimSpace(fieldValue)
	if raw != "" && strings.HasPrefix(raw, "{") {
		var payload struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			return "", "", fmt.Errorf("auth.basic field_value JSON parse failed: %w", err)
		}
		if payload.Username == "" || payload.Password == "" {
			return "", "", fmt.Errorf("auth.basic JSON requires username and password")
		}
		return payload.Username, payload.Password, nil
	}
	if fieldName != "" && raw != "" {
		return fieldName, raw, nil
	}
	if raw != "" {
		parts := strings.SplitN(raw, ":", 2)
		if len(parts) == 2 && parts[0] != "" {
			return parts[0], parts[1], nil
		}
	}
	return "", "", fmt.Errorf("auth.basic requires username/password via field_name+field_value, value 'user:pass', or JSON payload")
}

func rewriteJWTClaimsWithoutVerification(token string, claimsPatch map[string]interface{}) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 && len(parts) != 3 {
		return "", fmt.Errorf("JWT must have 2 or 3 segments")
	}

	headerBytes, err := decodeJWTBase64URL(parts[0])
	if err != nil {
		return "", fmt.Errorf("decode JWT header failed: %w", err)
	}
	payloadBytes, err := decodeJWTBase64URL(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode JWT payload failed: %w", err)
	}

	var header map[string]interface{}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return "", fmt.Errorf("parse JWT header failed: %w", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return "", fmt.Errorf("parse JWT payload failed: %w", err)
	}
	if payload == nil {
		payload = map[string]interface{}{}
	}
	for key, value := range claimsPatch {
		payload[key] = value
	}

	newHeaderBytes, err := json.Marshal(header)
	if err != nil {
		return "", fmt.Errorf("marshal JWT header failed: %w", err)
	}
	newPayloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal JWT payload failed: %w", err)
	}

	encodedHeader := encodeJWTBase64URL(newHeaderBytes)
	encodedPayload := encodeJWTBase64URL(newPayloadBytes)
	if len(parts) == 2 {
		return encodedHeader + "." + encodedPayload, nil
	}
	return encodedHeader + "." + encodedPayload + "." + parts[2], nil
}

func decodeJWTBase64URL(raw string) ([]byte, error) {
	if decoded, err := base64.RawURLEncoding.DecodeString(raw); err == nil {
		return decoded, nil
	}
	return base64.URLEncoding.DecodeString(raw)
}

func encodeJWTBase64URL(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}
