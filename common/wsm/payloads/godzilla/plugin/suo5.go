//go:generate gzip-embed -cache --source ./static --gz static.tar.gz --xor-key yaklang-plugin-v1 --no-embed
package plugin

import (
	"embed"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

const pluginXorKey = "yaklang-plugin-v1"

//go:embed static
var pluginRawFS embed.FS

var pluginFS = filesys.NewEmbedSubFS(pluginRawFS, "static")

func loadPluginB64(filename string) []byte {
	raw, err := pluginFS.ReadFile(filename)
	if err != nil {
		panic(err)
	}
	code, _ := codec.DecodeBase64(string(raw))
	return code
}

// GetSuo5MemFilterByteCode precompiled Suo5 byte code using java6
func GetSuo5MemFilterByteCode() []byte {
	return loadPluginB64("getsuo5memfilterbytecode.b64")
}

// GetSuo5MemServletByteCode precompiled Suo5 servlet byte code using java6
func GetSuo5MemServletByteCode() []byte {
	return loadPluginB64("getsuo5memservletbytecode.b64")
}

// GetWebAppComponentInfoScanByteCode precompiled WebApp component info scan byte code
func GetWebAppComponentInfoScanByteCode() []byte {
	return loadPluginB64("getwebappcomponentinfoscanbytecode.b64")
}
