// Package crawlerx
// @Author bcy2007  2023/7/12 17:42
package crawlerx

import "regexp"

const pageScript = `
function randomStr(length) {
	let str = '';
	let chars = '0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ';
	for (let i = 0; i < length; i++) {
		str += chars.charAt(Math.floor(Math.random() * chars.length));
	}
	return str;
}
window.__originOpen = window.open
window.open = function (url,name,specs,replace) {
    name = name+'_'+randomStr(8);
    window.__originOpen(url,name,specs,replace)
}
`

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

const getOnClickAction = `
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
	if (node.onclick !== null && node.onclick !== undefined) {
		var selectorStr = getSelector(node);
		if (selectorStr !== "") {
			clickSelectors.push(selectorStr);
		}
	}
}
clickSelectors
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
    var nodes = document.createNodeIterator(document.getRootNode())
    var clickSelectors = [];
    var node = nodes.nextNode();
    while ((node = nodes.nextNode())) {
		if (
			!node || 
			typeof node.getBoundingClientRect !== 'function'
		) {
			continue;
		}
		var t = node.getBoundingClientRect(), n = window.getComputedStyle(node);
		if (
			"none" == n.display ||
			"hidden" == n.visibility ||
			!(t.top || t.bottom || t.width || t.height)
		 ) {
			continue;
		}
		if (node.onclick !== null && node.onclick !== undefined) {
			var selectorStr = getSelector(node);
			if (selectorStr !== "") {
				clickSelectors.push(selectorStr);
			}
		} else {
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
