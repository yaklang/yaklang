package thirdparty_bin

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// ExtractFile 提取文件的统一入口函数
// archivePath: 压缩包路径
// targetPath: 目标路径（当isDir为true时是目录路径，否则是文件路径）
// archiveType: 压缩包类型，如果为空则根据文件扩展名自动判断
// pick: 从压缩包中提取的路径，支持以下格式:
//   - "build/*": 提取build目录下的所有内容到targetPath
//   - "build/": 提取build目录本身到targetPath
//   - "build": 提取build文件到targetPath
//   - "*": 提取所有文件到targetPath
//   - "": 提取第一个可执行文件到targetPath
//
// isDir: 如果为true，targetPath是目录路径；否则是文件路径
func ExtractFile(archivePath, targetPath, archiveType, pick string, isDir bool) error {
	// 根据文件扩展名选择解压方法
	// 增加详细日志用于调试IO流程
	log.Infof("start extracting archive: %s, target: %s, archiveType: %s, pick: %s, isDir: %v", archivePath, targetPath, archiveType, pick, isDir)
	var ext string
	if archiveType != "" {
		ext = archiveType
		log.Infof("archiveType specified by user: %s", ext)
	} else {
		ext = strings.ToLower(filepath.Ext(archivePath))
		log.Infof("archiveType detected from file extension: %s", ext)
	}
	if ext == "" {
		log.Infof("cannot determine archive type for file: %s, please specify archive_type", archivePath)
		return utils.Errorf("unsupported archive format: %s, please specify archive_type", archivePath)
	}

	switch ext {
	case ".zip":
		log.Infof("extracting zip archive: %s", archivePath)
		return extractZip(archivePath, targetPath, pick, isDir)
	case ".gz":
		if strings.HasSuffix(strings.ToLower(archivePath), ".tar.gz") {
			log.Infof("extracting tar.gz archive: %s", archivePath)
			return extractTarGz(archivePath, targetPath, pick, isDir)
		}
		log.Infof("extracting gz archive: %s", archivePath)
		return extractGz(archivePath, targetPath, isDir)
	case ".tar":
		log.Infof("extracting tar archive: %s", archivePath)
		return extractTar(archivePath, targetPath, pick, isDir)
	default:
		log.Infof("unsupported archive format: %s", ext)
		return utils.Errorf("unsupported archive format: %s", ext)
	}
}

// extractZip 解压ZIP文件
func extractZip(zipPath, targetPath, pick string, isDir bool) error {
	log.Infof("extractZip called, zipPath: %s, targetPath: %s, pick: %s, isDir: %v", zipPath, targetPath, pick, isDir)
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		log.Infof("failed to open zip file: %s, error: %v", zipPath, err)
		return err
	}
	defer reader.Close()

	// 如果指定了特定的文件或目录路径
	if pick != "" {
		log.Infof("pick is specified: %s", pick)
		// 检查是否是全部文件模式 (*)
		if pick == "*" {
			log.Infof("pick is '*', extracting all files to: %s", targetPath)
			return extractZipAll(reader, targetPath, isDir)
		}

		// 检查是否是通配符模式 (如 build/*)
		if strings.HasSuffix(pick, "/*") {
			dirPath := strings.TrimSuffix(pick, "/*")
			log.Infof("pick is directory contents: %s/*, extracting all contents to: %s", dirPath, targetPath)
			return extractZipDirectory(reader, targetPath, dirPath, true, isDir)
		}

		// 检查是否是目录模式 (如 build/)
		if strings.HasSuffix(pick, "/") {
			dirPath := strings.TrimSuffix(pick, "/")
			log.Infof("pick is directory: %s/, extracting directory to: %s", dirPath, targetPath)
			return extractZipDirectory(reader, targetPath, dirPath, false, isDir)
		}

		// 提取单个文件
		log.Infof("pick is single file: %s, searching in archive", pick)
		for _, file := range reader.File {
			log.Infof("checking file in archive: %s", file.Name)
			if file.Name == pick || strings.HasSuffix(file.Name, pick) {
				log.Infof("found file: %s, extracting to: %s", file.Name, targetPath)
				return extractZipFile(file, targetPath, isDir)
			}
		}
		log.Infof("file not found in archive: %s", pick)
		return utils.Errorf("file not found in archive: %s", pick)
	}

	// 提取第一个可执行文件
	log.Infof("pick is empty, extracting first executable file in archive")
	for _, file := range reader.File {
		log.Infof("checking file for executable: %s", file.Name)
		if !file.FileInfo().IsDir() && (file.FileInfo().Mode()&0111) != 0 {
			log.Infof("found executable file: %s, extracting to: %s", file.Name, targetPath)
			return extractZipFile(file, targetPath, isDir)
		}
	}

	log.Infof("no executable file found in archive: %s", zipPath)
	return utils.Error("no executable file found in archive")
}

// extractZipAll 提取ZIP中的所有文件
func extractZipAll(reader *zip.ReadCloser, targetPath string, isDir bool) error {
	log.Infof("extractZipAll called, targetPath: %s, isDir: %v", targetPath, isDir)
	if !isDir {
		log.Infof("cannot extract all files to a single file path, isDir must be true when pick is '*'")
		return utils.Error("cannot extract all files to a single file path, isDir must be true when pick is '*'")
	}

	for _, file := range reader.File {
		itemTargetPath := filepath.Join(targetPath, file.Name)
		log.Infof("processing file: %s, target: %s", file.Name, itemTargetPath)
		mode := file.Mode()
		switch {
		case mode&fs.ModeSymlink != 0:
			log.Infof("creating sym link: %s", itemTargetPath)
			rc, err := file.Open()
			if err != nil {
				return err
			}
			linkTarget, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return err
			}
			if err := os.Symlink(string(linkTarget), itemTargetPath); err != nil {
				return err
			}
		case mode.IsDir():
			log.Infof("creating directory: %s", itemTargetPath)
			if err := os.MkdirAll(itemTargetPath, file.FileInfo().Mode()); err != nil {
				log.Infof("create directory failed: %v", err)
				return utils.Errorf("create directory failed: %v", err)
			}
		case mode.IsRegular():
			log.Infof("extracting file: %s to %s", file.Name, itemTargetPath)
			if err := os.MkdirAll(filepath.Dir(itemTargetPath), 0755); err != nil {
				log.Infof("create file directory failed: %v", err)
				return utils.Errorf("create file directory failed: %v", err)
			}
			if err := extractZipFile(file, itemTargetPath, false); err != nil {
				log.Infof("extract file failed: %v", err)
				return err
			}
		default:
			log.Errorf("  -> Unsupported file mode: %v\n", mode)
		}
	}

	log.Infof("extractZipAll finished successfully")
	return nil
}

// extractZipDirectory 提取ZIP中的目录
func extractZipDirectory(reader *zip.ReadCloser, targetPath, dirPath string, contentsOnly bool, isDir bool) error {
	log.Infof("extractZipDirectory called, targetPath: %s, dirPath: %s, contentsOnly: %v, isDir: %v", targetPath, dirPath, contentsOnly, isDir)
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
				log.Infof("will extract (contentsOnly): %s as %s", file.Name, relativePath)
			}
		} else {
			// 提取整个目录 (build/)
			if strings.HasPrefix(file.Name, dirPath+"/") || file.Name == dirPath {
				relativePath = file.Name
				shouldExtract = true
				log.Infof("will extract (directory): %s as %s", file.Name, relativePath)
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
		log.Infof("directory not found in archive: %s", dirPath)
		return utils.Errorf("directory not found in archive: %s", dirPath)
	}

	// 如果是文件路径且只有一个文件，直接提取到targetPath
	if !isDir && len(files) == 1 && !files[0].isDir {
		log.Infof("single file in directory, extracting to: %s", targetPath)
		return extractZipFile(files[0].zipFile, targetPath, false)
	}

	// 如果是文件路径但有多个文件，返回错误
	if !isDir && len(files) > 1 {
		log.Infof("cannot extract multiple files to a single file path, isDir must be true")
		return utils.Error("cannot extract multiple files to a single file path, isDir must be true")
	}

	// 如果是目录路径，按照目录结构提取
	if !isDir {
		log.Infof("ensuring target directory exists: %s", targetPath)
		if err := os.MkdirAll(targetPath, 0755); err != nil {
			log.Infof("create target directory failed: %v", err)
			return utils.Errorf("create target directory failed: %v", err)
		}
	}

	for _, item := range files {
		var itemTargetPath string
		if !isDir && len(files) == 1 && !item.isDir {
			itemTargetPath = targetPath
		} else {
			itemTargetPath = filepath.Join(targetPath, item.relativePath)
		}

		if item.isDir {
			log.Infof("creating directory: %s", itemTargetPath)
			if err := os.MkdirAll(itemTargetPath, item.zipFile.FileInfo().Mode()); err != nil {
				log.Infof("create directory failed: %v", err)
				return utils.Errorf("create directory failed: %v", err)
			}
		} else {
			log.Infof("extracting file: %s to %s", item.zipFile.Name, itemTargetPath)
			if err := os.MkdirAll(filepath.Dir(itemTargetPath), 0755); err != nil {
				log.Infof("create file directory failed: %v", err)
				return utils.Errorf("create file directory failed: %v", err)
			}
			if err := extractZipFile(item.zipFile, itemTargetPath, false); err != nil {
				log.Infof("extract file failed: %v", err)
				return err
			}
		}
	}

	log.Infof("extractZipDirectory finished successfully")
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
func extractZipFile(file *zip.File, targetPath string, isDir bool) error {
	log.Infof("extractZipFile called, file: %s, targetPath: %s, isDir: %v", file.Name, targetPath, isDir)

	// 检查 targetPath 是否为目录
	targetInfo, err := os.Stat(targetPath)
	if err == nil && targetInfo.IsDir() {
		// 如果 targetPath 是目录，则将文件解压到该目录下，文件名与 zip 内部一致
		targetPath = filepath.Join(targetPath, filepath.Base(file.Name))
		log.Infof("targetPath is a directory, new targetPath: %s", targetPath)
	} else if err != nil && !os.IsNotExist(err) {
		log.Infof("failed to stat targetPath: %v", err)
		return utils.Errorf("failed to stat targetPath: %v", err)
	}

	reader, err := file.Open()
	if err != nil {
		log.Infof("failed to open zip file entry: %s, error: %v", file.Name, err)
		return err
	}
	defer reader.Close()

	var finalTargetPath string
	if isDir {
		// targetPath是目录，需要将文件提取到该目录中，使用原文件名
		if err := os.MkdirAll(targetPath, 0755); err != nil {
			return utils.Errorf("create target directory failed: %v", err)
		}
		finalTargetPath = filepath.Join(targetPath, filepath.Base(file.Name))
	} else {
		// targetPath是完整的文件路径，确保父目录存在
		parentDir := filepath.Dir(targetPath)
		log.Infof("ensuring parent directory exists: %s", parentDir)
		if err := os.MkdirAll(parentDir, 0755); err != nil {
			log.Infof("create parent directory failed: %v", err)
			return utils.Errorf("create parent directory failed: %v", err)
		}
		finalTargetPath = targetPath
	}

	log.Infof("creating file: %s with mode: %v", targetPath, file.FileInfo().Mode())
	targetFile, err := os.OpenFile(finalTargetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.FileInfo().Mode())
	if err != nil {
		log.Infof("failed to create file: %s, error: %v", targetPath, err)
		return err
	}
	defer targetFile.Close()

	written, err := io.Copy(targetFile, reader)
	if err != nil {
		log.Infof("failed to copy file content to: %s, error: %v", targetPath, err)
		return err
	}
	log.Infof("successfully extracted file: %s, bytes written: %d", targetPath, written)
	return nil
}

// extractTarGz 解压tar.gz文件
func extractTarGz(tarGzPath, targetPath, pick string, isDir bool) error {
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
	return extractFromTar(tarReader, targetPath, pick, isDir)
}

// extractTar 解压tar文件
func extractTar(tarPath, targetPath, pick string, isDir bool) error {
	file, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer file.Close()

	tarReader := tar.NewReader(file)
	return extractFromTar(tarReader, targetPath, pick, isDir)
}

// extractFromTar 从tar reader中提取文件
func extractFromTar(tarReader *tar.Reader, targetPath, pick string, isDir bool) error {
	var extracted bool
	var extractedFiles []string

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
			// 检查是否是全部文件模式 (*)
			if pick == "*" {
				if !isDir {
					return utils.Error("cannot extract all files to a single file path, isDir must be true when pick is '*'")
				}
				shouldExtract = true
				relativePath = header.Name
			} else if strings.HasSuffix(pick, "/*") {
				// 检查是否是通配符模式 (如 build/*)
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
					return extractTarFile(tarReader, targetPath, header.FileInfo().Mode(), isDir, header.Name)
				}
			}
		} else {
			// 查找可执行文件
			if (header.FileInfo().Mode() & 0111) != 0 {
				return extractTarFile(tarReader, targetPath, header.FileInfo().Mode(), isDir, header.Name)
			}
		}

		if shouldExtract {
			// 由于tar是流式读取，我们需要立即处理每个文件
			var itemTargetPath string
			if isDir {
				if strings.HasSuffix(pick, "/*") && relativePath != "" {
					// build/* 模式，直接使用相对路径
					itemTargetPath = filepath.Join(targetPath, relativePath)
				} else {
					// build/ 模式或 * 模式，保持完整路径
					itemTargetPath = filepath.Join(targetPath, relativePath)
				}
			} else {
				// 文件路径模式，只能提取单个文件
				if extracted {
					return utils.Error("cannot extract multiple files to a single file path, isDir must be true")
				}
				itemTargetPath = targetPath
			}

			if err := os.MkdirAll(filepath.Dir(itemTargetPath), 0755); err != nil {
				return utils.Errorf("create file directory failed: %v", err)
			}
			if err := extractTarFile(tarReader, itemTargetPath, header.FileInfo().Mode(), false, header.Name); err != nil {
				return err
			}
			extracted = true
			extractedFiles = append(extractedFiles, itemTargetPath)
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
func extractTarFile(tarReader *tar.Reader, targetPath string, mode os.FileMode, isDir bool, fileName string) error {
	var finalTargetPath string
	if isDir {
		// targetPath是目录，需要将文件提取到该目录中，使用原文件名
		if err := os.MkdirAll(targetPath, 0755); err != nil {
			return utils.Errorf("create target directory failed: %v", err)
		}
		finalTargetPath = filepath.Join(targetPath, filepath.Base(fileName))
	} else {
		// targetPath是完整的文件路径，确保父目录存在
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return utils.Errorf("create parent directory failed: %v", err)
		}
		finalTargetPath = targetPath
	}

	targetFile, err := os.OpenFile(finalTargetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer targetFile.Close()

	_, err = io.Copy(targetFile, tarReader)
	return err
}

// extractGz 解压单个gz文件
func extractGz(gzPath, targetPath string, isDir bool) error {
	if isDir {
		return utils.Error("cannot extract a single gz file to a directory, isDir must be false for .gz files")
	}

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

	// 确保父目录存在
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return utils.Errorf("create parent directory failed: %v", err)
	}

	targetFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer targetFile.Close()

	_, err = io.Copy(targetFile, gzReader)
	return err
}
