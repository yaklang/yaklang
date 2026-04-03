package main

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/carlmjohnson/requests"
	"go.goblog.app/app/pkgs/bodylimit"
	"go.goblog.app/app/pkgs/bufferpool"
	"go.goblog.app/app/pkgs/contenttype"
	"go.goblog.app/app/pkgs/gpxhelper"
	"go.hacdias.com/indielib/micropub"
	"gopkg.in/yaml.v3"
)

const editorPath = "/editor"

func (a *goBlog) serveEditor(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, a.renderEditor, &renderData{
		Data: &editorRenderData{
			presetParams: parsePresetPostParamsFromQuery(r),
		},
	})
}

func (a *goBlog) serveEditorPost(w http.ResponseWriter, r *http.Request) {
	switch action := r.FormValue("editoraction"); action { //nolint:gosec
	case "loadupdate":
		post, err := a.getPost(r.FormValue("path")) //nolint:gosec
		if err != nil {
			a.serveError(w, r, err.Error(), http.StatusBadRequest)
			return
		}
		a.render(w, r, a.renderEditor, &renderData{
			Data: &editorRenderData{
				presetParams:      parsePresetPostParamsFromQuery(r),
				updatePostURL:     a.fullPostURL(post),
				updatePostContent: post.contentWithParams(),
			},
		})
	case "createpost", "updatepost":
		reqBody := map[string]any{}
		if action == "updatepost" {
			reqBody["action"] = micropub.ActionUpdate
			reqBody["url"] = r.FormValue("url") //nolint:gosec
			reqBody["replace"] = map[string][]string{
				"content":       {r.FormValue("content")}, //nolint:gosec
				"goblog-editor": r.Form["options"],
			}
		} else {
			// Handle files first
			images, gpx, statusCode, err := a.editorHandleFileAttachments(r)
			if err != nil {
				a.serveError(w, r, err.Error(), statusCode)
				return
			}
			// Then handle the rest of the request
			blog, _ := a.getBlog(r)
			reqBody["type"] = []string{"h-entry"}
			reqBody["properties"] = map[string][]string{
				"content":                 {r.FormValue("content")}, //nolint:gosec
				"blog":                    {blog},
				a.cfg.Micropub.PhotoParam: images,
				gpxParameter:              {gpx},
			}
		}
		req, _ := requests.URL("").BodyJSON(reqBody).Request(r.Context())
		a.editorMicropubPost(w, req, false, "")
	case "upload":
		a.editorMicropubPost(w, r, true, "")
	case "delete", "undelete":
		req, _ := requests.URL("").
			Method(http.MethodPost).
			BodyForm(url.Values{"action": {action}, "url": {r.FormValue("url")}}). //nolint:gosec
			Request(r.Context())
		a.editorMicropubPost(w, req, false, r.FormValue("url")) //nolint:gosec
	case "visibility":
		reqBody := map[string]any{}
		reqBody["action"] = micropub.ActionUpdate
		reqBody["url"] = r.FormValue("url")                                                 //nolint:gosec
		reqBody["replace"] = map[string][]string{"visibility": {r.FormValue("visibility")}} //nolint:gosec
		req, _ := requests.URL("").BodyJSON(reqBody).Request(r.Context())
		a.editorMicropubPost(w, req, false, r.FormValue("url")) //nolint:gosec
	case "tts":
		parsedURL, err := url.Parse(r.FormValue("url")) //nolint:gosec
		if err != nil {
			a.serveError(w, r, err.Error(), http.StatusBadRequest)
			return
		}
		post, err := a.getPost(parsedURL.Path)
		if err != nil {
			a.serveError(w, r, err.Error(), http.StatusBadRequest)
			return
		}
		if err = a.createPostTTSAudio(post); err != nil {
			a.serveError(w, r, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, post.Path, http.StatusFound)
	case "helpgpx":
		err := r.ParseMultipartForm(10 * bodylimit.MB) //nolint:gosec
		if err != nil {
			a.serveError(w, r, err.Error(), http.StatusBadRequest)
			return
		}
		files := r.MultipartForm.File["files"]
		allFileContents := [][]byte{}
		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				a.serveError(w, r, err.Error(), http.StatusBadRequest)
				return
			}
			fileContent, err := io.ReadAll(file)
			_ = file.Close()
			if err != nil {
				a.serveError(w, r, err.Error(), http.StatusBadRequest)
				return
			}
			allFileContents = append(allFileContents, fileContent)
		}
		mergedGpx, err := gpxhelper.MergeGpx(allFileContents...)
		if err != nil {
			a.serveError(w, r, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set(contentType, contenttype.TextUTF8)
		buf := bufferpool.Get()
		defer bufferpool.Put(buf)
		err = a.min.Get().Minify(contenttype.XML, buf, bytes.NewReader(mergedGpx))
		if err != nil {
			a.serveError(w, r, err.Error(), http.StatusBadRequest)
			return
		}
		_ = yaml.NewEncoder(w).Encode(map[string]string{
			"gpx": buf.String(),
		})
	default:
		a.serveError(w, r, "Unknown or missing editoraction", http.StatusBadRequest)
	}
}

func (a *goBlog) editorHandleFileAttachments(r *http.Request) (images []string, gpx string, statusCode int, rerr error) {
	err := r.ParseMultipartForm(10 * bodylimit.MB)
	if err != nil {
		return nil, "", http.StatusBadRequest, err
	}
	files := []*multipart.FileHeader{}
	for _, name := range []string{"files1", "files2", "files3"} {
		files = append(files, r.MultipartForm.File[name]...)
	}
	if len(files) > 0 {
		gpxFiles := [][]byte{}
		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				return nil, "", http.StatusBadRequest, err
			}
			defer file.Close()
			// Create a new request for each file (if it's a GPX use, collect it first, later merge using the helper)
			if strings.HasSuffix(fileHeader.Filename, ".gpx") {
				fileContent, err := io.ReadAll(file)
				if err != nil {
					return nil, "", http.StatusBadRequest, err
				}
				gpxFiles = append(gpxFiles, fileContent)
				_ = file.Close()
				continue
			}
			// Create a new multipart request with the file and get the location header that is returned
			requestBody := bufferpool.Get()
			defer bufferpool.Put(requestBody)
			mpw := multipart.NewWriter(requestBody)
			part, err := mpw.CreateFormFile("file", fileHeader.Filename)
			if err != nil {
				return nil, "", http.StatusBadRequest, err
			}
			_, err = io.Copy(part, file)
			if err != nil {
				return nil, "", http.StatusBadRequest, err
			}
			err = mpw.Close()
			if err != nil {
				return nil, "", http.StatusBadRequest, err
			}
			req, err := requests.URL("").
				Method(http.MethodPost).
				BodyReader(requestBody).
				Header(contentType, mpw.FormDataContentType()).
				Request(r.Context())
			if err != nil {
				return nil, "", http.StatusBadRequest, err
			}
			recorder := httptest.NewRecorder()
			addAllScopes(a.getMicropubImplementation().getMediaHandler()).ServeHTTP(recorder, req)
			result := recorder.Result()
			_ = result.Body.Close()
			if result.StatusCode != http.StatusCreated {
				return nil, "", result.StatusCode, fmt.Errorf("failed to upload file")
			}
			location := result.Header.Get("Location")
			if location == "" {
				return nil, "", http.StatusBadRequest, fmt.Errorf("failed to get location header")
			}
			images = append(images, location)
		}
		// Merge the GPX files
		if len(gpxFiles) > 0 {
			mergedGpx, err := gpxhelper.MergeGpx(gpxFiles...)
			if err != nil {
				return nil, "", http.StatusBadRequest, err
			}
			buf := bufferpool.Get()
			defer bufferpool.Put(buf)
			err = a.min.Get().Minify(contenttype.XML, buf, bytes.NewReader(mergedGpx))
			if err != nil {
				return nil, "", http.StatusBadRequest, err
			}
			gpx = buf.String()
		}
	}
	return images, gpx, 0, nil
}

func (a *goBlog) editorMicropubPost(w http.ResponseWriter, r *http.Request, media bool, redirectSuccess string) {
	recorder := httptest.NewRecorder()
	if media {
		addAllScopes(a.getMicropubImplementation().getMediaHandler()).ServeHTTP(recorder, r)
	} else {
		addAllScopes(a.getMicropubImplementation().getHandler()).ServeHTTP(recorder, r)
	}
	result := recorder.Result()
	if location := result.Header.Get("Location"); location != "" {
		http.Redirect(w, r, location, http.StatusFound)
		return
	}
	if result.StatusCode >= 200 && result.StatusCode < 400 {
		redirectPath := cmp.Or(redirectSuccess, editorPath)
		http.Redirect(w, r, redirectPath, http.StatusFound)
		return
	}
	w.WriteHeader(result.StatusCode)
	_, _ = io.Copy(w, result.Body)
	_ = result.Body.Close()
}

func (*goBlog) editorPostTemplate(blog string, bc *configBlog, presetParams map[string][]string) string {
	builder := bufferpool.Get()
	defer bufferpool.Put(builder)
	marsh := func(param string, preset bool, i any) {
		if _, presetPresent := presetParams[param]; !preset && presetPresent {
			return
		}
		_ = yaml.NewEncoder(builder).Encode(map[string]any{
			param: i,
		})
	}
	builder.WriteString("---\n")
	marsh("blog", false, blog)
	marsh("section", false, bc.DefaultSection)
	marsh("status", false, statusDraft)
	marsh("visibility", false, visibilityPublic)
	marsh("priority", false, 0)
	marsh("slug", false, "")
	marsh("title", false, "")
	for _, t := range bc.Taxonomies {
		marsh(t.Name, false, []string{""})
	}
	for key, param := range presetParams {
		marsh(key, true, param)
	}
	builder.WriteString("---\n")
	return builder.String()
}

func (a *goBlog) editorPostDesc(bc *configBlog) string {
	t := a.ts.GetTemplateStringVariant(bc.Lang, "editorpostdesc")
	paramBuilder, statusBuilder, visibilityBuilder := bufferpool.Get(), bufferpool.Get(), bufferpool.Get()
	defer bufferpool.Put(paramBuilder, statusBuilder, visibilityBuilder)
	for i, param := range []string{
		"published",
		"updated",
		"summary",
		"translationkey",
		"original",
		a.cfg.Micropub.AudioParam,
		a.cfg.Micropub.BookmarkParam,
		a.cfg.Micropub.LikeParam,
		a.cfg.Micropub.LikeTitleParam,
		a.cfg.Micropub.LikeContextParam,
		a.cfg.Micropub.LocationParam,
		a.cfg.Micropub.PhotoParam,
		a.cfg.Micropub.PhotoDescriptionParam,
		a.cfg.Micropub.ReplyParam,
		a.cfg.Micropub.ReplyTitleParam,
		a.cfg.Micropub.ReplyContextParam,
		gpxParameter,
	} {
		if param == "" {
			continue
		}
		if i > 0 {
			paramBuilder.WriteString(", ")
		}
		paramBuilder.WriteByte('`')
		paramBuilder.WriteString(param)
		paramBuilder.WriteByte('`')
	}
	for i, status := range []postStatus{
		statusPublished, statusDraft, statusScheduled,
	} {
		if i > 0 {
			statusBuilder.WriteString(", ")
		}
		statusBuilder.WriteByte('`')
		statusBuilder.WriteString(string(status))
		statusBuilder.WriteByte('`')
	}
	for i, visibility := range []postVisibility{
		visibilityPublic, visibilityUnlisted, visibilityPrivate,
	} {
		if i > 0 {
			visibilityBuilder.WriteString(", ")
		}
		visibilityBuilder.WriteByte('`')
		visibilityBuilder.WriteString(string(visibility))
		visibilityBuilder.WriteByte('`')
	}
	return fmt.Sprintf(t, paramBuilder.String(), "status", "visibility", statusBuilder.String(), visibilityBuilder.String())
}

func parsePresetPostParamsFromQuery(r *http.Request) map[string][]string {
	m := map[string][]string{}
	for key, param := range r.URL.Query() {
		if paramKey, ok := strings.CutPrefix(key, "p:"); ok {
			m[paramKey] = append(m[paramKey], param...)
		}
	}
	return m
}
