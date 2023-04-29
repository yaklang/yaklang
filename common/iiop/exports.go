package iiop

var Exports = map[string]interface{}{
	"SendPayload":   SendPayload,
	"BindPayload":   GenBindOption,
	"RebindPayload": GenRebindOption,
	"InvokePayload": GenRemoteConstructorPayloadOption,
}
