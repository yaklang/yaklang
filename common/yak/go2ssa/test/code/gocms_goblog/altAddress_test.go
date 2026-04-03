package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.goblog.app/app/pkgs/contenttype"
	"go.hacdias.com/indielib/indieauth"
)

func Test_toApPersonForAltDomain(t *testing.T) {
	app := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	app.cfg.Server.PublicAddress = "https://new.example.com"
	app.cfg.Server.AltAddresses = []string{"https://old.example.com"}
	app.cfg.Blogs = map[string]*configBlog{
		"testblog": {
			Title:       "Test Blog",
			Description: "A test blog",
		},
	}
	app.cfg.ActivityPub = &configActivityPub{
		Enabled: true,
	}
	app.apPubKeyBytes = []byte("test-key")
	err := app.initConfig(false)
	require.NoError(t, err)
	_ = app.initTemplateStrings()

	// Test that the main domain actor has the old domain in alsoKnownAs
	mainPerson := app.toApPerson("testblog", "")
	assert.NotNil(t, mainPerson)
	foundAltDomain := false
	for _, aka := range mainPerson.AlsoKnownAs {
		if aka.GetLink().String() == "https://old.example.com" {
			foundAltDomain = true
			break
		}
	}
	assert.True(t, foundAltDomain, "main domain actor should have alt domain in alsoKnownAs")

	// Test that the alt domain actor has movedTo pointing to main domain
	altPerson := app.toApPerson("testblog", "https://old.example.com")
	assert.NotNil(t, altPerson)
	assert.Contains(t, altPerson.MovedTo.GetLink().String(), "new.example.com")

	// Check alsoKnownAs on alt domain actor
	foundMainDomain := false
	for _, aka := range altPerson.AlsoKnownAs {
		if aka.GetLink().String() == "https://new.example.com" {
			foundMainDomain = true
			break
		}
	}
	assert.True(t, foundMainDomain, "alt domain actor should have main domain in alsoKnownAs")

	// Verify the actor IDs are correct
	assert.Contains(t, altPerson.ID.GetLink().String(), "old.example.com")
	assert.Contains(t, mainPerson.ID.GetLink().String(), "new.example.com")
}

func Test_altDomainRouting(t *testing.T) {
	app := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	app.cfg.Server.PublicAddress = "https://new.example.com"
	app.cfg.Server.AltAddresses = []string{"https://old.example.com"}
	app.cfg.Blogs = map[string]*configBlog{
		"default": {
			Title:       "Test Blog",
			Description: "A test blog",
		},
	}
	app.cfg.DefaultBlog = "default"
	app.cfg.ActivityPub = &configActivityPub{
		Enabled: true,
	}
	app.apPubKeyBytes = []byte("test-key")
	err := app.initConfig(false)
	require.NoError(t, err)
	_ = app.initTemplateStrings()
	require.NoError(t, app.initActivityPub())

	// Build the router
	router := app.buildRouter()

	// Test ActivityStreams request to alt domain - should return actor with movedTo
	req := httptest.NewRequest(http.MethodGet, "https://old.example.com/", nil)
	req.Host = "old.example.com"
	req.Header.Set("Accept", contenttype.AS)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `"movedTo"`)
	assert.Contains(t, body, `new.example.com`)
	assert.Contains(t, body, `"id":"https://old.example.com"`)

	// Test non-ActivityStreams request to alt domain - should redirect to main domain
	req = httptest.NewRequest(http.MethodGet, "https://old.example.com/some-path?some-query=1", nil)
	req.Host = "old.example.com"
	req.Header.Set("Accept", "text/html")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusPermanentRedirect, rec.Code)
	assert.Equal(t, "https://new.example.com/some-path?some-query=1", rec.Header().Get("Location"))
}

func Test_altDomainRoutingWithoutActivityPub(t *testing.T) {
	app := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	app.cfg.Server.PublicAddress = "https://new.example.com"
	app.cfg.Server.AltAddresses = []string{"https://old.example.com"}

	err := app.initConfig(false)
	require.NoError(t, err)
	_ = app.initTemplateStrings()

	// Build the router
	router := app.buildRouter()

	// Test request to alt domain - should redirect to main domain
	req := httptest.NewRequest(http.MethodGet, "https://old.example.com/some-path?some-query=1", nil)
	req.Host = "old.example.com"
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusPermanentRedirect, rec.Code)
	assert.Equal(t, "https://new.example.com/some-path?some-query=1", rec.Header().Get("Location"))

	// Test request with ActivityStreams Accept header - should also redirect
	req = httptest.NewRequest(http.MethodGet, "https://old.example.com/some-path?some-query=1", nil)
	req.Host = "old.example.com"
	req.Header.Set("Accept", contenttype.AS)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusPermanentRedirect, rec.Code)
	assert.Equal(t, "https://new.example.com/some-path?some-query=1", rec.Header().Get("Location"))
}

func Test_isLocalURL(t *testing.T) {
	app := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	app.cfg.Server.PublicAddress = "https://new.example.com"
	app.cfg.Server.ShortPublicAddress = "https://short.example.com"
	app.cfg.Server.AltAddresses = []string{"https://old.example.com"}
	err := app.initConfig(false)
	require.NoError(t, err)

	// Test main public address
	assert.True(t, app.isLocalURL("https://new.example.com/some/path"))

	// Test short public address
	assert.True(t, app.isLocalURL("https://short.example.com/s/abc123"))

	// Test alt domain
	assert.True(t, app.isLocalURL("https://old.example.com/test"))

	// Test external domain
	assert.False(t, app.isLocalURL("https://external.example.com/test"))
}

func Test_indieAuthWithAltAddress(t *testing.T) {
	// This test verifies that IndieAuth works when accessed through an alternative address
	// configured as indieAuthAddress
	app := &goBlog{
		httpClient: newFakeHttpClient().Client,
		cfg:        createDefaultTestConfig(t),
	}
	app.cfg.Server.PublicAddress = "https://new.example.com"
	app.cfg.Server.AltAddresses = []string{"https://old.example.com"}
	app.cfg.Server.IndieAuthAddress = "https://old.example.com"
	app.cfg.Blogs = map[string]*configBlog{
		"en": {
			Lang: "en",
		},
	}
	app.cfg.User.Name = "John Doe"
	app.cfg.User.Nick = "jdoe"
	app.cfg.Cache.Enable = false

	err := app.initConfig(false)
	require.NoError(t, err)
	_ = app.initTemplateStrings()
	app.reloadRouter()
	app.initIndieAuth()

	app.ias.Client = newHandlerClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Create IndieAuth client pointing to the alt address (indieAuthAddress)
	iac := indieauth.NewClient(
		"https://client.example.com/",
		"https://client.example.com/redirect",
		newHandlerClient(app.d),
	)
	require.NotNil(t, iac)

	// Test: Discover metadata from the main domain should return endpoints on indieAuthAddress
	t.Run("discover metadata from main domain returns indieAuthAddress endpoints", func(t *testing.T) {
		metadata, err := iac.DiscoverMetadata(context.Background(), "https://new.example.com/")
		require.NoError(t, err)
		if assert.NotNil(t, metadata) {
			// Endpoints should be on the indieAuthAddress (alt address)
			assert.Equal(t, "https://old.example.com/indieauth", metadata.AuthorizationEndpoint)
			assert.Equal(t, "https://old.example.com/indieauth/token", metadata.TokenEndpoint)
		}
	})

	// Test: Full authentication flow via indieAuthAddress
	t.Run("full authentication flow via indieAuthAddress", func(t *testing.T) {
		// Authenticate using main domain - it should discover endpoints on alt domain
		authinfo, redirect, err := iac.Authenticate(context.Background(), "https://new.example.com/", "create")
		require.NoError(t, err)
		assert.NotNil(t, authinfo)
		assert.NotEmpty(t, redirect)
		// The redirect should be to the alt address (indieAuthAddress)
		assert.Contains(t, redirect, "old.example.com")

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, redirect, nil)
		app.d.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "https://client.example.com/redirect")

		parsedHtml, err := goquery.NewDocumentFromReader(strings.NewReader(rec.Body.String()))
		require.NoError(t, err)

		indieauthForm := parsedHtml.Find("form[action='/indieauth/accept']")
		assert.Equal(t, 1, indieauthForm.Length())
		indieAuthFormRedirectUri := indieauthForm.Find("input[name='redirect_uri']").AttrOr("value", "")
		assert.Equal(t, "https://client.example.com/redirect", indieAuthFormRedirectUri)
		indieAuthFormClientId := indieauthForm.Find("input[name='client_id']").AttrOr("value", "")
		assert.Equal(t, "https://client.example.com/", indieAuthFormClientId)
		indieAuthFormCodeChallenge := indieauthForm.Find("input[name='code_challenge']").AttrOr("value", "")
		assert.NotEmpty(t, indieAuthFormCodeChallenge)
		indieAuthFormCodeChallengeMethod := indieauthForm.Find("input[name='code_challenge_method']").AttrOr("value", "")
		assert.Equal(t, "S256", indieAuthFormCodeChallengeMethod)
		indieAuthFormState := indieauthForm.Find("input[name='state']").AttrOr("value", "")
		assert.NotEmpty(t, indieAuthFormState)

		rec = httptest.NewRecorder()
		reqBody := url.Values{
			"redirect_uri":          {indieAuthFormRedirectUri},
			"client_id":             {indieAuthFormClientId},
			"scopes":                {"create"},
			"code_challenge":        {indieAuthFormCodeChallenge},
			"code_challenge_method": {indieAuthFormCodeChallengeMethod},
			"state":                 {indieAuthFormState},
		}
		// Accept via alt address
		req = httptest.NewRequest(http.MethodPost, "https://old.example.com/indieauth/accept?"+reqBody.Encode(), nil)
		req.Host = "old.example.com"
		setLoggedIn(req, true)
		app.d.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusFound, rec.Code)

		redirectLocation := rec.Header().Get("Location")
		assert.NotEmpty(t, redirectLocation)
		redirectUrl, err := url.Parse(redirectLocation)
		require.NoError(t, err)
		assert.NotEmpty(t, redirectUrl.Query().Get("code"))
		assert.NotEmpty(t, redirectUrl.Query().Get("state"))
		// Verify me parameter is the alt address
		assert.Equal(t, "https://old.example.com/", redirectUrl.Query().Get("me"))

		validateReq := httptest.NewRequest(http.MethodGet, redirectLocation, nil)
		code, err := iac.ValidateCallback(authinfo, validateReq)
		require.NoError(t, err)
		assert.NotEmpty(t, code)

		// Get token
		token, _, err := iac.GetToken(context.Background(), authinfo, code)
		require.NoError(t, err)
		assert.NotNil(t, token)
		assert.NotEqual(t, "", token.AccessToken)

		// Verify token via alt address
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, "https://old.example.com/indieauth/token", nil)
		req.Host = "old.example.com"
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
		app.d.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), "\"active\":true")
		assert.Contains(t, rec.Body.String(), "\"me\":\"https://old.example.com/\"")
	})

	// Test: HTML header contains IndieAuth links pointing to indieAuthAddress
	t.Run("html header has indieauth links to indieAuthAddress", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "https://new.example.com/", nil)
		req.Host = "new.example.com"
		req.Header.Set("Accept", "text/html")

		rec := httptest.NewRecorder()
		app.d.ServeHTTP(rec, req)

		body := rec.Body.String()
		// Note: HTML may be minified with unquoted attributes
		assert.Contains(t, body, "href=https://old.example.com/indieauth")
		assert.Contains(t, body, "href=https://old.example.com/indieauth/token")
		assert.Contains(t, body, "href=https://old.example.com/.well-known/oauth-authorization-server")
	})
}

func Test_indieAuthAddressValidation(t *testing.T) {
	t.Run("indieAuthAddress must be in altAddresses", func(t *testing.T) {
		app := &goBlog{
			cfg: createDefaultTestConfig(t),
		}
		app.cfg.Server.PublicAddress = "https://new.example.com"
		app.cfg.Server.AltAddresses = []string{"https://old.example.com"}
		app.cfg.Server.IndieAuthAddress = "https://other.example.com" // Not in altAddresses

		err := app.initConfig(false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "indieAuthAddress must be one of the altAddresses")
	})

	t.Run("indieAuthAddress valid when in altAddresses", func(t *testing.T) {
		app := &goBlog{
			cfg: createDefaultTestConfig(t),
		}
		app.cfg.Server.PublicAddress = "https://new.example.com"
		app.cfg.Server.AltAddresses = []string{"https://old.example.com"}
		app.cfg.Server.IndieAuthAddress = "https://old.example.com"

		err := app.initConfig(false)
		require.NoError(t, err)
	})
}
