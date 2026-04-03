package main

import (
	"bytes"
	"context"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/url"

	"github.com/araddon/dateparse"
	ct "github.com/elnormous/contenttype"
	ap "go.goblog.app/app/pkgs/activitypub"
	"go.goblog.app/app/pkgs/activitypub/jsonld"
	"go.goblog.app/app/pkgs/contenttype"
)

const asRequestKey contextKey = "asRequest"

var asCheckMediaTypes []ct.MediaType

func init() {
	asCheckMediaTypes = []ct.MediaType{
		ct.NewMediaType(contenttype.HTML),
		ct.NewMediaType(contenttype.AS),
		ct.NewMediaType(contenttype.LDJSON),
		ct.NewMediaType(contenttype.LDJSON + "; profile=\"https://www.w3.org/ns/activitystreams\""),
	}
}

func (a *goBlog) checkActivityStreamsRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if a.apEnabled() {
			alreadyAsRequest, ok := r.Context().Value(asRequestKey).(bool)
			if (ok && alreadyAsRequest) || a.isActivityStreamsRequest(r) {
				next.ServeHTTP(rw, r.WithContext(context.WithValue(r.Context(), asRequestKey, true)))
				return
			}
		}
		next.ServeHTTP(rw, r)
	})
}

func (*goBlog) isActivityStreamsRequest(r *http.Request) bool {
	if mt, _, err := ct.GetAcceptableMediaType(r, asCheckMediaTypes); err == nil && mt.String() != asCheckMediaTypes[0].String() {
		return true
	}
	return false
}

func (a *goBlog) serveActivityStreamsPost(w http.ResponseWriter, r *http.Request, status int, p *post) {
	a.serveAPItem(w, r, status, a.toAPNote(p))
}

func (a *goBlog) toAPNote(p *post) *ap.Note {
	bc := a.getBlogFromPost(p)
	// Create a Note object
	note := ap.ObjectNew(ap.NoteType)
	note.ID = a.activityPubID(p)
	note.URL = ap.IRI(a.fullPostURL(p))
	note.AttributedTo = a.apAPIri(bc)
	// Audience
	switch p.Visibility {
	case visibilityPublic:
		note.To.Append(ap.PublicNS, a.apGetFollowersCollectionID(p.Blog))
	case visibilityUnlisted:
		note.To.Append(a.apGetFollowersCollectionID(p.Blog))
		note.CC.Append(ap.PublicNS)
	}
	for _, m := range p.Parameters[activityPubMentionsParameter] {
		note.CC.Append(ap.IRI(m))
	}
	// Name and Type
	if title := p.RenderedTitle; title != "" {
		note.Type = ap.ArticleType
		note.Name = ap.NaturalLanguageValues{{Lang: bc.Lang, Value: title}}
	}
	// Content
	note.MediaType = ap.MimeType(contenttype.HTML)
	note.Content = ap.NaturalLanguageValues{{Lang: bc.Lang, Value: a.postHTML(&postHTMLOptions{p: p, absolute: true, activityPub: true})}}
	// Attachments
	if images := p.Parameters[a.cfg.Micropub.PhotoParam]; len(images) > 0 {
		var attachments ap.ItemCollection
		for _, image := range images {
			apImage := ap.ObjectNew(ap.ImageType)
			apImage.URL = ap.IRI(image)
			attachments.Append(apImage)
		}
		note.Attachment = attachments
	}
	// Tags
	for _, tagTax := range a.cfg.ActivityPub.TagsTaxonomies {
		for _, tag := range p.Parameters[tagTax] {
			apTag := &ap.Object{Type: "Hashtag"}
			apTag.Name = ap.NaturalLanguageValues{{Lang: bc.Lang, Value: tag}}
			apTag.URL = ap.IRI(a.getFullAddress(a.getRelativePath(p.Blog, fmt.Sprintf("/%s/%s", tagTax, urlize(tag)))))
			note.Tag.Append(apTag)
		}
	}
	// Mentions
	for _, mention := range p.Parameters[activityPubMentionsParameter] {
		apMention := ap.ObjectNew(ap.MentionType)
		apMention.ID = ap.IRI(mention)
		apMention.Href = ap.IRI(mention)
		note.Tag.Append(apMention)
	}
	if replyLinkActor := p.firstParameter(activityPubReplyActorParameter); replyLinkActor != "" {
		apMention := ap.ObjectNew(ap.MentionType)
		apMention.ID = ap.IRI(replyLinkActor)
		apMention.Href = ap.IRI(replyLinkActor)
		note.Tag.Append(apMention)
	}
	// Dates
	if p.Published != "" {
		if t, err := dateparse.ParseLocal(p.Published); err == nil {
			note.Published = t
		}
	}
	if p.Updated != "" {
		if t, err := dateparse.ParseLocal(p.Updated); err == nil {
			note.Updated = t
		}
	}
	// Reply
	if replyLink := p.firstParameter(a.cfg.Micropub.ReplyParam); replyLink != "" {
		if replyObject := p.firstParameter(activityPubReplyObjectParameter); replyObject != "" {
			note.InReplyTo = ap.IRI(replyObject)
		} else {
			// Fallback to reply link if reply object is not available
			note.InReplyTo = ap.IRI(replyLink)
		}
	}
	return note
}

const activityPubVersionParam = "activitypubversion"

func (a *goBlog) activityPubID(p *post) ap.IRI {
	fu := a.fullPostURL(p)
	if version := p.firstParameter(activityPubVersionParam); version != "" {
		return ap.IRI(fu + "?activitypubversion=" + version)
	}
	return ap.IRI(fu)
}

func (a *goBlog) toApPerson(blog string, altAddress string) *ap.Actor {
	b := a.cfg.Blogs[blog]

	var iri string
	if altAddress != "" {
		iri = a.apIriForAddress(b, altAddress)
	} else {
		iri = a.apIri(b)
	}

	apIri := ap.IRI(iri)

	apBlog := ap.PersonNew(apIri)
	apBlog.URL = apIri

	apBlog.Name = ap.NaturalLanguageValues{{Lang: b.Lang, Value: a.renderMdTitle(b.Title)}}
	apBlog.Summary = ap.NaturalLanguageValues{{Lang: b.Lang, Value: b.Description}}
	apBlog.PreferredUsername = ap.NaturalLanguageValues{{Lang: b.Lang, Value: blog}}

	if altAddress != "" {
		apBlog.Inbox = ap.IRI(getFullAddressStatic(altAddress, apInboxPathTemplate+blog))
	} else {
		apBlog.Inbox = ap.IRI(a.getFullAddress(apInboxPathTemplate + blog))
	}
	apBlog.Followers = a.apGetFollowersCollectionIDForAddress(blog, altAddress)

	apBlog.PublicKey.Owner = apIri
	apBlog.PublicKey.ID = ap.IRI(iri + "#main-key")
	apBlog.PublicKey.PublicKeyPem = string(pem.EncodeToMemory(&pem.Block{
		Type:    "PUBLIC KEY",
		Headers: nil,
		Bytes:   a.apPubKeyBytes,
	}))

	if a.hasProfileImage() {
		icon := &ap.Image{}
		icon.Type = ap.ImageType
		icon.MediaType = ap.MimeType(contenttype.JPEG)
		icon.URL = ap.IRI(a.getFullAddress(a.profileImagePath(profileImageFormatJPEG, 0, 0)))
		apBlog.Icon = icon
	}

	for _, ad := range a.cfg.ActivityPub.AttributionDomains {
		apBlog.AttributionDomains = append(apBlog.AttributionDomains, ap.IRI(ad))
	}

	for _, aka := range a.cfg.ActivityPub.AlsoKnownAs {
		apBlog.AlsoKnownAs = append(apBlog.AlsoKnownAs, ap.IRI(aka))
	}

	if altAddress == "" {
		// Add alternative addresses as alsoKnownAs
		for _, altAddress := range a.cfg.Server.AltAddresses {
			apBlog.AlsoKnownAs = append(apBlog.AlsoKnownAs, ap.IRI(a.apIriForAddress(b, altAddress)))
		}
	} else {
		// Add main address as alsoKnownAs
		apBlog.AlsoKnownAs = append(apBlog.AlsoKnownAs, a.apAPIri(b))
	}

	// Check if this blog has a movedTo target set (account migration)
	if movedTo, err := a.getApMovedTo(blog); err == nil && movedTo != "" {
		apBlog.MovedTo = ap.IRI(movedTo)
	} else if altAddress != "" {
		// If this is an alternative domain, set movedTo to point to the main domain
		apBlog.MovedTo = a.apAPIri(b)
	}

	return apBlog
}

func (a *goBlog) serveActivityStreams(w http.ResponseWriter, r *http.Request, status int, blog string) {
	altAddress, _ := r.Context().Value(altAddressKey).(string)
	a.serveAPItem(w, r, status, a.toApPerson(blog, altAddress))
}

func (a *goBlog) serveAPItem(w http.ResponseWriter, r *http.Request, status int, item any) {
	// Encode
	binary, err := jsonld.WithContext(jsonld.IRI(ap.ActivityBaseURI), jsonld.IRI(ap.SecurityContextURI)).Marshal(item)
	if err != nil {
		a.serveError(w, r, "Encoding failed", http.StatusInternalServerError)
		return
	}
	// Send response
	w.Header().Set(contentType, contenttype.ASUTF8)
	w.WriteHeader(status)
	_ = a.min.Get().Minify(contenttype.AS, w, bytes.NewReader(binary))
}

func apUsername(actor *ap.Actor) string {
	preferredUsername := actor.PreferredUsername.First().String()
	u, err := url.Parse(actor.GetLink().String())
	if err != nil || u == nil || u.Host == "" || preferredUsername == "" {
		return actor.GetLink().String()
	}
	return fmt.Sprintf("@%s@%s", preferredUsername, u.Host)
}
