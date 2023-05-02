package bruteforce

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/rpa/implement"
	"github.com/yaklang/yaklang/common/rpa/web"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/go-rod/rod"
)

type BruteForce struct {
	implement.Runner

	// user_dict string
	usernames []string
	passwords []string
	user_pass [][]string

	web_navigate_load int
	click_interval    int
	response_wait     int

	//captcha error
	repeatCaptchaCount    int
	singleCaptchaErrorNum int
	CaptchaErrorNumCount  int

	//element
	usernameStr   string
	username      *implement.RelatedElements
	passwordStr   string
	password      *implement.RelatedElements
	captchaStr    string
	captchaIMGStr string
	captcha       *implement.RelatedElements
	buttonStr     string
	button        *rod.Element

	//captchaUrl string
}

func (bf *BruteForce) Init(url *string) {
	bf.Runner.Init()
	if len(bf.usernames) == 0 {
		bf.usernames = append(bf.usernames, "admin")
		bf.passwords = append(bf.passwords, "admin")
		// bf.usernames = append(bf.usernames, "sihongxing.21mba")
		// bf.passwords = append(bf.passwords, "admin", "root", "19900324")
	}
	if !strings.HasPrefix(*url, "http") {
		*url = "http://" + *url
	}
	bf.Domain = web.GetMainDomain(*url)
}

func (bf *BruteForce) CheckSuccess() {
}

func (bf *BruteForce) ReadUserPassList(filepaths ...string) {
	if len(filepaths) == 1 {
		filepath := filepaths[0]
		fi, err := os.Open(filepath)
		if err != nil {
			log.Errorf("open file %s error:%s", filepath, err)
			return
		}
		r := bufio.NewReader(fi)
		for {
			lineBytes, err := r.ReadBytes('\n')
			if err != nil && err != io.EOF {
				log.Errorf("read error:%s", err)
				return
			}
			if err == io.EOF {
				break
			}
			line := strings.TrimSpace(string(lineBytes))
			infos := strings.Fields(line)
			if len(infos) > 1 {
				bf.user_pass = append(bf.user_pass, []string{infos[0], infos[1]})
			}
		}
	} else if len(filepaths) > 1 {
		userpath := filepaths[0]
		passpath := filepaths[1]
		bf.usernames = append(bf.usernames, readFile2List(userpath)...)
		bf.passwords = append(bf.passwords, readFile2List(passpath)...)
	} else {
		return
	}
}

func (bf *BruteForce) GetKeyElements() error {
	elements, _ := bf.GetElements("input")
	log.Infof("get elements: %s", elements)

	var username_element, password_element, captcha_element *implement.RelatedElements
	// usename
	if bf.usernameStr != "" {
		username_elements, err := bf.GetElements(bf.usernameStr)
		if err != nil {
			return utils.Errorf("search username elements %s error: %s", bf.usernameStr, err)
		}
		if len(username_elements) == 0 {
			return utils.Errorf("search username elements %s not found.", bf.usernameStr)
		}
		username_element = &implement.RelatedElements{Element: *username_elements[0], RelatedElement: nil}
	} else {
		username_element = bf.GetKeywordElement(elements, "username")
		if username_element == nil {
			return utils.Error("username element not found.")
		}
	}

	// password
	if bf.passwordStr != "" {
		password_elements, err := bf.GetElements(bf.passwordStr)
		if err != nil {
			return utils.Errorf("search password elements %s error: %s", bf.passwordStr, err)
		}
		if len(password_elements) == 0 {
			return utils.Errorf("search password elements %s not found.", bf.passwordStr)
		}
		password_element = &implement.RelatedElements{Element: *password_elements[0], RelatedElement: nil}
	} else {
		password_element = bf.GetKeywordElement(elements, "password")
		if password_element == nil {
			return utils.Error("password element not found.")
		}
	}

	// captcha
	if bf.captchaStr != "" {
		captcha_elements, err := bf.GetElements(bf.captchaStr)
		if err != nil {
			return utils.Errorf("search captcha elements %s error: %s", bf.captchaStr, err)
		}
		if len(captcha_elements) == 0 {
			return utils.Errorf("search captcha elements %s not found.", bf.captchaStr)
		}
		captcha_IMG_elements, err := bf.GetElements(bf.captchaIMGStr)
		if err != nil {
			return utils.Errorf("search captcha IMG elements %s error: %s", bf.captchaIMGStr, err)
		}
		if len(captcha_IMG_elements) == 0 {
			return utils.Errorf("search captcha IMG elements %s not found.", bf.captchaIMGStr)
		}
		captcha_element = &implement.RelatedElements{Element: *captcha_elements[0], RelatedElement: captcha_IMG_elements[0]}
	} else {
		captcha_element = bf.GetKeywordElement(elements, "captcha")
		if captcha_element != nil && captcha_element.RelatedElement == nil {
			return utils.Errorf("element captcha found but captcha pic not found. persume cannot brute. end.")
		}
	}
	//*

	// button
	var button *rod.Element
	var errs error
	if bf.buttonStr != "" {
		buttons, err := bf.GetElements(bf.buttonStr)
		if err != nil {
			return utils.Errorf("search button elements %s error: %s", bf.buttonStr, err)
		}
		if len(buttons) == 0 {
			return utils.Errorf("search button elements %s not found.", bf.buttonStr)
		}
		button = buttons[0]
	} else {
		button, errs = bf.GetLatestClickElement(&username_element.Element)
		if errs != nil {
			return utils.Errorf("get latest button error: %s", errs)
		}
	}
	// log.Infof("get username element: %s, \n    password element: %s, \n    captcha element: %s, \n	button: %s\n",
	// 	username_element, password_element, captcha_element, button)
	fmt.Printf("get username element: %s, \n    password element: %s, \n    captcha element: %s,%s, \n	button: %s\n",
		username_element, password_element, captcha_element, captcha_element.RelatedElement, button)

	bf.username = username_element
	bf.password = password_element
	bf.captcha = captcha_element
	bf.button = button
	return nil
}

func (bf *BruteForce) RunBefore(opts []ConfigMethod) {
	for _, opt := range opts {
		wait := bf.WaitRequestIdle()
		opt(bf)
		bf.WaitLoad()
		wait()
	}
}

func (bf *BruteForce) Start(url string, opts ...ConfigMethod) (string, string) {
	bf.Init(&url)
	bf.Navigate(url)
	bf.RunBefore(opts)
	time.Sleep(time.Duration(bf.web_navigate_load) * time.Second)
	origin_url, err := bf.GetCurrentURL()
	if err != nil {
		log.Errorf("get current url error: %s", err)
		return "", ""
	}
	err = bf.GetKeyElements()
	if err != nil {
		log.Errorf("key elements found error: %s", err)
		return "", ""
	}
	log.Infof(origin_url, bf.username, bf.password, bf.captcha)
	//*
	// fmt.Println(bf.usernames, bf.passwords)
	for _, username := range bf.usernames {
		for _, password := range bf.passwords {
			log.Infof("test user-pass: %s:%s", username, password)
			fmt.Printf("test user-pass: %s:%s\n", username, password)
			bf.InputWords(&bf.username.Element, username)
			bf.InputWords(&bf.password.Element, password)
			if bf.captcha != nil {
				err = bf.InputCaptcha(bf.captcha)
				if err != nil {
					log.Errorf("input captcha error: %s", err)
					continue
				}
			}
			//
			err = bf.CreateObserver()
			if err != nil {
				log.Errorf("create observer error: %s", err)
			}
			wait := bf.WaitRequestIdle()
			bf.Click(bf.button)
			wait()
			obresult, err := bf.GetObserverResult()
			if err != nil {
				log.Errorf("get observer result error: %s", err)
			}
			// time.Sleep(time.Duration(bf.click_interval) * time.Second)
			//end
			//screen shot
			// name := fmt.Sprintf("%s_%s.png", username, password)
			// bf.ScreenShot(name)
			current_url, _ := bf.GetCurrentURL()
			if origin_url != current_url {
				log.Info("login success!")
				log.Infof("login from %s to %s.", origin_url, current_url)
				name := fmt.Sprintf("%s_%s.png", username, password)
				bf.ScreenShot(name)
				return username, password
			}
			// log.Info("obresult: %s", obresult)
			fmt.Println("obresult: ", obresult)
			if strings.Contains(obresult, "验证码") || strings.Contains(obresult, "captcha") || strings.Contains(obresult, "\\u9a8c\\u8bc1\\u7801") {
				if bf.captcha == nil {
					log.Errorf("captcha error happened but captcha not found. quit.")
					return "", ""
				}
				var repeatNum int
				for {
					if repeatNum >= bf.singleCaptchaErrorNum || bf.repeatCaptchaCount >= bf.CaptchaErrorNumCount {
						log.Errorf("captcha error more than %d times, all %d times, quit.", repeatNum, bf.repeatCaptchaCount)
						return "", ""
					}
					err = bf.InputCaptcha(bf.captcha)
					if err != nil {
						log.Errorf("input captcha error: %s", err)
						continue
					}
					bf.CreateObserver()
					wait := bf.WaitRequestIdle()
					bf.Click(bf.button)
					wait()
					obresult, _ = bf.GetObserverResult()
					current_url, _ := bf.GetCurrentURL()
					if origin_url != current_url {
						log.Info("login success!")
						log.Infof("login from %s to %s.", origin_url, current_url)
						name := fmt.Sprintf("%s_%s.png", username, password)
						bf.ScreenShot(name)
						return username, password
					}
					// log.Info("obresult: %s", obresult)
					fmt.Println("obresult: ", obresult)
					if obresult != "" && !strings.Contains(obresult, "验证码") && !strings.Contains(obresult, "captcha") && !strings.Contains(obresult, "\\u9a8c\\u8bc1\\u7801") {
						break
					}
					repeatNum++
					bf.repeatCaptchaCount++
				}
			}
		}
	}
	log.Infof("login failed.")
	fmt.Println("login failed.")
	//*/
	return "", ""
}

func readFile2List(filepath string) []string {
	log.Infof("read file %s", filepath)
	fi, err := os.Open(filepath)
	list := make([]string, 0)
	if err != nil {
		log.Errorf("read file %s error: %s", filepath, err)
		return list
	}
	r := bufio.NewReader(fi)
	breakFlag := false
	for {
		lineBytes, err := r.ReadBytes('\n')
		if err != nil && err != io.EOF {
			log.Errorf("read bytes error")
			return list
		}
		if err == io.EOF {
			breakFlag = true
		}
		line := strings.TrimSpace(string(lineBytes))
		if line != "" {
			log.Infof("read line: %s", line)
			list = append(list, line)
		}
		if breakFlag {
			break
		}
	}
	return list
}
