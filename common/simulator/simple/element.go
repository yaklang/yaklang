// Package simple
// @Author bcy2007  2024/11/11 15:16
package simple

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

type VElement struct {
	element *rod.Element
}

type VElements []*VElement

// Elements 返回该网页元素下的所有匹配css selector的子元素
// Example:
// page, _ = browser.Navigate("https://example.com", "")
// element, _ = page.Element("#test")
// elements, _ = element.Elements("#id")
func (element *VElement) Elements(selector string) (VElements, error) {
	var result = make(VElements, 0)
	elements, err := element.element.Elements(selector)
	if err != nil {
		return result, err
	}
	for _, e := range elements {
		result = append(result, &VElement{element: e})
	}
	return result, nil
}

// Text 返回一个网页元素所展示的文本内容
// Example:
// page, _ = browser.Navigate("https://example.com", "")
// element, _ = page.Element("#pageName") // 假设该元素为 <h2 id="pageName">welcome to our page</h2>
// text, _ = element.Text() // 返回 welcome to our page
func (element *VElement) Text() (string, error) {
	text, err := element.element.Text()
	if err != nil {
		return "", err
	}
	return text, nil
}

// HTML 返回一个网页元素的HTML内容
// Example:
// page, _ = browser.Navigate("https://example.com", "")
// element, _ = page.Element("#pageName") // 假设该元素为 <h2 id="pageName">welcome to our page</h2>
// html, _ = element.HTML() // 返回 <h2 id="pageName">welcome to our page</h2>
func (element *VElement) HTML() (string, error) {
	html, err := element.element.HTML()
	if err != nil {
		return "", err
	}
	return html, nil
}

// Attribute 返回一个网页元素的标签属性
// Example:
// page, _ = browser.Navigate("https://example.com", "")
// element, _ = page.Element("#pageName") // 假设该元素为 <h2 id="pageName">welcome to our page</h2>
// attribute, _ = element.Attribute("id") // 返回 pageName
func (element *VElement) Attribute(name string) (string, error) {
	value, err := element.element.Attribute(name)
	if err != nil {
		return "", err
	}
	if value == nil {
		return "", nil
	}
	return *value, nil
}

// Click 点击该网页元素
// Example:
// page, _ = browser.Navigate("https://example.com", "")
// element, _ = page.Element("#pageName")
// element.Click() // 点击id为pageName的元素
func (element *VElement) Click() error {
	return element.element.Click(proto.InputMouseButtonLeft, 1)
}

// Input 激活网页元素并输入内容
// 注意如果该元素不可输入或隐藏可能会卡死
// Example:
// page, _ = browser.Navigate("https://example.com", "")
// element, _ = page.Element("#pageName")
// element.Input("hello") // 在id为pageName的元素中输入hello
func (element *VElement) Input(info string) error {
	return element.element.Input(info)
}
