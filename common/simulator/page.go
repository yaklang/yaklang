// Package simulator
// @Author bcy2007  2023/8/23 11:06
package simulator

import (
	"encoding/base64"
	"github.com/go-rod/rod"
	"github.com/yaklang/yaklang/common/utils"
)

func ScreenShot(page *rod.Page) (string, error) {
	bytes, err := page.Screenshot(false, nil)
	if err != nil {
		return "", utils.Error(err)
	}
	b64 := base64.StdEncoding.EncodeToString(bytes)
	return "data:image/png;base64," + b64, nil
}
