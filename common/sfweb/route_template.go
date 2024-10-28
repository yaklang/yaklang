package sfweb

import (
	"embed"
	_ "embed"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	//go:embed templates/**
	templateFS embed.FS

	LangToTemplateMap     = make(map[string][]string)
	TemplateToFilenameMap = make(map[string]string)
	TemplateContentCache  = utils.NewTTLCache[[]byte](5 * time.Minute)
)

func init() {
	entries, _ := templateFS.ReadDir("templates")
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		_, lang := path.Split(entry.Name())
		templateEntries, err := templateFS.ReadDir(path.Join("templates", lang))
		if err == nil {
			LangToTemplateMap[lang] = lo.FilterMap(templateEntries, func(entry fs.DirEntry, _ int) (string, bool) {
				if entry.IsDir() {
					return "", false
				}
				fullname := entry.Name()
				name := fullname
				if strings.Contains(fullname, ".") {
					name, _, _ = strings.Cut(fullname, ".")
				}
				TemplateToFilenameMap[fmt.Sprintf("%s/%s", lang, name)] = fullname
				return name, true
			})
		} else {
			SfWebLogger.Errorf("read dir error: %v", err)
		}
	}
}

type InvalidLangError struct {
	lang string
}

func (e *InvalidLangError) Error() string {
	return fmt.Sprintf("invalid lang: %s", e.lang)
}

func NewInvalidLangError(lang string) error {
	return &InvalidLangError{lang}
}

type InvalidTemplateError struct {
	template string
}

func (e *InvalidTemplateError) Error() string {
	return fmt.Sprintf("invalid template: %s", e.template)
}

func NewInvalidTemplateError(template string) error {
	return &InvalidTemplateError{template}
}

type ReadFileError struct{}

func (e *ReadFileError) Error() string {
	return "read file error"
}

func NewReadFileError() error {
	return &ReadFileError{}
}

type TemplateLangResponse struct {
	// 支持的语言
	Language []consts.Language `json:"language"`
}

type TemplateListResponse struct {
	// 模板
	Template []string `json:"template"`
}

type TemplateContentResponse struct {
	Content []byte `json:"content"`
}

func toValidLang(lang string) (string, bool) {
	valid, err := consts.ValidateLanguage(lang)
	if err != nil {
		return "", false
	}
	lang = string(valid)
	if _, ok := LangToTemplateMap[lang]; !ok {
		return "", false
	}
	return lang, true
}

func GetAllSupportedLanguages() []consts.Language {
	return lo.FilterMap(consts.GetAllSupportedLanguages(), func(item consts.Language, index int) (consts.Language, bool) {
		return item, item != consts.JS
	})
}

func (s *SyntaxFlowWebServer) registerTemplateRoute() {
	router := s.router
	subRouter := router.Name("template").Subrouter()

	// 获取支持的语言
	subRouter.HandleFunc("/template_lang", func(w http.ResponseWriter, r *http.Request) {
		writeJson(w, TemplateLangResponse{Language: GetAllSupportedLanguages()})
	}).Name("template lang").Methods(http.MethodGet)

	// 获取语言内的模板列表
	subRouter.HandleFunc("/template/{lang}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		lang := vars["lang"]
		validLang, ok := toValidLang(lang)
		if !ok {
			writeErrorJson(w, NewInvalidLangError(lang))
			return
		}
		templates := LangToTemplateMap[validLang]
		writeJson(w, TemplateListResponse{Template: templates})
	}).Name("template list").Methods(http.MethodGet)

	// 获取模板内容
	subRouter.HandleFunc("/template/{lang}/{id}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		lang := vars["lang"]
		id := vars["id"]
		lang, ok := toValidLang(lang)
		if !ok {
			writeErrorJson(w, NewInvalidLangError(vars["lang"]))
			return
		}
		fullID := fmt.Sprintf("%s/%s", lang, id)
		// cache
		if content, ok := TemplateContentCache.Get(fullID); ok {
			writeJson(w, TemplateContentResponse{Content: content})
			return
		}

		filename, ok := TemplateToFilenameMap[fullID]
		if !ok {
			writeErrorJson(w, NewInvalidTemplateError(fullID))
			return
		}
		content, err := templateFS.ReadFile(path.Join("templates", lang, filename))
		if err != nil {
			writeErrorJson(w, utils.JoinErrors(err, NewReadFileError()))
			return
		}
		TemplateContentCache.Set(fullID, content)
		writeJson(w, TemplateContentResponse{Content: content})
	}).Name("template list").Methods(http.MethodGet)
}
