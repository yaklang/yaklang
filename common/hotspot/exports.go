package hotspot

// Exports exports the hotspot functions to Yaklang
var Exports = map[string]interface{}{
	"Enable":    Enable,
	"Disable":   Disable,
	"GetStatus": GetStatus,
}
