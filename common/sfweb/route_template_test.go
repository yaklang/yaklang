package sfweb_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/sfweb"
)

func TestTemplateLang(t *testing.T) {
	var data sfweb.TemplateLangResponse
	rawRsp, err := DoResponse("GET", "/template_lang", &data)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rawRsp.GetStatusCode())
	require.ElementsMatch(t, sfweb.GetAllSupportedLanguages(), data.Language)
}

func TestTemplateList(t *testing.T) {
	// positive
	var data sfweb.TemplateListResponse
	rawRsp, err := DoResponse("GET", "/template/yak", &data)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rawRsp.GetStatusCode())
	require.ElementsMatch(t, sfweb.LangToTemplateMap["yak"], data.Template)

	// negative
	id := uuid.NewString()
	var errData sfweb.ErrorResponse
	rawRsp, err = DoResponse("GET", "/template/"+id, &errData)
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, rawRsp.GetStatusCode())
	require.Equal(t, sfweb.NewInvalidLangError(id).Error(), errData.Message)
}

func TestTemplateContent(t *testing.T) {
	// positive
	var data sfweb.TemplateContentResponse
	rawRsp, err := DoResponse("GET", "/template/yak/example", &data)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rawRsp.GetStatusCode())
	content, ok := sfweb.TemplateContentCache.Get("yak/example")
	require.True(t, ok)
	require.Equal(t, data.Content, content)
	// hit cache
	rawRsp, err = DoResponse("GET", "/template/yak/example", &data)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rawRsp.GetStatusCode())
	content, ok = sfweb.TemplateContentCache.Get("yak/example")
	require.True(t, ok)
	require.Equal(t, data.Content, content)

	// negative invalid lang
	lang := uuid.NewString()
	var errData sfweb.ErrorResponse
	rawRsp, err = DoResponse("GET", fmt.Sprintf("/template/%s/example", lang), &errData)
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, rawRsp.GetStatusCode())
	require.Equal(t, sfweb.NewInvalidLangError(lang).Error(), errData.Message)

	// negative invalid id
	template := uuid.NewString()
	rawRsp, err = DoResponse("GET", fmt.Sprintf("/template/yak/%s", template), &errData)
	require.NoError(t, err)
	require.Equal(t, http.StatusInternalServerError, rawRsp.GetStatusCode())
	require.Equal(t, sfweb.NewInvalidTemplateError(fmt.Sprintf("yak/%s", template)).Error(), errData.Message)
}
