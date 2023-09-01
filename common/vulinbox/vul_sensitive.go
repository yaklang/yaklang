package vulinbox

import (
	"archive/zip"
	"bytes"
	"embed"
	"errors"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing/format/pktline"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/ziputil"
	"os"
	"path/filepath"

	gitServer "github.com/go-git/go-git/v5/plumbing/transport/server"
	"io"
	"net/http"
	"path"
	"strings"
)

//go:embed sensitivefs
var _sensitiveFS embed.FS

//go:embed fakegit/website.zip
var _fakeGitWebsite []byte

//go:embed fakegit/website-repository.git.zip
var _fakeGitRepository []byte

//go:embed fakegit/sca-testcase.git.zip
var _fakeGitSCARespos []byte

func GetSensitiveFile(name string) []byte {
	f, err := _sensitiveFS.Open(path.Join("sensitivefs", name))
	if err != nil {
		log.Errorf("cannot found sensitive file: %s", err)
		return nil
	}
	raw, _ := io.ReadAll(f)
	f.Close()
	return raw
}

func (s *VulinServer) registerSensitive() {
	r := s.router

	_sensitive := func(s string) string {
		return path.Join("/sensitive", s)
	}
	_ = _sensitive

	/*
		swagger demo
		1. v{1-3}/swagger.json
		2. v{1-3}/rest{/}
		3. /api-doc
		4. /swagger/v1/swagger.json

		{path}?/swagger/index.html
	*/

	zipGitFS, err := zip.NewReader(bytes.NewReader(_fakeGitWebsite), int64(len(_fakeGitWebsite)))
	if err != nil {
		log.Errorf("cannot open zip file: %s", err)
	}

	zipGitRepositoryFS, err := zip.NewReader(bytes.NewReader(_fakeGitRepository), int64(len(_fakeGitRepository)))
	if err != nil {
		log.Errorf("cannot open zip file: %s", err)
	}

	zipScaGitResposFS, err := zip.NewReader(bytes.NewReader(_fakeGitSCARespos), int64(len(_fakeGitSCARespos)))
	if err != nil {
		log.Errorf("cannot open zip file: %s", err)
	}

	var sensitiveGroup = r.PathPrefix("/sensitive").Name("敏感信息与敏感文件泄漏").Subrouter()
	var swaggerGroup = r.PathPrefix("/swagger").Name("敏感信息与敏感文件泄漏（Swagger）").Subrouter()
	var vuls = []*VulInfo{
		{
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.Write(GetSensitiveFile("openapi-2.json"))
			},
			Path:         `/v1/swagger.json`,
			Title:        "OpenAPI 2.0 Swagger 泄漏",
			RiskDetected: true,
		},
		{
			Path:  `/v2/swagger.json`,
			Title: "OpenAPI 3.0 Swagger 泄漏",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.Write(GetSensitiveFile("openapi-3.json"))
			},
			RiskDetected: true,
		},
		{
			Path:  `/`,
			Title: "Swagger UI 泄漏",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", `text/html`)
				writer.Write(GetSensitiveFile("swagger-ui.html"))
			},
			RiskDetected: true,
		},
		{
			Path:  `/website/`,
			Title: "Git Repository 泄漏",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Location", "/git/website/index.html")
				writer.WriteHeader(302)
			},
			RiskDetected: true,
		},
	}
	for _, v := range vuls {
		addRouteWithVulInfo(sensitiveGroup, v)
		addRouteWithVulInfo(swaggerGroup, v)
	}

	addRouteWithVulInfo(swaggerGroup, &VulInfo{
		Path: `/index.html`,
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", `text/html`)
			writer.Write(GetSensitiveFile("swagger-ui.html"))
		},
		RiskDetected: true,
	})
	fakeGitSubrouter := s.router.PathPrefix("/git/")
	fakeGitSubrouter.PathPrefix("/website/").HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if zipGitFS == nil {
			Failed(writer, request, "Create FAKE GIT Website FAILED")
			return
		}

		request.RequestURI = strings.TrimPrefix(request.RequestURI, `/git/`)
		request.URL.Path = strings.TrimPrefix(request.URL.Path, `/git/`)

		filePath := request.URL.Path
		if filePath == "website/" {
			filePath = "website/index.html"
		}

		var fp, err = zipGitFS.Open(filePath)
		if err != nil {
			Failed(writer, request, "Cannot found file(%v) in fake git website", request.URL.Path)
			return
		}
		defer fp.Close()
		raw, _ := io.ReadAll(fp)

		if strings.Contains(filePath, ".git/") {
			writer.Header().Set("Content-Type", `text/plain`)
		} else {
			writer.Header().Set("Content-Type", `text/html`)
		}
		writer.Write(raw)
	})

	var localBareRepos = filepath.Join(consts.GetDefaultYakitBaseTempDir(), "bare-repos")
	os.RemoveAll(localBareRepos)
	ziputil.DeCompressFromRaw(_fakeGitRepository, localBareRepos)
	var localBareReposPath = filepath.Join(localBareRepos, "website-repository.git")

	ziputil.DeCompressFromRaw(_fakeGitSCARespos, localBareRepos)
	var localSCAReposPath = filepath.Join(localBareRepos, "sca-testcase.git")

	s.router.PathPrefix("/gitserver/sca-testcase.git/").HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		request.URL.Path = strings.TrimPrefix(request.URL.Path, "/gitserver/")
		request.RequestURI = strings.TrimPrefix(request.RequestURI, "/gitserver/")

		rootDir := "sca-testcase.git"
		fileName := strings.TrimPrefix(request.URL.Path, "/")
		log.Infof("fetch %v: %s", rootDir, fileName)
		// website-repository.git
		serviceParams := request.URL.Query().Get("service")
		log.Infof("service: %s to %v", serviceParams, fileName)
		if serviceParams == "git-upload-pack" && strings.HasSuffix(fileName, `/info/refs`) {
			s.gitInfoRefs(localSCAReposPath, writer, request)
			return
		}

		if strings.HasSuffix(request.URL.Path, `/git-upload-pack`) && request.Method == "POST" {
			s.gitUploadPack(localSCAReposPath, writer, request)
			return
		}

		//if strings.HasSuffix(request.URL.Path, `/git-receive-pack`) && request.Method == "POST" {
		//	s.gitReceivePack(localBareRepos, writer, request)
		//	return
		//}
		filePath := path.Join(rootDir, fileName)
		var fp, err = zipScaGitResposFS.Open(filePath)
		if err != nil {
			writer.WriteHeader(404)
			return
		}
		defer fp.Close()
		raw, _ := io.ReadAll(fp)

		if strings.Contains(filePath, ".git/") {
			writer.Header().Set("Content-Type", `text/plain`)
		} else {
			writer.Header().Set("Content-Type", `text/html`)
		}
		writer.Write(raw)
	})
	s.router.PathPrefix("/gitserver/website-repository.git/").HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		request.URL.Path = strings.TrimPrefix(request.URL.Path, "/gitserver/")
		request.RequestURI = strings.TrimPrefix(request.RequestURI, "/gitserver/")

		rootDir := "website-repository.git"
		fileName := strings.TrimPrefix(request.URL.Path, "/")
		log.Infof("fetch %v: %s", rootDir, fileName)
		// website-repository.git
		serviceParams := request.URL.Query().Get("service")
		log.Infof("service: %s to %v", serviceParams, fileName)
		if serviceParams == "git-upload-pack" && strings.HasSuffix(fileName, `/info/refs`) {
			s.gitInfoRefs(localBareReposPath, writer, request)
			return
		}

		if strings.HasSuffix(request.URL.Path, `/git-upload-pack`) && request.Method == "POST" {
			s.gitUploadPack(localBareReposPath, writer, request)
			return
		}

		//if strings.HasSuffix(request.URL.Path, `/git-receive-pack`) && request.Method == "POST" {
		//	s.gitReceivePack(localBareRepos, writer, request)
		//	return
		//}
		filePath := path.Join(rootDir, fileName)
		var fp, err = zipGitRepositoryFS.Open(filePath)
		if err != nil {
			writer.WriteHeader(404)
			return
		}
		defer fp.Close()
		raw, _ := io.ReadAll(fp)

		if strings.Contains(filePath, ".git/") {
			writer.Header().Set("Content-Type", `text/plain`)
		} else {
			writer.Header().Set("Content-Type", `text/html`)
		}
		writer.Write(raw)
	})
}

func (d *VulinServer) gitInfoRefs(localDir string, w http.ResponseWriter, r *http.Request) {
	repo := localDir

	w.Header().Set("content-type", "application/x-git-upload-pack-advertisement")

	ep, err := transport.NewEndpoint("/")
	if err != nil {
		http.Error(w, err.Error(), 500)
		log.Printf("git: %s", err)
		return
	}

	billyfs := osfs.New(repo)
	loader := gitServer.NewFilesystemLoader(billyfs)
	srv := gitServer.NewServer(loader)
	session, err := srv.NewUploadPackSession(ep, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		log.Printf("git: %s", err)
		return
	}

	ar, err := session.AdvertisedReferencesContext(r.Context())
	if errors.Is(err, transport.ErrRepositoryNotFound) {
		http.Error(w, err.Error(), 404)
		return
	} else if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	ar.Prefix = [][]byte{
		[]byte("# service=git-upload-pack"),
		pktline.Flush,
	}

	if err = ar.Encode(w); err != nil {
		http.Error(w, err.Error(), 500)
		log.Printf("git: %s", err)
		return
	}
}

func (d *VulinServer) gitUploadPack(localDir string, w http.ResponseWriter, r *http.Request) {
	repo := localDir
	w.Header().Set("content-type", "application/x-git-upload-pack-result")
	upr := packp.NewUploadPackRequest()
	err := upr.Decode(r.Body)
	if err != nil {
		http.Error(w, err.Error(), 400)
		log.Printf("git: %s", err)
		return
	}

	ep, err := transport.NewEndpoint("/")
	if err != nil {
		http.Error(w, err.Error(), 500)
		log.Printf("git: %s", err)
		return
	}

	billyfs := osfs.New(repo)
	loader := gitServer.NewFilesystemLoader(billyfs)
	svr := gitServer.NewServer(loader)
	session, err := svr.NewUploadPackSession(ep, nil)
	if err != nil {
		http.Error(w, err.Error(), 500)
		log.Printf("git: %s", err)
		return
	}

	res, err := session.UploadPack(r.Context(), upr)
	if err != nil {
		http.Error(w, err.Error(), 500)
		log.Printf("git: %s", err)
		return
	}

	if err = res.Encode(w); err != nil {
		http.Error(w, err.Error(), 500)
		log.Printf("git: %s", err)
		return
	}
}
