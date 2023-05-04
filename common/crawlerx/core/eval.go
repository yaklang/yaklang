package core

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

const createObserver = `
()=>{
	const config = { attributes: true, childList: true, subtree: true, characterData: true };
	window.added = ""
	// 当观察到变动时执行的回调函数
	const callback = function(mutationsList, observer) {
		// Use traditional 'for loops' for IE 11
		for(let mutation of mutationsList) {
			if (mutation.type === 'childList') {
				for (let node of mutation.addedNodes) {
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

const getObserverResult = `
()=>{
	observer.disconnect();
	return added;
}
`

const getSelector = `
()=>{
    let e = this;
    let domPath = Array();
    if (e.id) {
        domPath.unshift('#'+e.id.toLocaleLowerCase());
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

const setFileUploadInfo = `
()=>{
	window.name = ""
	window.size = 0
	function fileSelect(e) {
		e = e || window.event;
		var files = e.target.files; //FileList 对象
		var output = [];
		for(var i = 0, f; f = files[i]; i++) {
		   //console.log(f);
			window.name = f.name
			window.size = f.size
		}
	}
	if(window.File && window.FileList && window.FileReader && window.Blob) {
		document.querySelector('#main_body > div > div > form > input[type=file]:nth-child(4)').addEventListener('change', fileSelect, false);
	} else {
		document.write('您的浏览器不支持File Api');
	}
}
`

const getFileUploadInfo = `
()=>name+" "+size
`

const CommentMatch = `() => {
	let comment = document.documentElement.innerHTML.matchAll("<!--(?:.|\n|\r)+?-->")
	let commentList = []
	for (c of comment) {
		commentList.push(c[0])
	}
	let resultList = []
	let results = commentList.toString().matchAll("(?:src|href)\s*?\=\s*?(?:\"|\')(.+?)(?:\"|\')")
	for (r of results) {
		resultList.push(r[1])
	}
	return resultList
}
`

// const hrefReg = "(src|href)\\s*=\\s*(?:\"(?<1>[^\"]*)\"|(?<1>\\S+))"
const hrefReg2 = `(?:src|href)\s*?\=\s*?(?:\"|\')(.+?)(?:\"|\')`

//const reg = "\\w\\.get\\([\\\"\\'](.*?)[\\\"\\']\\,"

var jsUrlRegExps = []string{
	`\w\.get\([\"\'](.*?)[\"\']\,`,
	`\w\.post\([\"\'](.*?)[\"\']\,`,
	`\w\.post\([\"\'](.*?)[\"\']`,
	`\w\.get\([\"\'](.*?)[\"\']`,
	`\w\+[\"\'][^'"].*?][\"\']\,`,
	`\:{\s*?url\:\s*?[\"\'](.*?)[\"\']\,`,
	`return\s.*?\[[\"\'].[\"\']\]\.post\([\"\'](.*?)[\"\']`,
	`return\s.*?\[[\"\'].[\"\']\]\.get\([\"\'](.*?)[\"\']`,
	`{\s*?url\:\s*?[\"\'](.*?)[\"\']`,
	`(?:URL|Url|url)\:\s*?[\"\'](.*?)[\"\']`,
	`(?:URL|Url|url)\(\"(.*?)\"`,
	`(?:URL|Url|url)\s*?=\s*?[\'\"](.*?)[\'\"]`,
	`(?:URL|Url|url)\s*?\+\s*?[\'\"]([^\'\"].*?)[\'\"]`,
}
