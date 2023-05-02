package yakit

import (
	"github.com/jinzhu/gorm"
	"regexp"
	"sort"
	"strings"
	"time"
	"yaklang/common/utils"
)

type RssFeed struct {
	gorm.Model

	SourceXmlUrl    string
	Hash            string     `gorm:"columns:hash;unique_index"`
	Title           string     `json:"title,omitempty"`
	Description     string     `json:"description,omitempty"`
	Link            string     `json:"link,omitempty"`
	FeedLink        string     `json:"feedLink,omitempty"`
	Updated         string     `json:"updated,omitempty"`
	UpdatedParsed   *time.Time `json:"updatedParsed,omitempty"`
	Published       string     `json:"published,omitempty"`
	PublishedParsed *time.Time `json:"publishedParsed,omitempty"`
	Author          string     `json:"author,omitempty"`
	AuthorEmail     string     `json:"author_email,omitempty"`
	Language        string     `json:"language,omitempty"`
	ImageUrl        string     `json:"image_url,omitempty"`
	ImageName       string     `json:"image_name,omitempty"`
	Copyright       string     `json:"copyright,omitempty"`
	Generator       string     `json:"generator,omitempty"`
	Categories      string     `json:"categories,omitempty"`
	FeedType        string     `json:"feedType"`
	FeedVersion     string     `json:"feedVersion"`
}

func (b *RssFeed) CalcHash() string {
	return utils.CalcSha1(
		b.Title, b.Description,
		b.Link, b.FeedLink, b.Author, b.AuthorEmail, b.ImageUrl,
	)
}

func (b *RssFeed) BeforeSave() error {
	b.Hash = b.CalcHash()
	return nil
}

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

var (
	tagConditions = map[string][]string{
		"工控":      {"工控", "IoT", "核电"},
		"内核安全":    {"内核", "Kernel Vul", "内核漏洞", "内核模块", "提权", "BIOS"},
		"RCE":     {"Remote Code Execution", "远程代码执行", "任意代码执行", "rbitrary code"},
		"远程代码执行":  {"Remote Code Execution", "远程代码执行", "任意代码执行", "rbitrary code", "code execution"},
		"黑产情报":    {"APT", "劫持", "投毒", "水坑", "鱼叉", "蠕虫", "钓鱼", "出售", "获利"},
		"SDLC":    {"安全生命周期", "sdl", "Devsecops", "devops"},
		"CTF":     {"awd", "ctf"},
		"0day":    {"0 day", "0day"},
		"提权":      {"privilege escalation", "提权"},
		"暴力破解":    {"brute-force", "暴力破解", "爆破"},
		"DoS":     {"dos", "拒绝服务攻击"},
		"SQL注入":   {"SQL injection", "SQL注入", "SQL 注入", "sqlinjection", "sqli"},
		"XSS":     {"cross-site scripting", "XSS", "跨站脚本"},
		"CSRF":    {"cross-site request forgery", "CSRF", "跨站请求伪造"},
		"Exploit": {"exploit", "漏洞利用程序", "利用"},
	}

	cveTitleRegexp = regexp.MustCompile(`CVE-\d+-\d+[ \t]*(\(.*?\))`)
)

func (b *Briefing) BeforeSave() error {
	b.Hash = b.CalcHash()

	rawMaterial := strings.Join([]string{b.Title, b.Description, b.Content, b.Link}, " ")
	var tags []string
	for tag, conds := range tagConditions {
		if utils.IStringContainsAnyOfSubString(rawMaterial, conds) {
			tags = append(tags, tag)
			//b.Tags = append(b.Tags, tag)
		}
	}

	if strings.HasPrefix(b.Title, "CVE") {
		for _, subs := range cveTitleRegexp.FindAllStringSubmatch(b.Title, -1) {
			if len(subs) > 1 {
				tag := subs[1]
				tag = strings.ReplaceAll(tag, ",", "|")
				if len(tag) > 20 {
					tag = tag[:17] + "..."
				}
				tags = append(tags, tag)
				//b.Tags = append(b.Tags, tag)
			}
		}
	}

	sort.Strings(tags)
	b.Tags = strings.Join(tags, ",")

	return nil
}

func (b *Briefing) CalcHash() string {
	return utils.CalcSha1(b.Title, b.Description, b.Content, b.Updated, b.Published, b.Tags)
}
