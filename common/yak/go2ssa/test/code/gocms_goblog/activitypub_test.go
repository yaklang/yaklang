package activitypub

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.goblog.app/app/pkgs/activitypub/jsonld"
)

func TestIRI(t *testing.T) {
	iri := IRI("https://example.com/users/alice")

	assert.Equal(t, "https://example.com/users/alice", iri.String())
	assert.True(t, iri.IsLink())
	assert.False(t, iri.IsObject())
	assert.Equal(t, iri, iri.GetLink())

	url, err := iri.URL()
	require.NoError(t, err)
	assert.Equal(t, "https", url.Scheme)
	assert.Equal(t, "example.com", url.Host)
}

func TestNaturalLanguageValues(t *testing.T) {
	// Test single value
	nlv := NaturalLanguageValues{{Lang: "en", Value: "Hello"}}
	assert.Equal(t, "Hello", nlv.First().String())

	// Test JSON marshaling - single value
	single := NaturalLanguageValues{{Lang: "en", Value: "Test"}}
	data, err := json.Marshal(single)
	require.NoError(t, err)
	assert.Equal(t, `"Test"`, string(data))

	// Test JSON unmarshaling - string
	var unmarshaled NaturalLanguageValues
	err = json.Unmarshal([]byte(`"Test"`), &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, "Test", unmarshaled.First().String())

	// Test JSON unmarshaling - map
	err = json.Unmarshal([]byte(`{"en":"Hello","fr":"Bonjour"}`), &unmarshaled)
	require.NoError(t, err)
	assert.Len(t, unmarshaled, 2)
}

func TestObjectNew(t *testing.T) {
	obj := ObjectNew(NoteType)
	assert.NotNil(t, obj)
	assert.Equal(t, NoteType, obj.Type)
	assert.False(t, obj.IsLink())
	assert.True(t, obj.IsObject())
}

func TestPersonNew(t *testing.T) {
	person := PersonNew(IRI("https://example.com/users/alice"))
	assert.NotNil(t, person)
	assert.Equal(t, PersonType, person.Type)
	assert.Equal(t, IRI("https://example.com/users/alice"), person.ID)
}

func TestItemCollection(t *testing.T) {
	col := ItemCollection{}

	// Test Append
	col.Append(IRI("https://example.com/1"))
	col.Append(IRI("https://example.com/2"))
	assert.Len(t, col, 2)

	// Test Contains
	assert.True(t, col.Contains(IRI("https://example.com/1")))
	assert.False(t, col.Contains(IRI("https://example.com/3")))

	// Test JSON marshaling - single item (now always returns array)
	single := ItemCollection{IRI("https://example.com/1")}
	data, err := json.Marshal(single)
	require.NoError(t, err)
	assert.Equal(t, `["https://example.com/1"]`, string(data))

	// Test JSON marshaling - multiple items
	data, err = json.Marshal(col)
	require.NoError(t, err)
	assert.Contains(t, string(data), "https://example.com/1")
	assert.Contains(t, string(data), "https://example.com/2")

	// Test JSON unmarshaling - single item
	var unmarshaled ItemCollection
	err = json.Unmarshal([]byte(`"https://example.com/1"`), &unmarshaled)
	require.NoError(t, err)
	assert.Len(t, unmarshaled, 1)
	assert.Equal(t, IRI("https://example.com/1"), unmarshaled[0].GetLink())

	// Test JSON unmarshaling - array
	err = json.Unmarshal([]byte(`["https://example.com/1","https://example.com/2"]`), &unmarshaled)
	require.NoError(t, err)
	assert.Len(t, unmarshaled, 2)
}

func TestUnmarshalJSON(t *testing.T) {
	t.Parallel()

	t.Run("Person", func(t *testing.T) {
		t.Parallel()

		personJSON := `{
			"@context": "https://www.w3.org/ns/activitystreams",
			"type": "Person",
			"id": "https://example.com/users/alice",
			"name": "Alice",
			"preferredUsername": "alice"
		}`

		item, err := UnmarshalJSON([]byte(personJSON))
		require.NoError(t, err)

		person, err := ToActor(item)
		require.NoError(t, err)
		assert.Equal(t, PersonType, person.Type)
		assert.Equal(t, "Alice", person.Name.First().String())
	})

	t.Run("Activity", func(t *testing.T) {
		t.Parallel()

		activityJSON := `{
			"type": "Create",
			"id": "https://example.com/activities/1",
			"actor": "https://example.com/users/alice",
			"object": {
				"type": "Note",
				"content": "Hello"
			}
		}`

		item, err := UnmarshalJSON([]byte(activityJSON))
		require.NoError(t, err)

		activity, err := ToActivity(item)
		require.NoError(t, err)
		assert.Equal(t, CreateType, activity.Type)
		assert.NotNil(t, activity.Actor)
		assert.NotNil(t, activity.Object)
	})

	t.Run("Undo Follow", func(t *testing.T) {
		t.Parallel()

		undoJSON := `{
			"type": "Undo",
			"id": "https://example.com/activities/2",
			"actor": "https://example.com/users/alice",
			"object": {
				"type": "Follow",
				"actor": "https://example.com/users/alice",
				"object": "https://example.org/users/bob"
			}
		}`

		item, err := UnmarshalJSON([]byte(undoJSON))
		require.NoError(t, err)

		activity, err := ToActivity(item)
		require.NoError(t, err)
		assert.Equal(t, UndoType, activity.Type)
		assert.NotNil(t, activity.Actor)
		assert.NotNil(t, activity.Object)

		objectActivity, err := ToActivity(activity.Object)
		require.NoError(t, err)
		assert.Equal(t, FollowType, objectActivity.Type)
	})

}

func TestUnmarshalJSONServiceActor(t *testing.T) {
	serviceJSON := `{
		"@context": "https://www.w3.org/ns/activitystreams",
		"type": "Service",
		"id": "https://example.com/services/bot",
		"name": "Bot",
		"preferredUsername": "bot"
	}`

	item, err := UnmarshalJSON([]byte(serviceJSON))
	require.NoError(t, err)

	actor, err := ToActor(item)
	require.NoError(t, err)
	assert.Equal(t, ServiceType, actor.Type)
	assert.Equal(t, "Bot", actor.Name.First().String())
}

func TestIsActorType(t *testing.T) {
	assert.True(t, IsActorType(PersonType))
	assert.True(t, IsActorType(ServiceType))
	assert.True(t, IsActorType(GroupType))
	assert.True(t, IsActorType(OrganizationType))
	assert.True(t, IsActorType(ApplicationType))
	assert.False(t, IsActorType(NoteType))
}

func TestToObject(t *testing.T) {
	// Test with Object
	obj := ObjectNew(NoteType)
	obj.ID = IRI("https://example.com/notes/1")

	converted, err := ToObject(obj)
	require.NoError(t, err)
	assert.Equal(t, obj.ID, converted.ID)

	// Test with Person
	person := PersonNew(IRI("https://example.com/users/alice"))
	converted, err = ToObject(person)
	require.NoError(t, err)
	assert.Equal(t, PersonType, converted.Type)

	// Test with IRI (should fail)
	iri := IRI("https://example.com/test")
	_, err = ToObject(iri)
	assert.Error(t, err)
}

func TestNoteMarshaling(t *testing.T) {
	note := ObjectNew(NoteType)
	note.ID = IRI("https://example.com/notes/1")
	note.Content = NaturalLanguageValues{{Lang: "en", Value: "Hello, world!"}}
	note.AttributedTo = IRI("https://example.com/users/alice")
	note.Published = time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	data, err := json.Marshal(note)
	require.NoError(t, err)

	var unmarshaled Object
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, note.ID, unmarshaled.ID)
	assert.Equal(t, note.Type, unmarshaled.Type)
	assert.Equal(t, "Hello, world!", unmarshaled.Content.First().String())
}

func TestPersonMarshaling(t *testing.T) {
	person := PersonNew(IRI("https://example.com/users/alice"))
	person.Name = NaturalLanguageValues{{Lang: "en", Value: "Alice"}}
	person.PreferredUsername = NaturalLanguageValues{{Lang: "en", Value: "alice"}}
	person.Inbox = IRI("https://example.com/users/alice/inbox")
	person.PublicKey.ID = IRI("https://example.com/users/alice#main-key")
	person.PublicKey.Owner = IRI("https://example.com/users/alice")
	person.PublicKey.PublicKeyPem = "-----BEGIN PUBLIC KEY-----\ntest\n-----END PUBLIC KEY-----"
	person.MovedTo = IRI("https://newexample.com/users/alice")
	person.AlsoKnownAs = ItemCollection{IRI("https://other.example/@alice")}

	data, err := json.Marshal(person)
	require.NoError(t, err)

	var unmarshaled Actor
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, person.ID, unmarshaled.ID)
	assert.Equal(t, person.Type, unmarshaled.Type)
	assert.Equal(t, "Alice", unmarshaled.Name.First().String())
	assert.Equal(t, "alice", unmarshaled.PreferredUsername.First().String())
	assert.Equal(t, person.PublicKey.PublicKeyPem, unmarshaled.PublicKey.PublicKeyPem)
	assert.Equal(t, person.MovedTo, unmarshaled.MovedTo)
	assert.Len(t, unmarshaled.AlsoKnownAs, 1)
}

func TestPersonMarshalingWithExtensions(t *testing.T) {
	// Test Person with AlsoKnownAs and AttributionDomains
	person := PersonNew(IRI("https://example.com/users/alice"))
	person.Name = NaturalLanguageValues{{Lang: "en", Value: "Alice"}}
	person.PreferredUsername = NaturalLanguageValues{{Lang: "en", Value: "alice"}}
	person.AlsoKnownAs = ItemCollection{IRI("https://other.example/@alice"), IRI("https://another.example/alice")}
	person.AttributionDomains = ItemCollection{IRI("example.com"), IRI("other.example")}

	data, err := json.Marshal(person)
	require.NoError(t, err)

	var unmarshaled Actor
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, person.ID, unmarshaled.ID)
	assert.Len(t, unmarshaled.AlsoKnownAs, 2)
	assert.Len(t, unmarshaled.AttributionDomains, 2)
	assert.Contains(t, string(data), "alsoKnownAs")
	assert.Contains(t, string(data), "attributionDomains")
}

func TestActivityMarshaling(t *testing.T) {
	note := ObjectNew(NoteType)
	note.ID = IRI("https://example.com/notes/1")
	note.Content = NaturalLanguageValues{{Lang: "en", Value: "Hello"}}

	create := ActivityNew(CreateType, IRI("https://example.com/activities/1"), note)
	create.Actor = IRI("https://example.com/users/alice")
	create.Published = time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	data, err := json.Marshal(create)
	require.NoError(t, err)

	var unmarshaled Activity
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, create.ID, unmarshaled.ID)
	assert.Equal(t, create.Type, unmarshaled.Type)
	assert.NotNil(t, unmarshaled.Actor)
	assert.NotNil(t, unmarshaled.Object)
}

func TestCollectionMarshaling(t *testing.T) {
	collection := CollectionNew(IRI("https://example.com/followers"))
	collection.Items.Append(IRI("https://example.com/users/alice"))
	collection.Items.Append(IRI("https://example.com/users/bob"))
	collection.TotalItems = 2

	data, err := json.Marshal(collection)
	require.NoError(t, err)

	var unmarshaled Collection
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, collection.ID, unmarshaled.ID)
	assert.Equal(t, uint(2), unmarshaled.TotalItems)
	assert.Len(t, unmarshaled.Items, 2)
}

func TestJSONLDMarshaling(t *testing.T) {
	note := ObjectNew(NoteType)
	note.ID = IRI("https://example.com/notes/1")
	note.Content = NaturalLanguageValues{{Lang: "en", Value: "Hello"}}

	data, err := jsonld.WithContext(
		jsonld.IRI(ActivityBaseURI),
		jsonld.IRI(SecurityContextURI),
	).Marshal(note)
	require.NoError(t, err)

	// Verify it contains the context
	var result map[string]any
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.NotNil(t, result["@context"])
	assert.Equal(t, "https://example.com/notes/1", result["id"])
	assert.Equal(t, "Note", result["type"])
}

func TestEndpoints(t *testing.T) {
	endpoints := &Endpoints{
		SharedInbox: IRI("https://example.com/inbox"),
	}

	data, err := json.Marshal(endpoints)
	require.NoError(t, err)

	var unmarshaled Endpoints
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.NotNil(t, unmarshaled.SharedInbox)
	assert.Equal(t, IRI("https://example.com/inbox"), unmarshaled.SharedInbox.GetLink())
}

func TestPublicKey(t *testing.T) {
	pk := PublicKey{
		ID:           IRI("https://example.com/users/alice#main-key"),
		Owner:        IRI("https://example.com/users/alice"),
		PublicKeyPem: "test-key",
	}

	data, err := json.Marshal(pk)
	require.NoError(t, err)

	var unmarshaled PublicKey
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, pk.ID, unmarshaled.ID)
	assert.Equal(t, pk.Owner, unmarshaled.Owner)
	assert.Equal(t, pk.PublicKeyPem, unmarshaled.PublicKeyPem)
}

func TestContentMapInsteadOfContent(t *testing.T) {
	// Test unmarshaling a Note with contentMap instead of content
	noteJSON := `{
		"@context": "https://www.w3.org/ns/activitystreams",
		"type": "Note",
		"id": "https://example.com/notes/1",
		"contentMap": {
			"en": "Hello",
			"fr": "Bonjour"
		}
	}`

	item, err := UnmarshalJSON([]byte(noteJSON))
	require.NoError(t, err)

	note, err := ToObject(item)
	require.NoError(t, err)
	assert.Equal(t, NoteType, note.Type)
	assert.Len(t, note.Content, 2)
}
