package core

import (
	"strings"
	"yaklang/common/log"
	"yaklang/common/rpa/character"
	"yaklang/common/utils"
	"yaklang/common/yak/yaklib/codec"
)

const findHref = `() => {
    let nodes = document.createNodeIterator(document.getRootNode())
    let hrefs = [];
    let node;
    while ((node = nodes.nextNode())) {
        let {href, src} = node;
        if (href) {
            hrefs.push(href)
        }
        if (src) {
            hrefs.push(src)
        }
    }
    return hrefs
}`

func (m *Manager) extractUrls(page_block *PageBlock) error {
	page := page_block.page
	r, err := page.Eval(findHref)
	if err != nil {
		return utils.Errorf("eval failed: %s", err)
	}
	tmp := r.Value.Arr()
	for _, r := range tmp {
		urlStr := r.Str()
		if urlStr == "" {
			continue
		}
		//remove url param and calculate hash
		hashStr := m.RemoveParamValue(urlStr)
		hash := requestToUniqueHash(hashStr, "GET", "", nil)
		if m.visited.Exist(hash) {
			continue
		} else {
			m.visited.Insert(hash)
		}

		if !m.checkFileSuffixValid(urlStr) {
			continue
		}

		if !m.checkHostIsValid(urlStr) {
			continue

		}
		var ifDanger string
		if m.rfmodel == nil {
			ifDanger = "0"
		} else if subString := character.CutLastSubUrl(urlStr); subString == "" {
			ifDanger = "0"
		} else {
			ifDanger = m.rfmodel.PredictX(subString)
			iffDanger := m.PredictX(subString)
			if ifDanger != iffDanger {
				ifDanger = "0"
			}
			if ifDanger == "1" {
				log.Infof("danger url: %s : %s", urlStr, subString)
			}
		}
		if page_block.depth < m.depth && ifDanger == "0" {
			// go deptch
			m.pageSizedWaitGroup.AddWithContext(m.rootContext)
			go func() {
				err = m.page(urlStr, page_block.depth+1)
				if err != nil && !strings.Contains(err.Error(), "context canceled") {
					log.Errorf("page error: %s", err)
				}
			}()
		} else {
			// do not go depth so need to send url data to channel
			// or sensitive url can not click, send url data to channel directly
			hash = codec.Sha256(urlStr)
			if m.hijacked.Exist(hash) {
				continue
			}
			m.hijacked.Insert(hash)
			r := &MakeReq{}
			r.url = urlStr
			m.channel <- r
			if m.urlCount != 0 && m.hijacked.Count() >= int64(m.urlCount) {
				m.rootCancel()
			}
		}
	}
	return nil
}

// use key words directly to detect whether url is sensitive
// a complement of random forest used by detect sensitive url
func (m *Manager) PredictX(s string) string {
	for _, sensiStr := range sensitiveWords {
		if strings.Contains(s, sensiStr) {
			return "1"
		}
	}
	return "0"
}
