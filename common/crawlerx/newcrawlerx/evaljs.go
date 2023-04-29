// Package newcrawlerx
// @Author bcy2007  2023/3/7 15:42
package newcrawlerx

const findOnlyHref = `()=>{
	let nodes = document.createNodeIterator(document.getRootNode())
    let hrefs = [];
    let node;
    while ((node = nodes.nextNode())) {
        let {href, src} = node;
        if (href) {
            hrefs.push(href)
        }
    }
    return hrefs
}`

const FindHref = `()=>{
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

const FindListener = findListener

const findListener = `() => {
	function getSelector(e){
		let domPath = Array();
		if (e.id) {
			domPath.unshift('#'+e.id);
		} else {
			while (e.nodeName.toLowerCase() !== "html") {
				if(e.id){
					domPath.unshift('#'+e.id);
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
    let node;
    while ((node = nodes.nextNode())) {
		var events = getEventListeners(node);
		for (var eventName in events) {
			if (eventName == "click") {
				var selectorStr = getSelector(node);
				if (selectorStr !== "") {
					clickSelectors.push(selectorStr);
				}
				break;
			}
		}
    }
    return clickSelectors
}`

const testJs = `
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

const getUniqueSelector = "(e)=>{function getUniqueSelector(node) {\n        let selector = \"\";\n        while (node.parentElement) {\n            const siblings = Array.from(node.parentElement.children).filter(\n                e => e.tagName === node.tagName\n            );\n            selector =\n                (siblings.indexOf(node) ?\n                    `${node.tagName}:nth-of-type(${siblings.indexOf(node) + 1})` :\n                    `${node.tagName}`) + `${selector ? \" > \" : \"\"}${selector}`;\n            node = node.parentElement;\n        }\n        return `html > ${selector.toLowerCase()}`;\n    }return getUniqueSelector(this)}"

const GetHrefSelector = `
	function normalGetSelector(element) {
		let domPath = []
		while (true){
			let tagName = element.tagName;
			if (tagName === "HTMl" || tagName === "BODY" || tagName === "HEAD") {
				domPath.unshift(tagName.toLocaleLowerCase())
				break
			}
			if (element.getAttribute("id")) {
				domPath.unshift("#" + element.getAttribute("id"))
				break
			}
			let parent = element.parentNode;
			let children = parent.children;
			for (let i = 0; i < children.length; i++) {
				if (children[i] == element) {
					domPath.unshift(tagName.toLocaleLowerCase() + ':nth-child(' + (i + 1) + ')')
					break
				}
			}
			element = element.parentNode
		}
		return domPath.toString().replaceAll(","," > ")
	}
	let nodes = document.createNodeIterator(document.getRootNode())
	let hrefs = {};
	let node;
	while ((node = nodes.nextNode())) {
		let {href} = node;
		if (href) {
			hrefs[href] = normalGetSelector(node);
		}
	}
	hrefs
`

var TestJs = testJs
