package larkrobot

// PostTagType 标签
type PostTagType string

const (
	// TextPostTagType text
	TextPostTagType PostTagType = "text"
	// APostTagType a
	APostTagType PostTagType = "a"
	// AtPostTagType at
	AtPostTagType PostTagType = "at"
	// ImgPostTagType img
	ImgPostTagType PostTagType = "img"
)

// PostTag 标签
type PostTag interface {
	// ToPostTagMessage  for JSON serialization
	ToPostTagMessage() map[string]interface{}
}

// TextTag 文本标签
type TextTag struct {
	// Text 文本内容
	Text string
	// UnEscape 表示是不是 unescape 解码，默认为 false ，不用可以不填
	UnEscape bool
}

// NewTextTag create TextTag
func NewTextTag(text string) *TextTag {
	return &TextTag{
		Text: text,
	}
}

// SetUnEscape set TextTag UnEscape
func (tag *TextTag) SetUnEscape(unEscape bool) *TextTag {
	tag.UnEscape = unEscape
	return tag
}

func (tag *TextTag) ToPostTagMessage() map[string]interface{} {
	msg := map[string]interface{}{}
	msg["tag"] = TextPostTagType
	msg["text"] = tag.Text
	msg["un_escape"] = tag.UnEscape
	return msg
}

// ATag a链接标签
type ATag struct {
	// Text 	文本内容
	Text string
	// Href 默认的链接地址
	Href string
}

// NewATag create ATag
func NewATag(text, href string) *ATag {
	return &ATag{
		text,
		href,
	}
}

func (tag *ATag) ToPostTagMessage() map[string]interface{} {
	msg := map[string]interface{}{}
	msg["tag"] = APostTagType
	msg["text"] = tag.Text
	msg["href"] = tag.Href
	return msg
}

// AtTag at标签
type AtTag struct {
	// UserId open_id，union_id或user_id
	UserId string
	// UserName 用户姓名
	UserName string
}

// NewAtAllAtTag create at_all AtTag
func NewAtAllAtTag() *AtTag {
	return &AtTag{
		UserId:   "all",
		UserName: "所有人",
	}
}

// NewAtTag create AtTag
func NewAtTag(userId string) *AtTag {
	return &AtTag{
		UserId: userId,
	}
}

// SetUserName set UserName
func (tag *AtTag) SetUserName(username string) *AtTag {
	tag.UserName = username
	return tag
}
func (tag *AtTag) ToPostTagMessage() map[string]interface{} {
	msg := map[string]interface{}{}
	msg["tag"] = AtPostTagType
	msg["user_id"] = tag.UserId
	msg["user_name"] = tag.UserName
	return msg
}

// ImgTag img tag
type ImgTag struct {
	ImageKey string
}

// NewImgTag create ImgTag
func NewImgTag(imgKey string) *ImgTag {
	return &ImgTag{
		imgKey,
	}
}
func (tag *ImgTag) ToPostTagMessage() map[string]interface{} {
	return map[string]interface{}{
		"tag":       ImgPostTagType,
		"image_key": tag.ImageKey,
	}
}

// PostTags post tag list
type PostTags struct {
	// PostTags post tag
	PostTags []PostTag
}

// NewPostTags create PostTags
func NewPostTags(tags ...PostTag) *PostTags {
	return &PostTags{
		PostTags: tags,
	}
}

// AddTags add post PostTag
func (tag *PostTags) AddTags(tags ...PostTag) *PostTags {
	tag.PostTags = append(tag.PostTags, tags...)
	return tag
}

// ToMessageMap to array message map
func (tag *PostTags) ToMessageMap() []map[string]interface{} {
	var postTags []map[string]interface{}
	for _, tags := range tag.PostTags {
		postTags = append(postTags, tags.ToPostTagMessage())
	}
	return postTags
}

// PostItems 富文本 段落
type PostItems struct {
	// Title 标题
	Title string
	// Content  段落
	Content []*PostTags
}

// NewPostItems create PostItems
func NewPostItems(title string, content ...*PostTags) *PostItems {
	return &PostItems{
		Title:   title,
		Content: content,
	}
}

// AddContent add PostItems Content
func (items *PostItems) AddContent(content ...*PostTags) *PostItems {
	items.Content = append(items.Content, content...)
	return items
}

func (items *PostItems) ToMessageMap() map[string]interface{} {
	var contentList [][]map[string]interface{}
	for _, content := range items.Content {
		contentList = append(contentList, content.ToMessageMap())
	}
	msg := map[string]interface{}{}
	msg["title"] = items.Title
	msg["content"] = contentList
	return msg
}

// LangPostItem language post item
type LangPostItem struct {
	// Lang language
	Lang string
	// Item PostItems
	Item *PostItems
}

// NewZhCnLangPostItem create zh_cn language post item
func NewZhCnLangPostItem(item *PostItems) *LangPostItem {
	return NewLangPostItem("zh_cn", item)
}

// NewLangPostItem create LangPostItem
func NewLangPostItem(lang string, item *PostItems) *LangPostItem {
	return &LangPostItem{
		lang,
		item,
	}
}

func (post *LangPostItem) ToMessageMap() map[string]interface{} {
	return map[string]interface{}{
		post.Lang: post.Item.ToMessageMap(),
	}
}

// CardInternal card message internal
type CardInternal interface {
	// ToMessage to message map
	ToMessage() map[string]interface{}
}

// CardConfig 卡片属性
type CardConfig struct {
	// EnableForward 是否允许卡片被转发,默认 true
	EnableForward bool
	// UpdateMulti 是否为共享卡片,默认为false
	UpdateMulti bool
	// WideScreenMode 是否根据屏幕宽度动态调整消息卡片宽度,默认值为true
	//
	// 2021/03/22之后，此字段废弃，所有卡片均升级为自适应屏幕宽度的宽版卡片
	WideScreenMode bool
}

// NewCardConfig create CardConfig
func NewCardConfig() *CardConfig {
	return &CardConfig{
		EnableForward:  true,
		UpdateMulti:    true,
		WideScreenMode: true,
	}
}

// SetEnableForward set EnableForward
func (config *CardConfig) SetEnableForward(enableForward bool) *CardConfig {
	config.EnableForward = enableForward
	return config
}

// SetUpdateMulti set UpdateMulti
func (config *CardConfig) SetUpdateMulti(updateMulti bool) *CardConfig {
	config.UpdateMulti = updateMulti
	return config
}

// SetWideScreenMode set WideScreenMode
func (config *CardConfig) SetWideScreenMode(wideScreenMode bool) *CardConfig {
	config.WideScreenMode = wideScreenMode
	return config
}

func (config *CardConfig) ToMessage() map[string]interface{} {
	return map[string]interface{}{
		"wide_screen_mode": config.WideScreenMode,
		"enable_forward":   config.EnableForward,
		"update_multi":     config.UpdateMulti,
	}
}

// CardTitle 标题
type CardTitle struct {
	// Content 内容
	Content string
	// I18n i18n替换content
	//
	//  "i18n": {
	//      "zh_cn": "中文文本",
	//      "en_us": "English text",
	//      "ja_jp": "日本語文案"
	//     }
	I18n map[string]string
}

// NewCardTitle create CardTitle
func NewCardTitle(content string, i18n map[string]string) *CardTitle {
	return &CardTitle{
		Content: content,
		I18n:    i18n,
	}
}

// SetI18n set I18n
func (title *CardTitle) SetI18n(i18n map[string]string) *CardTitle {
	title.I18n = i18n
	return title
}
func (title *CardTitle) ToMessage() map[string]interface{} {
	return map[string]interface{}{
		"content": title.Content,
		"i18n":    title.I18n,
		"tag":     "plain_text",
	}
}

// CardHeaderTemplate  CardHeader.Template
type CardHeaderTemplate string

const (
	// Blue  CardHeader.Template blue
	Blue CardHeaderTemplate = "blue"
	// Wathet  CardHeader.Template wathet
	Wathet CardHeaderTemplate = "wathet"
	// Turquoise  CardHeader.Template turquoise
	Turquoise CardHeaderTemplate = "turquoise"
	// Green  CardHeader.Template green
	Green CardHeaderTemplate = "green"
	// Yellow  CardHeader.Template yellow
	Yellow CardHeaderTemplate = "yellow"
	// Orange  CardHeader.Template orange
	Orange CardHeaderTemplate = "orange"
	// Red  CardHeader.Template red
	Red CardHeaderTemplate = "red"
	// Carmine  CardHeader.Template carmine
	Carmine CardHeaderTemplate = "carmine"
	// Violet  CardHeader.Template violet
	Violet CardHeaderTemplate = "violet"
	// Purple  CardHeader.Template purple
	Purple CardHeaderTemplate = "purple"
	// Indigo  CardHeader.Template indigo
	Indigo CardHeaderTemplate = "indigo"
	// Grey  CardHeader.Template grey
	Grey CardHeaderTemplate = "grey"
)

// CardHeader 卡片标题
type CardHeader struct {
	Title    *CardTitle
	Template CardHeaderTemplate
}

// NewCardHeader create CardHeader
func NewCardHeader(title *CardTitle) *CardHeader {
	return &CardHeader{
		Title: title,
	}
}

// SetTemplate  set Template
func (header *CardHeader) SetTemplate(template CardHeaderTemplate) *CardHeader {
	header.Template = template
	return header
}

func (header *CardHeader) ToMessage() map[string]interface{} {
	return map[string]interface{}{
		"title":    header.Title.ToMessage(),
		"template": header.Template,
	}
}

// CardContent 卡片内容
type CardContent interface {
	CardInternal
	// GetContentTag 卡片内容 tag标签
	GetContentTag() string
}

// CardElement 内容模块
type CardElement struct {
	// Text 单个文本展示，和fields至少要有一个
	Text *CardText
	// Fields 多个文本展示，和text至少要有一个
	Fields []*CardField
	// Extra 附加的元素展示在文本内容右侧
	//
	// 可附加的元素包括image、button、selectMenu、overflow、datePicker
	Extra CardInternal
}

// NewCardElement create CardElement
func NewCardElement(text *CardText, fields ...*CardField) *CardElement {
	return &CardElement{
		Text:   text,
		Fields: fields,
	}
}

// AddFields add CardElement.Fields
func (card *CardElement) AddFields(field ...*CardField) *CardElement {
	card.Fields = append(card.Fields, field...)
	return card
}

// SetExtra set CardElement.Extra
func (card *CardElement) SetExtra(extra CardInternal) *CardElement {
	card.Extra = extra
	return card
}
func (card *CardElement) GetContentTag() string {
	return "div"
}
func (card *CardElement) ToMessage() map[string]interface{} {
	msg := map[string]interface{}{}
	var fields []map[string]interface{}
	for _, field := range card.Fields {
		fields = append(fields, field.ToMessage())
	}
	msg["tag"] = card.GetContentTag()
	if card.Text != nil {
		msg["text"] = card.Text.ToMessage()
	}
	msg["fields"] = fields
	if card.Extra != nil {
		msg["extra"] = card.Extra.ToMessage()
	}
	return msg
}

// CardMarkdown Markdown模块
type CardMarkdown struct {
	// Content 	使用已支持的markdown语法构造markdown内容
	Content string
	// Href 差异化跳转
	Href *UrlElement
	// UrlVal 绑定变量
	UrlVal string
}

// NewCardMarkdown create CardMarkdown
func NewCardMarkdown(content string) *CardMarkdown {
	return &CardMarkdown{
		Content: content,
	}
}

// SetHref set CardMarkdown.Href
func (card *CardMarkdown) SetHref(url *UrlElement) *CardMarkdown {
	card.Href = url
	return card
}
func (card *CardMarkdown) GetContentTag() string {
	return "markdown"
}
func (card *CardMarkdown) ToMessage() map[string]interface{} {
	msg := map[string]interface{}{}
	msg["tag"] = card.GetContentTag()
	msg["content"] = card.Content
	if card.Href != nil {
		href := map[string]map[string]interface{}{
			card.UrlVal: card.Href.ToMessage(),
		}
		msg["href"] = href
	}
	return msg
}
func (card *CardMarkdown) ToMessageMap() map[string]interface{} {
	return card.ToMessage()
}

// CardHr 分割线模块
type CardHr struct {
}

// NewCardHr create CardHr
func NewCardHr() *CardHr {
	return &CardHr{}
}
func (hr *CardHr) GetContentTag() string {
	return "hr"
}
func (hr *CardHr) ToMessage() map[string]interface{} {
	return map[string]interface{}{
		"tag": hr.GetContentTag(),
	}
}

// CardImgMode CardImg.Mode
type CardImgMode string

const (
	// FitHorizontal 平铺模式，宽度撑满卡片完整展示上传的图片。 该属性会覆盖
	FitHorizontal CardImgMode = "fit_horizontal"
	// CropCenter  居中裁剪模式，对长图会限高，并居中裁剪后展示
	CropCenter CardImgMode = "crop_center"
)

// CardImg 图片模块
type CardImg struct {
	// ImgKey 图片资源
	ImgKey string
	// Alt 	hover图片时弹出的Tips文案,content取值为空时则不展示
	Alt *CardText
	// Title 图片标题
	Title *CardText
	// CustomWidth 自定义图片的最大展示宽度
	CustomWidth int
	// CompactWidth 	是否展示为紧凑型的图片 默认为false
	CompactWidth bool
	// Mode 图片显示模式 默认 crop_center
	Mode CardImgMode
	// Preview 点击后是否放大图片，缺省为true
	Preview bool
}

// NewCardImg create CardImg
func NewCardImg(ImgKey string, Alt *CardText) *CardImg {
	return &CardImg{
		ImgKey:       ImgKey,
		Alt:          Alt,
		CompactWidth: false,
		Mode:         CropCenter,
		Preview:      true,
	}
}

// SetTitle set CardImg.Title
func (img *CardImg) SetTitle(title *CardText) *CardImg {
	img.Title = title
	return img
}

// SetCustomWidth set CardImg.CustomWidth
func (img *CardImg) SetCustomWidth(customWidth int) *CardImg {
	img.CustomWidth = customWidth
	return img
}

// SetCompactWidth set CardImg.CompactWidth
func (img *CardImg) SetCompactWidth(compactWidth bool) *CardImg {
	img.CompactWidth = compactWidth
	return img
}

// SetMode set CardImg.Mode
func (img *CardImg) SetMode(mode CardImgMode) *CardImg {
	img.Mode = mode
	return img
}

// SetPreview set CardImg.Preview
func (img *CardImg) SetPreview(preview bool) *CardImg {
	img.Preview = preview
	return img
}
func (img *CardImg) GetContentTag() string {
	return "img"
}
func (img *CardImg) ToMessage() map[string]interface{} {
	msg := map[string]interface{}{}
	msg["tag"] = img.GetContentTag()
	msg["img_key"] = img.ImgKey
	msg["alt"] = img.Alt.ToMessage()
	if img.Title != nil {
		msg["title"] = img.Title.ToMessage()
	}
	if img.CustomWidth != 0 {
		msg["custom_width"] = img.CustomWidth
	}
	msg["compact_width"] = img.CompactWidth
	msg["mode"] = img.Mode
	msg["preview"] = img.Preview
	return msg
}

// CardNote 备注模块,用于展示次要信息
//
// 使用备注模块来展示用于辅助说明或备注的次要信息，支持小尺寸的图片和文本
type CardNote struct {
	// Elements 备注信息 text对象或image元素
	Elements []CardInternal
}

// NewCardNote create CardNote
func NewCardNote(elements ...CardInternal) *CardNote {
	return &CardNote{
		Elements: elements,
	}
}

// AddElements add CardNote.Elements
func (note *CardNote) AddElements(elements ...CardInternal) *CardNote {
	note.Elements = append(note.Elements, elements...)
	return note
}
func (note *CardNote) GetContentTag() string {
	return "note"
}
func (note *CardNote) ToMessage() map[string]interface{} {
	msg := map[string]interface{}{}
	var eles []map[string]interface{}
	for _, ele := range note.Elements {
		eles = append(eles, ele.ToMessage())
	}
	msg["tag"] = note.GetContentTag()
	msg["elements"] = eles
	return msg
}

// CardField 用于内容模块的field字段
type CardField struct {
	// Short 是否并排布局
	Short bool
	// Text 	国际化文本内容
	Text *CardText
}

// NewCardField create CardField
func NewCardField(short bool, text *CardText) *CardField {
	return &CardField{
		short,
		text,
	}
}
func (field *CardField) ToMessage() map[string]interface{} {
	return map[string]interface{}{
		"is_short": field.Short,
		"text":     field.Text.ToMessage(),
	}
}

// CardTextTag 卡片内容-可内嵌的非交互元素-text-tag属性
type CardTextTag string

const (
	// Text 文本
	Text CardTextTag = "plain_text"
	// Md markdown
	Md CardTextTag = "lark_md"
)

// CardText 卡片内容-可内嵌的非交互元素-text
type CardText struct {
	// Tag 元素标签
	Tag CardTextTag
	// Content 	文本内容
	Content string
	// Lines 内容显示行数
	Lines int
}

// NewCardText create CardText
func NewCardText(tag CardTextTag, content string) *CardText {
	return &CardText{
		Tag:     tag,
		Content: content,
	}
}

// SetLines set CardText Lines
func (text *CardText) SetLines(lines int) *CardText {
	text.Lines = lines
	return text
}

func (text *CardText) ToMessage() map[string]interface{} {
	return map[string]interface{}{
		"tag":     text.Tag,
		"content": text.Content,
		"lines":   text.Lines,
	}
}

// CardImage 作为图片元素被使用
// 可用于内容块的extra字段和备注块的elements字段。
type CardImage struct {
	// ImageKey 图片资源
	ImageKey string
	// Alt 图片hover说明
	Alt *CardText
	// Preview 点击后是否放大图片，缺省为true
	Preview bool
}

// NewCardImage create CardImage
func NewCardImage(imageKye string, alt *CardText) *CardImage {
	return &CardImage{
		ImageKey: imageKye,
		Alt:      alt,
		Preview:  true,
	}
}

// SetPreview set Preview
func (image *CardImage) SetPreview(preview bool) *CardImage {
	image.Preview = preview
	return image
}

func (image *CardImage) GetContentTag() string {
	return "img"
}
func (image *CardImage) ToMessage() map[string]interface{} {
	return map[string]interface{}{
		"tag":     image.GetContentTag(),
		"img_key": image.ImageKey,
		"alt":     image.Alt.ToMessage(),
		"preview": image.Preview,
	}
}

// LayoutAction 交互元素布局
type LayoutAction string

const (
	// Bisected 二等分布局，每行两列交互元素
	Bisected LayoutAction = "bisected"
	// Trisection 三等分布局，每行三列交互元素
	Trisection LayoutAction = "trisection"
	// Flow 流式布局元素会按自身大小横向排列并在空间不够的时候折行
	Flow LayoutAction = "flow"
)

// CardAction 交互模块
type CardAction struct {
	// Actions 交互元素
	Actions []ActionElement
	//  Layout 交互元素布局
	Layout LayoutAction
}

// NewCardAction create CardAction
func NewCardAction(actions ...ActionElement) *CardAction {
	return &CardAction{
		Actions: actions,
	}
}

// AddAction add CardAction.Actions
func (action *CardAction) AddAction(actions ...ActionElement) *CardAction {
	action.Actions = append(action.Actions, actions...)
	return action
}

// SetLayout set CardAction.Layout
func (action *CardAction) SetLayout(layout LayoutAction) *CardAction {
	action.Layout = layout
	return action
}
func (action *CardAction) GetContentTag() string {
	return "action"
}
func (action *CardAction) ToMessage() map[string]interface{} {
	msg := map[string]interface{}{}
	var actions []map[string]interface{}
	for _, a := range action.Actions {
		actions = append(actions, a.ToMessage())
	}
	msg["tag"] = action.GetContentTag()
	msg["actions"] = actions
	msg["layout"] = action.Layout
	return msg
}

// ActionElement 交互元素
type ActionElement interface {
	CardInternal
	// GetActionTag ActionElement tag
	GetActionTag() string
}

// DatePickerTag DatePickerActionElement.Tag
type DatePickerTag string

const (
	// DatePicker 日期
	DatePicker DatePickerTag = "date_picker"
	// PickerTime 时间
	PickerTime DatePickerTag = "picker_time"
	// PickerDatetime 日期+时间
	PickerDatetime DatePickerTag = "picker_datetime"
)

// DatePickerActionElement 提供时间选择的功能
//
// 可用于内容块的extra字段和交互块的actions字段。
type DatePickerActionElement struct {
	// Tag tag
	Tag DatePickerTag
	// InitialDate 日期模式的初始值 格式"yyyy-MM-dd"
	InitialDate string
	// InitialTime 时间模式的初始值 格式"HH:mm"
	InitialTime string
	// InitialDatetime 日期时间模式的初始值 	格式"yyyy-MM-dd HH:mm"
	InitialDatetime string
	// Placeholder 占位符，无初始值时必填
	Placeholder *CardText
	// Value 用户选定后返回业务方的数据 JSON
	Value map[string]interface{}
	// Confirm 二次确认的弹框
	Confirm *ConfirmElement
}

// NewDatePickerActionElement create DatePickerActionElement
func NewDatePickerActionElement(tag DatePickerTag) *DatePickerActionElement {
	return &DatePickerActionElement{Tag: tag}
}

// SetInitialDate set DatePickerActionElement.InitialDate
func (datePicker *DatePickerActionElement) SetInitialDate(initialDate string) *DatePickerActionElement {
	datePicker.InitialDate = initialDate
	return datePicker
}

// SetInitialTime set DatePickerActionElement.InitialTime
func (datePicker *DatePickerActionElement) SetInitialTime(initialTime string) *DatePickerActionElement {
	datePicker.InitialTime = initialTime
	return datePicker
}

// SetInitialDatetime set DatePickerActionElement.InitialDatetime
func (datePicker *DatePickerActionElement) SetInitialDatetime(initialDatetime string) *DatePickerActionElement {
	datePicker.InitialDatetime = initialDatetime
	return datePicker
}

// SetPlaceholder set DatePickerActionElement.Placeholder
func (datePicker *DatePickerActionElement) SetPlaceholder(placeholder *CardText) *DatePickerActionElement {
	datePicker.Placeholder = placeholder
	return datePicker
}

// SetValue set DatePickerActionElement.Value
func (datePicker *DatePickerActionElement) SetValue(value map[string]interface{}) *DatePickerActionElement {
	datePicker.Value = value
	return datePicker
}

// SetConfirm set DatePickerActionElement.Confirm
func (datePicker *DatePickerActionElement) SetConfirm(confirm *ConfirmElement) *DatePickerActionElement {
	datePicker.Confirm = confirm
	return datePicker
}
func (datePicker *DatePickerActionElement) GetActionTag() string {
	return string(datePicker.Tag)
}
func (datePicker *DatePickerActionElement) ToMessage() map[string]interface{} {
	msg := map[string]interface{}{}
	msg["tag"] = datePicker.Tag
	if len(datePicker.InitialDate) > 0 {
		msg["initial_date"] = datePicker.InitialDate
	}
	if len(datePicker.InitialTime) > 0 {
		msg["initial_time"] = datePicker.InitialTime
	}
	if len(datePicker.InitialDatetime) > 0 {
		msg["initial_datetime"] = datePicker.InitialDatetime
	}
	if datePicker.Placeholder != nil {
		msg["placeholder"] = datePicker.Placeholder.ToMessage()
	}
	if len(datePicker.Value) > 0 {
		msg["value"] = datePicker.Value
	}
	if datePicker.Confirm != nil {
		msg["confirm"] = datePicker.Confirm.ToMessage()
	}
	return msg
}

// OverflowActionElement 提供折叠的按钮型菜单
//
// overflow属于交互元素的一种，可用于内容块的extra字段和交互块的actions字段。
type OverflowActionElement struct {
	// Options 待选选项
	Options []*OptionElement
	// Value 用户选定后返回业务方的数据
	Value map[string]interface{}
	// Confirm 二次确认的弹框
	Confirm *ConfirmElement
}

// NewOverflowActionElement create OverflowActionElement
func NewOverflowActionElement(options ...*OptionElement) *OverflowActionElement {
	return &OverflowActionElement{
		Options: options,
	}
}

// AddOptions add OverflowActionElement.Options
func (overflow *OverflowActionElement) AddOptions(options ...*OptionElement) *OverflowActionElement {
	overflow.Options = append(overflow.Options, options...)
	return overflow
}

// SetValue set OverflowActionElement.Value
func (overflow *OverflowActionElement) SetValue(value map[string]interface{}) *OverflowActionElement {
	overflow.Value = value
	return overflow
}

// SetConfirm set OverflowActionElement.Confirm
func (overflow *OverflowActionElement) SetConfirm(confirm *ConfirmElement) *OverflowActionElement {
	overflow.Confirm = confirm
	return overflow
}
func (overflow *OverflowActionElement) GetActionTag() string {
	return "overflow"
}
func (overflow *OverflowActionElement) ToMessage() map[string]interface{} {
	msg := map[string]interface{}{}
	msg["tag"] = overflow.GetActionTag()
	var options []map[string]interface{}
	for _, option := range overflow.Options {
		options = append(options, option.ToMessage())
	}
	msg["options"] = options
	if len(overflow.Value) > 0 {
		msg["value"] = overflow.Value

	}
	if overflow.Confirm != nil {
		msg["confirm"] = overflow.Confirm.ToMessage()
	}
	return msg
}

// SelectMenuTag SelectMenuActionElement tag
type SelectMenuTag string

const (
	// SelectStatic  SelectMenuActionElement select_static tag 选项模式
	SelectStatic SelectMenuTag = "select_static"
	// SelectPerson  SelectMenuActionElement select_person tag 选人模式
	SelectPerson SelectMenuTag = "select_person"
)

// SelectMenuActionElement 作为selectMenu元素被使用，提供选项菜单的功能
type SelectMenuActionElement struct {
	// Tag tag
	Tag SelectMenuTag
	// Placeholder 占位符，无默认选项时必须有
	Placeholder *CardText
	// InitialOption 默认选项的value字段值。select_person模式下不支持此配置。
	InitialOption string
	// Options 	待选选项
	Options []*OptionElement
	// Value 用户选定后返回业务方的数据 	key-value形式的json结构，且key为String类型
	Value map[string]interface{}
	// Confirm 	二次确认的弹框
	Confirm *ConfirmElement
}

// NewSelectMenuActionElement create SelectMenuActionElement
func NewSelectMenuActionElement(tag SelectMenuTag) *SelectMenuActionElement {
	return &SelectMenuActionElement{
		Tag:     tag,
		Options: []*OptionElement{},
	}
}

// SetPlaceholder set SelectMenuActionElement.Placeholder
func (selectMenu *SelectMenuActionElement) SetPlaceholder(placeholder *CardText) *SelectMenuActionElement {
	selectMenu.Placeholder = placeholder
	return selectMenu
}

// SetInitialOption set SelectMenuActionElement.InitialOption
func (selectMenu *SelectMenuActionElement) SetInitialOption(initialOption string) *SelectMenuActionElement {
	selectMenu.InitialOption = initialOption
	return selectMenu
}

// SetOptions set SelectMenuActionElement.Options
func (selectMenu *SelectMenuActionElement) SetOptions(options ...*OptionElement) *SelectMenuActionElement {
	selectMenu.Options = options
	return selectMenu
}

// AddOptions add SelectMenuActionElement.Options
func (selectMenu *SelectMenuActionElement) AddOptions(options ...*OptionElement) *SelectMenuActionElement {
	selectMenu.Options = append(selectMenu.Options, options...)
	return selectMenu
}

// SetValue set SelectMenuActionElement.Value
func (selectMenu *SelectMenuActionElement) SetValue(value map[string]interface{}) *SelectMenuActionElement {
	selectMenu.Value = value
	return selectMenu
}

// SetConfirm set SelectMenuActionElement.Confirm
func (selectMenu *SelectMenuActionElement) SetConfirm(confirm *ConfirmElement) *SelectMenuActionElement {
	selectMenu.Confirm = confirm
	return selectMenu
}
func (selectMenu *SelectMenuActionElement) GetActionTag() string {
	return string(selectMenu.Tag)
}
func (selectMenu *SelectMenuActionElement) ToMessage() map[string]interface{} {
	msg := map[string]interface{}{}
	msg["tag"] = selectMenu.Tag
	if selectMenu.Placeholder != nil {
		msg["placeholder"] = selectMenu.Placeholder.ToMessage()
	}
	msg["initial_option"] = selectMenu.InitialOption
	if len(selectMenu.Options) > 0 {
		var options []map[string]interface{}
		for _, option := range selectMenu.Options {
			options = append(options, option.ToMessage())
		}
		msg["options"] = options
	}
	if len(selectMenu.Value) > 0 {
		msg["value"] = selectMenu.Value

	}
	if selectMenu.Confirm != nil {
		msg["confirm"] = selectMenu.Confirm.ToMessage()
	}
	return msg
}

// ButtonType ButtonActionElement.ButtonType
type ButtonType string

const (
	// DefaultType  default 次要按钮
	DefaultType ButtonType = "default"
	// PrimaryType  primary 主要按钮
	PrimaryType ButtonType = "primary"
	// DangerType  danger 警示按钮
	DangerType ButtonType = "danger"
)

// ButtonActionElement 交互组件, 可用于内容块的extra字段和交互块的actions字段
type ButtonActionElement struct {
	// Text 按钮中的文本
	Text *CardText
	// Url 跳转链接，和 ButtonActionElement.MultiUrl 互斥
	Url string
	//  MultiUrl 	多端跳转链接
	MultiUrl *UrlElement
	// ButtonType 配置按钮样式，默认为"default"
	ButtonType ButtonType
	// Value 点击后返回业务方,	仅支持key-value形式的json结构，且key为String类型。
	Value map[string]interface{}
	// Confirm 	二次确认的弹框
	Confirm *ConfirmElement
}

// NewButtonActionElement create ButtonActionElement
func NewButtonActionElement(text *CardText) *ButtonActionElement {
	return &ButtonActionElement{
		Text:       text,
		ButtonType: DefaultType,
	}
}

// SetUrl set ButtonActionElement.Url
func (button *ButtonActionElement) SetUrl(url string) *ButtonActionElement {
	button.Url = url
	return button
}

// SetMultiUrl set ButtonActionElement.MultiUrl
func (button *ButtonActionElement) SetMultiUrl(multiUrl *UrlElement) *ButtonActionElement {
	button.MultiUrl = multiUrl
	return button
}

// SetType set ButtonActionElement.ButtonType
func (button *ButtonActionElement) SetType(buttonType ButtonType) *ButtonActionElement {
	button.ButtonType = buttonType
	return button
}

// SetValue set ButtonActionElement.Value
func (button *ButtonActionElement) SetValue(value map[string]interface{}) *ButtonActionElement {
	button.Value = value
	return button
}

// SetConfirm set ButtonActionElement.Confirm
func (button *ButtonActionElement) SetConfirm(confirm *ConfirmElement) *ButtonActionElement {
	button.Confirm = confirm
	return button
}
func (button *ButtonActionElement) GetActionTag() string {
	return "button"
}
func (button *ButtonActionElement) ToMessage() map[string]interface{} {
	msg := map[string]interface{}{}
	msg["tag"] = button.GetActionTag()
	msg["text"] = button.Text.ToMessage()
	msg["url"] = button.Url
	if button.MultiUrl != nil {
		msg["multi_url"] = button.MultiUrl.ToMessage()
	}
	msg["type"] = button.ButtonType
	if len(button.Value) > 0 {
		msg["value"] = button.Value
	}
	if button.Confirm != nil {
		msg["confirm"] = button.Confirm.ToMessage()
	}
	return msg
}

// CardLinkElement 指定卡片整体的点击跳转链接
type CardLinkElement struct {
	*UrlElement
}

// NewCardLinkElement create CardLinkElement
func NewCardLinkElement(url string) *CardLinkElement {
	return &CardLinkElement{
		&UrlElement{
			Url: url,
		},
	}
}

// SetPcUrl set UrlElement.PcUrl
func (element *CardLinkElement) SetPcUrl(pcUrl string) *CardLinkElement {
	element.PcUrl = pcUrl
	return element
}

// SetIosUrl set UrlElement.IosUrl
func (element *CardLinkElement) SetIosUrl(iosUrl string) *CardLinkElement {
	element.IosUrl = iosUrl
	return element
}

// SetAndroidUrl set UrlElement.AndroidUrl
func (element *CardLinkElement) SetAndroidUrl(androidUrl string) *CardLinkElement {
	element.AndroidUrl = androidUrl
	return element
}
func (element *CardLinkElement) ToMessage() map[string]interface{} {
	return map[string]interface{}{
		"url":         element.Url,
		"pc_url":      element.PcUrl,
		"ios_url":     element.IosUrl,
		"android_url": element.AndroidUrl,
	}
}

// ConfirmElement  用于交互元素的二次确认
//
//	弹框默认提供确定和取消的按钮，无需开发者手动配置
type ConfirmElement struct {
	// Title 弹框标题 仅支持"plain_text"
	Title *CardText
	// Text 弹框内容  仅支持"plain_text"
	Text *CardText
}

// NewConfirmElement create ConfirmElement
func NewConfirmElement(title, text *CardText) *ConfirmElement {
	return &ConfirmElement{
		Text:  text,
		Title: title,
	}
}

func (element *ConfirmElement) ToMessage() map[string]interface{} {
	return map[string]interface{}{
		"title": element.Title.ToMessage(),
		"text":  element.Text.ToMessage(),
	}
}

// OptionElement
//
// 作为selectMenu的选项对象
//
// 作为overflow的选项对象
type OptionElement struct {
	// Text 选项显示内容，非待选人员时必填
	Text *CardText
	// Value 选项选中后返回业务方的数据，与url或multi_url必填其中一个
	Value string
	// Url *仅支持overflow，跳转指定链接，和multi_url字段互斥
	Url string
	// MultiUrl 	*仅支持overflow，跳转对应链接，和url字段互斥
	MultiUrl *UrlElement
}

// NewOptionElement create OptionElement
func NewOptionElement() *OptionElement {
	return &OptionElement{}
}

// SetText set OptionElement.Text
func (element *OptionElement) SetText(text *CardText) *OptionElement {
	element.Text = text
	return element
}

// SetValue set OptionElement.Value
func (element *OptionElement) SetValue(value string) *OptionElement {
	element.Value = value
	return element
}

// SetUrl set OptionElement.Url
func (element *OptionElement) SetUrl(url string) *OptionElement {
	element.Url = url
	return element
}

// SetMultiUrl set OptionElement.MultiUrl
func (element *OptionElement) SetMultiUrl(multiUrl *UrlElement) *OptionElement {
	element.MultiUrl = multiUrl
	return element
}

func (element *OptionElement) ToMessage() map[string]interface{} {
	msg := map[string]interface{}{}
	if element.Text != nil {
		msg["text"] = element.Text.ToMessage()
	}
	msg["value"] = element.Value
	msg["url"] = element.Url
	if element.MultiUrl != nil {
		msg["multi_url"] = element.MultiUrl.ToMessage()
	}
	return msg
}

// UrlElement url对象用作多端差异跳转链接
//
// 可用于button的multi_url字段，支持按键点击的多端跳转。
//
// 可用于lark_md类型text对象的href字段，支持超链接点击的多端跳转。
type UrlElement struct {
	// Url 	默认跳转链接
	Url string
	// AndroidUrl 	安卓端跳转链接
	AndroidUrl string
	// IosUrl 	ios端跳转链接
	IosUrl string
	// PcUrl 	pc端跳转链接
	PcUrl string
}

// NewUrlElement create UrlElement
func NewUrlElement(url, androidUrl, iosUrl, pcUrl string) *UrlElement {
	return &UrlElement{
		url,
		androidUrl,
		iosUrl,
		pcUrl,
	}
}
func (element *UrlElement) ToMessage() map[string]interface{} {
	return map[string]interface{}{
		"url":         element.Url,
		"android_url": element.AndroidUrl,
		"ios_url":     element.IosUrl,
		"pc_url":      element.PcUrl,
	}
}
