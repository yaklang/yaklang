package thirdparty_bin

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractFile_PickModes(t *testing.T) {
	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "extract_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 测试不同的压缩包格式
	tests := []struct {
		name       string
		archiveExt string
		createFunc func(t *testing.T, archivePath string)
	}{
		{
			name:       "ZIP",
			archiveExt: ".zip",
			createFunc: createTestZip,
		},
		{
			name:       "TAR_GZ",
			archiveExt: ".tar.gz",
			createFunc: createTestTarGz,
		},
		{
			name:       "TAR",
			archiveExt: ".tar",
			createFunc: createTestTar,
		},
		{
			name:       "GZ",
			archiveExt: ".gz",
			createFunc: createTestGz,
		},
	}

	for _, archiveTest := range tests {
		t.Run(archiveTest.name, func(t *testing.T) {
			// 创建测试压缩包
			archivePath := filepath.Join(tempDir, "test"+archiveTest.archiveExt)
			archiveTest.createFunc(t, archivePath)

			// 不同的pick模式测试
			pickTests := []struct {
				name       string
				pick       string
				expectFile string
				expectDir  string
				skipForGz  bool // gz格式只支持单文件
			}{
				{
					name:       "Pick specific file",
					pick:       "build/main.exe",
					expectFile: "extracted_file",
					skipForGz:  true,
				},
				{
					name:      "Pick directory contents",
					pick:      "build/*",
					expectDir: "extracted_dir_contents",
					skipForGz: true,
				},
				{
					name:      "Pick whole directory",
					pick:      "build/",
					expectDir: "extracted_dir_whole",
					skipForGz: true,
				},
				{
					name:       "Pick without pattern (first executable)",
					pick:       "",
					expectFile: "extracted_executable",
					skipForGz:  false,
				},
			}

			for _, pickTest := range pickTests {
				// 跳过gz格式不支持的测试
				if archiveTest.archiveExt == ".gz" && pickTest.skipForGz {
					continue
				}

				t.Run(pickTest.name, func(t *testing.T) {
					var targetPath string
					if pickTest.expectFile != "" {
						targetPath = filepath.Join(tempDir, archiveTest.name+"_"+pickTest.expectFile)
					} else {
						targetPath = filepath.Join(tempDir, archiveTest.name+"_"+pickTest.expectDir)
					}

					err := ExtractFile(archivePath, targetPath, "", pickTest.pick, true)
					if err != nil {
						t.Errorf("ExtractFile failed for %s with pick '%s': %v", archiveTest.name, pickTest.pick, err)
						return
					}

					// 验证文件/目录存在
					if _, err := os.Stat(targetPath); os.IsNotExist(err) {
						t.Errorf("Expected file/directory does not exist for %s: %s", archiveTest.name, targetPath)
					}
				})
			}
		})
	}
}

// createTestZip 创建测试用的ZIP文件
func createTestZip(t *testing.T, zipPath string) {
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Failed to create zip file: %v", err)
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	// 添加文件结构
	files := map[string]struct {
		content string
		mode    os.FileMode
	}{
		"build/main.exe":    {"executable content", 0755},
		"build/config.ini":  {"config content", 0644},
		"build/lib/dll.dll": {"library content", 0644},
		"src/main.go":       {"source code", 0644},
		"executable":        {"direct executable", 0755},
	}

	for path, info := range files {
		header := &zip.FileHeader{
			Name:   path,
			Method: zip.Deflate,
		}
		header.SetMode(info.mode)

		f, err := zipWriter.CreateHeader(header)
		if err != nil {
			t.Fatalf("Failed to create file in zip: %v", err)
		}
		if _, err := f.Write([]byte(info.content)); err != nil {
			t.Fatalf("Failed to write file content: %v", err)
		}
	}
}

// createTestTarGz 创建测试用的TAR.GZ文件
func createTestTarGz(t *testing.T, tarGzPath string) {
	file, err := os.Create(tarGzPath)
	if err != nil {
		t.Fatalf("Failed to create tar.gz file: %v", err)
	}
	defer file.Close()

	gzWriter := gzip.NewWriter(file)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	createTarContent(t, tarWriter)
}

// createTestTar 创建测试用的TAR文件
func createTestTar(t *testing.T, tarPath string) {
	file, err := os.Create(tarPath)
	if err != nil {
		t.Fatalf("Failed to create tar file: %v", err)
	}
	defer file.Close()

	tarWriter := tar.NewWriter(file)
	defer tarWriter.Close()

	createTarContent(t, tarWriter)
}

// createTarContent 创建TAR内容的辅助函数
func createTarContent(t *testing.T, tarWriter *tar.Writer) {
	files := map[string]struct {
		content string
		mode    os.FileMode
	}{
		"build/main.exe":    {"executable content", 0755},
		"build/config.ini":  {"config content", 0644},
		"build/lib/dll.dll": {"library content", 0644},
		"src/main.go":       {"source code", 0644},
		"executable":        {"direct executable", 0755},
	}

	for path, info := range files {
		header := &tar.Header{
			Name: path,
			Mode: int64(info.mode),
			Size: int64(len(info.content)),
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("Failed to write tar header: %v", err)
		}

		if _, err := tarWriter.Write([]byte(info.content)); err != nil {
			t.Fatalf("Failed to write tar content: %v", err)
		}
	}
}

// createTestGz 创建测试用的GZ文件（单个文件压缩）
func createTestGz(t *testing.T, gzPath string) {
	file, err := os.Create(gzPath)
	if err != nil {
		t.Fatalf("Failed to create gz file: %v", err)
	}
	defer file.Close()

	gzWriter := gzip.NewWriter(file)
	defer gzWriter.Close()

	// GZ只能压缩单个文件，我们创建一个可执行文件
	content := "#!/bin/bash\necho 'Hello from executable'"
	if _, err := gzWriter.Write([]byte(content)); err != nil {
		t.Fatalf("Failed to write gz content: %v", err)
	}
}

// TestExtractFile_ErrorCases 测试错误情况
func TestExtractFile_ErrorCases(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "extract_error_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tests := []struct {
		name        string
		archivePath string
		targetPath  string
		archiveType string
		pick        string
		expectError bool
	}{
		{
			name:        "Non-existent archive",
			archivePath: filepath.Join(tempDir, "nonexistent.zip"),
			targetPath:  filepath.Join(tempDir, "target"),
			expectError: true,
		},
		{
			name:        "Unsupported format",
			archivePath: filepath.Join(tempDir, "test.rar"),
			targetPath:  filepath.Join(tempDir, "target"),
			archiveType: ".rar",
			expectError: true,
		},
		{
			name:        "Pick non-existent file",
			archivePath: "",
			targetPath:  filepath.Join(tempDir, "target"),
			pick:        "non/existent/file",
			expectError: true,
		},
	}

	// 为Pick non-existent file测试创建一个有效的ZIP文件
	zipPath := filepath.Join(tempDir, "test.zip")
	createTestZip(t, zipPath)
	tests[2].archivePath = zipPath

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ExtractFile(tt.archivePath, tt.targetPath, tt.archiveType, tt.pick, true)
			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

// TestExtractFile_ArchiveTypeSpecified 测试显式指定压缩包类型
func TestExtractFile_ArchiveTypeSpecified(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "extract_archive_type_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 创建一个ZIP文件但使用.data扩展名
	archivePath := filepath.Join(tempDir, "test.data")
	createTestZip(t, archivePath)

	targetPath := filepath.Join(tempDir, "extracted")

	// 不指定类型应该失败
	err = ExtractFile(archivePath, targetPath, "", "executable", true)
	if err == nil {
		t.Errorf("Expected error when archive type cannot be determined")
	}

	// 显式指定ZIP类型应该成功
	err = ExtractFile(archivePath, targetPath, ".zip", "executable", true)
	if err != nil {
		t.Errorf("Failed to extract with explicit archive type: %v", err)
	}

	// 验证文件存在
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		t.Errorf("Expected file does not exist: %s", targetPath)
	}
}

// TestExtractFile_DirectoryExtraction 测试目录提取的详细行为
func TestExtractFile_DirectoryExtraction(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "extract_dir_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 创建包含复杂目录结构的ZIP文件
	zipPath := filepath.Join(tempDir, "complex.zip")
	createComplexTestZip(t, zipPath)

	tests := []struct {
		name        string
		pick        string
		targetPath  string
		expectFiles []string
		expectDirs  []string
	}{
		{
			name:       "Extract build directory contents",
			pick:       "build/*",
			targetPath: filepath.Join(tempDir, "build_contents"),
			expectFiles: []string{
				"main.exe",
				"config.ini",
				"lib/dll.dll",
			},
		},
		{
			name:       "Extract entire build directory",
			pick:       "build/",
			targetPath: filepath.Join(tempDir, "build_whole"),
			expectFiles: []string{
				"build/main.exe",
				"build/config.ini",
				"build/lib/dll.dll",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ExtractFile(zipPath, tt.targetPath, "", tt.pick, true)
			if err != nil {
				t.Errorf("Failed to extract: %v", err)
				return
			}

			// 验证期望的文件存在
			for _, expectedFile := range tt.expectFiles {
				filePath := filepath.Join(tt.targetPath, expectedFile)
				if _, err := os.Stat(filePath); os.IsNotExist(err) {
					t.Errorf("Expected file does not exist: %s", filePath)
				}
			}
		})
	}
}

// createComplexTestZip 创建包含复杂目录结构的测试ZIP文件
func createComplexTestZip(t *testing.T, zipPath string) {
	file, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Failed to create zip file: %v", err)
	}
	defer file.Close()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	files := map[string]struct {
		content string
		mode    os.FileMode
	}{
		"build/main.exe":      {"main executable", 0755},
		"build/config.ini":    {"config file", 0644},
		"build/lib/dll.dll":   {"library file", 0644},
		"build/lib/helper.so": {"shared library", 0755},
		"src/main.c":          {"source code", 0644},
		"src/utils/helper.c":  {"utility source", 0644},
		"docs/README.md":      {"documentation", 0644},
		"test_executable":     {"test program", 0755},
	}

	for path, info := range files {
		header := &zip.FileHeader{
			Name:   path,
			Method: zip.Deflate,
		}
		header.SetMode(info.mode)

		f, err := zipWriter.CreateHeader(header)
		if err != nil {
			t.Fatalf("Failed to create file in zip: %v", err)
		}
		if _, err := f.Write([]byte(info.content)); err != nil {
			t.Fatalf("Failed to write file content: %v", err)
		}
	}
}
