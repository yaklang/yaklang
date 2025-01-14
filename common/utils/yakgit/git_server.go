package yakgit

import (
	"bytes"
	"errors"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing/format/pktline"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitServer "github.com/go-git/go-git/v5/plumbing/transport/server"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/ziputil"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

/*
* use zipfile and routPrefix should be same
* zipRaw should be zip `.git` folder
* zipfileName should be the name of the zip file
 */
func GeneratorGitHTTPHandler(routePrefix, zipfileName string, zipRaw []byte) (string, func(http.ResponseWriter, *http.Request)) {

	var rootDir string
	var fs filesys_interface.FileSystem
	{
		templateDir := os.TempDir()
		rootDir = filepath.Join(templateDir, zipfileName)
		os.RemoveAll(rootDir)

		var err error
		fs, err = filesys.NewZipFSRaw(bytes.NewReader(zipRaw), int64(len(zipRaw)))
		if err != nil {
			log.Errorf("cannot open zip file: %s", err)
		}
		// unzip to templateDir
		ziputil.DeCompressFromRaw(zipRaw, templateDir)
		// in decompress `${templateDir}/${zipfileName} = ${rootDir}`
		// then use rootDir
		log.Infof("decompress %s to %s", zipfileName, rootDir)
	}

	route := path.Join("/", routePrefix, zipfileName)

	return route, func(writer http.ResponseWriter, request *http.Request) {
		// request.URL.Path = strings.TrimPrefix(request.URL.Path, route)
		// request.RequestURI = strings.TrimPrefix(request.RequestURI, route)

		fileName := strings.TrimPrefix(request.URL.Path, route)
		log.Infof("fetch %v: %s", rootDir, fileName)
		serviceParams := request.URL.Query().Get("service")
		log.Infof("service: %s to %v", serviceParams, fileName)
		if serviceParams == "git-upload-pack" && strings.HasSuffix(request.URL.Path, `/info/refs`) {
			gitInfoRefs(rootDir, writer, request)
			return
		}

		if strings.HasSuffix(request.URL.Path, `/git-upload-pack`) && request.Method == "POST" {
			gitUploadPack(rootDir, writer, request)
			return
		}

		//if strings.HasSuffix(request.URL.Path, `/git-receive-pack`) && request.Method == "POST" {
		//	s.gitReceivePack(localBareRepos, writer, request)
		//	return
		//}
		filePath := path.Join(rootDir, fileName)
		var fp, err = fs.Open(filePath)
		if err != nil {
			writer.WriteHeader(404)
			return
		}
		defer fp.Close()
		raw, _ := io.ReadAll(fp)

		if strings.Contains(filePath, ".git/") {
			writer.Header().Set("Content-Type", `application/octet-stream`)
		} else {
			writer.Header().Set("Content-Type", `text/html`)
		}
		writer.Write(raw)

	}
}

func gitInfoRefs(localDir string, w http.ResponseWriter, r *http.Request) {
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
		log.Printf("git: %s with local: %s", err, localDir)
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

func gitUploadPack(localDir string, w http.ResponseWriter, r *http.Request) {
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
