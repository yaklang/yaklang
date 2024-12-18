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

func (c *CliApp) SetUISchema(params ...UISchemaParams) *UISchema {
	return new(UISchema)
}

func (c *CliApp) SetUISchemaGlobalFieldPosition(style UISchemaFieldClassName) UISchemaParams {
	return func(*uiSchemaParams) {}
}

func (c *CliApp) SetUISchemaGroups(groups ...uiSchemaGroup) UISchemaParams {
	return func(*uiSchemaParams) {}
}

func (c *CliApp) NewUISchemaGroup(fields ...uiSchemaField) *uiSchemaGroup {
	return new(uiSchemaGroup)
}

func (c *CliApp) NewUISchemaField(name string, widthPercent float64, opts ...UISchemaFieldParams) *uiSchemaField {
	return new(uiSchemaField)
}

func (c *CliApp) SetUISchemaFieldPosition(position UISchemaFieldClassName) UISchemaFieldParams {
	return func(*uiSchemaField) {}
}

func (c *CliApp) SetUISchemaFieldStyle(m map[string]any) UISchemaFieldParams {
	return func(*uiSchemaField) {}
}

func (c *CliApp) SetUISchemaFieldComponentStyle(m map[string]any) UISchemaFieldParams {
	return func(*uiSchemaField) {}
}

func (c *CliApp) SetUISchemaFieldWidget(widget UISchemaWidgetType) UISchemaFieldParams {
	return func(*uiSchemaField) {}
}

func (c *CliApp) SetUISchemaInnerGroups(groups ...uiSchemaGroup) UISchemaFieldParams {
	return func(*uiSchemaField) {}
}
