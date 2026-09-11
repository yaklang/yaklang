package plugin

import (
	"embed"

	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/utils/xorencoded"
)

const pluginXorKey = "yaklang-plugin-v1"

//go:embed embed_data/getsuo5memfilterbytecode.b64.enc
//go:embed embed_data/getsuo5memservletbytecode.b64.enc
//go:embed embed_data/getwebappcomponentinfoscanbytecode.b64.enc
var pluginEncFS embed.FS

func loadPluginB64(filename string) []byte {
	content, err := xorencoded.LoadEmbedFile(pluginEncFS, filename, []byte(pluginXorKey))
	if err != nil {
		panic(err)
	}
	code, _ := codec.DecodeBase64(content)
	return code
}

// GetSuo5MemFilterByteCode precompiled Suo5 byte code using java6
func GetSuo5MemFilterByteCode() []byte {
	return loadPluginB64("embed_data/getsuo5memfilterbytecode.b64.enc")
}

// GetSuo5MemServletByteCode precompiled Suo5 servlet byte code using java6
func GetSuo5MemServletByteCode() []byte {
	return loadPluginB64("embed_data/getsuo5memservletbytecode.b64.enc")
}

// GetWebAppComponentInfoScanByteCode precompiled WebApp component info scan byte code
func GetWebAppComponentInfoScanByteCode() []byte {
	return loadPluginB64("embed_data/getwebappcomponentinfoscanbytecode.b64.enc")
}
