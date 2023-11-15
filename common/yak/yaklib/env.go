package yaklib

import "os"

// Get 获取对应键名的环境变量值
// ! 已弃用，可以使用 `os.Getenv` 代替
// Example:
// ```
// env.Get("PATH")
// ```
func _getEnv(key string) string {
	return os.Getenv(key)
}

// Set 设置对应键名的环境变量值
// ! 已弃用，可以使用 `os.Setenv` 代替
// Example:
// ```
// env.Set("YAK_PROXY", "http://127.0.0.1:10808")
// ```
func _setEnv(key string, value string) {
	os.Setenv(key, value)
}

var EnvExports = map[string]interface{}{
	"Get": _getEnv,
	"Set": _setEnv,
}
