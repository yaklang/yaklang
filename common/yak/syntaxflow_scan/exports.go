package syntaxflow_scan

import "github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"

var Exports = map[string]any{
	"StartScan":     StartScan,
	"ResumeScan":    ResumeScan,
	"GetScanStatus": GetScanStatus,
	// 进度
	"withScanProcessCallback": WithProcessCallback,
	"withScanResultCallback":  WithScanResultCallback,
	"withScanPrograms":        withPrograms,
	"withScanConcurrency":     ssaconfig.WithScanConcurrency,
}
