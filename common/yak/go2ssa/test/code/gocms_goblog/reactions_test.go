package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_reactionsLowLevel(t *testing.T) {
	app := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	app.cfg.Reactions = &configReactions{Enabled: true}

	_ = app.initConfig(false)

	err := app.saveReaction("🖕", "/testpost")
	assert.ErrorContains(t, err, "not allowed")

	err = app.saveReaction("❤️", "/testpost")
	assert.ErrorContains(t, err, "constraint failed")

	// Create a post
	err = app.createPost(&post{
		Path:    "/testpost",
		Content: "test",
		Status:  statusPublished,
	})
	require.NoError(t, err)

	// Create 4 reactions
	for range 4 {
		err = app.saveReaction("❤️", "/testpost")
		assert.NoError(t, err)
	}

	// Check if reaction count is 4
	reacts, err := app.getReactionsFromDatabase("/testpost")
	require.NoError(t, err)
	assert.Equal(t, "{\"❤️\":4}", reacts)

	// Change post path
	err = app.replacePost(&post{
		Path:    "/newpost",
		Content: "test",
		Status:  statusPublished,
	}, "/testpost", statusPublished, visibilityPublic, false)
	require.NoError(t, err)

	// Check if reaction count is 4
	reacts, err = app.getReactionsFromDatabase("/newpost")
	require.NoError(t, err)
	assert.Equal(t, "{\"❤️\":4}", reacts)

	// Delete post
	err = app.deletePost("/newpost")
	require.NoError(t, err)
	err = app.deletePost("/newpost")
	require.NoError(t, err)

	// Check if reaction count is 0
	reacts, err = app.getReactionsFromDatabase("/newpost")
	require.NoError(t, err)
	assert.Equal(t, "{}", reacts)

	// Create a post with disabled reactions
	err = app.createPost(&post{
		Path:    "/testpost2",
		Content: "test",
		Status:  statusPublished,
		Parameters: map[string][]string{
			"reactions": {"false"},
		},
	})
	require.NoError(t, err)

	// Create reaction
	err = app.saveReaction("❤️", "/testpost2")
	require.NoError(t, err)

	// Check if reaction count is 0
	reacts, err = app.getReactionsFromDatabase("/testpost2")
	require.NoError(t, err)
	assert.Equal(t, "{}", reacts)

}

func Test_reactionsHighLevel(t *testing.T) {
	app := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	app.cfg.Reactions = &configReactions{Enabled: true}

	_ = app.initConfig(false)

	// Send unsuccessful reaction
	form := url.Values{
		"reaction": {"❤️"},
		"path":     {"/testpost"},
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.postReaction(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Create a post
	err := app.createPost(&post{
		Path:    "/testpost",
		Content: "test",
	})
	require.NoError(t, err)

	// Send successful reaction
	form = url.Values{
		"reaction": {"❤️"},
		"path":     {"/testpost"},
	}
	req = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	app.postReaction(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)

	// Check if reaction count is 1
	req = httptest.NewRequest(http.MethodGet, "/?path=/testpost", nil)
	rec = httptest.NewRecorder()
	app.getReactions(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "{\"❤️\":1}", rec.Body.String())

	// Get reactions for a non-existing post
	req = httptest.NewRequest(http.MethodGet, "/?path=/non-existing-post", nil)
	rec = httptest.NewRecorder()
	app.getReactions(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "{}", rec.Body.String())

}
