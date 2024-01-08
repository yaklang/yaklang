package openapi

import (
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strings"
)

func OpenAPITypeToMockDataLiteral(t string, defaults ...any) any {
	defaults = funk.Filter(defaults, funk.NotEmpty).([]any)
	if len(defaults) > 0 {
		return defaults[0]
	}
	switch ret := strings.ToLower(t); ret {
	case "string":
		return "mock_string_data"
	case "integer", "int":
		return 1
	case "number":
		return 1
	case "boolean", "bool":
		return false
	}
	return "{}"
}

func ValueViaField(field string, t string, defaults ...any) any {
	switch strings.ToLower(t) {
	case "string", "":
		field = strings.ToLower(field)
		switch {
		case utils.MatchAnyOfGlob(field, "base64", "base64*", "*base64"):
			return codec.EncodeBase64([]byte("mock_base64_data"))
		case utils.MatchAnyOfGlob(field, "sid", "uid", "uuid", "*uid"):
			return uuid.NewV4().String()
		case utils.MatchAnyOfGlob(field, "nick*name", "last*name", "first*name", "name"):
			return utils.RandChoice("Foo", "Bar", "Tom", "Jerry", "Alice", "Bob", "John", "Jane")
		case utils.MatchAnyOfGlob(field, "user*name", "user", "username", "userid", "user*id"):
			return utils.RandChoice("admin", "root", "user", "guest", "test")
		case utils.MatchAnyOfGlob(field, "password"):
			return utils.RandChoice("admin123", "root123", "user123", "guest123", "test123")
		case utils.MatchAnyOfGlob(field, "email", "mail"):
			return utils.RandChoice("admin@example.com")
		case utils.MatchAnyOfGlob(field, "phone", "mobile"):
			return utils.RandChoice("13800000001", "13900000001", "13800000002", "13900000002")
		case utils.MatchAnyOfGlob(field, "url", "link"):
			return utils.RandChoice("https://www.example.com", "https://www.example.org", "https://www.example.net")
		case utils.MatchAnyOfGlob(field, "ip", "ipaddr", "ipaddress", "ip*addr", "ip*address"):
			return utils.RandChoice("127.0.0.1")
		case utils.MatchAnyOfGlob(field, "title", "subject"):
			return utils.RandChoice("mock_title")
		default:
			return "mock_" + field
		}
	}
	return OpenAPITypeToMockDataLiteral(t, defaults...)
}
