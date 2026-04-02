package activitypub

import (
	"encoding/json"
	"fmt"
	"time"
)

// UnmarshalJSON unmarshals JSON data into an Item
func UnmarshalJSON(data []byte) (Item, error) {
	// First, peek at the type
	var peek struct {
		Type ActivityType `json:"type"`
	}
	if err := json.Unmarshal(data, &peek); err != nil {
		// Might be an IRI string
		var iri string
		if err := json.Unmarshal(data, &iri); err == nil {
			return IRI(iri), nil
		}
		return nil, err
	}

	// Based on type, unmarshal into the appropriate struct
	switch peek.Type {
	case PersonType, ServiceType, GroupType, OrganizationType, ApplicationType:
		var actor Actor
		if err := json.Unmarshal(data, &actor); err != nil {
			return nil, err
		}
		return &actor, nil
	case CreateType, UpdateType, DeleteType, FollowType, AcceptType, UndoType, AnnounceType, LikeType, BlockType, MoveType:
		var activity Activity
		if err := json.Unmarshal(data, &activity); err != nil {
			return nil, err
		}
		return &activity, nil
	case CollectionType:
		var collection Collection
		if err := json.Unmarshal(data, &collection); err != nil {
			return nil, err
		}
		return &collection, nil
	default:
		// Default to Object for unknown or generic types
		var obj Object
		if err := json.Unmarshal(data, &obj); err != nil {
			return nil, err
		}
		return &obj, nil
	}
}

// UnmarshalJSON populates Object, decoding link fields into Items.
func (o *Object) UnmarshalJSON(data []byte) error {
	type raw struct {
		Context      any                   `json:"@context,omitempty"`
		ID           IRI                   `json:"id,omitempty"`
		Type         ActivityType          `json:"type,omitempty"`
		Name         NaturalLanguageValues `json:"name,omitempty"`
		NameMap      NaturalLanguageValues `json:"nameMap,omitempty"`
		Summary      NaturalLanguageValues `json:"summary,omitempty"`
		SummaryMap   NaturalLanguageValues `json:"summaryMap,omitempty"`
		Content      NaturalLanguageValues `json:"content,omitempty"`
		ContentMap   NaturalLanguageValues `json:"contentMap,omitempty"`
		MediaType    MimeType              `json:"mediaType,omitempty"`
		URL          json.RawMessage       `json:"url,omitempty"`
		Href         json.RawMessage       `json:"href,omitempty"`
		AttributedTo json.RawMessage       `json:"attributedTo,omitempty"`
		InReplyTo    json.RawMessage       `json:"inReplyTo,omitempty"`
		To           ItemCollection        `json:"to,omitempty"`
		CC           ItemCollection        `json:"cc,omitempty"`
		Tag          ItemCollection        `json:"tag,omitempty"`
		Attachment   any                   `json:"attachment,omitempty"`
		Published    time.Time             `json:"published,omitzero"`
		Updated      time.Time             `json:"updated,omitzero"`
	}
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}

	o.Context = r.Context
	o.ID = r.ID
	o.Type = r.Type
	o.Name = r.Name
	if len(o.Name) == 0 && len(r.NameMap) > 0 {
		o.Name = r.NameMap
	}
	o.Summary = r.Summary
	if len(o.Summary) == 0 && len(r.SummaryMap) > 0 {
		o.Summary = r.SummaryMap
	}
	o.Content = r.Content
	if len(o.Content) == 0 && len(r.ContentMap) > 0 {
		o.Content = r.ContentMap
	}
	o.MediaType = r.MediaType
	o.To = r.To
	o.CC = r.CC
	o.Tag = r.Tag
	o.Attachment = r.Attachment
	o.Published = r.Published
	o.Updated = r.Updated

	if len(r.AttributedTo) > 0 {
		item, err := UnmarshalJSON(r.AttributedTo)
		if err != nil {
			return err
		}
		o.AttributedTo = item
	}
	if len(r.InReplyTo) > 0 {
		item, err := UnmarshalJSON(r.InReplyTo)
		if err != nil {
			return err
		}
		o.InReplyTo = item
	}
	if len(r.URL) > 0 {
		item, err := UnmarshalJSON(r.URL)
		if err != nil {
			return err
		}
		o.URL = item
	}
	if len(r.Href) > 0 {
		item, err := UnmarshalJSON(r.Href)
		if err != nil {
			return err
		}
		o.Href = item
		if o.URL == nil {
			o.URL = item
		}
	}

	return nil
}

// UnmarshalJSON populates Activity, converting interface fields.
func (a *Activity) UnmarshalJSON(data []byte) error {
	type raw struct {
		ID        IRI             `json:"id,omitempty"`
		Type      ActivityType    `json:"type,omitempty"`
		Actor     json.RawMessage `json:"actor,omitempty"`
		Object    json.RawMessage `json:"object,omitempty"`
		Target    json.RawMessage `json:"target,omitempty"`
		To        ItemCollection  `json:"to,omitempty"`
		CC        ItemCollection  `json:"cc,omitempty"`
		Published time.Time       `json:"published,omitzero"`
		Updated   time.Time       `json:"updated,omitzero"`
	}
	var r raw
	if err := json.Unmarshal(data, &r); err != nil {
		return err
	}
	*a = Activity{
		ID:        r.ID,
		Type:      r.Type,
		To:        r.To,
		CC:        r.CC,
		Published: r.Published,
		Updated:   r.Updated,
	}
	if len(r.Actor) > 0 {
		item, err := UnmarshalJSON(r.Actor)
		if err != nil {
			return err
		}
		a.Actor = item
	}
	if len(r.Object) > 0 {
		item, err := UnmarshalJSON(r.Object)
		if err != nil {
			return err
		}
		a.Object = item
	}
	if len(r.Target) > 0 {
		item, err := UnmarshalJSON(r.Target)
		if err != nil {
			return err
		}
		a.Target = item
	}
	return nil
}

// UnmarshalJSON populates Actor, converting interface fields and embedded Object.
func (p *Actor) UnmarshalJSON(data []byte) error {
	if err := p.Object.UnmarshalJSON(data); err != nil {
		return err
	}

	type extras struct {
		PreferredUsername    NaturalLanguageValues `json:"preferredUsername,omitempty"`
		PreferredUsernameMap NaturalLanguageValues `json:"preferredUsernameMap,omitempty"`
		Inbox                IRI                   `json:"inbox,omitempty"`
		Outbox               IRI                   `json:"outbox,omitempty"`
		Following            IRI                   `json:"following,omitempty"`
		Followers            IRI                   `json:"followers,omitempty"`
		PublicKey            PublicKey             `json:"publicKey"`
		Endpoints            *Endpoints            `json:"endpoints,omitempty"`
		Icon                 json.RawMessage       `json:"icon,omitempty"`
		MovedTo              json.RawMessage       `json:"movedTo,omitempty"`
		AlsoKnownAs          ItemCollection        `json:"alsoKnownAs,omitempty"`
		AttributionDomains   ItemCollection        `json:"attributionDomains,omitempty"`
	}
	var ex extras
	if err := json.Unmarshal(data, &ex); err != nil {
		return err
	}

	p.PreferredUsername = ex.PreferredUsername
	if len(p.PreferredUsername) == 0 && len(ex.PreferredUsernameMap) > 0 {
		p.PreferredUsername = ex.PreferredUsernameMap
	}
	p.Inbox = ex.Inbox
	p.Outbox = ex.Outbox
	p.Following = ex.Following
	p.Followers = ex.Followers
	p.PublicKey = ex.PublicKey
	p.Endpoints = ex.Endpoints
	p.AlsoKnownAs = ex.AlsoKnownAs
	p.AttributionDomains = ex.AttributionDomains

	if len(ex.Icon) > 0 {
		item, err := UnmarshalJSON(ex.Icon)
		if err != nil {
			return err
		}
		p.Icon = item
	}

	if len(ex.MovedTo) > 0 {
		item, err := UnmarshalJSON(ex.MovedTo)
		if err != nil {
			return err
		}
		p.MovedTo = item
	}

	return nil
}

// UnmarshalJSON populates Collection while reusing Object parsing.
func (c *Collection) UnmarshalJSON(data []byte) error {
	if err := c.Object.UnmarshalJSON(data); err != nil {
		return err
	}

	var extras struct {
		TotalItems uint           `json:"totalItems,omitempty"`
		Items      ItemCollection `json:"items,omitempty"`
	}
	if err := json.Unmarshal(data, &extras); err != nil {
		return err
	}

	c.TotalItems = extras.TotalItems
	c.Items = extras.Items

	return nil
}

// UnmarshalJSON implements json.Unmarshaler for Endpoints
func (e *Endpoints) UnmarshalJSON(data []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}

	if raw, ok := m["sharedInbox"]; ok {
		item, err := UnmarshalJSON(raw)
		if err != nil {
			return err
		}
		e.SharedInbox = item
	}

	return nil
}

// UnmarshalJSON implements json.Unmarshaler for NaturalLanguageValues
func (n *NaturalLanguageValues) UnmarshalJSON(data []byte) error {
	// Try as string first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*n = NaturalLanguageValues{{Value: s, Lang: ""}}
		return nil
	}

	// Try as map
	var m map[string]string
	if err := json.Unmarshal(data, &m); err == nil {
		*n = make(NaturalLanguageValues, 0, len(m))
		for lang, value := range m {
			if lang == "@value" {
				lang = ""
			}
			*n = append(*n, NaturalLanguageValue{Lang: lang, Value: value})
		}
		return nil
	}

	return fmt.Errorf("invalid natural language value")
}

// UnmarshalJSON implements json.Unmarshaler for ItemCollection
func (i *ItemCollection) UnmarshalJSON(data []byte) error {
	// Try as array first
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err == nil {
		*i = make(ItemCollection, 0, len(arr))
		for _, raw := range arr {
			item, err := UnmarshalJSON(raw)
			if err != nil {
				return err
			}
			*i = append(*i, item)
		}
		return nil
	}

	// Try as single item
	item, err := UnmarshalJSON(data)
	if err != nil {
		return err
	}
	*i = ItemCollection{item}
	return nil
}
