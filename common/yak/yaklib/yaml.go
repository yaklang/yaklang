package yaklib

import (
	"gopkg.in/yaml.v2"
)

// Marshal 将一个对象序列化为 YAML 格式的字节切片
// 参数:
//   - in: 待序列化的对象，可以是 map、切片或结构体
//
// 返回值:
//   - 序列化后的 YAML 字节切片
//   - 序列化失败时返回的错误
//
// Example:
// ```
// // VARS: 把 map 序列化为 YAML
// out = yaml.Marshal({"name": "yak"})~
// text = string(out)
// // assert: 输出包含对应键值(YAML 多行输出顺序可能变化，用 Contains 判断)
// assert str.Contains(text, "name: yak"), "marshal output should contain the key-value"
// ```
func yamlMarshal(in interface{}) ([]byte, error) {
	return yaml.Marshal(in)
}

// Unmarshal 将 YAML 格式的字节切片反序列化为对应的对象
// 参数:
//   - b: 待解析的 YAML 字节切片
//
// 返回值:
//   - 解析得到的对象(通常是 map 或切片)
//   - 解析失败时返回的错误
//
// Example:
// ```
// // VARS: 把 YAML 文本解析为 map
// m = yaml.Unmarshal([]byte("name: yak\nport: 80\n"))~
// // STDOUT: 打印 name 字段
// println(m["name"])   // OUT: yak
// // assert: 数值字段被解析为整数
// assert m["port"] == 80, "unmarshal should parse port as 80"
// ```
func yamlUnmarshal(b []byte) (interface{}, error) {
	var i interface{}
	err := yaml.Unmarshal(b, &i)
	if err != nil {
		return nil, err
	}
	return i, nil
}

// UnmarshalStrict 严格模式反序列化 YAML，遇到未知字段或重复键会报错
// 参数:
//   - b: 待解析的 YAML 字节切片
//
// 返回值:
//   - 解析得到的对象(通常是 map 或切片)
//   - 解析失败时返回的错误
//
// Example:
// ```
// // VARS: 严格模式解析 YAML 文本
// m = yaml.UnmarshalStrict([]byte("name: yak\nport: 80\n"))~
// // STDOUT: 打印 name 字段
// println(m["name"])   // OUT: yak
// // assert: 数值字段被解析为整数
// assert m["port"] == 80, "strict unmarshal should parse port as 80"
// ```
func yamlUnmarshalStrict(b []byte) (interface{}, error) {
	var i interface{}
	err := yaml.UnmarshalStrict(b, &i)
	if err != nil {
		return nil, err
	}
	return i, nil
}

var YamlExports = map[string]interface{}{
	"Marshal":         yamlMarshal,
	"Unmarshal":       yamlUnmarshal,
	"UnmarshalStrict": yamlUnmarshalStrict,
}
