//go:build gzip_embed

package consts

import (
	"embed"
	"fmt"
)

//go:embed aitool-metadata.json.gz coreplugin-metadata.json.gz forge-metadata.json.gz syntaxflow-metadata.json.gz
var builtinMetadataFiles embed.FS

// BuiltinMetadata returns the compressed artifact and its expected SHA-256.
func BuiltinMetadata(kind string) ([]byte, string, error) {
	var hash string
	switch kind {
	case "aitool":
		hash = ExistedBuildInAIToolEmbedFSHash
	case "coreplugin":
		hash = ExistedCorePluginEmbedFSHash
	case "forge":
		hash = ExistedBuildInForgeEmbedFSHash
	case "syntaxflow":
		hash = ExistedSyntaxFlowEmbedFSHash
	default:
		return nil, "", fmt.Errorf("unknown script set: %s", kind)
	}
	data, err := builtinMetadataFiles.ReadFile(kind + "-metadata.json.gz")
	return data, hash, err
}
