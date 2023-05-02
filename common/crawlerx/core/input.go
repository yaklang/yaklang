package core

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"strings"
	"github.com/yaklang/yaklang/common/log"
)

func (crawler *CrawlerX) DoInput(element *rod.Element) error {
	attribute, _ := element.Attribute("type")
	if attribute == nil {
		return nil
	}
	switch *attribute {
	case "text", "password":
		crawler.DoTextInput(element)
	case "file":
		crawler.DoFileInput(element)
	case "radio", "checkbox":
		crawler.DoClick(element)
	default:

	}
	return nil
}

func (crawler *CrawlerX) DoTextInput(element *rod.Element) error {
	keywords := GetAllKeyWords(element)
	for k, v := range crawler.formFill {
		if strings.Contains(keywords, k) {
			element.Type([]input.Key(v)...)
			return nil
		}
	}
	runeStr := []input.Key("test")
	element.Type(runeStr...)
	return nil
}

func (crawler *CrawlerX) DoFileInput(element *rod.Element) error {
	//element.MustSetFiles("")
	log.Info("pretend upload file.")
	return nil
}

func (crawler *CrawlerX) DoClick(element *rod.Element) error {
	element.Click(proto.InputMouseButtonLeft)
	return nil
}
