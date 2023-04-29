package core

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/yak/yaklib/codec"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func (m *Manager) PageBack(page *rod.Page) error {
	err := page.NavigateBack()
	if err != nil {
		return utils.Errorf("page back error: %s", err)
	}
	return nil
}

func (m *Manager) extractCommit(page_block *PageBlock) error {
	// origin_url := page.MustInfo().URL
	page := page_block.page
	origin_info, err := page.Info()
	if err != nil {
		return utils.Errorf("get page info error: %s", err)
	}
	origin_url := origin_info.URL
	html, err := page.HTML()
	if err != nil {
		return utils.Errorf("get page html error: %s", err)
	}
	hash := requestToUniqueHash(origin_url, "unknown", html, nil)
	if m.visited.Exist(hash) {
		return nil
	} else {
		m.visited.Insert(hash)
	}
	// do push every button
	// cotinue push button while url jump not happen
	// do scan next while url jump happen
	buttons, err := page.Elements("button")
	if err != nil {
		return utils.Errorf("get element button error: %s", err)
	}
	for _, button := range buttons {
		button_type, err := button.Attribute("type")
		if err != nil {
			if m.detailLog {
				log.Errorf("__element inputs type get error:%s", err)
			}
			// return utils.Errorf("__element inputs type get error:%s", err)
			continue
		}
		if button_type != nil && strings.Compare(*button_type, "hidden") == 0 {
			continue
		}
		button.Eval(`()=>this.click()`)
		// page.MustWaitLoad()
		current_url, err := m.GetCurrentUrl(page)
		if err != nil {
			// return utils.Errorf("get current url error: %s", err)
			if m.detailLog {
				log.Errorf("get current url error: %s", err)
			}
			continue
		}
		if strings.Compare(origin_url, current_url) != 0 {
			page_block.GoDeeper()
			m.extractInput(page)
			m.extractUrls(page_block)
			m.PageBack(page)
			page_block.GoBack()
		} else {
			m.extractInput(page)
			m.extractUrls(page_block)
		}
	}
	// so do the submit as button
	inputs, err := page.Elements("input")
	if err != nil {
		return utils.Errorf("__element inputs get error:%s", err)
	}
	for _, input := range inputs {
		input_type, err := input.Attribute("type")
		if err != nil {
			// return utils.Errorf("__element inputs type get error:%s", err)
			if m.detailLog {
				log.Errorf("__element inputs type get error:%s", err)
			}
			continue
		}
		if input_type != nil && strings.Compare(*input_type, "submit") != 0 {
			continue
		}
		input.Eval(`()=>this.click()`)
		current_url, err := m.GetCurrentUrl(page)
		if err != nil {
			// return utils.Errorf("__element input current url error: %s", err)
			if m.detailLog {
				log.Errorf("__element input current url error: %s", err)
			}
			continue
		}
		if strings.Compare(origin_url, current_url) != 0 {
			page_block.GoDeeper()
			m.extractInput(page)
			m.extractUrls(page_block)
			m.PageBack(page)
			page_block.GoBack()
		} else {
			m.extractInput(page)
			m.extractUrls(page_block)
		}
	}
	return nil
}

func (m *Manager) CheckSensitive(element *rod.Element) bool {
	value, err := element.Attribute("value")
	if err != nil {
		return false
	} else if value == nil {
		return false
	}
	for _, sensiStr := range sensitiveWords {
		if strings.Contains(*value, sensiStr) {
			return true
		}
	}
	for _, sensiStr := range sensitiveWordsCN {
		if strings.Contains(*value, sensiStr) {
			return true
		}
	}
	return false
}

func requestToUniqueHash(url string, method string, body string, cookie []*proto.NetworkCookie) string {
	var cookieRaw []string
	for _, c := range cookie {
		if c == nil {
			continue
		}
		cookieRaw = append(cookieRaw, cookieToHash(c))
	}
	identify := fmt.Sprintf(
		"%v METHOD[%v] BODYSHA256:%v COOKIE:SHA256:%v",
		url, method, codec.Sha256(body), codec.Sha256(strings.Join(cookieRaw, "|")),
	)
	return codec.Sha256(identify)
}

func cookieToHash(c *proto.NetworkCookie) string {
	return CalcSha1(
		c.Value, c.Name,
		c.Domain, c.Path, c.Expires.String(),
		c.Size, c.HTTPOnly, c.Secure,
		c.Secure, c.SameSite,
		c.Priority, c.SameParty,
		c.SourceScheme, c.SourcePort,
	)
}

func CalcSha1(items ...interface{}) string {
	s := fmt.Sprintf("%v", items)
	raw := sha1.Sum([]byte(s))
	return hex.EncodeToString(raw[:])
}
