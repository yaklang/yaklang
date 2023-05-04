package core

const GETNAME = `
()=>{
    let result = this.tagName.toLowerCase();
	if (this.id !== ""){
		result += "#" + this.id;
	}
	if (this.className !== ""){
		result += "." + this.className;
    }
	return result
}
`

const getSelectorNew = `
()=>{
    let e = this;
    let domPath = Array();
    if (e.id) {
        domPath.unshift('#'+e.id);
		domPath = domPath.toString();
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
        domPath = domPath.toString().replaceAll(',', '>');
    }
    return domPath
}
`

const GETSELECTOR = `
()=>{
    let e = this;
    let domPath = Array();
    if (e.id) {
        domPath.unshift('#'+e.id.toLocaleLowerCase());
		domPath = domPath.toString();
    } else {
        while (e.nodeName.toLowerCase() !== "html") {
            if(e.id){
                domPath.unshift('#'+e.id.toLocaleLowerCase());
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
        domPath = domPath.toString().replaceAll(',', '>');
    }
    return domPath
}
`

const OBSERVER = `
()=>{
	const config = { attributes: true, childList: true, subtree: true, characterData: true };
	window.added = "";
	window.addednode = null;
	// 当观察到变动时执行的回调函数
	const callback = function(mutationsList, observer) {
		// Use traditional 'for loops' for IE 11
		for(let mutation of mutationsList) {
			if (mutation.type === 'childList') {
				//window.node = node;
				for (let node of mutation.addedNodes) {
					addednode = node
					// added += node.innerHTML;
					if (node.innerHTML !== undefined) {
						added += node.innerHTML
					} else if (node.data !== undefined){
						added += node.data
					} else {
						added += node.nodeValue
					} 
				}
			}
			else if (mutation.type === 'attributes') {
			}
			else if (mutation.type === 'characterData') {
				added += mutation.target.data;
			}
		}
	};
	// 创建一个观察器实例并传入回调函数
	window.observer = new MutationObserver(callback);
	// 以上述配置开始观察目标节点
	observer.observe(document, config);
}
`

const OBSERVERRESULT = `
()=>{
	if (typeof(added) !== "string"){
		return ""
	}
	try {
		ahrefs = addednode.getElementsByTagName("a")
		if (ahrefs.length !== 0){
			ahrefs[0].click()
		}
	} catch (err) {}
	observer.disconnect();
	return added;
}
`
