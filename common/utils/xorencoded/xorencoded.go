// Package xorencoded 提供简单的 embed + XOR 解码工具，
// 用于将可能被杀软误报的字符串内容从 Go 源码中抽离到独立文件，
// 运行时通过 XOR 解码恢复，避免二进制中出现明文特征。
package xorencoded

import (
	"embed"
	"fmt"
	"os"
	"strings"
)

// XorDecode 对 raw 做 XOR 解码（key 循环使用）。
func XorDecode(raw []byte, key []byte) []byte {
	if len(key) == 0 {
		return raw
	}
	out := make([]byte, len(raw))
	for i, b := range raw {
		out[i] = b ^ key[i%len(key)]
	}
	return out
}

// XorEncode 对 raw 做 XOR 编码（key 循环使用），在构建期生成编码文件时使用。
func XorEncode(raw []byte, key []byte) []byte {
	return XorDecode(raw, key) // XOR 编解码对称
}

// LoadEmbedFile 从 embed.FS 读取指定文件并 XOR 解码，返回解码后的字符串。
func LoadEmbedFile(fs embed.FS, filename string, key []byte) (string, error) {
	raw, err := fs.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("read embed file %s failed: %v", filename, err)
	}
	decoded := XorDecode(raw, key)
	return string(decoded), nil
}

// LoadEmbedBytes 从 embed.FS 读取指定文件并 XOR 解码，返回解码后的字节。
func LoadEmbedBytes(fs embed.FS, filename string, key []byte) ([]byte, error) {
	raw, err := fs.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read embed file %s failed: %v", filename, err)
	}
	return XorDecode(raw, key), nil
}

// EncodeFileToFile 对 src 文件做 XOR 编码后写入 dst，在构建期使用。
func EncodeFileToFile(src, dst string, key []byte) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read source file %s failed: %v", src, err)
	}
	encoded := XorEncode(raw, key)
	if err := os.WriteFile(dst, encoded, 0o644); err != nil {
		return fmt.Errorf("write encoded file %s failed: %v", dst, err)
	}
	return nil
}

// EncodeStringToFile 将字符串 XOR 编码后写入文件。
func EncodeStringToFile(content, dst string, key []byte) error {
	encoded := XorEncode([]byte(content), key)
	return os.WriteFile(dst, encoded, 0o644)
}

// LinesFromEmbed 从 embed.FS 读取并 XOR 解码文件，返回按行分割的字符串切片（去掉空行）。
func LinesFromEmbed(fs embed.FS, filename string, key []byte) ([]string, error) {
	content, err := LoadEmbedFile(fs, filename, key)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}
	return result, nil
}
