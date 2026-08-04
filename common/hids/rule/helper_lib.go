//go:build hids

package rule

import (
	pathpkg "path"
	"reflect"
	"strings"

	"github.com/yaklang/yaklang/common/hids/model"
	"github.com/yaklang/yaklang/common/hids/policy"
)

func pathNormalize(value string) string {
	return policy.NormalizePath(value)
}

func pathGlob(pattern string, value string) bool {
	normalizedPattern := normalizeHelperGlobPattern(pattern)
	normalizedValue := policy.NormalizePath(value)
	if normalizedPattern == "" || normalizedValue == "" {
		return false
	}
	matched, err := pathpkg.Match(normalizedPattern, normalizedValue)
	return err == nil && matched
}

func pathAnyGlob(value string, patterns ...any) bool {
	for _, pattern := range flattenHelperStrings(patterns...) {
		if pathGlob(pattern, value) {
			return true
		}
	}
	return false
}

func pathUnder(root string, value string) bool {
	normalizedRoot := policy.NormalizePath(root)
	normalizedValue := policy.NormalizePath(value)
	if normalizedRoot == "" || normalizedValue == "" {
		return false
	}
	normalizedRoot = strings.TrimSuffix(normalizedRoot, "/")
	return normalizedValue == normalizedRoot ||
		strings.HasPrefix(normalizedValue, normalizedRoot+"/")
}

func pathAnyUnder(value string, roots ...any) bool {
	for _, root := range flattenHelperStrings(roots...) {
		if pathUnder(root, value) {
			return true
		}
	}
	return false
}

func artifactPath(input any) string {
	return helperStringField(helperGeneralMap(input), "path")
}

func artifactExists(input any) bool {
	return helperBoolField(helperGeneralMap(input), "exists")
}

func artifactFileType(input any) string {
	return strings.ToLower(helperStringField(helperGeneralMap(input), "file_type"))
}

func artifactFileTypeIs(input any, fileTypes ...any) bool {
	fileType := artifactFileType(input)
	if fileType == "" {
		return false
	}
	for _, candidate := range flattenHelperStrings(fileTypes...) {
		if strings.EqualFold(fileType, candidate) {
			return true
		}
	}
	return false
}

func artifactIsELF(input any) bool {
	if artifactFileTypeIs(input, "elf") {
		return true
	}
	artifact := helperGeneralMap(input)
	if len(helperGeneralMap(artifact["elf"])) != 0 {
		return true
	}
	return strings.HasPrefix(strings.ToLower(helperStringField(artifact, "magic")), "7f454c46")
}

func artifactSHA256(input any) string {
	hashes := helperGeneralMap(helperGeneralMap(input)["hashes"])
	return strings.ToLower(helperStringField(hashes, "sha256"))
}

func artifactSHA256In(input any, hashes ...any) bool {
	current := artifactSHA256(input)
	if current == "" {
		return false
	}
	for _, candidate := range flattenHelperStrings(hashes...) {
		if strings.EqualFold(current, candidate) {
			return true
		}
	}
	return false
}

func artifactMD5(input any) string {
	hashes := helperGeneralMap(helperGeneralMap(input)["hashes"])
	return strings.ToLower(helperStringField(hashes, "md5"))
}

func artifactMD5In(input any, hashes ...any) bool {
	current := artifactMD5(input)
	if current == "" {
		return false
	}
	for _, candidate := range flattenHelperStrings(hashes...) {
		if strings.EqualFold(current, candidate) {
			return true
		}
	}
	return false
}

func artifactMachine(input any) string {
	elf := helperGeneralMap(helperGeneralMap(input)["elf"])
	return helperStringField(elf, "machine")
}

func artifactPathGlob(input any, pattern string) bool {
	return pathGlob(pattern, artifactPath(input))
}

func artifactPathAnyGlob(input any, patterns ...any) bool {
	return pathAnyGlob(artifactPath(input), patterns...)
}

func artifactPathUnder(input any, root string) bool {
	return pathUnder(root, artifactPath(input))
}

func artifactPathAnyUnder(input any, roots ...any) bool {
	return pathAnyUnder(artifactPath(input), roots...)
}

func auditFamilyIs(input any, families ...any) bool {
	return helperStringIn(helperStringField(helperGeneralMap(input), "family"), families...)
}

func auditResultIs(input any, results ...any) bool {
	return helperStringIn(helperStringField(helperGeneralMap(input), "result"), results...)
}

func auditActionIs(input any, actions ...any) bool {
	return helperStringIn(helperStringField(helperGeneralMap(input), "action"), actions...)
}

func auditHasRecordType(input any, recordType string) bool {
	return helperStringSliceContains(helperStringSliceField(helperGeneralMap(input), "record_types"), recordType)
}

func auditAnyRecordType(input any, recordTypes ...any) bool {
	for _, recordType := range flattenHelperStrings(recordTypes...) {
		if auditHasRecordType(input, recordType) {
			return true
		}
	}
	return false
}

func auditHasRemotePeer(input any) bool {
	values := helperGeneralMap(input)
	return helperStringField(values, "remote_ip") != "" || helperStringField(values, "remote_host") != ""
}

func normalizeHelperGlobPattern(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, `\`, "/")
	value = pathpkg.Clean(value)
	if value == "." {
		return ""
	}
	return value
}

func flattenHelperStrings(values ...any) []string {
	if len(values) == 0 {
		return nil
	}
	flattened := make([]string, 0, len(values))
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				flattened = append(flattened, trimmed)
			}
		case []string:
			for _, item := range typed {
				if trimmed := strings.TrimSpace(item); trimmed != "" {
					flattened = append(flattened, trimmed)
				}
			}
		case []any:
			flattened = append(flattened, flattenHelperStrings(typed...)...)
		default:
			reflected := reflect.ValueOf(value)
			if reflected.IsValid() && reflected.Kind() == reflect.Slice {
				items := make([]any, 0, reflected.Len())
				for index := 0; index < reflected.Len(); index++ {
					items = append(items, reflected.Index(index).Interface())
				}
				flattened = append(flattened, flattenHelperStrings(items...)...)
			}
		}
	}
	return flattened
}

func helperGeneralMap(input any) map[string]any {
	switch typed := input.(type) {
	case nil:
		return nil
	case map[string]any:
		return typed
	case model.Artifact:
		return buildArtifactContext(&typed)
	case *model.Artifact:
		return buildArtifactContext(typed)
	}

	reflected := reflect.ValueOf(input)
	if !reflected.IsValid() || reflected.Kind() != reflect.Map {
		return nil
	}
	result := make(map[string]any, reflected.Len())
	for _, key := range reflected.MapKeys() {
		if key.Kind() != reflect.String {
			continue
		}
		result[key.String()] = reflected.MapIndex(key).Interface()
	}
	return result
}

func helperStringField(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	value, ok := values[key]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func helperBoolField(values map[string]any, key string) bool {
	if len(values) == 0 {
		return false
	}
	value, ok := values[key]
	if !ok {
		return false
	}
	typed, ok := value.(bool)
	return ok && typed
}

func helperStringSliceField(values map[string]any, key string) []string {
	if len(values) == 0 {
		return nil
	}
	return flattenHelperStrings(values[key])
}

func helperStringIn(value string, candidates ...any) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, candidate := range flattenHelperStrings(candidates...) {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}
	return false
}

func helperStringSliceContains(values []string, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return false
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), want) {
			return true
		}
	}
	return false
}
