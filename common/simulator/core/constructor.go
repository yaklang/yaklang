package core

import (
	"fmt"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"golang.org/x/net/context"
	"yaklang/common/log"
	"yaklang/common/simulator/config"
	"yaklang/common/utils"
	"time"
)

type GeneralPage struct {
	currentPage *rod.Page
	url         string
	wait        func()
	context     context.Context
}

type GeneralElement struct {
	element  *rod.Element
	page     *GeneralPage
	name     string
	selector string
	context  context.Context
}

type GeneralElements []*GeneralElement

func (generalPage *GeneralPage) Info() string {
	return fmt.Sprintf("<Page: %s, Url: %s>", generalPage.currentPage, generalPage.url)
}

func (generalPage *GeneralPage) String() string {
	return fmt.Sprintf("<page: %s, url: %s>", generalPage.currentPage.String(), generalPage.url)
}

func (generalPage *GeneralPage) createWait() {
	wait := generalPage.currentPage.WaitRequestIdle(time.Second, nil, nil)
	generalPage.wait = wait
}

func (generalPage *GeneralPage) Url() string {
	return generalPage.url
}

func (generalPage *GeneralPage) Close() {
	generalPage.currentPage.Close()
}

func (generalPage *GeneralPage) OriginPage() *rod.Page {
	return generalPage.currentPage
}

func CreateElement(element *rod.Element, page *GeneralPage) *GeneralElement {
	newElement := &GeneralElement{
		element: element,
		page:    page,
		context: page.context,
	}
	newElement.Selector()
	return newElement
}

func (generalElement *GeneralElement) String() string {
	if generalElement.name == "" {
		result, err := generalElement.element.Eval(GETNAME)
		if err != nil {
			log.Errorf("element doing eval find name error: %s", err)
			generalElement.name = "unknown"
		} else {
			generalElement.name = result.Value.Str()
		}
	}
	return fmt.Sprintf("<element: %s>", generalElement.name)
}

func (generalElement *GeneralElement) Selector() string {
	if generalElement.selector == "" {
		result, err := generalElement.element.Eval(getSelectorNew)
		if err != nil {
			log.Errorf("element doing eval find selector error: %s", err)
			generalElement.selector = "unknown"
		} else {
			generalElement.selector = result.Value.Str()
		}
	}
	return generalElement.selector
}

func (generalElement *GeneralElement) Url() string {
	return generalElement.page.url
}

func (generalElement *GeneralElement) Origin() *rod.Element {
	return generalElement.element
}

func (generalElement *GeneralElement) HTML() string {
	html, err := generalElement.element.HTML()
	if err != nil {
		return ""
	}
	return html
}

func (generalElements GeneralElements) String() string {
	var result string = ""
	result += "< elements: "
	for _, generalElement := range generalElements {
		result += generalElement.String() + ", "
	}
	result += ">"
	return result
}

func (generalElements GeneralElements) First() *GeneralElement {
	if generalElements.Empty() {
		return nil
	}
	return generalElements[0]
}

func (generalElements GeneralElements) Last() *GeneralElement {
	if generalElements.Empty() {
		return nil
	}
	return generalElements[len(generalElements)-1]
}

func (generalElements GeneralElements) Single() bool {
	return len(generalElements) == 1
}

func (generalElements GeneralElements) Multi() bool {
	return len(generalElements) > 1
}

func (generalElements GeneralElements) Empty() bool {
	return len(generalElements) == 0
}

func (generalElements GeneralElements) Length() int {
	return len(generalElements)
}

func (generalElements *GeneralElements) Slice(generalElement *GeneralElement) *GeneralElements {
	tmp := make(GeneralElements, 0, generalElements.Length())
	generalElementSelector := generalElement.Selector()
	for _, element := range *generalElements {
		selector := element.Selector()
		if selector != generalElementSelector {
			tmp = append(tmp, element)
		}
	}
	return &tmp
}

func CreatePage(conf config.PageConfig) (*GeneralPage, error) {
	if conf.Url() == "" {
		return nil, utils.Errorf("url not nil.")
	}
	proxy, proxyUser, proxyPass := conf.Proxy()
	page := &GeneralPage{
		url: conf.Url(),
	}

	browser := rod.New()

	if conf.WsAddress() != "" {
		launch, _ := launcher.NewManaged(conf.WsAddress())
		launchCtx := context.Background()
		launch = launch.Context(launchCtx)
		if proxy != "" {
			launch.Proxy(proxy)
		}
		serviceUrl, header := launch.ClientHeader()
		client, _ := cdp.StartWithURL(launchCtx, serviceUrl, header)
		browser = browser.Client(client)
	} else {
		launch := launcher.New()
		if proxy != "" {
			launch.Proxy(proxy)
		}
		controlUrl, _ := launch.Launch()
		browser = browser.ControlURL(controlUrl)
	}

	browser = browser.Context(conf.Context())
	err := browser.Connect()
	if err != nil {
		return nil, utils.Errorf("browser connection error: %s", err)
	}
	if proxyUser != "" && proxyPass != "" {
		go browser.MustHandleAuth(proxyUser, proxyPass)
	}
	//if strings.Contains(proxy, "localhost") || strings.Contains(proxy, "127.0.0.1") {
	browser.MustIgnoreCertErrors(true)

	rodPage, err := browser.Page(proto.TargetCreateTarget{
		URL: "about:blank",
	})
	if err != nil {
		return nil, utils.Errorf("create page error: %s", err)
	}
	rodPage.MustNavigate(conf.Url())
	rodPage.MustWaitLoad()
	page.currentPage = rodPage
	page.context = conf.Context()
	page.createWait()
	page.wait()
	return page, nil
}
