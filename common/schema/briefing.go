package schema

import (
	"github.com/jinzhu/gorm"
	"time"
)

type Briefing struct {
	gorm.Model

	SourceXmlUrl    string
	RssFeedHash     string
	Hash            string     `gorm:"columns:hash;unique_index"`
	Title           string     `json:"title,omitempty"`
	Description     string     `json:"description,omitempty"`
	Content         string     `json:"content,omitempty"`
	Link            string     `json:"link,omitempty"`
	Updated         string     `json:"updated,omitempty"`
	UpdatedParsed   *time.Time `json:"updatedParsed,omitempty"`
	Published       string     `json:"published,omitempty"`
	PublishedParsed *time.Time `json:"publishedParsed,omitempty"`
	Author          string     `json:"author,omitempty"`
	AuthorEmail     string     `json:"author_email,omitempty"`
	GUID            string     `json:"guid,omitempty"`
	ImageUrl        string     `json:"image_url,omitempty"`
	ImageName       string     `json:"image_name,omitempty"`
	Categories      string     `json:"categories,omitempty"`
	Tags            string     `json:"tags"`
	IsRead          bool       `json:"is_read"`
}
