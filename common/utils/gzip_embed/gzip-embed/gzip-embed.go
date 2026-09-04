package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/utils"
)

var template = `
import (
	"embed"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/gzip_embed"
)

//go:embed $gz
var resourceFS embed.FS

var FS *gzip_embed.PreprocessingEmbed

func init() {
	var err error
	FS, err = gzip_embed.NewPreprocessingEmbed(&resourceFS, "$gz", $cache)
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
	app.Version = "v1.1"
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name: "cache,c",
		},
		cli.BoolFlag{
			Name: "no-embed",
		},
		cli.BoolFlag{
			Name:  "root-path,r",
			Usage: "include root directory name in archive paths (e.g., static/a.png instead of a.png)",
		},
		cli.StringSliceFlag{
			Name:  "source,s",
			Usage: "source dir to pack, can be given multiple times to merge into a single archive",
		},
		cli.StringFlag{
			Name:  "base,b",
			Usage: "reference dir used to compute archive paths when packing multiple sources (entries are stored relative to it)",
		},
		cli.StringFlag{
			Name: "gz",
		},
		cli.BoolFlag{
			Name:  "include-targz",
			Usage: "include *.tar.gz files from source (default: excluded to avoid packaging nested archives)",
		},
		cli.StringFlag{
			Name:  "xor-key",
			Usage: "if set, XOR-encode the output tar.gz with this key",
		},
	}
	app.Action = func(c *cli.Context) {
		sources := c.StringSlice("source")
		if len(sources) == 0 {
			sources = []string{"static"}
		}
		baseDir := c.String("base")
		gzName := c.String("gz")
		withRootPath := c.Bool("root-path")
		includeTarGz := c.Bool("include-targz")
		xorKey := c.String("xor-key")
		if gzName == "" {
			gzName = fmt.Sprintf("%s.tar.gz", sources[0])
		}
		err := targz(sources, baseDir, gzName, withRootPath, includeTarGz)
		if err != nil {
			log.Error(err)
			return
		}
		if xorKey != "" {
			if err := xorEncodeFile(gzName, []byte(xorKey)); err != nil {
				log.Error(err)
				return
			}
		}
		if !c.Bool("no-embed") {
			writeEmbedFile(c.IsSet("cache"), strings.Join(sources, " "), gzName)
			log.Infof("generate embed file and compress file success, compress file name: %s", gzName)
		} else {
			log.Infof("generate compress file success (skip embed file), compress file name: %s", gzName)
		}
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Error(err)
	}
}

func xorEncodeFile(path string, key []byte) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	encoded := make([]byte, len(raw))
	for i, b := range raw {
		encoded[i] = b ^ key[i%len(key)]
	}
	return os.WriteFile(path, encoded, 0o644)
}
func writeEmbedFile(cache bool, sourceDir string, gzName string) {
	dir, _ := os.Getwd()
	cacheStr := "false"
	if cache {
		cacheStr = "true"
	}
	code := fmt.Sprintf("package %s\n%s", filepath.Base(dir), utils.Format(template, map[string]string{
		"source": sourceDir,
		"gz":     gzName,
		"cache":  cacheStr,
	}))
	os.WriteFile("embed.go", []byte(code), 0644)
}

// targz 把若干个源目录合并打包成一个 tar.gz。
// baseDir 为空时沿用单目录语义：withRootPath 决定条目是否包含源目录名；
// baseDir 非空时（多源打包），条目路径相对 baseDir 计算，例如
// --base . --source ./behinder/static 会得到 behinder/static/CmdGo.php。
func targz(sources []string, baseDir string, gzName string, withRootPath bool, includeTarGz bool) error {
	for _, source := range sources {
		if _, err := os.Stat(source); os.IsNotExist(err) {
			return err
		}
	}

	// 读取文件或目录
	outFile, err := os.Create(gzName)
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

	// 如果开启 withRootPath，使用父目录作为基准，这样相对路径会包含根目录名称
	// 例如：path="static", withRootPath=true -> relBase=filepath.Dir("static")="."
	// 这样 static/a.png 相对于 "." 的路径就是 "static/a.png"
	resolveRelBase := func(path string) string {
		if baseDir != "" {
			return baseDir
		}
		if withRootPath {
			return filepath.Dir(path)
		}
		return path
	}

	gzAbs := gzName
	if cwd, err := os.Getwd(); err == nil && !filepath.IsAbs(gzAbs) {
		gzAbs = filepath.Join(cwd, gzAbs)
	}
	gzAbs = filepath.Clean(gzAbs)
	gzInfo, _ := os.Stat(gzAbs)

	// 递归地添加文件夹内容到 tar 归档
	for _, source := range sources {
		relBase := resolveRelBase(source)
		err = filepath.Walk(source, func(filePath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			return addFileToTarWriter(filePath, info, relBase, tarWriter, gzAbs, gzInfo, includeTarGz)
		})
		if err != nil {
			return err
		}
	}
	return nil
}
func addFileToTarWriter(path string, info os.FileInfo, rootDir string, tarWriter *tar.Writer, gzAbs string, gzInfo os.FileInfo, includeTarGz bool) error {
	if abs, err := filepath.Abs(path); err == nil && filepath.Clean(abs) == gzAbs {
		return nil
	}
	// When paths contain symlinks, string-cleaned comparisons are insufficient.
	// Use os.SameFile to avoid accidentally packing the output tar.gz into itself.
	if gzInfo != nil {
		if curInfo, err := os.Stat(path); err == nil && os.SameFile(curInfo, gzInfo) {
			return nil
		}
	}

	// 获取文件的基本名称
	baseName := filepath.Base(path)

	// Default behavior: exclude *.tar.gz to avoid packaging nested archives.
	// For source distributions that legitimately embed .tar.gz, pass --include-targz.
	if !includeTarGz && strings.HasSuffix(baseName, ".tar.gz") {
		return nil
	}
	if baseName == "output.tar.gz" {
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
	// 统一使用正斜杠作为路径分隔符（tar 标准）
	relativePath = filepath.ToSlash(relativePath)
	// 创建 tar 头
	header, err := tar.FileInfoHeader(info, relativePath)
	if err != nil {
		return err
	}
	header.Name = relativePath

	// 如果是符号链接，需要特殊处理
	if info.Mode()&os.ModeSymlink != 0 {
		linkTarget, err := os.Readlink(path)
		if err != nil {
			return err
		}
		header.Linkname = linkTarget
	}

	// 清空可能过长的用户名和组名字段，避免 "write too long" 错误
	// 在 PAX 格式下，这些信息会被保存在扩展属性中
	header.Uname = ""
	header.Gname = ""

	// 使用 PAX 格式支持长文件名和路径
	header.Format = tar.FormatPAX

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
