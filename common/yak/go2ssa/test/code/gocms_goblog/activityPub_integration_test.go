//go:build ignore
// +build ignore

package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/carlmjohnson/requests"
	"github.com/google/uuid"
	"github.com/mattn/go-mastodon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.goblog.app/app/pkgs/activitypub"
	"go.goblog.app/app/pkgs/bufferpool"
)

const (
	gtsTestEmail       = "gtsuser@example.com"
	gtsTestUsername    = "gtsuser"
	gtsTestPassword    = "GtsPassword123!@#"
	gtsServiceEmail    = "gtsservice@example.com"
	gtsServiceUsername = "gtsservice"
	gtsServicePassword = "GtsService123!@#"
)

func TestIntegrationActivityPubWithGoToSocial(t *testing.T) {
	t.Parallel()

	requireDocker(t)

	// Speed up the AP send queue for testing
	apSendInterval = time.Second

	// Start GoBlog ActivityPub server and GoToSocial instance
	gb := startApIntegrationServer(t)
	gts, mc := startGoToSocialInstance(t, gb.cfg.Server.Port)

	goBlogAcct := fmt.Sprintf("%s@%s", gb.cfg.DefaultBlog, gb.cfg.Server.publicHost)

	// Search for GoBlog account on GoToSocial and follow it
	searchResults, err := mc.Search(t.Context(), goBlogAcct, true)
	require.NoError(t, err)
	require.NotNil(t, searchResults)
	require.Greater(t, len(searchResults.Accounts), 0)
	lookup := searchResults.Accounts[0]
	_, err = mc.AccountFollow(t.Context(), lookup.ID)
	require.NoError(t, err)

	// Verify that GoBlog has the GoToSocial user as a follower
	require.Eventually(t, func() bool {
		followers, err := gb.db.apGetAllFollowers(gb.cfg.DefaultBlog)
		if err != nil {
			return false
		}
		for _, f := range followers {
			if strings.Contains(f.follower, fmt.Sprintf("/users/%s", gtsTestUsername)) {
				return true
			}
		}
		return false
	}, time.Minute, time.Second)

	t.Run("Follow from service actor", func(t *testing.T) {
		t.Parallel()

		clientID, clientSecret := gtsRegisterApp(t, gts.baseURL)
		serviceToken := gtsAuthorizeToken(t, gts.baseURL, clientID, clientSecret, gtsServiceEmail, gtsServicePassword)
		mcService := mastodon.NewClient(&mastodon.Config{Server: gts.baseURL, AccessToken: serviceToken})
		mcService.Client = http.Client{Timeout: time.Minute}

		// Convert service actor to bot account
		err := requests.
			URL(gts.baseURL+"/api/v1/accounts/update_credentials").
			Method(http.MethodPatch).
			Client(&mcService.Client).
			Header("Authorization", "Bearer "+serviceToken).
			BodyForm(url.Values{"bot": {"true"}}).
			Fetch(t.Context())
		require.NoError(t, err)

		// Verify that the account is now a bot
		accountService, err := mcService.GetAccountCurrentUser(t.Context())
		require.NoError(t, err)
		require.True(t, accountService.Bot)
		actor, err := gb.apGetRemoteActor(gb.cfg.DefaultBlog, activitypub.IRI(fmt.Sprintf("%s/users/%s", gts.baseURL, gtsServiceUsername)))
		require.NoError(t, err)
		require.NotNil(t, actor)
		require.Equal(t, activitypub.ServiceType, actor.GetType())

		// Follow GoBlog from the service actor
		searchResultsService, err := mcService.Search(t.Context(), goBlogAcct, true)
		require.NoError(t, err)
		require.NotNil(t, searchResultsService)
		require.Greater(t, len(searchResultsService.Accounts), 0)
		serviceLookup := searchResultsService.Accounts[0]
		_, err = mcService.AccountFollow(t.Context(), serviceLookup.ID)
		require.NoError(t, err)

		// Verify that GoBlog has the service actor as a follower
		require.Eventually(t, func() bool {
			followers, err := gb.db.apGetAllFollowers(gb.cfg.DefaultBlog)
			if err != nil {
				return false
			}
			for _, f := range followers {
				if strings.Contains(f.follower, fmt.Sprintf("/users/%s", gtsServiceUsername)) {
					return true
				}
			}
			return false
		}, time.Minute, time.Second)
	})

	t.Run("Verify follow", func(t *testing.T) {
		// Verify that GoBlog created the follow notification
		require.Eventually(t, func() bool {
			notifications, err := gb.db.getNotifications(&notificationsRequestConfig{limit: 10})
			if err != nil {
				return false
			}
			for _, n := range notifications {
				if strings.Contains(n.Text, "started following") && strings.Contains(n.Text, "/@"+gtsTestUsername) {
					return true
				}
			}
			return false
		}, time.Minute, time.Second)

		// Verify that GoToSocial received the follow accept activity
		require.Eventually(t, func() bool {
			rs, err := mc.GetAccountRelationships(t.Context(), []string{string(lookup.ID)})
			if err != nil {
				return false
			}
			if len(rs) == 0 {
				return false
			}
			return rs[0].Following
		}, time.Minute, time.Second)
	})

	t.Run("Update profile", func(t *testing.T) {
		// Update blog title and check that GoToSocial received the update
		gb.cfg.Blogs[gb.cfg.DefaultBlog].Title = "GoBlog ActivityPub Test Blog Updated"
		gb.apSendProfileUpdates()

		require.Eventually(t, func() bool {
			account, err := mc.GetAccount(t.Context(), lookup.ID)
			if err != nil {
				return false
			}
			return strings.Contains(account.DisplayName, "GoBlog ActivityPub Test Blog Updated")
		}, time.Minute, time.Second)
	})

	t.Run("Post flow", func(t *testing.T) {
		// Create a post on GoBlog and check that it appears on GoToSocial
		p := &post{
			Content: "Hello from GoBlog to GoToSocial!",
		}
		require.NoError(t, gb.createPost(p))
		postURL := gb.fullPostURL(p)

		require.Eventually(t, func() bool {
			statuses, err := mc.GetAccountStatuses(t.Context(), lookup.ID, nil)
			if err != nil {
				return false
			}
			for _, status := range statuses {
				if status.URL == postURL && strings.Contains(status.Content, "Hello from GoBlog to GoToSocial!") {
					return true
				}
			}
			return false
		}, time.Minute, time.Second)

		// Update the post on GoBlog and verify the update appears on GoToSocial
		p.Content = "Updated content from GoBlog to GoToSocial!"
		require.NoError(t, gb.replacePost(p, p.Path, statusPublished, visibilityPublic, false))

		var statusId mastodon.ID
		require.Eventually(t, func() bool {
			statuses, err := mc.GetAccountStatuses(t.Context(), lookup.ID, nil)
			if err != nil {
				return false
			}
			for _, status := range statuses {
				if strings.Contains(status.Content, "Updated content from GoBlog to GoToSocial!") {
					statusId = status.ID
					return true
				}
			}
			return false
		}, time.Minute, time.Second)

		// Favorite the post on GoToSocial and verify GoBlog creates a notification
		_, err = mc.Favourite(t.Context(), statusId)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			notifications, err := gb.db.getNotifications(&notificationsRequestConfig{limit: 10})
			if err != nil {
				return false
			}
			for _, n := range notifications {
				if strings.Contains(n.Text, "liked") && strings.Contains(n.Text, p.Path) {
					return true
				}
			}
			return false
		}, time.Minute, time.Second)

		// Announce the post on GoToSocial and verify GoBlog creates a notification
		_, err = mc.Reblog(t.Context(), statusId)
		require.NoError(t, err)

		require.Eventually(t, func() bool {
			notifications, err := gb.db.getNotifications(&notificationsRequestConfig{limit: 10})
			if err != nil {
				return false
			}
			for _, n := range notifications {
				if strings.Contains(n.Text, "announced") && strings.Contains(n.Text, p.Path) {
					return true
				}
			}
			return false
		}, time.Minute, time.Second)

		// Delete the post on GoBlog and verify it is removed from GoToSocial
		require.NoError(t, gb.deletePost(p.Path))
		require.Eventually(t, func() bool {
			statuses, err := mc.GetAccountStatuses(t.Context(), lookup.ID, nil)
			if err != nil {
				return false
			}
			for _, status := range statuses {
				if status.URL == postURL {
					return false
				}
			}
			return true
		}, time.Minute, time.Second)
	})

	t.Run("Mention to GoToSocial", func(t *testing.T) {
		t.Parallel()

		// Send a new post with a mention from GoBlog to GoToSocial and verify it appears
		p := &post{
			Content: fmt.Sprintf("Hello [@%s@%s](%s/@%s) from GoBlog!", gtsTestUsername, strings.ReplaceAll(gts.baseURL, "http://", ""), gts.baseURL, gtsTestUsername),
		}
		require.NoError(t, gb.createPost(p))
		post2URL := gb.fullPostURL(p)

		// Check that GoToSocial received the post with mention
		require.Eventually(t, func() bool {
			statuses, err := mc.GetAccountStatuses(t.Context(), lookup.ID, nil)
			if err != nil {
				return false
			}
			for _, status := range statuses {
				if status.URL == post2URL {
					return true
				}
			}
			return false
		}, time.Minute, time.Second)
		// Check that GoToSocial created a notification for the mention
		require.Eventually(t, func() bool {
			notifications, err := mc.GetNotifications(t.Context(), nil)
			if err != nil {
				return false
			}
			for _, n := range notifications {
				if n.Status != nil && n.Status.URL == post2URL {
					return true
				}
			}
			return false
		}, time.Minute, time.Second)
	})

	t.Run("Replies to GoToSocial", func(t *testing.T) {
		t.Parallel()

		// Create a post on GoToSocial side and verify that replies are working
		testStatus, err := mc.PostStatus(t.Context(), &mastodon.Toot{Status: "Test", Visibility: mastodon.VisibilityPublic})
		require.NoError(t, err)
		p := &post{
			Parameters: map[string][]string{
				"replylink": {testStatus.URL}, // Using URL to check if the mapping to URI works
			},
			Content: "Replying to GoToSocial post",
		}
		require.NoError(t, gb.createPost(p))
		pUrl := gb.fullPostURL(p)

		// Verify that the reply appears on GoToSocial
		require.Eventually(t, func() bool {
			refreshedStatus, err := mc.GetStatus(t.Context(), testStatus.ID)
			if err != nil {
				return false
			}
			if refreshedStatus.RepliesCount == 1 {
				return true
			}
			return false
		}, time.Minute, time.Second)
		// Check that GoToSocial created a notification for the reply
		require.Eventually(t, func() bool {
			notifications, err := mc.GetNotifications(t.Context(), nil)
			if err != nil {
				return false
			}
			for _, n := range notifications {
				if n.Status != nil && n.Status.URL == pUrl {
					return true
				}
			}
			return false
		}, time.Minute, time.Second)
	})

	t.Run("Reply to GoBlog", func(t *testing.T) {
		t.Parallel()

		// Create a post on GoBlog side
		p := &post{
			Content: "Post to be replied to",
		}
		require.NoError(t, gb.createPost(p))
		postURL := gb.fullPostURL(p)

		// Create a reply on GoToSocial side
		sr, err := mc.Search(t.Context(), postURL, true)
		require.NoError(t, err)
		require.NotNil(t, sr)
		require.Greater(t, len(sr.Statuses), 0)
		replyToStatus := sr.Statuses[0]
		replyStatus, err := mc.PostStatus(t.Context(), &mastodon.Toot{
			Status:      "@" + goBlogAcct + " This is a reply from GoToSocial",
			InReplyToID: replyToStatus.ID,
			Visibility:  mastodon.VisibilityPublic,
		})
		require.NoError(t, err)

		// Verify that GoBlog created a comment for the reply
		require.Eventually(t, func() bool {
			comments, err := gb.db.getComments(&commentsRequestConfig{})
			if err != nil {
				return false
			}
			for _, c := range comments {
				if strings.Contains(c.Comment, "reply") {
					return true
				}
			}
			return false
		}, time.Minute, time.Second)

		// Update the reply on GoToSocial
		_, err = mc.UpdateStatus(t.Context(), &mastodon.Toot{
			Status:      "@" + goBlogAcct + " This is an updated reply from GoToSocial",
			InReplyToID: replyToStatus.ID,
			Visibility:  mastodon.VisibilityPublic,
		}, replyStatus.ID)
		require.NoError(t, err)

		// Verify that GoBlog updated the comment
		require.Eventually(t, func() bool {
			comments, err := gb.db.getComments(&commentsRequestConfig{})
			if err != nil {
				return false
			}
			for _, c := range comments {
				if strings.Contains(c.Comment, "updated reply") {
					return true
				}
			}
			return false
		}, time.Minute, time.Second)

		// Delete the reply on GoToSocial
		err = mc.DeleteStatus(t.Context(), replyStatus.ID)
		require.NoError(t, err)

		// Verify that GoBlog deleted the comment
		require.Eventually(t, func() bool {
			comments, err := gb.db.getComments(&commentsRequestConfig{})
			if err != nil {
				return false
			}
			for _, c := range comments {
				if strings.Contains(c.Comment, "updated reply") {
					return false
				}
			}
			return true
		}, time.Minute, time.Second)

	})

	t.Run("Unfollow", func(t *testing.T) {
		_, err := mc.AccountUnfollow(t.Context(), lookup.ID)
		require.NoError(t, err)

		// Verify that GoBlog removed the follower
		require.Eventually(t, func() bool {
			followers, err := gb.db.apGetAllFollowers(gb.cfg.DefaultBlog)
			if err != nil {
				return false
			}
			for _, f := range followers {
				if strings.Contains(f.follower, gtsTestUsername) {
					return false
				}
			}
			return true
		}, 30*time.Second, time.Second)
	})

}

const (
	gtsTestEmail2    = "gtsuser2@example.com"
	gtsTestUsername2 = "gtsuser2"
	gtsTestPassword2 = "GtsPassword456!@#"
)

func TestIntegrationActivityPubMoveFollowers(t *testing.T) {
	t.Parallel()

	requireDocker(t)

	// Speed up the AP send queue for testing
	apSendInterval = time.Second

	// Start GoBlog ActivityPub server and GoToSocial instance
	gb := startApIntegrationServer(t)
	gts, mc := startGoToSocialInstance(t, gb.cfg.Server.Port)

	// Create a second GTS user account to be the move target
	runDocker(t,
		"exec", gts.containerName,
		"/gotosocial/gotosocial",
		"--config-path", "/config/config.yaml",
		"admin", "account", "create",
		"--username", gtsTestUsername2,
		"--email", gtsTestEmail2,
		"--password", gtsTestPassword2,
	)

	// Get access token for second user
	clientID, clientSecret := gtsRegisterApp(t, gts.baseURL)
	accessToken2 := gtsAuthorizeToken(t, gts.baseURL, clientID, clientSecret, gtsTestEmail2, gtsTestPassword2)
	mc2 := mastodon.NewClient(&mastodon.Config{Server: gts.baseURL, AccessToken: accessToken2})
	mc2.Client = http.Client{Timeout: time.Minute}

	goBlogAcct := fmt.Sprintf("%s@%s", gb.cfg.DefaultBlog, gb.cfg.Server.publicHost)

	// First user follows GoBlog
	searchResults, err := mc.Search(t.Context(), goBlogAcct, true)
	require.NoError(t, err)
	require.NotNil(t, searchResults)
	require.Greater(t, len(searchResults.Accounts), 0)
	lookup := searchResults.Accounts[0]
	_, err = mc.AccountFollow(t.Context(), lookup.ID)
	require.NoError(t, err)

	// Verify that GoBlog has the first GTS user as a follower
	require.Eventually(t, func() bool {
		followers, err := gb.db.apGetAllFollowers(gb.cfg.DefaultBlog)
		if err != nil {
			return false
		}
		return len(followers) >= 1 && strings.Contains(followers[0].follower, fmt.Sprintf("/users/%s", gtsTestUsername))
	}, time.Minute, time.Second)

	// Get the second user's account info
	_, err = mc2.GetAccountCurrentUser(t.Context())
	require.NoError(t, err)

	// Unlock gtsuser2 so follows are auto-accepted during Move
	err = requests.URL(gts.baseURL+"/api/v1/accounts/update_credentials").
		Client(&http.Client{Timeout: time.Minute}).
		Header("Authorization", "Bearer "+accessToken2).
		Method(http.MethodPatch).
		BodyJSON(map[string]any{
			"locked": false,
		}).
		Fetch(t.Context())
	require.NoError(t, err)

	// Construct the ActivityPub actor URI for user2
	account2ActorURI := fmt.Sprintf("%s/users/%s", gts.baseURL, gtsTestUsername2)

	// Set alsoKnownAs on the target account to include the GoBlog account
	err = requests.URL(gts.baseURL+"/api/v1/accounts/alias").
		Client(&http.Client{Timeout: time.Minute}).
		Header("Authorization", "Bearer "+accessToken2).
		Method(http.MethodPost).
		BodyJSON(map[string]any{
			"also_known_as_uris": []string{gb.cfg.Server.PublicAddress},
		}).
		Fetch(t.Context())
	require.NoError(t, err)

	// Now have GoBlog send a Move activity to all followers
	err = gb.apMoveFollowers(gb.cfg.DefaultBlog, account2ActorURI)
	require.NoError(t, err)

	// Verify that the movedTo setting was saved in the database
	movedTo, err := gb.getApMovedTo(gb.cfg.DefaultBlog)
	require.NoError(t, err)
	assert.Equal(t, account2ActorURI, movedTo)

	// Verify that GTS user1 now follows user2 (the move target)
	// GoToSocial processes the Move and automatically creates a follow to the target account
	require.Eventually(t, func() bool {
		// Search for user2 from user1's perspective (local search)
		searchResults2, err := mc.Search(t.Context(), gtsTestUsername2, true)
		if err != nil || len(searchResults2.Accounts) == 0 {
			return false
		}
		user2Account := searchResults2.Accounts[0]

		// Check if user1 is now following user2
		relationships, err := mc.GetAccountRelationships(t.Context(), []string{string(user2Account.ID)})
		if err != nil || len(relationships) == 0 {
			return false
		}
		return relationships[0].Following
	}, time.Minute, time.Second, "GTS user1 should now follow user2 after Move")
}

func TestIntegrationActivityPubDomainMove(t *testing.T) {
	t.Parallel()

	requireDocker(t)

	// Speed up the AP send queue for testing
	apSendInterval = time.Second

	// Start GoBlog with old domain (goblog.example) and follow it
	gb := startApIntegrationServer(t)
	// Pre-register socat proxy for newgoblog.example too (GTS needs it after domain change)
	gts, mc := startGoToSocialInstance(t, gb.cfg.Server.Port, "newgoblog.example")

	goBlogAcct := fmt.Sprintf("%s@%s", gb.cfg.DefaultBlog, gb.cfg.Server.publicHost)

	// Search for GoBlog account on GoToSocial and follow it
	searchResults, err := mc.Search(t.Context(), goBlogAcct, true)
	require.NoError(t, err)
	require.NotNil(t, searchResults)
	require.Greater(t, len(searchResults.Accounts), 0)
	lookup := searchResults.Accounts[0]
	_, err = mc.AccountFollow(t.Context(), lookup.ID)
	require.NoError(t, err)

	// Verify that GoBlog has the GoToSocial user as a follower
	require.Eventually(t, func() bool {
		followers, err := gb.db.apGetAllFollowers(gb.cfg.DefaultBlog)
		if err != nil {
			return false
		}
		for _, f := range followers {
			if strings.Contains(f.follower, fmt.Sprintf("/users/%s", gtsTestUsername)) {
				return true
			}
		}
		return false
	}, time.Minute, time.Second)

	// Now simulate a domain change:
	// 1. Add old domain as an alternative domain
	// 2. Change public address to new domain
	// 3. Rebuild router
	oldDomain := gb.cfg.Server.PublicAddress // "http://goblog.example"
	newDomain := "http://newgoblog.example"

	// Add old domain to alt domains
	gb.cfg.Server.AltAddresses = []string{oldDomain}
	// Change public address to new domain
	gb.cfg.Server.PublicAddress = newDomain

	// Reload config, simulating a restart with the new domain
	gb.cfg.initialized = false
	require.NoError(t, gb.initConfig(false))
	gb.prepareWebfinger()
	gb.reloadRouter()
	gb.purgeCache()

	// Wait for the new domain proxy to be ready
	require.Eventually(t, func() bool {
		acct := "acct:" + gb.cfg.DefaultBlog + "@newgoblog.example"
		cmd := exec.Command("docker", "run", "--rm", "--network", gts.networkName, "docker.io/alpine/curl", "-sS", "-m", "2", "-G", "--data-urlencode", fmt.Sprintf("resource=%s", acct), "http://newgoblog.example/.well-known/webfinger")
		out, err := cmd.CombinedOutput()
		return err == nil && strings.Contains(string(out), "newgoblog.example")
	}, time.Minute, time.Second, "New domain proxy should be ready")

	// Helper to make requests to GoBlog with a custom Host header (tests
	// that don't need Docker DNS can hit localhost directly).
	localGet := func(host, path string, headers map[string]string) (*http.Response, error) {
		req, reqErr := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d%s", gb.cfg.Server.Port, path), nil)
		if reqErr != nil {
			return nil, reqErr
		}
		req.Host = host
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		return (&http.Client{
			Timeout:       5 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		}).Do(req)
	}

	t.Run("Old domain webfinger should work via alt domain handler", func(t *testing.T) {
		t.Parallel()

		acct := "acct:" + gb.cfg.DefaultBlog + "@goblog.example"
		resp, err := localGet("goblog.example", "/.well-known/webfinger?resource="+url.QueryEscape(acct), nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, string(body), "goblog.example", "old domain webfinger should work via alt domain handler")
	})

	t.Run("New domain webfinger should work", func(t *testing.T) {
		t.Parallel()

		acct := "acct:" + gb.cfg.DefaultBlog + "@newgoblog.example"
		resp, err := localGet("newgoblog.example", "/.well-known/webfinger?resource="+url.QueryEscape(acct), nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, string(body), "newgoblog.example", "new domain webfinger should work")
	})

	t.Run("Old domain actor shows movedTo", func(t *testing.T) {
		t.Parallel()

		resp, err := localGet("goblog.example", "/", map[string]string{"Accept": "application/activity+json"})
		require.NoError(t, err)
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, string(body), "movedTo", "old domain actor should have movedTo field")
		assert.Contains(t, string(body), "://newgoblog.example", "old domain actor movedTo should point to new domain")
	})

	t.Run("New domain actor has alsoKnownAs", func(t *testing.T) {
		t.Parallel()

		resp, err := localGet("newgoblog.example", "/", map[string]string{"Accept": "application/activity+json"})
		require.NoError(t, err)
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, string(body), "alsoKnownAs", "new domain actor should have alsoKnownAs field")
		assert.Contains(t, string(body), "://goblog.example", "new domain actor alsoKnownAs should include old domain")
	})

	t.Run("Alt domain redirects to new domain", func(t *testing.T) {
		resp, err := localGet("goblog.example", "/test-path", nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusPermanentRedirect, resp.StatusCode)
		assert.Contains(t, resp.Header.Get("Location"), "newgoblog.example", "non-AP request should redirect to new domain")
	})

	// Send profile update (like on startup)
	gb.apSendProfileUpdates()
	// Now send domain move activities
	err = gb.apDomainMove(oldDomain, newDomain)
	require.NoError(t, err)

	// Test that GTS automatically follows the new domain actor
	newAcct := fmt.Sprintf("%s@newgoblog.example", gb.cfg.DefaultBlog)
	require.Eventually(t, func() bool {
		searchResults, err := mc.Search(t.Context(), newAcct, true)
		if err != nil || searchResults == nil || len(searchResults.Accounts) == 0 {
			return false
		}
		newLookup := searchResults.Accounts[0]
		relationships, err := mc.GetAccountRelationships(t.Context(), []string{string(newLookup.ID)})
		if err != nil || len(relationships) == 0 {
			return false
		}
		return relationships[0].Following
	}, time.Minute, time.Second)

	// Test that GTS is still registered as a follower on the GoBlog side
	followers, err := gb.db.apGetAllFollowers(gb.cfg.DefaultBlog)
	require.NoError(t, err)
	assert.Len(t, followers, 1)
}

// startDomainProxy creates a lightweight TCP forwarder for the given hostname
// that forwards port 80 inside Docker to GoBlog's port on the host.
func startDomainProxy(t *testing.T, netName string, hostname string, goblogPort int) {
	t.Helper()
	proxyName := fmt.Sprintf("goblog-proxy-%s", uuid.New().String())
	runDocker(t,
		"run", "-d", "--rm",
		"--name", proxyName,
		"--network", netName,
		"--network-alias", hostname,
		"--add-host", "host.docker.internal:host-gateway",
		"docker.io/alpine/socat",
		"TCP-LISTEN:80,fork,reuseaddr",
		fmt.Sprintf("TCP:host.docker.internal:%d", goblogPort),
	)
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", proxyName).Run()
	})
}

func requireDocker(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not installed")
	}
	cmd := exec.Command("docker", "info")
	if err := cmd.Run(); err != nil {
		t.Skipf("docker not available: %v", err)
	}
}

func getFreePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func runDocker(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "docker %s: %s", strings.Join(args, " "), string(output))
	return strings.TrimSpace(string(output))
}

func startApIntegrationServer(t *testing.T) *goBlog {
	t.Helper()
	port := getFreePort(t)
	app := &goBlog{
		cfg:        createDefaultTestConfig(t),
		httpClient: newHTTPClient(),
	}
	// Externally expose GoBlog as goblog.example (proxied to the test port)
	app.cfg.Server.PublicAddress = "http://goblog.example"
	app.cfg.Server.Port = port
	app.cfg.ActivityPub.Enabled = true
	// Initialize the app
	require.NoError(t, app.initConfig(false))
	require.NoError(t, app.initTemplateStrings())
	require.NoError(t, app.initActivityPub())
	// Enable comments for testing reply flows
	app.cfg.Blogs[app.cfg.DefaultBlog].Comments = &configComments{Enabled: true}

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           app.buildRouter(),
		ReadHeaderTimeout: time.Minute,
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	require.NoError(t, err)
	app.shutdown.Add(app.shutdownServer(server, "integration server"))
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		app.shutdown.ShutdownAndWait()
	})

	return app
}

type goToSocialInstance struct {
	baseURL       string
	containerName string
	port          int
	networkName   string
}

func startGoToSocialInstance(t *testing.T, goblogPort int, extraProxyHosts ...string) (*goToSocialInstance, *mastodon.Client) {
	t.Helper()

	// Create Docker network for container DNS resolution
	netName := fmt.Sprintf("goblog-net-%s", uuid.New().String())
	runDocker(t, "network", "create", netName)
	t.Cleanup(func() {
		_ = exec.Command("docker", "network", "rm", netName).Run()
	})

	// Start lightweight TCP forwarders.
	// Each forwards port 80 inside Docker to GoBlog's port on the host.
	for _, hostname := range append([]string{"goblog.example"}, extraProxyHosts...) {
		startDomainProxy(t, netName, hostname, goblogPort)
	}

	// Wait for proxy to be ready
	require.Eventually(t, func() bool {
		acct := "acct:default@goblog.example"
		cmd := exec.Command("docker", "run", "--rm", "--network", netName, "docker.io/alpine/curl", "-sS", "-m", "2", "-G", "--data-urlencode", fmt.Sprintf("resource=%s", acct), "http://goblog.example/.well-known/webfinger")
		out, err := cmd.CombinedOutput()
		return err == nil && strings.Contains(string(out), acct)
	}, time.Minute, time.Second)

	// Create config and data directories
	containerName := fmt.Sprintf("goblog-gts-%s", uuid.New().String())
	port := getFreePort(t)
	gtsDir := t.TempDir()
	gtsConfigPath := filepath.Join(gtsDir, "config.yaml")
	gtsConfig := fmt.Sprintf(`host: "127.0.0.1:%d"
protocol: "http"
bind-address: "0.0.0.0"
port: %d
db-type: "sqlite"
db-address: "/data/sqlite.db"
storage-local-base-path: "/data/storage"
http-client:
  insecure-outgoing: true
  allow-ips:
    - 0.0.0.0/0
trusted-proxies:
  - "0.0.0.0/0"
cache:
  memory-target: "50MiB"
`, port, port)
	require.NoError(t, os.WriteFile(gtsConfigPath, []byte(gtsConfig), 0o644))

	// Start GoToSocial Docker container on the test network
	runDocker(t,
		"run", "-d", "--rm",
		"--name", containerName,
		"--network", netName,
		"-p", fmt.Sprintf("%d:%d", port, port),
		"-v", fmt.Sprintf("%s:/config/config.yaml", gtsConfigPath),
		"--tmpfs", "/data",
		"--tmpfs", "/gotosocial/storage",
		"--tmpfs", "/gotosocial/.cache",
		"docker.io/superseriousbusiness/gotosocial:latest",
		"--config-path", "/config/config.yaml", "server", "start",
	)
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", containerName).Run()
	})
	gts := &goToSocialInstance{
		baseURL:       fmt.Sprintf("http://127.0.0.1:%d", port),
		containerName: containerName,
		port:          port,
		networkName:   netName,
	}

	// Wait for GoToSocial to be ready
	waitForHTTP(t, gts.baseURL+"/api/v1/instance", 2*time.Minute)

	// Create admin account
	runDocker(t,
		"exec", gts.containerName,
		"/gotosocial/gotosocial",
		"--config-path", "/config/config.yaml",
		"admin", "account", "create",
		"--username", gtsTestUsername,
		"--email", gtsTestEmail,
		"--password", gtsTestPassword,
	)
	// Create service actor account (will be converted to a bot via API)
	runDocker(t,
		"exec", gts.containerName,
		"/gotosocial/gotosocial",
		"--config-path", "/config/config.yaml",
		"admin", "account", "create",
		"--username", gtsServiceUsername,
		"--email", gtsServiceEmail,
		"--password", gtsServicePassword,
	)

	clientID, clientSecret := gtsRegisterApp(t, gts.baseURL)
	accessToken := gtsAuthorizeToken(t, gts.baseURL, clientID, clientSecret, gtsTestEmail, gtsTestPassword)
	mc := mastodon.NewClient(&mastodon.Config{Server: gts.baseURL, AccessToken: accessToken})
	mc.Client = http.Client{Timeout: time.Minute}

	return gts, mc
}

func waitForHTTP(t *testing.T, endpoint string, timeout time.Duration) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	require.Eventually(t, func() bool {
		req, err := requests.URL(endpoint).Method(http.MethodGet).Request(t.Context())
		if err != nil {
			return false
		}
		resp, err := client.Do(req)
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusInternalServerError
	}, timeout, 2*time.Second)
}

func gtsRegisterApp(t *testing.T, baseURL string) (string, string) {
	t.Helper()
	appCfg := &mastodon.AppConfig{
		Server:       baseURL,
		ClientName:   "goblog-activitypub-test",
		RedirectURIs: "urn:ietf:wg:oauth:2.0:oob",
		Scopes:       "read write follow",
		Website:      "https://goblog.app",
	}
	app, err := mastodon.RegisterApp(t.Context(), appCfg)
	require.NoError(t, err)
	require.NotEmpty(t, app.ClientID)
	require.NotEmpty(t, app.ClientSecret)
	return app.ClientID, app.ClientSecret
}

// gtsAuthorizeToken performs the OAuth2 authorization code flow to get an access token.
// This simulates a user logging in via web browser and authorizing the application.
func gtsAuthorizeToken(t *testing.T, baseURL, clientID, clientSecret, email, password string) string {
	t.Helper()

	// Create HTTP client with cookie jar to maintain session state
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{
		Timeout: time.Minute,
		Jar:     jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects automatically
		},
	}

	// Step 1: Initiate OAuth authorization flow
	query := url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {"urn:ietf:wg:oauth:2.0:oob"},
		"response_type": {"code"},
		"scope":         {"read write follow"},
	}
	var signInURL string
	err = requests.URL(baseURL + "/oauth/authorize").Params(query).Client(client).
		AddValidator(requests.CheckStatus(http.StatusSeeOther)).
		Handle(func(resp *http.Response) error {
			defer resp.Body.Close()
			signInURL = resp.Header.Get("Location")
			require.NotEmpty(t, signInURL)
			if strings.HasPrefix(signInURL, "/") {
				signInURL = baseURL + signInURL
			}
			return nil
		}).Fetch(t.Context())
	require.NoError(t, err)

	// Step 2: Submit login credentials
	signInValues := url.Values{
		"username": {email},
		"password": {password},
	}
	var authorizeURL string
	err = requests.URL(signInURL).Client(client).BodyForm(signInValues).
		AddValidator(requests.CheckStatus(http.StatusFound)).
		Handle(func(resp *http.Response) error {
			defer resp.Body.Close()
			authorizeURL = resp.Header.Get("Location")
			require.NotEmpty(t, authorizeURL)
			if strings.HasPrefix(authorizeURL, "/") {
				authorizeURL = baseURL + authorizeURL
			}
			return nil
		}).Fetch(t.Context())
	require.NoError(t, err)

	// Step 3: Get authorization page
	err = requests.URL(authorizeURL).Client(client).Fetch(t.Context())
	require.NoError(t, err)

	// Step 4: Approve authorization request
	var oobURL string
	err = requests.URL(authorizeURL).Client(client).BodyForm(url.Values{}).
		AddValidator(requests.CheckStatus(http.StatusFound)).
		Handle(func(resp *http.Response) error {
			defer resp.Body.Close()
			oobURL = resp.Header.Get("Location")
			require.NotEmpty(t, oobURL)
			if strings.HasPrefix(oobURL, "/") {
				oobURL = baseURL + oobURL
			}
			return nil
		}).Fetch(t.Context())
	require.NoError(t, err)

	// Step 5: Retrieve authorization code from out-of-band page
	var code string
	buf := bufferpool.Get()
	defer bufferpool.Put(buf)
	err = requests.URL(oobURL).Client(client).
		ToBytesBuffer(buf).
		Fetch(t.Context())
	code = extractCode(buf)
	require.NotEmpty(t, code)
	require.NoError(t, err)

	// Step 6: Exchange authorization code for access token
	tokenData := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"redirect_uri":  {"urn:ietf:wg:oauth:2.0:oob"},
		"grant_type":    {"authorization_code"},
		"code":          {code},
	}
	var tokenResult struct {
		AccessToken string `json:"access_token"`
	}
	err = requests.URL(baseURL + "/oauth/token").BodyForm(tokenData).ToJSON(&tokenResult).Fetch(t.Context())
	require.NoError(t, err)
	require.NotEmpty(t, tokenResult.AccessToken)

	return tokenResult.AccessToken
}

func extractCode(body io.Reader) string {
	doc, err := goquery.NewDocumentFromReader(body)
	if err != nil {
		return ""
	}
	code := strings.TrimSpace(doc.Find("code").First().Text())
	if code == "" {
		return ""
	}
	return code
}
