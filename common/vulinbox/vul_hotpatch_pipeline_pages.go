package vulinbox

import (
	_ "embed"
	"net/http"
)

//go:embed html/vul_hotpatch_pipeline_console.html
var hotPatchPipelineConsoleHTML string

func hotPatchPipelineRenderDocs(writer http.ResponseWriter, request *http.Request) {
	unsafeTemplateRender(writer, request, hotPatchPipelineConsoleHTML, hotPatchPipelinePageData())
}

func hotPatchPipelineRedirectConsole(writer http.ResponseWriter, request *http.Request) {
	http.Redirect(writer, request, hotPatchPipelineDocsPath+"#console", http.StatusFound)
}

func hotPatchPipelinePageData() map[string]any {
	return map[string]any{
		"BootstrapKey":  string(hotPatchPipelineBootstrapKey),
		"DocsPath":      hotPatchPipelineDocsPath,
		"BootstrapPath": hotPatchPipelineBootstrapPath,
		"SearchPath":    hotPatchPipelineOrdersPath,
		"NormalKeyword": "商品4",
		"InjectKeyword": "' OR 1=1 -- ",
		"Status":        hotPatchPipelineDefaultStatus,
		"Page":          1,
		"Size":          hotPatchPipelineDefaultPerPage,
	}
}
