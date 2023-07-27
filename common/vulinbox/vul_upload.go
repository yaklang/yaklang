package vulinbox

import (
	"bytes"
	_ "embed"
	"fmt"
	"github.com/h2non/filetype"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
)

//go:embed vul_upload_main.html
var uploadMain []byte

//go:embed vul_upload_result.html
var uploadResult []byte

//go:embed vul_upload_failed.html
var uploadFailed string

func (s *VulinServer) registerUploadCases() {
	r := s.router

	defaultHandler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var route string
		switch LoadFromGetParams(request, "case") {
		case "nullbyte":
			route = "nullbyte"
		case "cve-2017-15715":
			route = "cve-2017-15715"
		case "mime":
			route = "mime"
		case "fileheader":
			route = "fileheader"
		default:
			route = "safe"
		}
		unsafeTemplateRender(writer, request, string(uploadMain), map[string]any{
			"action": "/upload/case/" + route,
		})
	})
	var uploadGroup = r.PathPrefix("/upload").Name("文件上传案例").Subrouter()
	var vuls = []*VulInfo{
		{
			Title:        "基础文件上传案例",
			Path:         "/main",
			DefaultQuery: "case=safe",
			Handler:      defaultHandler,
		},
		{
			Title:        "图片上传（NullByte 截断类型）绕过",
			Path:         "/main",
			DefaultQuery: "case=nullbyte",
			Handler:      defaultHandler,
		},
		{
			Title:        "图片上传（MIME 类型伪造）绕过",
			Path:         "/main",
			DefaultQuery: "case=mime",
			Handler:      defaultHandler,
		},
		{
			Title:        "CVE-2017-15715：Apache HTTPD 换行解析漏洞",
			Path:         "/main",
			DefaultQuery: "case=cve-2017-15715",
			Handler:      defaultHandler,
		},
		{
			Title:        "图片上传：检查文件头",
			Path:         "/main",
			DefaultQuery: "case=fileheader",
			Handler:      defaultHandler,
		},
	}
	for _, v := range vuls {
		addRouteWithVulInfo(uploadGroup, v)
	}

	r.Handle("/upload/case/safe", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fp, header, err := request.FormFile("filename")
		if err != nil {
			Failed(writer, request, "Parse Multipart File Failed: %s", err)
			return
		}
		if header.Filename == "" {
			Failed(writer, request, "Empty Filename")
			return
		}

		fn := header.Filename

		ext := fn[strings.LastIndexByte(fn, '.'):]
		tfp, err := consts.TempFile("temp-*.txt")
		if err != nil {
			Failed(writer, request, "Create Temporary File Failed: %s", err)
			return
		}
		io.Copy(tfp, fp)
		tfp.Close()
		unsafeTemplateRender(writer, request, string(uploadResult), map[string]any{
			"filesize":   utils.ByteSize(uint64(header.Size)),
			"originName": fn,
			"handledExt": ext,
			"path":       tfp.Name(),
		})
	}))

	r.Handle("/upload/case/nullbyte", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fp, header, err := request.FormFile("filename")
		if err != nil {
			Failed(writer, request, "Parse Multipart File Failed: %s", err)
			return
		}
		if header.Filename == "" {
			Failed(writer, request, "Empty Filename")
			return
		}

		fn := header.Filename

		var serverExt string
		var fileSystemExt string
		writeFile, after, ok := strings.Cut(fn, "\x00")
		if ok {
			writeFile = fn
		}
		fileSystemExt = filepath.Ext(writeFile)
		if after != "" {
			serverExt = after
		} else {
			serverExt = fileSystemExt
		}

		if !utils.MatchAnyOfSubString(strings.ToLower(serverExt), "jpg", "png", "jpeg", "ico") {
			unsafeTemplateRender(writer, request, uploadFailed, map[string]any{
				"reason": fmt.Sprintf(
					"u upload file: %v, fileSystemExt: %v, serverExt: %v",
					strconv.Quote(fn), strconv.Quote(fileSystemExt), strconv.Quote(serverExt),
				),
			})
			return
		}

		tfp, err := consts.TempFile("temp-*-" + writeFile)
		if err != nil {
			Failed(writer, request, "Create Temporary File Failed: %s", err)
			return
		}
		io.Copy(tfp, fp)
		tfp.Close()

		unsafeTemplateRender(writer, request, string(uploadResult), map[string]any{
			"filesize":   utils.ByteSize(uint64(header.Size)),
			"originName": fn,
			"handledExt": `ServerExt: ` + strconv.Quote(serverExt) + "  FileSystemExt: " + strconv.Quote(fileSystemExt),
			"path":       tfp.Name(),
		})
	}))
	//mime
	r.Handle("/upload/case/mime", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fp, header, err := request.FormFile("filename")
		if err != nil {
			Failed(writer, request, "Parse Multipart File Failed: %s", err)
			return
		}
		if header.Filename == "" {
			Failed(writer, request, "Empty Filename")
			return
		}

		fn := header.Filename
		t, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
		if err != nil {
			Failed(writer, request, "mimt.ParseMediaType Failed: %s", err)
			return
		}

		if !utils.IContains(t, "image/") {
			unsafeTemplateRender(writer, request, uploadFailed, map[string]any{
				"reason": fmt.Sprintf(
					"found mime type: %s, not a image/*", t,
				),
			})
			return
		}
		var ext = filepath.Ext(fn)
		tfp, err := consts.TempFile("temp-*-" + fn)
		if err != nil {
			Failed(writer, request, "Create Temporary File Failed: %s", err)
			return
		}
		io.Copy(tfp, fp)
		tfp.Close()

		unsafeTemplateRender(writer, request, string(uploadResult), map[string]any{
			"filesize":   utils.ByteSize(uint64(header.Size)),
			"originName": fn,
			"handledExt": ext,
			"path":       tfp.Name(),
		})
	}))

	//cve-2017-15715
	r.Handle("/upload/case/cve-2017-15715", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fp, header, err := request.FormFile("filename")
		if err != nil {
			Failed(writer, request, "Parse Multipart File Failed: %s", err)
			return
		}
		if header.Filename == "" {
			Failed(writer, request, "Empty Filename")
			return
		}

		fn := header.Filename

		var serverExt string
		var fileSystemExt string
		writeFile, after, ok := strings.Cut(fn, "\x0a")
		if ok {
			writeFile = fn
		}
		fileSystemExt = filepath.Ext(writeFile)
		if after != "" {
			serverExt = after
		} else {
			serverExt = fileSystemExt
		}

		if !utils.MatchAnyOfSubString(strings.ToLower(serverExt), "jpg", "png", "jpeg", "ico") {
			unsafeTemplateRender(writer, request, uploadFailed, map[string]any{
				"reason": fmt.Sprintf(
					"u upload file: %v, fileSystemExt: %v, serverExt: %v",
					strconv.Quote(fn), strconv.Quote(fileSystemExt), strconv.Quote(serverExt),
				),
			})
			return
		}

		tfp, err := consts.TempFile("temp-*-" + writeFile)
		if err != nil {
			Failed(writer, request, "Create Temporary File Failed: %s", err)
			return
		}
		io.Copy(tfp, fp)
		tfp.Close()

		unsafeTemplateRender(writer, request, string(uploadResult), map[string]any{
			"filesize":   utils.ByteSize(uint64(header.Size)),
			"originName": fn,
			"handledExt": `ServerExt: ` + strconv.Quote(serverExt) + "  FileSystemExt: " + strconv.Quote(fileSystemExt),
			"path":       tfp.Name(),
		})
	}))

	//mime
	r.Handle("/upload/case/fileheader", http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fp, header, err := request.FormFile("filename")
		if err != nil {
			Failed(writer, request, "Parse Multipart File Failed: %s", err)
			return
		}
		if header.Filename == "" {
			Failed(writer, request, "Empty Filename")
			return
		}

		fn := header.Filename
		t, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
		if err != nil {
			Failed(writer, request, "mimt.ParseMediaType Failed: %s", err)
			return
		}
		_ = t

		var ext = filepath.Ext(fn)
		tfp, err := consts.TempFile("temp-*-" + fn)
		if err != nil {
			Failed(writer, request, "Create Temporary File Failed: %s", err)
			return
		}
		var buf bytes.Buffer
		io.Copy(tfp, io.TeeReader(fp, &buf))
		tfp.Close()

		mimeCheckType, _ := filetype.MatchReader(&buf)
		if !utils.IContains(mimeCheckType.MIME.Value, "image/") {
			unsafeTemplateRender(writer, request, uploadFailed, map[string]any{
				"reason": fmt.Sprintf("upload failed: mime header: %v, filetype actually: %v", t, mimeCheckType.MIME.Value),
			})
			return
		}
		unsafeTemplateRender(writer, request, string(uploadResult), map[string]any{
			"extrainfo":  fmt.Sprintf("MITM TYPE: %v FileTypeHeaderType: %v(%v)", t, mimeCheckType.MIME.Value, mimeCheckType.Extension),
			"filesize":   utils.ByteSize(uint64(header.Size)),
			"originName": fn,
			"handledExt": ext,
			"path":       tfp.Name(),
		})
	}))
}
