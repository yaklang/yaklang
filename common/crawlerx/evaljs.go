// Package crawlerx
// @Author bcy2007  2023/7/12 17:42
package crawlerx

import "regexp"

const getSelector = `
()=>{
    let e = this;
    let domPath = Array();
    if (e.getAttribute("id")) {
        domPath.unshift('#'+e.id);
    } else {
        while (e.nodeName.toLowerCase() !== "html") {
            if(e.getAttribute("id")){
                domPath.unshift('#'+e.getAttribute("id"));
                break;
            }else if(e.tagName.toLocaleLowerCase() == "body") {
                domPath.unshift(e.tagName.toLocaleLowerCase());
            }else{
                for (i = 0; i < e.parentNode.childElementCount; i++) {
                    if (e.parentNode.children[i] == e) {
                        domPath.unshift(e.tagName.toLocaleLowerCase() + ':nth-child(' + (i + 1) + ')');
                    }
                }
            }
            e = e.parentNode;
        }
    }
	domPath = domPath.toString().replaceAll(',', '>');
    return domPath
}
`

const getClickEventElement = `
function getSelector(e){
		let domPath = Array();
		if (e.getAttribute("id")) {
			domPath.unshift('#'+e.getAttribute("id"));
		} else {
			while (e.nodeName.toLowerCase() !== "html") {
				if(e.id){
					domPath.unshift('#'+e.getAttribute("id"));
					break;
				}else if(e.tagName.toLocaleLowerCase() == "body") {
					domPath.unshift(e.tagName.toLocaleLowerCase());
				}else{
					for (i = 0; i < e.parentNode.childElementCount; i++) {
						if (e.parentNode.children[i] == e) {
							domPath.unshift(e.tagName.toLocaleLowerCase() + ':nth-child(' + (i + 1) + ')');
						}
					}
				}
				e = e.parentNode;
			}
		}
		domPath = domPath.toString().replaceAll(',', '>');
		return domPath
	}
    let nodes = document.createNodeIterator(document.getRootNode())
    let clickSelectors = [];
    let node = nodes.nextNode();
    while ((node = nodes.nextNode())) {
		var events = getEventListeners(node);
		for (var eventName in events) {
			if (eventName === "click") {
				var selectorStr = getSelector(node);
				if (selectorStr !== "") {
					clickSelectors.push(selectorStr);
				}
				break;
			}
		}
    }
    clickSelectors
`

type JSEval struct {
	targetUrl *regexp.Regexp
	js        []string
}

func CreateJsEval() *JSEval {
	return &JSEval{
		js: make([]string, 0),
	}
}

type JsResultSave struct {
	TargetUrl string `json:"target_url"`
	Js        string `json:"js"`
	Result    string `json:"result"`
}
type JsResults []string
