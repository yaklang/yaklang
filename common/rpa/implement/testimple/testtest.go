package testimple

import (
	"fmt"
	"github.com/yaklang/yaklang/common/rpa/implement"
)

type TestSth struct {
	implement.Runner
}

func (ts *TestSth) Start() {
	ts.Init()
	ts.Navigate("https://0.zone/login")
	elements, _ := ts.GetElements("input")
	username_element := ts.GetKeywordElement(elements, "username")
	fmt.Println(username_element)
	password_element := ts.GetKeywordElement(elements, "password")
	fmt.Println(password_element)
	captcha_element := ts.GetKeywordElement(elements, "captcha")
	fmt.Println(captcha_element)
	// button, _ := ts.GetLatestClickElement(&username_element.Element)
	// fmt.Println(button)
	button, _ := ts.GetElements("#app > div.page-login > div.login > div.login-content > div.login-form > div.bouuton")
	fmt.Println(button)
}
