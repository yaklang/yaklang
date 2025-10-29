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
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

var (
	//go:embed templates/**
	TemplateFS embed.FS

	LangToTemplateMap     = make(map[string][]string)
	TemplateToFilenameMap = make(map[string]string)
	TemplateContentCache  = utils.NewTTLCache[string](5 * time.Minute)
)

func init() {
	entries, _ := TemplateFS.ReadDir("templates")
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		_, lang := path.Split(entry.Name())
		templateEntries, err := TemplateFS.ReadDir(path.Join("templates", lang))
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
	Language []ssaconfig.Language `json:"language"`
}

type TemplateListResponse struct {
	// 模板
	Template []string `json:"template"`
}

type TemplateContentResponse struct {
	Content string `json:"content"`
}

func toValidLang(lang string) (string, bool) {
	valid, err := ssaconfig.ValidateLanguage(lang)
	if err != nil {
		return "", false
	}
	lang = string(valid)
	if _, ok := LangToTemplateMap[lang]; !ok {
		return "", false
	}
	return lang, true
}

func GetAllSupportedLanguages() []ssaconfig.Language {
	return lo.FilterMap(ssaconfig.GetAllSupportedLanguages(), func(v string, index int) (ssaconfig.Language, bool) {
		item := ssaconfig.Language(v)
		return item, item != ssaconfig.JS
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
		if contentStr, ok := TemplateContentCache.Get(fullID); ok {
			writeJson(w, TemplateContentResponse{Content: contentStr})
			return
		}

		filename, ok := TemplateToFilenameMap[fullID]
		if !ok {
			writeErrorJson(w, NewInvalidTemplateError(fullID))
			return
		}
		content, err := TemplateFS.ReadFile(path.Join("templates", lang, filename))
		if err != nil {
			writeErrorJson(w, utils.JoinErrors(err, NewReadFileError()))
			return
		}
		TemplateContentCache.Set(fullID, string(content))
		writeJson(w, TemplateContentResponse{Content: string(content)})
	}).Name("template list").Methods(http.MethodGet)
}
