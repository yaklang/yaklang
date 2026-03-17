package ssatest

import (
	"archive/zip"
	"embed"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/yakgit"
)

//go:embed testfile
var JavaTestFile embed.FS

func GetJarFile() (string, error) {
	dir := os.TempDir()
	jar, err := JavaTestFile.ReadFile("testfile/test.jar")
	if err != nil {
		return "", err
	}

	jarPath := dir + "/test.jar"
	err = os.WriteFile(jarPath, jar, 0644)
	if err != nil {
		return "", err
	}
	return jarPath, nil
}

func GetJarContent() ([]byte, error) {
	jar, err := JavaTestFile.ReadFile("testfile/test.jar")
	if err != nil {
		return []byte{}, err
	}
	return jar, nil
}

func GetZipFile() (string, error) {
	// write java zip file to template directory
	dir := os.TempDir()
	zipData, err := JavaTestFile.ReadFile("testfile/java-realworld.zip")
	if err != nil {
		return "", err
	}

	zipPath := dir + "/java-realworld.zip"
	err = os.WriteFile(zipPath, zipData, 0644)
	if err != nil {
		return "", err
	}
	return zipPath, nil
}

func GetZipWithJarFile() (string, error) {
	// write zip file containing test.jar to template directory
	dir := os.TempDir()
	zipData, err := JavaTestFile.ReadFile("testfile/test-with-jar.zip")
	if err != nil {
		return "", err
	}

	zipPath := dir + "/test-with-jar.zip"
	err = os.WriteFile(zipPath, zipData, 0644)
	if err != nil {
		return "", err
	}
	return zipPath, nil
}

// CreateZipWithContents 创建包含指定文件的 zip，用于测试语言检测。files: 相对路径 -> 文件内容
func CreateZipWithContents(files map[string]string) (string, error) {
	dir := os.TempDir()
	zipPath := dir + "/test-" + uuid.New().String() + ".zip"
	f, err := os.Create(zipPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	w := zip.NewWriter(f)
	for name, content := range files {
		writer, err := w.Create(name)
		if err != nil {
			w.Close()
			os.Remove(zipPath)
			return "", err
		}
		_, err = writer.Write([]byte(content))
		if err != nil {
			w.Close()
			os.Remove(zipPath)
			return "", err
		}
	}
	err = w.Close()
	if err != nil {
		os.Remove(zipPath)
		return "", err
	}
	return zipPath, nil
}

// GetZipWithJSFile 创建仅包含 main.js 的 zip，用于测试 zip 内 .js 语言自动检测
func GetZipWithJSFile() (string, error) {
	return CreateZipWithContents(map[string]string{
		"main.js": "console.log('hello');\nmodule.exports = {};",
	})
}

// GetZipWithPackageJSONAndJS 创建包含 package.json + main.js 的 zip，用于测试 jsFiles 特征文件检测
func GetZipWithPackageJSONAndJS() (string, error) {
	return CreateZipWithContents(map[string]string{
		"package.json": `{"name":"test","version":"1.0.0"}`,
		"main.js":      "console.log('hello');",
	})
}

func GetNestedJarFile() (string, error) {
	// write nested jar file (test-nested.jar contains test.jar) to template directory
	dir := os.TempDir()
	jarData, err := JavaTestFile.ReadFile("testfile/test-nested.jar")
	if err != nil {
		return "", err
	}

	jarPath := dir + "/test-nested.jar"
	err = os.WriteFile(jarPath, jarData, 0644)
	if err != nil {
		return "", err
	}
	return jarPath, nil
}

func GetLocalGit() (string, error) {
	// address
	address := fmt.Sprintf("127.0.0.1:%d", utils.GetRandomAvailableTCPPort())
	lis, err := net.Listen("tcp", address)
	if err != nil {
		return "", err
	}
	// path route
	zipData, err := JavaTestFile.ReadFile("testfile/java-realworld.git.zip")
	if err != nil {
		return "", err
	}
	var router = mux.NewRouter()
	routePath, handler := yakgit.GeneratorGitHTTPHandler("", "java-realworld.git", zipData)
	router.PathPrefix(routePath).HandlerFunc(handler)
	// serve
	go func() {
		err := http.Serve(lis, router)
		if err != nil {
			log.Errorf("serve failed: %v", err)
		}
	}()

	url := "http://" + address + routePath
	return url, nil
}
