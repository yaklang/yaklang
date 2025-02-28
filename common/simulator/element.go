// Package simulator
// @Author bcy2007  2023/8/17 16:19
package simulator

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

const maxParentLevel = 3

func customizedGetElement(page *rod.Page, searchInfo map[string]map[string][]string) (rod.Elements, error) {
	resultElements := make([]*rod.Element, 0)
	for tagName, tagInfo := range searchInfo {
		elements, err := customizedCheckTagElements(page, tagName, tagInfo)
		if err != nil {
			continue
		}
		resultElements = append(resultElements, elements...)
	}
	return resultElements, nil
}

func customizedCheckTagElements(page *rod.Page, tagName string, tagInfo map[string][]string) (rod.Elements, error) {
	elements, err := page.Elements(tagName)
	if err != nil {
		return nil, err
	}
	if len(tagInfo) == 0 {
		return elements, nil
	}
	resultElements := make(rod.Elements, 0)
	for _, element := range elements {
		if visible, err := element.Visible(); err != nil || !visible {
			continue
		}
		if customizedCheckElementAttribute(element, tagInfo) {
			resultElements = append(resultElements, element)
		}
	}
	return resultElements, nil
}

func customizedCheckElementAttribute(element *rod.Element, attributeInfo map[string][]string) bool {
	for attribute, attributeList := range attributeInfo {
		info, _ := GetAttribute(element, attribute)
		//if info == "" {
		//	continue
		//}
		info = strings.ToLower(info)
		if StringArrayContains(attributeList, info) {
			return true
		}
	}
	return false
}

func GetElementParam(element *rod.Element, param string) (string, error) {
	if StringArrayContains(ElementAttribute, param) {
		return GetAttribute(element, param)
	} else if StringArrayContains(ElementProperty, param) {
		return GetProperty(element, param)
	}
	return "", nil
}

func GetAttribute(element *rod.Element, attr string) (string, error) {
	attribute, err := element.Attribute(attr)
	if err != nil {
		return "", err
	}
	if attribute == nil {
		return "", nil
	}
	return *attribute, nil
}

func GetProperty(element *rod.Element, prop string) (string, error) {
	property, err := element.Property(prop)
	if err != nil {
		return "", err
	}
	if property.Nil() {
		return "", nil
	}
	return property.String(), nil
}

var getSelector = `()=>{
    function getElementIdentifier(el) {
        let identifier = el.tagName.toLowerCase();
        let identifiers = [];
        
        // 1. 尝试所有可能的有效标识
        if (el.id) {
            identifiers.push('#' + el.id);
        }
        
        if (el.name) {
            identifiers.push(identifier + '[name="' + el.name + '"]');
        }
        
        if (el.type) {
            identifiers.push(identifier + '[type="' + el.type + '"]');
        }
        
        // 2. 使用placeholder属性（登录表单常用）
        if (el.placeholder) {
            identifiers.push(identifier + '[placeholder="' + el.placeholder + '"]');
        }
        
        // 3. 使用value属性（按钮常用）
        if (el.value && (el.type === 'submit' || el.type === 'button')) {
            identifiers.push(identifier + '[value="' + el.value + '"]');
        }
        
        // 4. 使用class属性
        if (el.className) {
            let classes = el.className.split(' ')
                .filter(c => c && c.length > 0)
                .filter(c => {
                    // 过滤掉明显的动态类名
                    return !c.match(/^(ng-|react-|js-|dynamic-|generated-|[0-9])/i) &&
                           !c.includes('active') &&
                           !c.includes('hidden') &&
                           !c.includes('show') &&
                           !c.includes('disabled');
                });
            
            if (classes.length > 0) {
                // 使用最短的类名
                let shortestClass = classes.reduce((a, b) => a.length <= b.length ? a : b);
                identifiers.push(identifier + '.' + shortestClass);
            }
        }
        
        // 5. 检查周围的文本内容
        let prevEl = el.previousElementSibling;
        if (prevEl && prevEl.textContent) {
            let text = prevEl.textContent.trim();
            if (text && text.length < 20) {  // 只使用短文本
                identifiers.push(identifier + '[aria-label="' + text + '"]');
            }
        }
        
        // 6. 如果没有找到任何有效标识，使用位置索引
        if (identifiers.length === 0) {
            let parent = el.parentNode;
            if (parent) {
                let similarElements = parent.querySelectorAll(identifier);
                if (similarElements.length > 1) {
                    for (let i = 0; i < parent.children.length; i++) {
                        if (parent.children[i] === el) {
                            identifiers.push(identifier + ':nth-child(' + (i + 1) + ')');
                            break;
                        }
                    }
                } else {
                    identifiers.push(identifier);
                }
            }
        }
        
        // 7. 测试每个标识符的唯一性，返回最简单且唯一的那个
        for (let id of identifiers) {
            if (document.querySelectorAll(id).length === 1) {
                return id;
            }
        }
        
        // 如果没有唯一标识，返回第一个或基本标识
        return identifiers[0] || identifier;
    }
    
    function isUniqueSelector(selector) {
        try {
            return document.querySelectorAll(selector).length === 1;
        } catch (e) {
            return false;
        }
    }
    
    let el = this;
    let path = [];
	// 增加最大路径长度以提高准确性
    let maxPathLength = 5;
    
    while (el && el.nodeType === 1) {
        let identifier = getElementIdentifier(el);
        
        // 如果找到了id选择器，直接使用
        if (identifier.startsWith('#')) {
            path = [identifier];
            break;
        }
        
        path.unshift(identifier);
        
        // 检查当前路径是否已经足够唯一
        let selector = path.join(' > ');
        if (isUniqueSelector(selector)) {
            break;
        }
        
        // 如果路径太长，尝试不同的组合方式
        if (path.length >= maxPathLength) {
            // 尝试使用空格而不是直接子元素选择器
            selector = path.join(' ');
            if (isUniqueSelector(selector)) {
                return selector;
            }
            
            // 还是不行，保留开头和结尾的几个元素
            if (path.length > 3) {
                let start = path.slice(0, 2);
                let end = path.slice(-2);
                selector = start.join(' > ') + ' ' + end.join(' > ');
                if (isUniqueSelector(selector)) {
                    return selector;
                }
            }
            
            break;
        }
        
        el = el.parentNode;
    }
    
    // 返回最终的选择器
    let finalSelector = path.join(' > ');
    
    // 如果选择器无效或找不到元素，尝试使用更宽松的选择器
    if (!isUniqueSelector(finalSelector)) {
        // 尝试只使用最后两个元素
        if (path.length >= 2) {
            let shortSelector = path.slice(-2).join(' > ');
            if (isUniqueSelector(shortSelector)) {
                return shortSelector;
            }
        }
        
        // 尝试只使用最后一个元素
        if (path.length >= 1) {
            let lastSelector = path[path.length - 1];
            if (isUniqueSelector(lastSelector)) {
                return lastSelector;
            }
        }
    }
    
    return finalSelector;
}`

func ElementsToSelectors(elements ...*rod.Element) []string {
	selectors := make([]string, 0)
	for _, element := range elements {
		selector, err := GetSelector(element)
		if err != nil {
			log.Error(err)
			continue
		}
		selectors = append(selectors, selector)
	}
	return selectors
}

var getName = `()=>{
    let result = this.tagName.toLowerCase();
	if (this.id !== ""){
		result += "#" + this.id;
	}
	if (this.className !== ""){
		result += "." + this.className;
    }
	return result
}`

func ElementsToIds(elements ...*rod.Element) []string {
	ids := make([]string, 0)
	for _, element := range elements {
		obj, err := element.Eval(getName)
		if err != nil {
			log.Error(err)
			continue
		}
		ids = append(ids, obj.Value.String())
	}
	return ids
}

func ElementsToValue(elements rod.Elements, eval string) []string {
	results := make([]string, 0)
	for _, element := range elements {
		value, err := ElementToValue(element, eval)
		if err != nil {
			log.Error(err)
			continue
		}
		results = append(results, value)
	}
	return results
}

func ElementToValue(element *rod.Element, eval string) (string, error) {
	obj, err := element.Eval(eval)
	if err != nil {
		return "", utils.Error(err)
	}
	return obj.Value.String(), nil
}

func GetElement(page *rod.Page, selector string) (*rod.Element, error) {
	elements, err := page.Elements(selector)
	if err != nil {
		return nil, utils.Error(err)
	}
	if len(elements) == 0 {
		return nil, utils.Errorf(`%v not found`, selector)
	}
	element := elements.First()
	return element, nil
}

func ElementInput(page *rod.Page, selector, inputInfo string) (err error) {
	element, err := GetElement(page, selector)
	if err != nil {
		return
	}
	err = element.SelectAllText()
	if err != nil {
		return
	}
	err = element.Type(input.Backspace)
	if err != nil {
		return
	}
	inputKeys := []input.Key(inputInfo)
	err = element.Type(inputKeys...)
	if err != nil {
		return
	}
	return
}

func ElementClick(page *rod.Page, selector string) (err error) {
	element, err := GetElement(page, selector)
	if err != nil {
		return
	}
	return element.Click(proto.InputMouseButtonLeft, 1)
}

func FindLatestElement(page *rod.Page, origin, tagName string, maxLevel int) (rod.Elements, error) {
	originElements, err := page.Elements(origin)
	if err != nil {
		return nil, utils.Error(err)
	}
	if len(originElements) == 0 {
		return nil, utils.Errorf(`element %v not found`, origin)
	}
	originElement := originElements.First()
	elements, err := originElement.Elements(tagName)
	if err != nil {
		return nil, utils.Error(err)
	}
	if len(elements) != 0 {
		return elements, nil
	}
	parent, err := originElement.Parent()
	if err != nil {
		return nil, utils.Error(err)
	}
	currentLevel := 0
	for parent != nil {
		elements, err := parent.Elements(tagName)
		if err != nil {
			return nil, utils.Error(err)
		}
		if len(elements) != 0 {
			return elements, nil
		}
		if currentLevel >= maxLevel {
			break
		}
		currentLevel++
		parent, err = parent.Parent()
		if err != nil {
			return nil, utils.Error(err)
		}
	}
	return nil, utils.Errorf(`cannot find %v's latest %v element`, origin, tagName)
}

func GetSelector(element *rod.Element) (string, error) {
	obj, err := element.Eval(getSelector)
	if err != nil {
		return "", utils.Error(err)
	}
	return obj.Value.String(), nil
}

func CheckTagElementFromParent(elements rod.Elements, tags []string) (map[string]string, error) {
	result := make(map[string]string)
	for _, tag := range tags {
		result[tag] = ""
	}
	for _, element := range elements {
		if len(tags) == 0 {
			break
		}
		tag, err := checkTagElementFromParent(element, tags)
		if err != nil {
			log.Errorf("check element tag error: %v", err)
			continue
		}
		if tag != "" {
			selector, err := GetSelector(element)
			if err != nil {
				return result, utils.Errorf("get element selector error: %v", err)
			}
			result[tag] = selector
			tags = ListRemove(tags, tag)
		}
	}
	return result, nil
}

func checkTagElementFromParent(element *rod.Element, tags []string) (string, error) {
	for i := 0; i < maxParentLevel; i++ {
		element, err := element.Parent()
		if err != nil {
			return "", utils.Errorf("get element parent error: %v", err)
		}
		outer, err := ElementToValue(element, `()=>this.outerHTML.replace(this.innerHTML, "")`)
		if err != nil {
			return "", utils.Errorf("get element html error: %v", err)
		}
		for _, tag := range tags {
			simpleElementTypeList, _ := SimpleKeywordDict[tag]
			if ArrayStringContains(simpleElementTypeList, outer) {
				return tag, nil
			}
		}
	}
	return "", nil
}

func ElementsFilter(elements rod.Elements, selectors []string) rod.Elements {
	result := make(rod.Elements, 0)
	for _, element := range elements {
		elementSelector, err := GetSelector(element)
		if err != nil {
			log.Errorf("get element selector error: %v", err)
			continue
		}
		if !StringArrayContains(selectors, elementSelector) {
			result = append(result, element)
		}
	}
	return result
}
