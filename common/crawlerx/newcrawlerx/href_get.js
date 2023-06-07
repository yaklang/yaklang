function getUrl(){
    let nodes = document.createNodeIterator(document.getRootNode())
    let hrefs = Array();
    let node;
    while ((node = nodes.nextNode())) {
        let {href} = node;
        if (href) {
            hrefs[href] = normalGetSelector(node)
        }
    }
    return hrefs
}

// 递归
function getSelector(element){
    let tagName = element.tagName;
    if (tagName === "HTMl" || tagName === "BODY" || tagName === "HEAD"){
        return tagName.toLocaleLowerCase()
    }
    if (element.getAttribute("id")){
        return '#' + element.getAttribute("id")
    }
    let parent = element.parentNode
    let parentSelector = getSelector(parent)
    let children = parent.children
    for (let i = 0; i < children.length; i++) {
        if (children[i] == element) {
            let selector = parentSelector + " > " + tagName.toLocaleLowerCase() + ':nth-child(' + (i + 1) + ')'
            return selector
        }
    }
}

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
let clickSelectors = [];
let node;
while ((node = nodes.nextNode())) {
    var events = getEventListeners(node);
    for (var eventName in events) {
        if (eventName == "click") {
            clickSelectors.push(node);
            break;
        }
    }
}

// "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --headless --remote-debugging-port=9222