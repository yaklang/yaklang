package larkrobot

type MsgType string

const (
	// TextMsg 文本类型
	TextMsg MsgType = "text"
	// PostMsg 富文本
	PostMsg MsgType = "post"
	// ImageMsg 图片
	ImageMsg MsgType = "image"
	// InteractiveMsg 卡片
	InteractiveMsg MsgType = "interactive"
)

// Message feishu robot send message struct
type Message interface {
	// ToMessageMap for JSON serialization
	ToMessageMap() map[string]interface{}
}

// TextMessage 文本消息
type TextMessage struct {
	// Text 文本消息内容
	Text string
	// AtAll 是否@所有人
	AtAll bool
}

// NewTextMessage create TextMessage
func NewTextMessage(text string, atAll bool) *TextMessage {
	return &TextMessage{
		Text:  text,
		AtAll: atAll,
	}
}

func (text *TextMessage) ToMessageMap() map[string]interface{} {
	var isAtAll = ""
	if text.AtAll {
		isAtAll = "\r\n<at user_id=\"all\"> </at>"
	}
	textMsg := map[string]interface{}{}
	textMsg["text"] = text.Text + isAtAll
	msg := map[string]interface{}{}
	msg["msg_type"] = string(TextMsg) // 确保转换为字符串
	msg["content"] = textMsg
	return msg
}

// PostMessage post message
type PostMessage struct {
	LangItem []*LangPostItem
}

// NewPostMessage create PostMessage
func NewPostMessage(langItem ...*LangPostItem) *PostMessage {
	return &PostMessage{
		LangItem: langItem,
	}
}

// AddLangItem add LangPostItem
func (post *PostMessage) AddLangItem(langItem ...*LangPostItem) *PostMessage {
	post.LangItem = append(post.LangItem, langItem...)
	return post
}

func (post *PostMessage) ToMessageMap() map[string]interface{} {
	langMsg := map[string]interface{}{}
	for _, lang := range post.LangItem {
		for k, v := range lang.ToMessageMap() {
			langMsg[k] = v
		}
	}
	postMsg := map[string]interface{}{
		"post": langMsg,
	}
	msg := map[string]interface{}{}
	msg["msg_type"] = string(PostMsg) // 确保转换为字符串
	msg["content"] = postMsg
	return msg
}

// ImageMessage 图片
type ImageMessage struct {
	// ImageKey image_key
	ImageKey string
}

// NewImageMessage create ImageMessage
func NewImageMessage(imageKey string) *ImageMessage {
	return &ImageMessage{
		ImageKey: imageKey,
	}
}

func (image *ImageMessage) ToMessageMap() map[string]interface{} {
	imgMsg := map[string]interface{}{
		"image_key": image.ImageKey,
	}
	msg := map[string]interface{}{}
	msg["msg_type"] = string(ImageMsg) // 确保转换为字符串
	msg["content"] = imgMsg
	return msg
}

// InteractiveMessage 消息卡片
type InteractiveMessage struct {
	// Config 用于描述卡片的功能属性
	Config *CardConfig
	// Header 用于配置卡片标题内容
	Header *CardHeader
	// CarLink 指定卡片整体的点击跳转链接
	CarLink *CardLinkElement
	// Elements 用于定义卡片正文内容
	Elements []CardContent
	// i18nElements 为卡片的正文部分定义多语言内容
	i18nElements *I18nInteractiveElement
}

// NewInteractiveMessage create  InteractiveMessage
func NewInteractiveMessage() *InteractiveMessage {
	return &InteractiveMessage{
		Elements: []CardContent{},
	}
}

// SetConfig set InteractiveMessage.Config
func (message *InteractiveMessage) SetConfig(config *CardConfig) *InteractiveMessage {
	message.Config = config
	return message
}

// SetHeader set InteractiveMessage.Header
func (message *InteractiveMessage) SetHeader(header *CardHeader) *InteractiveMessage {
	message.Header = header
	return message
}

// SetCardLink set InteractiveMessage.CarLink
func (message *InteractiveMessage) SetCardLink(link *CardLinkElement) *InteractiveMessage {
	message.CarLink = link
	return message
}

// SetElements set InteractiveMessage.Elements
func (message *InteractiveMessage) SetElements(elements ...CardContent) *InteractiveMessage {
	message.Elements = elements
	return message
}

// AddElements add InteractiveMessage.Elements
func (message *InteractiveMessage) AddElements(elements ...CardContent) *InteractiveMessage {
	message.Elements = append(message.Elements, elements...)
	return message
}

// SetI18nElements set InteractiveMessage.i18nElements
func (message *InteractiveMessage) SetI18nElements(i18nElements *I18nInteractiveElement) *InteractiveMessage {
	message.i18nElements = i18nElements
	return message
}

func (message *InteractiveMessage) ToMessageMap() map[string]interface{} {
	interactiveMsg := map[string]interface{}{}
	if message.Header != nil {
		interactiveMsg["header"] = message.Header.ToMessage()
	}
	if message.Config != nil {
		interactiveMsg["config"] = message.Config.ToMessage()
	}
	if message.CarLink != nil {
		interactiveMsg["card_link"] = message.CarLink.ToMessage()
	}
	if len(message.Elements) > 0 {
		var eles []map[string]interface{}
		for _, ele := range message.Elements {
			eles = append(eles, ele.ToMessage())
		}
		interactiveMsg["elements"] = eles
	}
	if message.i18nElements != nil {
		interactiveMsg["i18n_elements"] = message.i18nElements.ToMap()
	}
	return map[string]interface{}{
		"msg_type": string(InteractiveMsg), // 确保转换为字符串
		"card":     interactiveMsg,
	}
}

// I18nInteractiveElement 为卡片的正文部分定义多语言内容
type I18nInteractiveElement struct {
	// Elements 多语言内容
	//
	//"en_us": [
	//			//English - card content
	//                  {module-1},
	//                  {module-2},
	//                  {module-3},
	//                  ......
	//			],
	//			"zh_cn": [
	//			//Chinese - card content
	//                  {module-1},
	//                  {module-2},
	//                  {module-3},
	//                  ......
	//			],
	Elements map[string][]CardContent
}

// NewI18nInteractiveElement create  I18nInteractiveElement
func NewI18nInteractiveElement(elements map[string][]CardContent) *I18nInteractiveElement {
	return &I18nInteractiveElement{
		Elements: elements,
	}
}

// Put add i18n contents
func (element *I18nInteractiveElement) Put(lang string, contents ...CardContent) *I18nInteractiveElement {
	eles, ok := element.Elements[lang]
	if ok {
		eles = append(eles, contents...)
	} else {
		eles = contents
	}
	element.Elements[lang] = eles
	return element
}

// ToMap to map
func (element *I18nInteractiveElement) ToMap() map[string]interface{} {
	msg := map[string]interface{}{}
	for k, eles := range element.Elements {
		var contents []map[string]interface{}
		for _, el := range eles {
			contents = append(contents, el.ToMessage())
		}
		msg[k] = contents
	}
	return msg
}
