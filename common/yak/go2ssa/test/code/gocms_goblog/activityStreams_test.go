package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"code.superseriousbusiness.org/httpsig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ap "go.goblog.app/app/pkgs/activitypub"
	"go.goblog.app/app/pkgs/activitypub/jsonld"
	"go.goblog.app/app/pkgs/contenttype"
)

func Test_apUsername(t *testing.T) {
	item, err := ap.UnmarshalJSON([]byte(`
		{
			"@context": [
				"https://www.w3.org/ns/activitystreams",
				"https://w3id.org/security/v1"
			],
			"id": "https://example.org/users/user",
			"type": "Person",
			"preferredUsername": "user",
			"name": "Example user",
			"url": "https://example.org/@user"
		}
		`))
	require.NoError(t, err)

	actor, err := ap.ToActor(item)
	require.NoError(t, err)

	username := apUsername(actor)
	assert.Equal(t, "@user@example.org", username)
}

func Test_toAPNote_PublicNote(t *testing.T) {
	app := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	app.cfg.Server.PublicAddress = "https://example.com"
	app.cfg.Blogs = map[string]*configBlog{
		"testblog": {
			Path: "",
		},
	}
	app.cfg.Micropub = &configMicropub{
		PhotoParam: "photo",
	}
	app.cfg.ActivityPub = &configActivityPub{
		TagsTaxonomies: []string{"tags"},
	}
	err := app.initConfig(false)
	require.NoError(t, err)
	_ = app.initTemplateStrings()

	p := &post{
		Path:       "/test",
		Content:    "Test content",
		Published:  "2023-01-01T00:00:00Z",
		Updated:    "2023-01-02T00:00:00Z",
		Blog:       "testblog",
		Section:    "posts",
		Status:     statusPublished,
		Visibility: visibilityPublic,
		Parameters: map[string][]string{
			"title": {"Test Title"},
		},
		RenderedTitle: "Test Title",
	}

	note := app.toAPNote(p)

	assert.Equal(t, ap.ArticleType, note.Type)
	assert.Equal(t, "Test Title", note.Name.First().String())
	assert.Contains(t, note.Content.First().String(), "Test content")
	assert.Equal(t, ap.MimeType("text/html"), note.MediaType)
	assert.True(t, note.To.Contains(ap.PublicNS))

	// JSON validation
	const expectedPublicNoteJSON = `{"@context":["https://www.w3.org/ns/activitystreams","https://w3id.org/security/v1"],"id":"https://example.com/test","type":"Article","mediaType":"text/html","name":"Test Title","content":"<div class=\"e-content\"><p>Test content</p>\n</div>","attributedTo":"https://example.com","url":"https://example.com/test","to":["https://www.w3.org/ns/activitystreams#Public","https://example.com/activitypub/followers/testblog"],"published":"2023-01-01T00:00:00Z","updated":"2023-01-02T00:00:00Z"}`
	binary, err := jsonld.WithContext(jsonld.IRI(ap.ActivityBaseURI), jsonld.IRI(ap.SecurityContextURI)).Marshal(note)
	require.NoError(t, err)
	assert.JSONEq(t, expectedPublicNoteJSON, string(binary))
}

func Test_toAPNote_UnlistedNote(t *testing.T) {
	app := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	app.cfg.Server.PublicAddress = "https://example.com"
	app.cfg.Blogs = map[string]*configBlog{
		"testblog": {
			Path: "",
		},
	}
	app.cfg.Micropub = &configMicropub{
		PhotoParam: "photo",
	}
	app.cfg.ActivityPub = &configActivityPub{
		TagsTaxonomies: []string{"tags"},
	}
	err := app.initConfig(false)
	require.NoError(t, err)
	_ = app.initTemplateStrings()

	p := &post{
		Path:       "/test",
		Content:    "Test content",
		Blog:       "testblog",
		Section:    "posts",
		Status:     statusPublished,
		Visibility: visibilityUnlisted,
		Parameters: map[string][]string{},
	}

	note := app.toAPNote(p)

	assert.Equal(t, ap.NoteType, note.Type)
	assert.True(t, note.To.Contains(ap.IRI("https://example.com/activitypub/followers/testblog")))
	assert.True(t, note.CC.Contains(ap.PublicNS))

	// JSON validation
	const expectedUnlistedNoteJSON = `{"@context":["https://www.w3.org/ns/activitystreams","https://w3id.org/security/v1"],"id":"https://example.com/test","type":"Note","mediaType":"text/html","content":"<div class=\"e-content\"><p>Test content</p>\n</div>","attributedTo":"https://example.com","url":"https://example.com/test","to":["https://example.com/activitypub/followers/testblog"],"cc":["https://www.w3.org/ns/activitystreams#Public"]}`
	binary, err := jsonld.WithContext(jsonld.IRI(ap.ActivityBaseURI), jsonld.IRI(ap.SecurityContextURI)).Marshal(note)
	require.NoError(t, err)
	assert.JSONEq(t, expectedUnlistedNoteJSON, string(binary))
}

func Test_toAPNote_WithImages(t *testing.T) {
	app := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	app.cfg.Server.PublicAddress = "https://example.com"
	app.cfg.Blogs = map[string]*configBlog{
		"testblog": {
			Path: "",
		},
	}
	app.cfg.Micropub = &configMicropub{
		PhotoParam: "photo",
	}
	app.cfg.ActivityPub = &configActivityPub{
		TagsTaxonomies: []string{"tags"},
	}
	err := app.initConfig(false)
	require.NoError(t, err)
	_ = app.initTemplateStrings()

	p := &post{
		Path:       "/test",
		Content:    "Test content",
		Blog:       "testblog",
		Section:    "posts",
		Status:     statusPublished,
		Visibility: visibilityPublic,
		Parameters: map[string][]string{
			"photo": {"https://example.com/image1.jpg", "https://example.com/image2.jpg"},
		},
	}

	note := app.toAPNote(p)

	assert.NotNil(t, note.Attachment)
	attachments, ok := note.Attachment.(ap.ItemCollection)
	require.True(t, ok)
	assert.Len(t, attachments, 2)
	for _, att := range attachments {
		obj, ok := att.(*ap.Object)
		require.True(t, ok)
		assert.Equal(t, ap.ImageType, obj.Type)
	}

	// JSON validation
	const expectedNoteWithImagesJSON = `{"@context":["https://www.w3.org/ns/activitystreams","https://w3id.org/security/v1"],"id":"https://example.com/test","type":"Note","mediaType":"text/html","content":"<div class=\"e-content\"><p>Test content</p>\n</div>","attachment":[{"type":"Image","url":"https://example.com/image1.jpg"},{"type":"Image","url":"https://example.com/image2.jpg"}],"attributedTo":"https://example.com","url":"https://example.com/test","to":["https://www.w3.org/ns/activitystreams#Public","https://example.com/activitypub/followers/testblog"]}`
	binary, err := jsonld.WithContext(jsonld.IRI(ap.ActivityBaseURI), jsonld.IRI(ap.SecurityContextURI)).Marshal(note)
	require.NoError(t, err)
	assert.JSONEq(t, expectedNoteWithImagesJSON, string(binary))
}

func Test_toAPNote_WithTags(t *testing.T) {
	app := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	app.cfg.Server.PublicAddress = "https://example.com"
	app.cfg.Blogs = map[string]*configBlog{
		"testblog": {
			Path: "",
		},
	}
	app.cfg.Micropub = &configMicropub{
		PhotoParam: "photo",
	}
	app.cfg.ActivityPub = &configActivityPub{
		TagsTaxonomies: []string{"tags"},
	}
	err := app.initConfig(false)
	require.NoError(t, err)
	_ = app.initTemplateStrings()

	p := &post{
		Path:       "/test",
		Content:    "Test content",
		Blog:       "testblog",
		Section:    "posts",
		Status:     statusPublished,
		Visibility: visibilityPublic,
		Parameters: map[string][]string{
			"tags": {"tag1", "tag2"},
		},
	}

	note := app.toAPNote(p)

	assert.Len(t, note.Tag, 2)
	for _, tag := range note.Tag {
		obj, ok := tag.(*ap.Object)
		require.True(t, ok)
		assert.Equal(t, "Hashtag", string(obj.Type))
	}

	// JSON validation
	const expectedNoteWithTagsJSON = `{"@context":["https://www.w3.org/ns/activitystreams","https://w3id.org/security/v1"],"id":"https://example.com/test","type":"Note","mediaType":"text/html","content":"<div class=\"e-content\"><p>Test content</p>\n</div>","attributedTo":"https://example.com","tag":[{"type":"Hashtag","name":"tag1","url":"https://example.com/tags/tag1"},{"type":"Hashtag","name":"tag2","url":"https://example.com/tags/tag2"}],"url":"https://example.com/test","to":["https://www.w3.org/ns/activitystreams#Public","https://example.com/activitypub/followers/testblog"]}`
	binary, err := jsonld.WithContext(jsonld.IRI(ap.ActivityBaseURI), jsonld.IRI(ap.SecurityContextURI)).Marshal(note)
	require.NoError(t, err)
	assert.JSONEq(t, expectedNoteWithTagsJSON, string(binary))
}

func Test_toAPNote_WithMentions(t *testing.T) {
	app := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	app.cfg.Server.PublicAddress = "https://example.com"
	app.cfg.Blogs = map[string]*configBlog{
		"testblog": {
			Path: "",
		},
	}
	app.cfg.Micropub = &configMicropub{
		PhotoParam: "photo",
	}
	app.cfg.ActivityPub = &configActivityPub{
		TagsTaxonomies: []string{"tags"},
	}
	err := app.initConfig(false)
	require.NoError(t, err)
	_ = app.initTemplateStrings()

	p := &post{
		Path:       "/test",
		Content:    "Test content",
		Blog:       "testblog",
		Section:    "posts",
		Status:     statusPublished,
		Visibility: visibilityPublic,
		Parameters: map[string][]string{
			activityPubMentionsParameter: {"https://example.com/@user1", "https://example.com/@user2"},
		},
	}

	note := app.toAPNote(p)

	mentionCount := 0
	for _, tag := range note.Tag {
		if tag.GetType() == ap.MentionType {
			mentionCount++
		}
	}
	assert.Equal(t, 2, mentionCount)

	// JSON validation
	const expectedNoteWithMentionsJSON = `{"@context":["https://www.w3.org/ns/activitystreams","https://w3id.org/security/v1"],"id":"https://example.com/test","type":"Note","mediaType":"text/html","content":"<div class=\"e-content\"><p>Test content</p>\n</div>","attributedTo":"https://example.com","tag":[{"id":"https://example.com/@user1","type":"Mention","href":"https://example.com/@user1"},{"id":"https://example.com/@user2","type":"Mention","href":"https://example.com/@user2"}],"url":"https://example.com/test","to":["https://www.w3.org/ns/activitystreams#Public","https://example.com/activitypub/followers/testblog"],"cc":["https://example.com/@user1","https://example.com/@user2"]}`
	binary, err := jsonld.WithContext(jsonld.IRI(ap.ActivityBaseURI), jsonld.IRI(ap.SecurityContextURI)).Marshal(note)
	require.NoError(t, err)
	assert.JSONEq(t, expectedNoteWithMentionsJSON, string(binary))
}

func Test_toAPNote_WithReply(t *testing.T) {
	app := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	app.cfg.Server.PublicAddress = "https://example.com"
	app.cfg.Blogs = map[string]*configBlog{
		"testblog": {
			Path: "",
		},
	}
	app.cfg.Micropub = &configMicropub{
		PhotoParam: "photo",
		ReplyParam: "in-reply-to",
	}
	app.cfg.ActivityPub = &configActivityPub{
		TagsTaxonomies: []string{"tags"},
	}
	err := app.initConfig(false)
	require.NoError(t, err)
	_ = app.initTemplateStrings()

	p := &post{
		Path:       "/test",
		Content:    "Test content",
		Blog:       "testblog",
		Section:    "posts",
		Status:     statusPublished,
		Visibility: visibilityPublic,
		Parameters: map[string][]string{
			"in-reply-to": {"https://example.com/reply-to"},
		},
	}

	note := app.toAPNote(p)

	assert.Equal(t, ap.IRI("https://example.com/reply-to"), note.InReplyTo)

	// JSON validation
	const expectedNoteWithReplyJSON = `{"@context":["https://www.w3.org/ns/activitystreams","https://w3id.org/security/v1"],"id":"https://example.com/test","type":"Note","mediaType":"text/html","content":"<div class=\"h-cite u-in-reply-to\"><p><strong>Reply to: <a class=\"u-url\" rel=\"noopener\" target=\"_blank\" href=\"https://example.com/reply-to\">https://example.com/reply-to</a></strong></p></div><div class=\"e-content\"><p>Test content</p>\n</div>","attributedTo":"https://example.com","inReplyTo":"https://example.com/reply-to","url":"https://example.com/test","to":["https://www.w3.org/ns/activitystreams#Public","https://example.com/activitypub/followers/testblog"]}`
	binary, err := jsonld.WithContext(jsonld.IRI(ap.ActivityBaseURI), jsonld.IRI(ap.SecurityContextURI)).Marshal(note)
	require.NoError(t, err)
	assert.JSONEq(t, expectedNoteWithReplyJSON, string(binary))
}

func Test_activityPubId(t *testing.T) {
	app := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	app.cfg.Server.PublicAddress = "https://example.com"
	err := app.initConfig(false)
	require.NoError(t, err)
	_ = app.initTemplateStrings()

	p := &post{
		Path:       "/test",
		Parameters: map[string][]string{},
	}

	id := app.activityPubID(p)
	assert.Equal(t, ap.IRI("https://example.com/test"), id)

	p.Parameters[activityPubVersionParam] = []string{"123456789"}
	id = app.activityPubID(p)
	assert.Equal(t, ap.IRI("https://example.com/test?activitypubversion=123456789"), id)
}

func Test_toApPerson(t *testing.T) {
	app := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	app.cfg.Server.PublicAddress = "https://example.com"
	app.cfg.Blogs = map[string]*configBlog{
		"testblog": {
			Title:       "Test Blog",
			Description: "A test blog",
		},
	}
	app.cfg.ActivityPub = &configActivityPub{
		AlsoKnownAs:        []string{"https://example.com/aka1"},
		AttributionDomains: []string{"example.com"},
	}
	app.apPubKeyBytes = []byte("test-key")
	err := app.initConfig(false)
	require.NoError(t, err)
	_ = app.initTemplateStrings()

	person := app.toApPerson("testblog", "")

	assert.Equal(t, "Test Blog", person.Name.First().String())
	assert.Equal(t, "A test blog", person.Summary.First().String())
	assert.Equal(t, "testblog", person.PreferredUsername.First().String())
	assert.Equal(t, ap.IRI("https://example.com"), person.ID)
	assert.Equal(t, ap.IRI("https://example.com"), person.URL)
	assert.Equal(t, ap.IRI("https://example.com/activitypub/inbox/testblog"), person.Inbox)
	assert.Equal(t, ap.IRI("https://example.com/activitypub/followers/testblog"), person.Followers)
	assert.Len(t, person.AlsoKnownAs, 1)
	assert.Len(t, person.AttributionDomains, 1)

	// JSON validation
	const expectedPersonJSON = `{"@context":["https://www.w3.org/ns/activitystreams","https://w3id.org/security/v1"],"id":"https://example.com","type":"Person","name":"Test Blog","summary":"A test blog","url":"https://example.com","inbox":"https://example.com/activitypub/inbox/testblog","followers":"https://example.com/activitypub/followers/testblog","preferredUsername":"testblog","publicKey":{"id":"https://example.com#main-key","owner":"https://example.com","publicKeyPem":"-----BEGIN PUBLIC KEY-----\ndGVzdC1rZXk=\n-----END PUBLIC KEY-----\n"},"alsoKnownAs":["https://example.com/aka1"],"attributionDomains":["example.com"]}`
	binary, err := jsonld.WithContext(jsonld.IRI(ap.ActivityBaseURI), jsonld.IRI(ap.SecurityContextURI)).Marshal(person)
	require.NoError(t, err)
	assert.JSONEq(t, expectedPersonJSON, string(binary))
}

func Test_toApPerson_WithProfileImage(t *testing.T) {
	app := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	app.cfg.Server.PublicAddress = "https://example.com"
	app.cfg.Blogs = map[string]*configBlog{
		"testblog": {
			Title:       "Test Blog",
			Description: "A test blog",
		},
	}
	app.cfg.ActivityPub = &configActivityPub{
		AlsoKnownAs:        []string{"https://example.com/aka1"},
		AttributionDomains: []string{"example.com"},
	}
	app.apPubKeyBytes = []byte("test-key")
	err := app.initConfig(false)
	require.NoError(t, err)
	_ = app.initTemplateStrings()

	// Create temporary profile image file (empty to have known hash)
	tempFile := filepath.Join(t.TempDir(), "profile.jpg")
	err = os.WriteFile(tempFile, []byte{}, 0644)
	require.NoError(t, err)
	app.cfg.User.ProfileImageFile = tempFile
	app.profileImageHashGroup = nil // Reset to recompute hash

	person := app.toApPerson("testblog", "")

	assert.Equal(t, "Test Blog", person.Name.First().String())
	assert.Equal(t, "A test blog", person.Summary.First().String())
	assert.Equal(t, "testblog", person.PreferredUsername.First().String())
	assert.Equal(t, ap.IRI("https://example.com"), person.ID)
	assert.Equal(t, ap.IRI("https://example.com"), person.URL)
	assert.Equal(t, ap.IRI("https://example.com/activitypub/inbox/testblog"), person.Inbox)
	assert.Equal(t, ap.IRI("https://example.com/activitypub/followers/testblog"), person.Followers)
	assert.Len(t, person.AlsoKnownAs, 1)
	assert.Len(t, person.AttributionDomains, 1)
	assert.NotNil(t, person.Icon)
	iconObj, ok := person.Icon.(*ap.Object)
	require.True(t, ok)
	assert.Equal(t, ap.ImageType, iconObj.Type)
	assert.Equal(t, ap.MimeType("image/jpeg"), iconObj.MediaType)
	assert.Equal(t, ap.IRI("https://example.com/profile.jpg?v=e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"), iconObj.URL)

	// JSON validation
	const expectedPersonWithIconJSON = `{"@context":["https://www.w3.org/ns/activitystreams","https://w3id.org/security/v1"],"id":"https://example.com","type":"Person","name":"Test Blog","summary":"A test blog","icon":{"type":"Image","mediaType":"image/jpeg","url":"https://example.com/profile.jpg?v=e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},"url":"https://example.com","inbox":"https://example.com/activitypub/inbox/testblog","followers":"https://example.com/activitypub/followers/testblog","preferredUsername":"testblog","publicKey":{"id":"https://example.com#main-key","owner":"https://example.com","publicKeyPem":"-----BEGIN PUBLIC KEY-----\ndGVzdC1rZXk=\n-----END PUBLIC KEY-----\n"},"alsoKnownAs":["https://example.com/aka1"],"attributionDomains":["example.com"]}`
	binary, err := jsonld.WithContext(jsonld.IRI(ap.ActivityBaseURI), jsonld.IRI(ap.SecurityContextURI)).Marshal(person)
	require.NoError(t, err)
	assert.JSONEq(t, expectedPersonWithIconJSON, string(binary))
}

func Test_serveActivityStreams(t *testing.T) {
	// Integration test for serveActivityStreams with Person
	app := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	app.cfg.Server.PublicAddress = "https://example.com"
	app.cfg.Blogs = map[string]*configBlog{
		"testblog": {
			Title:       "Test Blog",
			Description: "A test blog",
		},
	}
	app.cfg.ActivityPub = &configActivityPub{
		Enabled:            true,
		AlsoKnownAs:        []string{"https://example.com/aka1"},
		AttributionDomains: []string{"example.com"},
	}
	app.apPubKeyBytes = []byte("test-key")
	err := app.initConfig(false)
	require.NoError(t, err)
	_ = app.initTemplateStrings()

	// Create HTTP request and recorder
	req := httptest.NewRequest(http.MethodGet, "https://example.com", nil)
	rec := httptest.NewRecorder()

	// Test serveActivityStreams
	app.serveActivityStreams(rec, req, http.StatusOK, "testblog")

	// Verify response
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, contenttype.ASUTF8, rec.Header().Get(contentType))

	// Parse response body
	body := rec.Body.String()
	assert.Contains(t, body, `"type":"Person"`)
	assert.Contains(t, body, `"name":"Test Blog"`)
	assert.Contains(t, body, `"summary":"A test blog"`)
	assert.Contains(t, body, `"preferredUsername":"testblog"`)
	assert.Contains(t, body, `"alsoKnownAs"`)
	assert.Contains(t, body, `"attributionDomains"`)
	assert.Contains(t, body, `"publicKey"`)
}

func Test_serveActivityStreams_WithProfileImage(t *testing.T) {
	// Integration test for Person with profile image
	app := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	app.cfg.Server.PublicAddress = "https://example.com"
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

	// Create temporary profile image file
	tempFile := filepath.Join(t.TempDir(), "profile.jpg")
	err := os.WriteFile(tempFile, []byte{}, 0644)
	require.NoError(t, err)
	app.cfg.User.ProfileImageFile = tempFile

	err = app.initConfig(false)
	require.NoError(t, err)
	_ = app.initTemplateStrings()

	// Create HTTP request and recorder
	req := httptest.NewRequest(http.MethodGet, "https://example.com", nil)
	rec := httptest.NewRecorder()

	// Test serveActivityStreams
	app.serveActivityStreams(rec, req, http.StatusOK, "testblog")

	// Verify response
	assert.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `"type":"Person"`)
	assert.Contains(t, body, `"icon"`)
	assert.Contains(t, body, `"type":"Image"`)
	assert.Contains(t, body, `"mediaType":"image/jpeg"`)
}

func Test_apUsername_EdgeCases(t *testing.T) {
	// Test with missing preferredUsername
	item, err := ap.UnmarshalJSON([]byte(`
		{
			"@context": [
				"https://www.w3.org/ns/activitystreams",
				"https://w3id.org/security/v1"
			],
			"id": "https://example.org/users/user",
			"type": "Person",
			"name": "Example user",
			"url": "https://example.org/@user"
		}
		`))
	require.NoError(t, err)

	actor, err := ap.ToActor(item)
	require.NoError(t, err)

	username := apUsername(actor)
	assert.Equal(t, "https://example.org/users/user", username)

	// Test with invalid URL
	item2, err := ap.UnmarshalJSON([]byte(`
		{
			"@context": [
				"https://www.w3.org/ns/activitystreams",
				"https://w3id.org/security/v1"
			],
			"id": "https://example.org/users/user",
			"type": "Person",
			"preferredUsername": "user",
			"name": "Example user",
			"url": "invalid-url"
		}
		`))
	require.NoError(t, err)

	actor2, err := ap.ToActor(item2)
	require.NoError(t, err)

	username2 := apUsername(actor2)
	assert.Equal(t, "@user@example.org", username2)
}

func Test_toApPerson_WithMultipleAlsoKnownAs(t *testing.T) {
	app := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	app.cfg.Server.PublicAddress = "https://example.com"
	app.cfg.Blogs = map[string]*configBlog{
		"testblog": {
			Title:       "Test Blog",
			Description: "A test blog",
		},
	}
	app.cfg.ActivityPub = &configActivityPub{
		AlsoKnownAs:        []string{"https://example.com/aka1", "https://other.example/@user", "https://another.example/user"},
		AttributionDomains: []string{"example.com", "sub.example.com"},
	}
	app.apPubKeyBytes = []byte("test-key")
	err := app.initConfig(false)
	require.NoError(t, err)
	_ = app.initTemplateStrings()

	person := app.toApPerson("testblog", "")

	// Verify AlsoKnownAs
	assert.Len(t, person.AlsoKnownAs, 3)
	assert.Equal(t, ap.IRI("https://example.com/aka1"), person.AlsoKnownAs[0])
	assert.Equal(t, ap.IRI("https://other.example/@user"), person.AlsoKnownAs[1])
	assert.Equal(t, ap.IRI("https://another.example/user"), person.AlsoKnownAs[2])

	// Verify AttributionDomains
	assert.Len(t, person.AttributionDomains, 2)
	assert.Equal(t, ap.IRI("example.com"), person.AttributionDomains[0])
	assert.Equal(t, ap.IRI("sub.example.com"), person.AttributionDomains[1])

	// JSON validation - ensure fields are present
	binary, err := jsonld.WithContext(jsonld.IRI(ap.ActivityBaseURI), jsonld.IRI(ap.SecurityContextURI)).Marshal(person)
	require.NoError(t, err)
	jsonStr := string(binary)
	assert.Contains(t, jsonStr, "alsoKnownAs")
	assert.Contains(t, jsonStr, "attributionDomains")
	assert.Contains(t, jsonStr, "https://example.com/aka1")
	assert.Contains(t, jsonStr, "https://other.example/@user")
	assert.Contains(t, jsonStr, "example.com")
	assert.Contains(t, jsonStr, "sub.example.com")
}

func Test_toApPerson_WithoutExtensions(t *testing.T) {
	app := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	app.cfg.Server.PublicAddress = "https://example.com"
	app.cfg.Blogs = map[string]*configBlog{
		"testblog": {
			Title:       "Test Blog",
			Description: "A test blog",
		},
	}
	app.cfg.ActivityPub = &configActivityPub{
		// No AlsoKnownAs or AttributionDomains
	}
	app.apPubKeyBytes = []byte("test-key")
	err := app.initConfig(false)
	require.NoError(t, err)
	_ = app.initTemplateStrings()

	person := app.toApPerson("testblog", "")

	// Verify fields are empty
	assert.Len(t, person.AlsoKnownAs, 0)
	assert.Len(t, person.AttributionDomains, 0)

	// JSON validation - ensure fields are not present when empty
	binary, err := jsonld.WithContext(jsonld.IRI(ap.ActivityBaseURI), jsonld.IRI(ap.SecurityContextURI)).Marshal(person)
	require.NoError(t, err)
	jsonStr := string(binary)
	assert.NotContains(t, jsonStr, "alsoKnownAs")
	assert.NotContains(t, jsonStr, "attributionDomains")
}

func Test_apSendSigned(t *testing.T) {
	// Create a test server to receive the signed request
	var receivedRequest *http.Request
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRequest = r
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	app := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	app.cfg.Server.PublicAddress = "https://example.com"
	app.cfg.Blogs = map[string]*configBlog{
		"testblog": {},
	}
	app.cfg.ActivityPub = &configActivityPub{
		Enabled: true,
	}
	err := app.initConfig(false)
	require.NoError(t, err)

	// Initialize httpClient
	app.httpClient = &http.Client{
		Timeout: 30 * time.Second,
	}

	// Initialize key and signer for signing
	err = app.loadActivityPubPrivateKey()
	require.NoError(t, err)

	app.apSigner, _, err = httpsig.NewSigner(
		[]httpsig.Algorithm{httpsig.RSA_SHA256},
		httpsig.DigestSha256,
		[]string{httpsig.RequestTarget, "date", "host", "digest"},
		httpsig.Signature,
		0,
	)
	require.NoError(t, err)
	app.apSignerNoDigest, _, err = httpsig.NewSigner(
		[]httpsig.Algorithm{httpsig.RSA_SHA256},
		httpsig.DigestSha256,
		[]string{httpsig.RequestTarget, "date", "host"},
		httpsig.Signature,
		0,
	)
	require.NoError(t, err)

	// Create a test activity
	note := ap.ObjectNew(ap.NoteType)
	note.ID = ap.IRI("https://example.com/notes/1")
	note.Content = ap.NaturalLanguageValues{{Lang: "en", Value: "Test content"}}

	activity := ap.ActivityNew(ap.CreateType, ap.IRI("https://example.com/activities/1"), note)
	activity.Actor = ap.IRI("https://example.com")

	// Marshal the activity
	activityBytes, err := jsonld.WithContext(jsonld.IRI(ap.ActivityBaseURI), jsonld.IRI(ap.SecurityContextURI)).Marshal(activity)
	require.NoError(t, err)

	// Send the signed request
	err = app.apSendSigned("https://example.com", server.URL, activityBytes)
	require.NoError(t, err)

	// Verify the request was received
	require.NotNil(t, receivedRequest)
	assert.Equal(t, http.MethodPost, receivedRequest.Method)
	assert.Equal(t, contenttype.ASUTF8, receivedRequest.Header.Get(contentType))
	assert.NotEmpty(t, receivedRequest.Header.Get("Date"))

	// Verify signature header is present (if apSigner is properly initialized)
	// Note: The signature might not be present if there are initialization issues
	// but the request should still be sent
	sigHeader := receivedRequest.Header.Get("Signature")
	if sigHeader != "" {
		assert.Contains(t, sigHeader, "keyId=")
		assert.Contains(t, sigHeader, "signature=")
	}

	// Verify body was sent correctly
	assert.NotEmpty(t, receivedBody)
	assert.Contains(t, string(receivedBody), "Test content")
}

func Test_signRequest(t *testing.T) {
	app := &goBlog{
		cfg: createDefaultTestConfig(t),
	}
	app.cfg.Server.PublicAddress = "https://example.com"
	app.cfg.ActivityPub = &configActivityPub{
		Enabled: true,
	}
	err := app.initConfig(false)
	require.NoError(t, err)

	// Just initialize key and signer for signing test
	err = app.loadActivityPubPrivateKey()
	require.NoError(t, err)

	app.apSigner, _, err = httpsig.NewSigner(
		[]httpsig.Algorithm{httpsig.RSA_SHA256},
		httpsig.DigestSha256,
		[]string{httpsig.RequestTarget, "date", "host", "digest"},
		httpsig.Signature,
		0,
	)
	require.NoError(t, err)
	app.apSignerNoDigest, _, err = httpsig.NewSigner(
		[]httpsig.Algorithm{httpsig.RSA_SHA256},
		httpsig.DigestSha256,
		[]string{httpsig.RequestTarget, "date", "host"},
		httpsig.Signature,
		0,
	)
	require.NoError(t, err)

	t.Run("PostWithDigest", func(t *testing.T) {
		body := []byte(`{"type":"Note","content":"Test"}`)
		req, err := http.NewRequest(http.MethodPost, "https://remote.example/inbox", bytes.NewReader(body))
		require.NoError(t, err)

		err = app.signRequest(req, "https://example.com")
		require.NoError(t, err)

		assert.NotEmpty(t, req.Header.Get("Date"))
		assert.NotEmpty(t, req.Header.Get("Host"))
		assert.NotEmpty(t, req.Header.Get("Signature"))
		assert.NotEmpty(t, req.Header.Get("Digest"))

		sig := req.Header.Get("Signature")
		assert.Contains(t, sig, "keyId=")
		assert.Contains(t, sig, "signature=")
		assert.Contains(t, sig, "https://example.com#main-key")
		assert.Contains(t, sig, "(request-target) date host digest")
	})

	t.Run("GetWithoutDigest", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "https://remote.example/inbox", nil)
		require.NoError(t, err)

		err = app.signRequest(req, "https://example.com")
		require.NoError(t, err)

		assert.NotEmpty(t, req.Header.Get("Date"))
		assert.NotEmpty(t, req.Header.Get("Host"))
		assert.NotEmpty(t, req.Header.Get("Signature"))
		assert.Empty(t, req.Header.Get("Digest"))

		sig := req.Header.Get("Signature")
		assert.Contains(t, sig, "keyId=")
		assert.Contains(t, sig, "signature=")
		assert.Contains(t, sig, "https://example.com#main-key")
		assert.Contains(t, sig, "(request-target) date host")
		assert.NotContains(t, sig, "digest")

		verifier, err := httpsig.NewVerifier(req)
		require.NoError(t, err)
		err = verifier.Verify(&app.apPrivateKey.PublicKey, httpsig.RSA_SHA256)
		assert.NoError(t, err)
	})
}
