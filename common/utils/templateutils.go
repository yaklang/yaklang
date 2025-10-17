package utils

import (
	"bytes"
	"text/template"

	"github.com/yaklang/yaklang/common/log"
)

// RenderTemplate 渲染模板字符串，支持map和struct作为数据源
// template: 模板字符串，使用Go模板语法
// data: 数据源，可以是map[string]any或struct实例
// 返回渲染后的字符串和可能的错误
func RenderTemplate(templateStr string, data any) (string, error) {
	// 创建新的模板实例
	tmpl, err := template.New("template").Parse(templateStr)
	if err != nil {
		log.Errorf("parse template failed: %v", err)
		return "", err
	}

	// 创建缓冲区存储渲染结果
	var buf bytes.Buffer

	// 执行模板渲染
	err = tmpl.Execute(&buf, data)
	if err != nil {
		log.Errorf("execute template failed: %v", err)
		return "", err
	}

	return buf.String(), nil
}

func MustRenderTemplate(templateStr string, data any) string {
	// 创建新的模板实例
	tmpl, err := template.New("template").Parse(templateStr)
	if err != nil {
		log.Errorf("parse template failed: %v", err)
		return templateStr
	}

	// 创建缓冲区存储渲染结果
	var buf bytes.Buffer

	// 执行模板渲染
	err = tmpl.Execute(&buf, data)
	if err != nil {
		log.Errorf("execute template failed: %v", err)
		if len(buf.String()) > 0 {
			return buf.String()
		}
		return templateStr
	}

	return buf.String()
}
