package yaklib

import "os"

// Get 获取对应键名的环境变量值
// ! 已弃用，可以使用 `os.Getenv` 代替
// 参数:
//   - key: 环境变量名
//
// 返回值:
//   - 环境变量的值，不存在时返回空字符串
//
// Example:
// ```
// // VARS: 先写入再读取，保证示例确定性
// env.Set("YAK_DOC_ENV", "demo")
// result = env.Get("YAK_DOC_ENV")
// // STDOUT: 打印读取到的值
// println(result)   // OUT: demo
// // assert: 锁定结论
// assert result == "demo", "Get should read the env var just set"
// ```
func _getEnv(key string) string {
	return os.Getenv(key)
}

// Set 设置对应键名的环境变量值
// ! 已弃用，可以使用 `os.Setenv` 代替
// 参数:
//   - key: 环境变量名
//   - value: 要设置的值
//
// Example:
// ```
// // VARS: 设置环境变量后再读回校验
// env.Set("YAK_DOC_ENV2", "ok")
// result = env.Get("YAK_DOC_ENV2")
// // STDOUT: 打印读取到的值
// println(result)   // OUT: ok
// // assert: 锁定结论
// assert result == "ok", "Set should persist the env var"
// ```
func _setEnv(key string, value string) {
	os.Setenv(key, value)
}

var EnvExports = map[string]interface{}{
	"Get": _getEnv,
	"Set": _setEnv,
}
