package ziputil

import (
	zip "github.com/yaklang/yaklang/common/utils/zipx"
)

// 加密方法类型与常量
// 关键词: zip 加密, ZipCrypto, AES 加密
//
// EncryptionMethod 是 zip 文件的加密算法标识，与 yeka/zip 的 EncryptionMethod 一致。
// StandardEncryption 即传统 PKWARE/ZipCrypto，仅用于兼容老旧工具，已被密码学界证明不安全。
// 在新生成的带密码 zip 上推荐使用 AES256Encryption。
type EncryptionMethod = zip.EncryptionMethod

const (
	// StandardEncryption 即 PKWARE / ZipCrypto，兼容性最好但不安全。
	StandardEncryption = zip.StandardEncryption
	// AES128Encryption WinZip AES-128
	AES128Encryption = zip.AES128Encryption
	// AES192Encryption WinZip AES-192
	AES192Encryption = zip.AES192Encryption
	// AES256Encryption WinZip AES-256，推荐默认值
	AES256Encryption = zip.AES256Encryption
)

// CompressConfig 压缩选项
// 关键词: zip 压缩, 密码压缩, AES 加密
type CompressConfig struct {
	Password         string
	EncryptionMethod EncryptionMethod
}

// CompressOption 压缩选项函数
type CompressOption func(*CompressConfig)

// newCompressConfig 创建默认压缩配置
func newCompressConfig(opts ...CompressOption) *CompressConfig {
	cfg := &CompressConfig{
		Password:         "",
		EncryptionMethod: AES256Encryption,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// compressPassword 为 zip 压缩设置加密密码
// 关键词: zip 压缩密码, 加密 zip 创建
// 参数:
//   - password: 加密密码
//
// 返回值:
//   - 一个压缩配置选项
//
// Example:
// ```
// zip.CompressByNameWithOptions(["/tmp/a.txt"], "/tmp/out.zip", zip.compressPassword("123456"))~
// ```
func WithCompressPassword(password string) CompressOption {
	return func(c *CompressConfig) {
		c.Password = password
	}
}

// compressEncryption 设置 zip 压缩的加密方法（默认 AES256）
// 关键词: zip 加密方法, AES256
// 参数:
//   - method: 加密方法（如 zip.AES256、zip.AES128、zip.StandardEncryption）
//
// 返回值:
//   - 一个压缩配置选项
//
// Example:
// ```
// zip.CompressByNameWithOptions(["/tmp/a.txt"], "/tmp/out.zip", zip.compressPassword("123456"), zip.compressEncryption(zip.AES256))~
// ```
func WithCompressEncryption(method EncryptionMethod) CompressOption {
	return func(c *CompressConfig) {
		c.EncryptionMethod = method
	}
}

// DecompressConfig 解压选项
// 关键词: zip 解压, 密码解压
type DecompressConfig struct {
	Password string
}

// DecompressOption 解压选项函数
type DecompressOption func(*DecompressConfig)

// newDecompressConfig 创建默认解压配置
func newDecompressConfig(opts ...DecompressOption) *DecompressConfig {
	cfg := &DecompressConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// decompressPassword 为 zip 解压设置解密密码
// 关键词: zip 解压密码
// 参数:
//   - password: 解密密码
//
// 返回值:
//   - 一个解压配置选项
//
// Example:
// ```
// zip.DecompressWithOptions("/tmp/enc.zip", "/tmp/dest", zip.decompressPassword("123456"))~
// ```
func WithDecompressPassword(password string) DecompressOption {
	return func(c *DecompressConfig) {
		c.Password = password
	}
}

// ExtractConfig 单文件提取选项
// 关键词: zip 提取, 密码提取
type ExtractConfig struct {
	Password string
}

// ExtractOption 提取选项函数
type ExtractOption func(*ExtractConfig)

// newExtractConfig 创建默认提取配置
func newExtractConfig(opts ...ExtractOption) *ExtractConfig {
	cfg := &ExtractConfig{}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// extractPassword 为 zip 提取设置解密密码
// 关键词: zip 提取密码
// 参数:
//   - password: 解密密码
//
// 返回值:
//   - 一个提取配置选项
//
// Example:
// ```
// zipBytes = zip.CompressRawWithPassword({"s.txt": "secret"}, "123456")~
// content = zip.ExtractFileFromRawWithOptions(zipBytes, "s.txt", zip.extractPassword("123456"))~
// assert string(content) == "secret", "extractPassword should allow decrypting the entry"
// ```
func WithExtractPassword(password string) ExtractOption {
	return func(c *ExtractConfig) {
		c.Password = password
	}
}

// applyZipPassword 在打开 zip 文件条目前根据需要设置密码
// 关键词: zip 加密读, SetPassword
func applyZipPassword(f *zip.File, password string) {
	if f == nil {
		return
	}
	if !f.IsEncrypted() {
		return
	}
	if password == "" {
		return
	}
	f.SetPassword(password)
}
