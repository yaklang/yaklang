package crep

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/h2non/filetype"
	"github.com/h2non/filetype/types"
	"github.com/yaklang/yaklang/common/utils"
	"time"
)

func Snapshot(s string, timeout time.Duration) ([]byte, *types.Type, error) {
	defer func() {
		if err := recover(); err != nil {
			return
		}
	}()

	ins := rod.New()
	defer ins.Close()

	ins.SetCookies([]*proto.NetworkCookieParam{
		{
			Name:         "",
			Value:        "",
			URL:          "",
			Domain:       "",
			Path:         "",
			Secure:       false,
			HTTPOnly:     false,
			SameSite:     "",
			Expires:      0,
			Priority:     "",
			SameParty:    false,
			SourceScheme: "",
			SourcePort:   nil,
			PartitionKey: nil,
		},
	})

	err := ins.Connect()
	if err != nil {
		return nil, nil, utils.Errorf("connect failed: %s", err)
	}

	pInt := func(i int) *int {
		return &i
	}
	page, err := ins.Timeout(timeout).Page(proto.TargetCreateTarget{
		URL:    s,
		Width:  pInt(2000),
		Height: pInt(2000),
	})
	if err != nil {
		return nil, nil, utils.Errorf("new page: %v failed: %s", s, err)
	}
	page.Eval(`window.alert = () => {}`)

	err = page.WaitLoad()
	if err != nil {
		return nil, nil, utils.Errorf("wait page failed: %s", err)
	}

	var raw []byte
	raw, err = page.Screenshot(true, &proto.PageCaptureScreenshot{})
	if err != nil {
		return nil, nil, utils.Errorf("screenshot failed: %s", err)
	}

	imgType, err := filetype.Match(raw)
	if err != nil {
		return nil, nil, utils.Errorf("not a valid img: %v", err)
	}

	return raw, &imgType, nil
}
