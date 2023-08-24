// Package simulator
// @Author bcy2007  2023/8/23 15:32
package simulator

const observer = `
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

const getObverserResult = `
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
