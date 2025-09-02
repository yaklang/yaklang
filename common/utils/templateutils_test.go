package utils

import (
	"strings"
	"testing"
)

// 测试用的结构体
type TestStruct struct {
	Hello   string
	Name    string
	Age     int
	Enabled bool
}

type NestedStruct struct {
	User TestStruct
	Meta map[string]any
}

func TestRenderTemplate_MapData(t *testing.T) {
	tests := []struct {
		name     string
		template string
		data     map[string]any
		expected string
		hasError bool
	}{
		{
			name:     "simple string substitution",
			template: "{{ .Hello }}",
			data:     map[string]any{"Hello": "World"},
			expected: "World",
			hasError: false,
		},
		{
			name:     "multiple variables",
			template: "Hello {{ .Name }}, you are {{ .Age }} years old",
			data:     map[string]any{"Name": "Alice", "Age": 25},
			expected: "Hello Alice, you are 25 years old",
			hasError: false,
		},
		{
			name:     "with whitespace and newlines",
			template: "{{ .Hello }}\n{{ .World }}",
			data:     map[string]any{"Hello": "Hi", "World": "Earth"},
			expected: "Hi\nEarth",
			hasError: false,
		},
		{
			name:     "boolean value",
			template: "Status: {{ .Enabled }}",
			data:     map[string]any{"Enabled": true},
			expected: "Status: true",
			hasError: false,
		},
		{
			name:     "empty template",
			template: "",
			data:     map[string]any{"Hello": "World"},
			expected: "",
			hasError: false,
		},
		{
			name:     "template without variables",
			template: "Static text",
			data:     map[string]any{"Hello": "World"},
			expected: "Static text",
			hasError: false,
		},
		{
			name:     "missing variable in data",
			template: "{{ .Missing }}",
			data:     map[string]any{"Hello": "World"},
			expected: "<no value>",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderTemplate(tt.template, tt.data)

			if tt.hasError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.hasError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestRenderTemplate_StructData(t *testing.T) {
	tests := []struct {
		name     string
		template string
		data     any
		expected string
		hasError bool
	}{
		{
			name:     "simple struct field",
			template: "{{ .Hello }}",
			data:     TestStruct{Hello: "World"},
			expected: "World",
			hasError: false,
		},
		{
			name:     "multiple struct fields",
			template: "Name: {{ .Name }}, Age: {{ .Age }}",
			data:     TestStruct{Name: "Bob", Age: 30},
			expected: "Name: Bob, Age: 30",
			hasError: false,
		},
		{
			name:     "struct with boolean",
			template: "Enabled: {{ .Enabled }}",
			data:     TestStruct{Enabled: true},
			expected: "Enabled: true",
			hasError: false,
		},
		{
			name:     "nested struct access",
			template: "User: {{ .User.Name }}, Meta: {{ .Meta.version }}",
			data: NestedStruct{
				User: TestStruct{Name: "Charlie"},
				Meta: map[string]any{"version": "1.0"},
			},
			expected: "User: Charlie, Meta: 1.0",
			hasError: false,
		},
		{
			name:     "pointer to struct",
			template: "{{ .Hello }}",
			data:     &TestStruct{Hello: "Pointer"},
			expected: "Pointer",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderTemplate(tt.template, tt.data)

			if tt.hasError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.hasError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestRenderTemplate_ErrorCases(t *testing.T) {
	tests := []struct {
		name     string
		template string
		data     any
		hasError bool
	}{
		{
			name:     "invalid template syntax",
			template: "{{ .Hello",
			data:     map[string]any{"Hello": "World"},
			hasError: true,
		},
		{
			name:     "invalid template action",
			template: "{{ .Hello.NonExistent.Field }}",
			data:     map[string]any{"Hello": "World"},
			hasError: true,
		},
		{
			name:     "nil data",
			template: "{{ .Hello }}",
			data:     nil,
			hasError: false, // 应该不会出错，但会输出<no value>
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderTemplate(tt.template, tt.data)

			if tt.hasError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.hasError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			// 对于某些错误情况，即使有错误也应该有一些输出
			t.Logf("Result: %q, Error: %v", result, err)
		})
	}
}

func TestRenderTemplate_ComplexTemplates(t *testing.T) {
	tests := []struct {
		name     string
		template string
		data     any
		expected string
		hasError bool
	}{
		{
			name:     "template with conditionals",
			template: `{{if .Enabled}}Status: Active{{else}}Status: Inactive{{end}}`,
			data:     map[string]any{"Enabled": true},
			expected: "Status: Active",
			hasError: false,
		},
		{
			name:     "template with range",
			template: `{{range .Items}}Item: {{.}} {{end}}`,
			data:     map[string]any{"Items": []string{"A", "B", "C"}},
			expected: "Item: A Item: B Item: C ",
			hasError: false,
		},
		{
			name:     "template with functions",
			template: `{{.Name | printf "Hello, %s!"}}`,
			data:     map[string]any{"Name": "World"},
			expected: "Hello, World!",
			hasError: false,
		},
		{
			name: "multiline template",
			template: `Name: {{.Name}}
Age: {{.Age}}
Status: {{if .Enabled}}Active{{else}}Inactive{{end}}`,
			data: TestStruct{Name: "David", Age: 35, Enabled: true},
			expected: `Name: David
Age: 35
Status: Active`,
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderTemplate(tt.template, tt.data)

			if tt.hasError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.hasError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestRenderTemplate_TDDExamples(t *testing.T) {
	// 测试TDD注释中的具体示例
	t.Run("TDD example 1 - map data", func(t *testing.T) {
		result, err := RenderTemplate(`
{{ .Hello }}
`, map[string]any{
			"Hello": "World",
		})

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		expected := "\nWorld\n"
		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	})

	t.Run("TDD example 2 - struct instance", func(t *testing.T) {
		structInstance := TestStruct{Hello: "World"}
		result, err := RenderTemplate(`
{{ .Hello }}
`, structInstance)

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		expected := "\nWorld\n"
		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	})
}

// Benchmark测试
func BenchmarkRenderTemplate_SimpleMap(b *testing.B) {
	template := "Hello {{ .Name }}, you are {{ .Age }} years old"
	data := map[string]any{"Name": "Alice", "Age": 25}

	for i := 0; i < b.N; i++ {
		_, err := RenderTemplate(template, data)
		if err != nil {
			b.Errorf("Unexpected error: %v", err)
		}
	}
}

func BenchmarkRenderTemplate_SimpleStruct(b *testing.B) {
	template := "Hello {{ .Name }}, you are {{ .Age }} years old"
	data := TestStruct{Name: "Alice", Age: 25}

	for i := 0; i < b.N; i++ {
		_, err := RenderTemplate(template, data)
		if err != nil {
			b.Errorf("Unexpected error: %v", err)
		}
	}
}

func BenchmarkRenderTemplate_ComplexTemplate(b *testing.B) {
	template := `
Name: {{.Name}}
Age: {{.Age}}
Status: {{if .Enabled}}Active{{else}}Inactive{{end}}
{{range .Items}}Item: {{.}} {{end}}
`
	data := map[string]any{
		"Name":    "Alice",
		"Age":     25,
		"Enabled": true,
		"Items":   []string{"A", "B", "C"},
	}

	for i := 0; i < b.N; i++ {
		_, err := RenderTemplate(template, data)
		if err != nil {
			b.Errorf("Unexpected error: %v", err)
		}
	}
}

// 测试边界情况
func TestRenderTemplate_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		template string
		data     any
		hasError bool
	}{
		{
			name:     "empty data map",
			template: "{{ .Hello }}",
			data:     map[string]any{},
			hasError: false, // 应该输出<no value>
		},
		{
			name:     "zero value struct",
			template: "{{ .Hello }}",
			data:     TestStruct{},
			hasError: false, // 应该输出空字符串
		},
		{
			name:     "very long template",
			template: strings.Repeat("{{ .Hello }}", 1000),
			data:     map[string]any{"Hello": "A"},
			hasError: false,
		},
		{
			name:     "unicode in template",
			template: "你好 {{ .Name }}！欢迎使用 {{ .Product }}",
			data:     map[string]any{"Name": "张三", "Product": "Yaklang"},
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderTemplate(tt.template, tt.data)

			if tt.hasError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.hasError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}

			t.Logf("Result length: %d", len(result))
		})
	}
}
