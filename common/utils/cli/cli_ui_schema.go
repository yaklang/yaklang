package cli

type uiSchemaParams struct {
	// groups           []*uiSchemaGroup
	// globalFieldStyle uiSchemaFieldStyle
}

type uiSchemaGroup struct {
	// items []*uiSchemaField
}

type uiSchemaField struct {
	// fieldName string
	// width     int
	// Style     uiSchemaFieldStyle
}

type UISchemaFieldClassName string

const (
	UISchemaFieldPosDefault    UISchemaFieldClassName = ""
	UISchemaFieldPosHorizontal UISchemaFieldClassName = "json-schema-row-form"
)

type UISchemaWidgetType string

const (
	UISchemaWidgetDefault  UISchemaWidgetType = ""
	UISchemaWidgetTable    UISchemaWidgetType = "table"
	UISchemaWidgetRadio    UISchemaWidgetType = "radio"
	UISchemaWidgetSelect   UISchemaWidgetType = "select"
	UISchemaWidgetCheckbox UISchemaWidgetType = "checkbox"
	UISchemaWidgetTextArea UISchemaWidgetType = "textarea"
	UISchemaWidgetPassword UISchemaWidgetType = "password"
	UISchemaWidgetColor    UISchemaWidgetType = "color"
	UISchemaWidgetEmail    UISchemaWidgetType = "email"
	UISchemaWidgetUri      UISchemaWidgetType = "uri"
	UISchemaWidgetDate     UISchemaWidgetType = "date"
	UISchemaWidgetDateTime UISchemaWidgetType = "date-time"
	UISchemaWidgetTime     UISchemaWidgetType = "time"
	UISchemaWidgetUpdown   UISchemaWidgetType = "updown"
	UISchemaWidgetRange    UISchemaWidgetType = "range"
	UISchemaWidgetFile     UISchemaWidgetType = "file"
	UISchemaWidgetFiles    UISchemaWidgetType = "files"
	UISchemaWidgetFolder   UISchemaWidgetType = "folder"
)

type (
	UISchema            func(...UISchemaParams)
	UISchemaParams      func(*uiSchemaParams)
	UISchemaFieldParams func(*uiSchemaField)
)

// setUISchema 是一个选项参数,用于对JsonSchema设置的参数进行图形化的调整
// 详情参考:
// 1. https://json-schema.org/docs
// 2. https://rjsf-team.github.io/react-jsonschema-form/
// 3. https://rjsf-team.github.io/react-jsonschema-form/docs/api-reference/uiSchema/
// Example:
// ```
// info = cli.Json(
//
//	"info",
//	cli.setVerboseName("项目信息"),
//	cli.setJsonSchema(
//	    <<<JSON
//
// {"title":"A registration form","description":"A simple form example.","type":"object","required":["firstName","lastName"],"properties":{"name":{"type":"string","title":"Name","default":"Chuck"},"password":{"type":"string","title":"Password","minLength":3},"telephone":{"type":"string","title":"Telephone","minLength":10}}}
// JSON,
//
//	    cli.setUISchema(cli.uiGroups(
//	        cli.uiGroup(
//	            cli.uiField("name", 0.5),
//	            cli.uiField("password", 0.5, cli.uiFieldWidget(cli.uiWidgetPassword)),
//	        ),
//	        cli.uiGroup(cli.uiField("telephone", 1)),
//	    )),
//	),
//
// cli.setRequired(true),
// )
// cli.check()
// ```
func (c *CliApp) SetUISchema(params ...UISchemaParams) *UISchema {
	return new(UISchema)
}

// uiGlobalFieldPosition 是一个选项参数,用于指定UISchema中全局的字段位置,默认为垂直排列
// Example:
// ```
// info = cli.Json(
//
//	"info",
//	cli.setVerboseName("项目信息"),
//	cli.setJsonSchema(
//	    <<<JSON
//
// {"title":"A registration form","description":"A simple form example.","type":"object","required":["firstName","lastName"],"properties":{"name":{"type":"string","title":"Name","default":"Chuck"},"password":{"type":"string","title":"Password","minLength":3},"telephone":{"type":"string","title":"Telephone","minLength":10}}}
// JSON,
//
//	    cli.setUISchema(cli.uiGlobalFieldPosition(cli.uiPosHorizontal)),
//	),
//
// cli.setRequired(true),
// )
// cli.check()
// ```
func (c *CliApp) SetUISchemaGlobalFieldPosition(style UISchemaFieldClassName) UISchemaParams {
	return func(*uiSchemaParams) {}
}

// uiGroups 是一个选项参数,用于指定UISchema中的字段整体分组,接受多个分组(cli.uiGroup)
// Example:
// ```
// info = cli.Json(
//
//	"info",
//	cli.setVerboseName("项目信息"),
//	cli.setJsonSchema(
//	    <<<JSON
//
// {"title":"A registration form","description":"A simple form example.","type":"object","required":["firstName","lastName"],"properties":{"name":{"type":"string","title":"Name","default":"Chuck"},"password":{"type":"string","title":"Password","minLength":3},"telephone":{"type":"string","title":"Telephone","minLength":10}}}
// JSON,
//
//	    cli.setUISchema(cli.uiGroups(
//	        cli.uiGroup(
//	            cli.uiField("name", 0.5),
//	            cli.uiField("password", 0.5, cli.uiFieldWidget(cli.uiWidgetPassword)),
//	        ),
//	        cli.uiGroup(cli.uiField("telephone", 1)),
//	    )),
//	),
//
// cli.setRequired(true),
// )
// cli.check()
// ```
func (c *CliApp) SetUISchemaGroups(groups ...uiSchemaGroup) UISchemaParams {
	return func(*uiSchemaParams) {}
}

// uiGroup 是一个选项参数,用于指定UISchema中的一个分组,接收多个字段(cli.Field),同一分组的字段会放在一行
// Example:
// ```
// info = cli.Json(
//
//	"info",
//	cli.setVerboseName("项目信息"),
//	cli.setJsonSchema(
//	    <<<JSON
//
// {"title":"A registration form","description":"A simple form example.","type":"object","required":["firstName","lastName"],"properties":{"name":{"type":"string","title":"Name","default":"Chuck"},"password":{"type":"string","title":"Password","minLength":3},"telephone":{"type":"string","title":"Telephone","minLength":10}}}
// JSON,
//
//	    cli.setUISchema(cli.uiGroups(
//	        cli.uiGroup(
//	            cli.uiField("name", 0.5),
//	            cli.uiField("password", 0.5, cli.uiFieldWidget(cli.uiWidgetPassword)),
//	        ),
//	        cli.uiGroup(cli.uiField("telephone", 1)),
//	    )),
//	),
//
// cli.setRequired(true),
// )
// cli.check()
// ```
func (c *CliApp) NewUISchemaGroup(fields ...uiSchemaField) *uiSchemaGroup {
	return new(uiSchemaGroup)
}

// uiField 是一个选项参数,用于指定UISchema中的一个字段
// 第一个参数指定字段名
// 第二个参数指定这个字段所占的宽度比(0.0-1.0)
// 接下来可以接收零个到多个选项，用于对此字段进行其他的设置,例如内嵌分组(cli.uiFieldGroups)或者指定其部件(cli.uiFieldWidget)
// Example:
// ```
// info = cli.Json(
//
//	"info",
//	cli.setVerboseName("项目信息"),
//	cli.setJsonSchema(
//	    <<<JSON
//
// {"title":"A registration form","description":"A simple form example.","type":"object","required":["firstName","lastName"],"properties":{"name":{"type":"string","title":"Name","default":"Chuck"},"password":{"type":"string","title":"Password","minLength":3},"telephone":{"type":"string","title":"Telephone","minLength":10}}}
// JSON,
//
//	    cli.setUISchema(cli.uiGroups(
//	        cli.uiGroup(
//	            cli.uiField("name", 0.5),
//	            cli.uiField("password", 0.5, cli.uiFieldWidget(cli.uiWidgetPassword)),
//	        ),
//	        cli.uiGroup(cli.uiField("telephone", 1)),
//	    )),
//	),
//
// cli.setRequired(true),
// )
// cli.check()
// ```
func (c *CliApp) NewUISchemaField(name string, widthPercent float64, opts ...UISchemaFieldParams) *uiSchemaField {
	return new(uiSchemaField)
}

// uiTableField 是一个选项参数,用于指定UISchema中的一个表格字段
// 第一个参数指定字段名
// 第二个参数指定这个字段所占宽度
// 接下来可以接收零个到多个选项，用于对此字段进行其他的设置,例如内嵌分组(cli.uiFieldGroups)或者指定其部件(cli.uiFieldWidget)
// Example:
// ```
// args = cli.Json(
//
//	"kv",
//	cli.setVerboseName("键值对abc"),
//	cli.setJsonSchema(
//	    <<<JSON
//
// {"type":"object","properties":{"kvs":{"type":"array","title":"键值对","minItems":1,"items":{"properties":{"key":{"type":"string","title":"键"},"value":{"type":"string","title":"值"}},"require":["key","value"]}}}}
// JSON,
// cli.setUISchema(
//
//	cli.uiGroups(
//	    cli.uiGroup(
//	        cli.uiField("kvs", 1, cli.uiFieldWidget(cli.uiWidgetTable), cli.uiFieldGroups(
//	            cli.uiGroup(
//	                cli.uiField("items", 1, cli.uiFieldGroups(
//	                    cli.uiGroup(
//	                        cli.uiTableField("key", 100),
//	                        cli.uiTableField("value", 100),
//	                    )
//	                ))
//	            )
//	        ))
//	    )
//	)
//
// ),
//
// ),
//
//	cli.setRequired(true),
//
// )
// cli.check()
// ```
func (c *CliApp) NewUISchemaTableField(name string, width float64, opts ...UISchemaFieldParams) *uiSchemaField {
	return new(uiSchemaField)
}

// uiFieldPosition 是一个选项参数,用于指定UISchema中的字段位置,默认为垂直排列
// Example:
// ```
// info = cli.Json(
//
//	"info",
//	cli.setVerboseName("项目信息"),
//	cli.setJsonSchema(
//	    <<<JSON
//
// {"title":"A registration form","description":"A simple form example.","type":"object","required":["firstName","lastName"],"properties":{"name":{"type":"string","title":"Name","default":"Chuck"},"password":{"type":"string","title":"Password","minLength":3},"telephone":{"type":"string","title":"Telephone","minLength":10}}}
// JSON,
//
//	    cli.setUISchema(cli.uiGroups(
//	        cli.uiGroup(
//	            cli.uiField("name", 0.5),
//	            cli.uiField("password", 0.5, cli.uiFieldWidget(cli.uiWidgetPassword)),
//	        ),
//	        cli.uiGroup(cli.uiField("telephone", 1, cli.uiFieldPosition(cli.uiPosHorizontal))),
//	    )),
//	),
//
// cli.setRequired(true),
// )
// cli.check()
// ```
func (c *CliApp) SetUISchemaFieldPosition(position UISchemaFieldClassName) UISchemaFieldParams {
	return func(*uiSchemaField) {}
}

func (c *CliApp) SetUISchemaFieldStyle(css map[string]any) UISchemaFieldParams {
	return func(*uiSchemaField) {}
}

// uiFieldComponentStyle 是一个选项参数,用于指定UISchema中的CSS样式
// Example:
// ```
// info = cli.Json(
//
//	"info",
//	cli.setVerboseName("项目信息"),
//	cli.setJsonSchema(
//	    <<<JSON
//
// {"title":"A registration form","description":"A simple form example.","type":"object","required":["firstName","lastName"],"properties":{"name":{"type":"string","title":"Name","default":"Chuck"},"password":{"type":"string","title":"Password","minLength":3},"telephone":{"type":"string","title":"Telephone","minLength":10}}}
// JSON,
//
//	    cli.setUISchema(cli.uiGroups(
//	        cli.uiGroup(
//	            cli.uiField("name", 0.5),
//	            cli.uiField("password", 0.5, cli.uiFieldWidget(cli.uiWidgetPassword)),
//	        ),
//	        cli.uiGroup(cli.uiField(
//	            "telephone",
//	            1,
//	            cli.uiFieldComponentStyle({"width": "50%"}),
//	        )),
//	    )),
//	),
//
// cli.setRequired(true),
// )
// cli.check()
// ```
func (c *CliApp) SetUISchemaFieldComponentStyle(css map[string]any) UISchemaFieldParams {
	return func(*uiSchemaField) {}
}

// uiFieldWidget 是一个选项参数,用于指定UISchema中的字段使用的组件
// Example:
// ```
// info = cli.Json(
//
//	"info",
//	cli.setVerboseName("项目信息"),
//	cli.setJsonSchema(
//	    <<<JSON
//
// {"title":"A registration form","description":"A simple form example.","type":"object","required":["firstName","lastName"],"properties":{"name":{"type":"string","title":"Name","default":"Chuck"},"password":{"type":"string","title":"Password","minLength":3},"telephone":{"type":"string","title":"Telephone","minLength":10}}}
// JSON,
//
//	    cli.setUISchema(cli.uiGroups(
//	        cli.uiGroup(
//	            cli.uiField("name", 0.5),
//	            cli.uiField("password", 0.5, cli.uiFieldWidget(cli.uiWidgetPassword)),
//	        ),
//	        cli.uiGroup(cli.uiField("telephone", 1)),
//	    )),
//	),
//
// cli.setRequired(true),
// )
// cli.check()
// ```
func (c *CliApp) SetUISchemaFieldWidget(widget UISchemaWidgetType) UISchemaFieldParams {
	return func(*uiSchemaField) {}
}

// uiFieldGroups 是一个选项参数,用于设置UISchema中字段的嵌套组
// Example:
// ```
// info = cli.Json(
//
//	"info",
//	cli.setVerboseName("项目信息"),
//	cli.setJsonSchema(
//	    <<<JSON
//
// {"title":"A list of tasks","type":"object","required":["title"],"properties":{"title":{"type":"string","title":"Task list title"},"tasks":{"type":"array","title":"Tasks","items":{"type":"object","required":["title"],"properties":{"title":{"type":"string","title":"Title","description":"A sample title"},"details":{"type":"string","title":"Task details","description":"Enter the task details"},"done":{"type":"boolean","title":"Done?","default":false}}}}}}
// JSON,
//
//	    cli.setUISchema(cli.uiGroups(
//	        cli.uiGroup(cli.uiField("title", 1)),
//	        cli.uiGroup(cli.uiField(
//	            "tasks",
//	            1,
//	            cli.uiFieldGroups(cli.uiGroup(cli.uiField(
//	                "items",
//	                1,
//	                cli.uiFieldGroups(
//	                    cli.uiGroup(cli.uiField("title", 1)),
//	                    cli.uiGroup(cli.uiField("details", 1, cli.uiFieldWidget(cli.uiWidgetTextarea))),
//	                    cli.uiGroup(cli.uiField("done", 1)),
//	                ),
//	            ))),
//	        )),
//	    )),
//	),
//
// cli.setRequired(true),
// )
// cli.check()
// ```
func (c *CliApp) SetUISchemaInnerGroups(groups ...uiSchemaGroup) UISchemaFieldParams {
	return func(*uiSchemaField) {}
}
