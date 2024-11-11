// Package simple
// @Author bcy2007  2024/11/11 15:16
package simple

import "github.com/go-rod/rod"

type VElement struct {
	element *rod.Element
}

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

func (element *VElement) Text() (string, error) {
	text, err := element.element.Text()
	if err != nil {
		return "", err
	}
	return text, nil
}

func (element *VElement) HTML() (string, error) {
	html, err := element.element.HTML()
	if err != nil {
		return "", err
	}
	return html, nil
}

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

type VElements []*VElement
