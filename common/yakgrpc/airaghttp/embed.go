package airaghttp

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"io"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
)

// frontendHTMLGzip 内置只读搜索前端页面的 gzip 压缩字节.
// 仅嵌入压缩后的内容以尽量减小编译产物体积; 运行时解压一次并缓存.
// 源文件为同目录 web/index.html, 修改后需重新生成 web/index.html.gz:
//
//	cd common/yakgrpc/airaghttp && go generate ./...
//
// 关键词: go:embed gzip frontend, smaller binary, decompress once
//
//go:generate sh -c "gzip -9 -c web/index.html > web/index.html.gz"
//
//go:embed web/index.html.gz
var frontendHTMLGzip []byte

// 页面中用于注入实际路由前缀的占位符
const frontendPrefixPlaceholder = "__RAG_ROUTE_PREFIX__"

var (
	frontendHTMLOnce sync.Once
	frontendHTMLTpl  string
)

// frontendHTML 解压并缓存嵌入的前端 HTML 模板 (含占位符)
func frontendHTML() string {
	frontendHTMLOnce.Do(func() {
		r, err := gzip.NewReader(bytes.NewReader(frontendHTMLGzip))
		if err != nil {
			log.Errorf("open embedded frontend gzip failed: %v", err)
			return
		}
		defer r.Close()
		raw, err := io.ReadAll(r)
		if err != nil {
			log.Errorf("decompress embedded frontend gzip failed: %v", err)
			return
		}
		frontendHTMLTpl = string(raw)
	})
	return frontendHTMLTpl
}

// renderFrontendHTML 将占位符替换为实际路由前缀后返回完整 HTML
func renderFrontendHTML(routePrefix string) string {
	if routePrefix == "" {
		routePrefix = "/api/rag-server"
	}
	return strings.ReplaceAll(frontendHTML(), frontendPrefixPlaceholder, routePrefix)
}
