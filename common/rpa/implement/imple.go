package implement

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/rpa/captcha"
	"github.com/yaklang/yaklang/common/rpa/core"
	"github.com/yaklang/yaklang/common/utils"
	"regexp"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

const OBSERVER = `
()=>{
	const config = { attributes: true, childList: true, subtree: true, characterData: true };
	window.added = ""
	// 当观察到变动时执行的回调函数
	const callback = function(mutationsList, observer) {
		// Use traditional 'for loops' for IE 11
		for(let mutation of mutationsList) {
			if (mutation.type === 'childList') {
				for (let node of mutation.addedNodes) {
					// added += node.innerHTML;
					if (node.innerHTML !== undefined) {
						added += node.innerHTML
					} else if (node.data !== undefined){
						added += node.data
					} else {
						added += node.nodeValue
					} 
				}
			}
			else if (mutation.type === 'attributes') {
			}
			else if (mutation.type === 'characterData') {
				added += mutation.target.data;
			}
		}
	};
	// 创建一个观察器实例并传入回调函数
	window.observer = new MutationObserver(callback);
	// 以上述配置开始观察目标节点
	observer.observe(document, config);
}
`

const OBRESULT = `
()=>{
	observer.disconnect();
	return added;
}
`

type Runner struct {
	page    *rod.Page
	timeout int

	// domain
	Domain     string
	CaptchaUrl string
}

func (r *Runner) init() error {
	if r.timeout == 0 {
		r.timeout = 30
	}
	wsUrl, _ := launcher.New().Set("ignore-certificate-errors").Launch()
	browser := rod.New().ControlURL(wsUrl)
	err := browser.Connect()
	if err != nil {
		return utils.Errorf("create browser error: %s", err)
	}
	// page, err := browser.Timeout(time.Duration(r.timeout) * time.Second).Page(proto.TargetCreateTarget{
	page, err := browser.Page(proto.TargetCreateTarget{
		URL: "about:blank",
	})
	if err != nil {
		return utils.Errorf("create page error: %s", err)
	}
	r.page = page
	return nil
}

func (r *Runner) Init() error {
	return r.init()
}

func (r *Runner) Navigate(url string) error {
	err := r.page.Navigate(url)
	if err != nil {
		return utils.Errorf("create page error: %s", err)
	}
	r.page.MustWaitLoad()
	return nil
}

func (r *Runner) GetElement(ele string) (*rod.Element, error) {
	elements, err := r.GetElements(ele)
	if err != nil {
		return nil, err
	}
	if len(elements) == 0 {
		return nil, utils.Errorf("element: %s not found.", ele)
	}
	return elements[0], nil
}

func (r *Runner) GetElements(ele string) (rod.Elements, error) {
	elements, err := r.page.Elements(ele)
	if err != nil {
		return nil, utils.Errorf("get elements error: %s", err)
	}
	return elements, nil
}

func (r *Runner) GetKeywordElement(elements rod.Elements, keyword string) *RelatedElements {
	var readyElements []*rod.Element
	// fmt.Println("filter all elements: ", elements)
	for _, element := range elements {
		if simplecheckelement(element, keyword) {
			readyElements = append(readyElements, element)
		}
	}
	var result *rod.Element
	if len(readyElements) == 0 {
		if keyword == "captcha" {
			capelement, err := r.SearchCaptchafromIMG()
			if err != nil {
				return nil
			}
			return capelement
		}
		return nil
	} else if len(readyElements) > 1 {
		result = r.GetDeterminedElement(readyElements, keyword)
	} else {
		result = readyElements[0]
	}
	if keyword == "captcha" {
		capIMGElement, err := r.GetLatestElementofElement(result, "img")
		if err != nil {
			log.Infof("find captcha element but img not found。")
			return nil
		} else {
			capElement := &RelatedElements{
				Element:        *result,
				RelatedElement: capIMGElement,
			}
			return capElement
		}
	}
	return &RelatedElements{Element: *result, RelatedElement: nil}
}

func (r *Runner) GetDeterminedElement(elements rod.Elements, keyword string) *rod.Element {
	var maxpercent float32
	var result *rod.Element
	for _, element := range elements {
		_, percent := checkelement(element, keyword)
		if percent > maxpercent {
			maxpercent = percent
			result = element
		}
	}
	if result == nil {
		return nil
	}
	return result
}

func (r *Runner) GetLatestClickElement(element *rod.Element) (*rod.Element, error) {
	// find latest button or submit input from given element
	return getLatestClickElementofEachLevel(element, 0)
}

func (r *Runner) GetCurrentURL() (string, error) {
	result, err := r.page.Eval(`()=>document.URL`)
	if err != nil {
		return "", utils.Errorf("eval get current url error:%s", err)
	}
	return result.Value.Str(), nil
}

func (r *Runner) GetLatestElementofElement(element *rod.Element, target string) (*rod.Element, error) {
	return getLatestElementofElement(element, target, 0)
}

func getLatestElementofElement(element *rod.Element, target string, level int) (*rod.Element, error) {
	target_elements, _ := element.Elements(target)
	if len(target_elements) != 0 {
		return target_elements[0], nil
	}
	if level < 3 {
		parent, err := element.Parent()
		if err != nil {
			return nil, utils.Errorf("%s not found and no more parent.", target)
		}
		return getLatestElementofElement(parent, target, level+1)
	}
	return nil, utils.Errorf("%s not found.", target)
}

func getLatestClickElementofEachLevel(element *rod.Element, level int) (*rod.Element, error) {
	// fmt.Println("level ", level, " ", element)
	buttons, _ := element.Elements("button")
	if len(buttons) != 0 {
		return buttons[0], nil
	}
	inputs, _ := element.Elements("input")
	if len(inputs) != 0 {
		for _, input := range inputs {
			attribute, err := input.Attribute("type")
			if err != nil || attribute == nil {
				continue
			}
			if *attribute == "submit" || *attribute == "button" {
				return input, nil
			}
		}
	}
	if level <= 3 {
		parent, err := element.Parent()
		if err != nil {
			return nil, utils.Errorf("button or submit not found. and parent not found.")
		}
		return getLatestClickElementofEachLevel(parent, level+1)
	}
	return nil, utils.Errorf("button or submit input not found.")
}

func (r *Runner) ScreenShot(path string) {
	r.page.MustScreenshot(path)
}

func (r *Runner) WaitLoad() {
	r.page.MustWaitLoad()
}

func (r *Runner) WaitRequestIdle() func() {
	// wait := r.page.MustWaitRequestIdle()
	wait := r.page.WaitRequestIdle(time.Second, nil, nil, nil)
	return wait
	// r.page.WaitRequestIdle(time.Second, nil, nil)
}

func (r *Runner) GetInfo() {
	// headers := r.page.MustCookies()
}

func (r *Runner) CreateObserver() error {
	_, err := r.page.Eval(OBSERVER)
	if err != nil {
		return utils.Errorf("create mutation observer error: %s", err)
	}
	return nil
}

func (r *Runner) GetObserverResult() (string, error) {
	result, err := r.page.Eval(OBRESULT)
	if err != nil {
		return "", utils.Errorf("get mutation observer result error:%s", err)
	}
	return result.Value.Str(), nil
}

func (r *Runner) InputWords(element *rod.Element, words string) {
	element.MustSelectAllText().MustType(input.Backspace)
	rune := []input.Key(words)
	element.Type(rune...)
}

func (r *Runner) InputCaptcha(cap_elements *RelatedElements) error {
	capt := captcha.Captcha{
		Domain:     r.Domain,
		CaptchaUrl: r.CaptchaUrl,
	}
	capt.SetCapElement(cap_elements.RelatedElement)
	capString, err := capt.GetCaptcha()
	// rand.Seed(time.Now().UnixNano())
	// intt := rand.Intn(2)
	// log.Infof("random int:%d", intt)
	// if intt == 1 {
	// capString = "aaaa"
	// }
	if err != nil {
		return utils.Errorf("get captcha code error:%s", err)
	}
	r.InputWords(&cap_elements.Element, capString)
	// r.InputWords(&cap_elements.Element, "aaaa")
	return nil
}

func (r *Runner) Click(element *rod.Element) {
	element.Click(proto.InputMouseButtonLeft, 1)
}

func (r *Runner) WaitEvent(e proto.Event) func() {
	wait := r.page.WaitEvent(e)
	// proto.NetworkSearchInResponseBody
	return wait
}

func (r *Runner) CreateHijack() (chan string, *rod.HijackRouter) {
	pageRouters := r.page.HijackRequests()
	ch := make(chan string)
	pageRouters.MustAdd("*", func(hijack *rod.Hijack) {
		hijack.MustLoadResponse()
		content := hijack.Response.Headers().Get("Content-Type")
		if !strings.Contains(content, "application/json") {
			return
		}
		response := hijack.Response.Body()
		ch <- response
	})
	go func() {
		pageRouters.Run()
	}()
	return ch, pageRouters
}

func (r *Runner) SearchCaptchafromIMG() (*RelatedElements, error) {
	elements, err := r.GetElements("img")
	if err != nil {
		return nil, utils.Errorf("get img error: %s", err)
	}
	for _, element := range elements {
		if simplecheckelement(element, "captcha") {
			captcha_input, err := r.GetLatestElementofElement(element, "input")
			if err != nil {
				return nil, utils.Errorf("get latest element of img %s error: %s", element, err)
			}
			return &RelatedElements{
				Element:        *captcha_input,
				RelatedElement: element,
			}, nil
		}
	}
	return nil, utils.Errorf("captcha img not found.")
}

type RelatedElements struct {
	rod.Element
	RelatedElement *rod.Element
}

func simplecheckelement(element *rod.Element, keyword string) bool {
	element_style, err := element.Attribute("style")
	reg := regexp.MustCompile("\\s+")
	if element_style != nil && strings.Contains(reg.ReplaceAllString(*element_style, ""), "display:none") {
		return false
	}
	element_type, err := element.Attribute("type")
	if element_type != nil && *element_type == "hidden" {
		return false
	}
	if err == nil && keyword == "password" && element_type != nil && *element_type == "password" {
		return true
	}
	// fmt.Println("simple check element: ", element, ":", keyword)
	result, _ := checkElementTypefromAttribute(element, keyword, true, core.DefaultKeyword)
	return result
}

func checkelement(element *rod.Element, keyword string) (bool, float32) {
	// check hidden
	element_style, err := element.Attribute("style")
	reg := regexp.MustCompile("\\s+")
	if element_style != nil && strings.Contains(reg.ReplaceAllString(*element_style, ""), "display:none") {
		return false, 0
	}
	element_type, err := element.Attribute("type")
	if element_type != nil && *element_type == "hidden" {
		return false, 0
	}
	if err == nil && keyword == "password" && element_type != nil && *element_type == "password" {
		return true, 0
	}
	// fmt.Println("check element: ", element, ":", keyword)
	return checkElementTypefromAttribute(element, keyword, true, core.StrictKeyword)
}

func checkElementTypefromAttribute(element *rod.Element, keyword string, checkParent bool, keywordmap map[string][]string) (bool, float32) {
	// check different attibute info to get element's type. e.g.:username,password
	keywords, ok := keywordmap[keyword]
	if !ok {
		return false, 0
	}
	attributes := []string{"placeholder", "id", "name", "value", "alt"}
	var maxRate float32 = 0.0
	for _, attribute := range attributes {
		err, result, percent := checkAttribute(element, attribute, keywords)
		// fmt.Printf("check attribute: %s:%s:%s:%f\n", element, attribute, keyword, percent)
		if percent > maxRate {
			maxRate = percent
		}
		if err != nil {
			continue
		}
		// return result, percent
		if result == true {
			return result, percent
		}
	}
	text, err := element.Text()
	if err == nil && text != "" {
		for _, kw := range keywords {
			status, rate := checkStr(text, kw)
			// fmt.Printf("check text: %s:%s:%s:%f\n", element, text, keyword, rate)
			if rate > maxRate {
				maxRate = rate
			}
			if status {
				return status, rate
			}
		}
	}
	if maxRate > 0 {
		return false, maxRate
	}
	if checkParent {
		parent, err := element.Parent()
		if err != nil {
			return false, 0
		}
		// fmt.Println("check parent.")
		return checkElementTypefromAttribute(parent, keyword, false, keywordmap)
	}
	return false, 0
}

func checkAttribute(element *rod.Element, attributeStr string, keywords []string) (error, bool, float32) {
	attribute, err := element.Attribute(attributeStr)
	if err != nil {
		return err, false, 0
	}
	if attribute == nil {
		return utils.Errorf("attribute none"), false, 0
	}
	if *attribute == "" {
		return nil, false, 0
	}
	var maxRate float32 = 0.0
	for _, k := range keywords {
		// if checkStr(*attribute, k) {
		// 	// fmt.Println(*attribute)
		// 	value := float32(len(getRepeatStr(*attribute, k))) / float32(len(k))
		// 	fmt.Println(attributeStr, k, value)
		// 	return nil, true, value
		// }
		status, rate := checkStr(*attribute, k)
		// fmt.Printf("check %s:%s:%f\n", *attribute, k, rate)
		if rate > maxRate {
			maxRate = rate
		}
		if status {
			return nil, status, rate
		}
	}
	return nil, false, maxRate
}

func checkStr(origin, source string) (bool, float32) {
	// fmt.Println("checkStr", origin, source)
	// return strings.Contains(strings.ToLower(strings.Replace(origin, " ", "", -1)), source)
	originLower := strings.ToLower(strings.Replace(origin, " ", "", -1))
	repeatStr := getRepeatStr(originLower, source)
	return repeatStr == source, float32(len(repeatStr)) / float32(utils.Max(len(origin), len(source)))
}

func getRepeatStr(origin, source string) string {
	originBytes := []byte(origin)
	sourceBytes := []byte(source)
	var maxTemp []byte
	for num, ob := range originBytes {
		i := 1
		if ob != sourceBytes[0] {
			continue
		}
		temp := []byte{ob}
		if num+i < len(originBytes) {
			for originBytes[num+i] == sourceBytes[i] {
				temp = append(temp, originBytes[num+i])
				i++
				if i >= len(sourceBytes) {
					break
				}
				if num+i >= len(originBytes) {
					break
				}
			}
		}
		if len(temp) >= len(maxTemp) {
			maxTemp = temp
		}
	}
	return string(maxTemp)
}
