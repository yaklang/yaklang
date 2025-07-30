package thirdparty_bin

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

// ExtractFile 提取文件的统一入口函数
// archivePath: 压缩包路径
// targetPath: 目标路径
// archiveType: 压缩包类型，如果为空则根据文件扩展名自动判断
// pick: 从压缩包中提取的路径，支持以下格式:
//   - "build/*": 提取build目录下的所有内容到targetPath
//   - "build/": 提取build目录本身到targetPath
//   - "build": 提取build文件到targetPath
//   - "": 提取第一个可执行文件到targetPath
func ExtractFile(archivePath, targetPath, archiveType, pick string) error {
	// 根据文件扩展名选择解压方法
	var ext string
	if archiveType != "" {
		ext = archiveType
	} else {
		ext = strings.ToLower(filepath.Ext(archivePath))
	}
	if ext == "" {
		// 无法判断文件类型，直接返回错误并告知用户可以手动指定archive_type
		return utils.Errorf("unsupported archive format: %s, please specify archive_type", archivePath)
	}

	switch ext {
	case ".zip":
		return extractZip(archivePath, targetPath, pick)
	case ".gz":
		if strings.HasSuffix(strings.ToLower(archivePath), ".tar.gz") {
			return extractTarGz(archivePath, targetPath, pick)
		}
		return extractGz(archivePath, targetPath)
	case ".tar":
		return extractTar(archivePath, targetPath, pick)
	default:
		return utils.Errorf("unsupported archive format: %s", ext)
	}
}

// extractZip 解压ZIP文件
func extractZip(zipPath, targetPath, pick string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	// 如果指定了特定的文件或目录路径
	if pick != "" {
		// 检查是否是通配符模式 (如 build/*)
		if strings.HasSuffix(pick, "/*") {
			// 提取指定目录下的所有内容
			dirPath := strings.TrimSuffix(pick, "/*")
			return extractZipDirectory(reader, targetPath, dirPath, true)
		}

		// 检查是否是目录模式 (如 build/)
		if strings.HasSuffix(pick, "/") {
			// 提取整个目录
			dirPath := strings.TrimSuffix(pick, "/")
			return extractZipDirectory(reader, targetPath, dirPath, false)
		}

		// 提取单个文件
		for _, file := range reader.File {
			if file.Name == pick || strings.HasSuffix(file.Name, pick) {
				return extractZipFile(file, targetPath)
			}
		}
		return utils.Errorf("file not found in archive: %s", pick)
	}

	// 提取第一个可执行文件
	for _, file := range reader.File {
		if !file.FileInfo().IsDir() && (file.FileInfo().Mode()&0111) != 0 {
			return extractZipFile(file, targetPath)
		}
	}

	return utils.Error("no executable file found in archive")
}

// extractZipDirectory 提取ZIP中的目录
func extractZipDirectory(reader *zip.ReadCloser, targetPath, dirPath string, contentsOnly bool) error {
	var extracted bool
	var files []extractItem

	// 收集需要提取的文件
	for _, file := range reader.File {
		var shouldExtract bool
		var relativePath string

		if contentsOnly {
			// 提取目录下的所有内容 (build/*)
			if strings.HasPrefix(file.Name, dirPath+"/") {
				relativePath = strings.TrimPrefix(file.Name, dirPath+"/")
				shouldExtract = true
			}
		} else {
			// 提取整个目录 (build/)
			if strings.HasPrefix(file.Name, dirPath+"/") || file.Name == dirPath {
				relativePath = file.Name
				shouldExtract = true
			}
		}

		if shouldExtract {
			files = append(files, extractItem{
				zipFile:      file,
				relativePath: relativePath,
				isDir:        file.FileInfo().IsDir(),
			})
			extracted = true
		}
	}

	if !extracted {
		return utils.Errorf("directory not found in archive: %s", dirPath)
	}

	// 如果只提取目录内容且只有一个文件，直接提取到targetPath
	if contentsOnly && len(files) == 1 && !files[0].isDir {
		return extractZipFile(files[0].zipFile, targetPath)
	}

	// 否则，按照目录结构提取
	for _, item := range files {
		var itemTargetPath string
		if contentsOnly && len(files) == 1 {
			// 单个文件直接提取到目标路径
			itemTargetPath = targetPath
		} else {
			// 保持目录结构
			itemTargetPath = filepath.Join(targetPath, item.relativePath)
		}

		if item.isDir {
			// 创建目录
			if err := os.MkdirAll(itemTargetPath, item.zipFile.FileInfo().Mode()); err != nil {
				return utils.Errorf("create directory failed: %v", err)
			}
		} else {
			// 提取文件
			if err := os.MkdirAll(filepath.Dir(itemTargetPath), 0755); err != nil {
				return utils.Errorf("create file directory failed: %v", err)
			}
			if err := extractZipFile(item.zipFile, itemTargetPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// extractItem 提取项目信息
type extractItem struct {
	zipFile      *zip.File
	tarHeader    *tar.Header
	relativePath string
	isDir        bool
}

// extractZipFile 提取ZIP中的单个文件
func extractZipFile(file *zip.File, targetPath string) error {
	reader, err := file.Open()
	if err != nil {
		return err
	}
	defer reader.Close()

	targetFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.FileInfo().Mode())
	if err != nil {
		return err
	}
	defer targetFile.Close()

	_, err = io.Copy(targetFile, reader)
	return err
}

// extractTarGz 解压tar.gz文件
func extractTarGz(tarGzPath, targetPath, pick string) error {
	file, err := os.Open(tarGzPath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	return extractFromTar(tarReader, targetPath, pick)
}

// extractTar 解压tar文件
func extractTar(tarPath, targetPath, pick string) error {
	file, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer file.Close()

	tarReader := tar.NewReader(file)
	return extractFromTar(tarReader, targetPath, pick)
}

// extractFromTar 从tar reader中提取文件
func extractFromTar(tarReader *tar.Reader, targetPath, pick string) error {
	var extracted bool

	// 收集需要提取的文件
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		// 跳过目录
		if header.Typeflag == tar.TypeDir {
			continue
		}

		var shouldExtract bool
		var relativePath string

		if pick != "" {
			// 检查是否是通配符模式 (如 build/*)
			if strings.HasSuffix(pick, "/*") {
				dirPath := strings.TrimSuffix(pick, "/*")
				if strings.HasPrefix(header.Name, dirPath+"/") {
					relativePath = strings.TrimPrefix(header.Name, dirPath+"/")
					shouldExtract = true
				}
			} else if strings.HasSuffix(pick, "/") {
				// 检查是否是目录模式 (如 build/)
				dirPath := strings.TrimSuffix(pick, "/")
				if strings.HasPrefix(header.Name, dirPath+"/") || header.Name == dirPath {
					relativePath = header.Name
					shouldExtract = true
				}
			} else {
				// 单个文件
				if header.Name == pick || strings.HasSuffix(header.Name, pick) {
					return extractTarFile(tarReader, targetPath, header.FileInfo().Mode())
				}
			}
		} else {
			// 查找可执行文件
			if (header.FileInfo().Mode() & 0111) != 0 {
				return extractTarFile(tarReader, targetPath, header.FileInfo().Mode())
			}
		}

		if shouldExtract {
			// 由于tar是流式读取，我们需要立即处理每个文件
			var itemTargetPath string
			if strings.HasSuffix(pick, "/*") && relativePath != "" {
				// build/* 模式，直接使用相对路径
				itemTargetPath = filepath.Join(targetPath, relativePath)
			} else {
				// build/ 模式，保持完整路径
				itemTargetPath = filepath.Join(targetPath, relativePath)
			}

			if err := os.MkdirAll(filepath.Dir(itemTargetPath), 0755); err != nil {
				return utils.Errorf("create file directory failed: %v", err)
			}
			if err := extractTarFile(tarReader, itemTargetPath, header.FileInfo().Mode()); err != nil {
				return err
			}
			extracted = true
		}
	}

	if pick != "" && !extracted {
		return utils.Errorf("file or directory not found in archive: %s", pick)
	}

	if pick == "" && !extracted {
		return utils.Error("no executable file found in archive")
	}

	return nil
}

// extractTarFile 提取tar中的单个文件
func extractTarFile(tarReader *tar.Reader, targetPath string, mode os.FileMode) error {
	targetFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer targetFile.Close()

	_, err = io.Copy(targetFile, tarReader)
	return err
}

// extractGz 解压单个gz文件
func extractGz(gzPath, targetPath string) error {
	file, err := os.Open(gzPath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	targetFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer targetFile.Close()

	_, err = io.Copy(targetFile, gzReader)
	return err
}
