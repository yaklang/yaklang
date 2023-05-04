package yaklib

import "os"

var EnvExports = map[string]interface{}{
	"Get": func(key string) string {
		return os.Getenv(key)
	},
	"Set": func(key string, value string) {
		_ = os.Setenv(key, value)
	},
}
