package ssatest

import (
	"embed"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/yakgit"
	"net"
	"net/http"
	"os"

	"github.com/gorilla/mux"

	"github.com/yaklang/yaklang/common/utils"
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
