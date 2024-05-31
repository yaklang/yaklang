package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"os"
	"path/filepath"
)

var template = `
import (
	"embed"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

//go:embed static.tar.gz
var resourceFS embed.FS

var FS *gzip_embed.PreprocessingEmbed

func init() {
	var err error
	FS, err = gzip_embed.NewPreprocessingEmbed(&resourceFS, "static.tar.gz", $cache)
	if err != nil {
		log.Errorf("init embed failed: %v", err)
		FS = gzip_embed.NewEmptyPreprocessingEmbed()
	}
}
`

func main() {
	app := cli.NewApp()
	app.Name = "gzip-embed"
	app.Usage = `help you generate compress file and embed file reader`
	app.Version = "v1.0"
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name: "cache,c",
		},
	}
	app.Action = func(c *cli.Context) {
		sourceDir := "static"
		err := targz(sourceDir)
		if err != nil {
			log.Error(err)
		}
		writeEmbedFile(c.IsSet("cache"))
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Error(err)
	}
	return
}
func writeEmbedFile(cache bool) {
	dir, _ := os.Getwd()
	cacheStr := "false"
	if cache {
		cacheStr = "true"
	}
	code := fmt.Sprintf("package %s\n%s", filepath.Base(dir), utils.Format(template, map[string]string{
		"cache": cacheStr,
	}))
	os.WriteFile("embed.go", []byte(code), 0644)
}
func targz(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return err
	}
	// 读取文件或目录
	outFile, err := os.Create("static.tar.gz")
	if err != nil {
		return err
	}
	defer outFile.Close()

	// 创建 gzip 压缩器
	gzWriter := gzip.NewWriter(outFile)
	defer gzWriter.Close()

	// 创建 tar 归档器
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()
	rootPath := path
	// 递归地添加文件夹内容到 tar 归档
	err = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return addFileToTarWriter(path, info, rootPath, tarWriter)
	})

	if err != nil {
		return err
	}
	return nil
}
func addFileToTarWriter(path string, info os.FileInfo, rootDir string, tarWriter *tar.Writer) error {

	// 确保不将输出文件包括在内
	if path == "output.tar.gz" {
		return nil
	}

	// 创建适用于 tar 的相对路径
	relativePath, err := filepath.Rel(rootDir, path)
	if err != nil {
		return err
	}
	if relativePath == "." {
		return nil
	}
	// 创建 tar 头
	header, err := tar.FileInfoHeader(info, relativePath)
	if err != nil {
		return err
	}
	header.Name = relativePath

	// 写入头信息
	err = tarWriter.WriteHeader(header)
	if err != nil {
		return err
	}

	// 如果是普通文件，则写入它的内容
	if !info.IsDir() {
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(tarWriter, file)
		return err
	}
	return nil
}
func XORKeyStream(data, key []byte) []byte {
	// 创建一个与数据长度相同的切片用于存储结果
	result := make([]byte, len(data))
	// 获取密钥的长度
	keyLen := len(key)

	// 对每一个字节进行异或操作
	for i, b := range data {
		result[i] = b ^ key[i%keyLen] // 使用密钥循环
	}

	return result
}
