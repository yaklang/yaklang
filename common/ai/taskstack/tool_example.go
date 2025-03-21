package taskstack

import (
	"encoding/json"
	"fmt"
	"io"
)

// ExampleSearchTool 创建一个示例搜索工具
func ExampleSearchTool() {
	// 创建搜索回调函数
	searchCallback := func(params map[string]interface{}, stdout io.Writer, stderr io.Writer) (interface{}, error) {
		query, _ := params["query"].(string)
		limit, _ := params["limit"].(float64)
		tags, _ := params["tags"].([]interface{})

		// 输出到标准输出
		fmt.Fprintf(stdout, "执行搜索查询: %s\n", query)
		fmt.Fprintf(stdout, "结果限制: %.0f\n", limit)
		if len(tags) > 0 {
			fmt.Fprintf(stdout, "标签: %v\n", tags)
		}

		// 模拟搜索结果
		results := []map[string]interface{}{}
		for i := 0; i < int(limit); i++ {
			results = append(results, map[string]interface{}{
				"id":    i + 1,
				"title": fmt.Sprintf("搜索结果 %d for %s", i+1, query),
				"score": 100 - i*10,
			})
		}

		// 输出到标准错误（如有必要）
		if len(results) == 0 {
			fmt.Fprintf(stderr, "警告: 没有找到结果\n")
		}

		// 返回结果
		return map[string]interface{}{
			"query":   query,
			"limit":   limit,
			"tags":    tags,
			"results": results,
		}, nil
	}

	// 创建搜索工具
	searchTool, err := NewTool("search",
		WithTool_Description("通用搜索工具"),
		WithTool_Callback(searchCallback),
		WithTool_StringParam("query",
			WithParam_Description("搜索查询字符串"),
			WithParam_Required(),
		),
		WithTool_NumberParam("limit",
			WithParam_Description("返回结果数量限制"),
			WithParam_Default(5),
		),
		WithTool_StringArrayParamEx("tags",
			[]PropertyOption{
				WithParam_Description("搜索标签"),
				WithParam_Required(),
			},
			WithParam_Description("标签名"),
			WithParam_Required(),
		),
	)

	if err != nil {
		fmt.Printf("创建工具错误: %v\n", err)
		return
	}

	// 获取工具的JSON Schema
	schemaStr := searchTool.ToJSONSchemaString()
	fmt.Println("工具的JSON Schema:")
	fmt.Println(schemaStr)

	// 使用工具
	jsonInput := `{
		"tool": "search",
		"@action": "invoke",
		"params": {
			"query": "golang",
			"limit": 3,
			"tags": ["programming", "language"]
		}
	}`

	// 调用工具
	result, err := searchTool.InvokeWithJSON(jsonInput)
	if err != nil {
		fmt.Printf("工具调用错误: %v\n", err)
		return
	}

	// 美化输出结果
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println("\n调用结果:")
	fmt.Println(string(resultJSON))

	// 使用参数映射直接调用工具
	params := map[string]interface{}{
		"query": "直接调用",
		"limit": 2,
		"tags":  []string{"test"},
	}

	// 直接调用
	result, err = searchTool.InvokeWithParams(params)
	if err != nil {
		fmt.Printf("直接调用错误: %v\n", err)
		return
	}

	// 输出结果
	resultJSON, _ = json.MarshalIndent(result, "", "  ")
	fmt.Println("\n直接调用结果:")
	fmt.Println(string(resultJSON))

	// 展示参数验证失败的情况
	invalidJSON := `{
		"tool": "search",
		"@action": "invoke",
		"params": {
			"limit": "not a number"
		}
	}`

	// 调用工具
	result, err = searchTool.InvokeWithJSON(invalidJSON)
	fmt.Println("\n无效参数调用结果:")
	resultJSON, _ = json.MarshalIndent(result, "", "  ")
	fmt.Println(string(resultJSON))
	fmt.Printf("错误: %v\n", err)
}

// ExampleJSONDefinedTool 展示如何从JSON定义创建工具
func ExampleJSONDefinedTool() {
	// 工具定义JSON
	toolDefJSON := `{
		"name": "calculator",
		"description": "简单计算器工具",
		"params": [
			{
				"name": "operation",
				"type": "string",
				"description": "运算类型: add, subtract, multiply, divide",
				"required": true
			},
			{
				"name": "a",
				"type": "number",
				"description": "第一个操作数",
				"required": true
			},
			{
				"name": "b",
				"type": "number",
				"description": "第二个操作数",
				"required": true
			}
		]
	}`

	// 创建计算器回调
	calcCallback := func(params map[string]interface{}, stdout io.Writer, stderr io.Writer) (interface{}, error) {
		operation, _ := params["operation"].(string)
		a, _ := params["a"].(float64)
		b, _ := params["b"].(float64)

		// 输出到标准输出
		fmt.Fprintf(stdout, "执行计算操作: %s\n", operation)
		fmt.Fprintf(stdout, "操作数: %.2f %s %.2f\n", a, getOperationSymbol(operation), b)

		var result float64

		switch operation {
		case "add":
			result = a + b
		case "subtract":
			result = a - b
		case "multiply":
			result = a * b
		case "divide":
			if b == 0 {
				fmt.Fprintf(stderr, "错误: 除数不能为零\n")
				return nil, fmt.Errorf("除数不能为零")
			}
			result = a / b
		default:
			fmt.Fprintf(stderr, "错误: 不支持的操作: %s\n", operation)
			return nil, fmt.Errorf("不支持的操作: %s", operation)
		}

		// 输出结果到标准输出
		fmt.Fprintf(stdout, "计算结果: %.2f\n", result)

		return map[string]interface{}{
			"operation": operation,
			"a":         a,
			"b":         b,
			"result":    result,
		}, nil
	}

	// 从JSON创建工具
	calcTool, err := NewToolFromJSON(toolDefJSON, calcCallback)
	if err != nil {
		fmt.Printf("从JSON创建工具错误: %v\n", err)
		return
	}

	// 输出创建的工具
	fmt.Println("\nJSON创建的工具:")
	fmt.Printf("名称: %s\n", calcTool.Name)
	fmt.Printf("描述: %s\n", calcTool.Description)
	fmt.Printf("参数数量: %d\n", len(calcTool.Params()))

	// 使用工具
	jsonInput := `{
		"tool": "calculator",
		"@action": "invoke",
		"params": {
			"operation": "add",
			"a": 10,
			"b": 5
		}
	}`

	// 调用工具
	result, err := calcTool.InvokeWithJSON(jsonInput)
	if err != nil {
		fmt.Printf("计算器调用错误: %v\n", err)
		return
	}

	// 输出结果
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println("\n计算器调用结果:")
	fmt.Println(string(resultJSON))
}

// getOperationSymbol 获取操作符号
func getOperationSymbol(operation string) string {
	switch operation {
	case "add":
		return "+"
	case "subtract":
		return "-"
	case "multiply":
		return "*"
	case "divide":
		return "/"
	default:
		return "?"
	}
}
