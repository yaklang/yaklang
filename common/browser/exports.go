package browser

var Exports = map[string]interface{}{
	"Open":     Open,
	"Get":      Get,
	"List":     List,
	"Close":    CloseByID,
	"CloseAll": CloseAll,

	"id":        WithID,
	"headless":  WithHeadless,
	"proxy":     WithProxy,
	"timeout":   WithTimeout,
	"exePath":   WithExePath,
	"wsAddress": WithWsAddress,
	"noSandBox": WithNoSandBox,
	"leakless":  WithLeakless,
}
