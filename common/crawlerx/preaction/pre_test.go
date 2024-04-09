// Package crawlerx
// @Author bcy2007  2024/4/2 15:54
package preaction

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

const testHTML = `<html>
  <head>
    <title>Test</title>
  </head>
  <body>
    <div>
      <select name="id">
        <option value="">---</option>
        <option value="1">1</option>
        <option value="2">2</option>
        <option value="3">3</option>
        <option value="4">4</option>
        <option value="5">5</option>
        <option value="6">6</option>
      </select>
    </div>
    <div>
      <input
        type="file"
        name="uploadfile"
      />
      <br />
      <input class="sub" type="submit" name="submit" value="开始上传" />
    </div>
    <div>
        <input type="text" name="username" placeholder="Username">
        <input type="password" name="password" placeholder="Password">
        <input class="submit" name="submit" type="submit" value="Login">
    </div>
  </body>
</html>
`

func GetBrowser() *rod.Browser {
	launch := launcher.New().Headless(true).MustLaunch()
	browser := rod.New().SlowMotion(500 * time.Millisecond).ControlURL(launch).MustConnect()
	browser.MustIgnoreCertErrors(true)
	return browser
}

func TestPreAct(t *testing.T) {
	base := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(testHTML))
	}))
	defer base.Close()

	browser := GetBrowser()
	page := browser.MustPage(base.URL).MustWaitLoad()
	type args struct {
		action *PreAction
	}
	tests := []struct {
		name string
		args args
		want error
	}{
		{
			name: "element select",
			args: args{
				action: &PreAction{
					Action:   SelectAction,
					Selector: "body > div:nth-child(1) > select",
					Params:   "3",
				},
			},
			want: nil,
		},
		{
			name: "element set file",
			args: args{
				action: &PreAction{
					Action:   SetFileAction,
					Selector: "body > div:nth-child(2) > input[type=file]:nth-child(1)",
					Params:   "/Users/chenyangbao/1.jpg",
				},
			},
			want: nil,
		},
		{
			name: "element hover",
			args: args{
				action: &PreAction{
					Action:   HoverAction,
					Selector: "body > div:nth-child(2) > input[type=file]:nth-child(1)",
					Params:   "",
				},
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PreAct(page, tt.args.action); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PreAct() = %v, want %v", got, tt.want)
			}
			time.Sleep(time.Second)
		})
	}

	type actsArgs struct {
		actions []*PreAction
	}
	actsTests := []struct {
		name string
		args actsArgs
		want error
	}{
		{
			name: "input & click acts",
			args: actsArgs{
				actions: []*PreAction{
					{
						Action:   InputAction,
						Selector: "body > div:nth-child(3) > input[type=text]:nth-child(1)",
						Params:   "admin",
					},
					{
						Action:   InputAction,
						Selector: "body > div:nth-child(3) > input[type=password]:nth-child(2)",
						Params:   "pass",
					},
					{
						Action:   ClickAction,
						Selector: "body > div:nth-child(3) > input.submit",
						Params:   "",
					},
				},
			},
			want: nil,
		},
	}
	for _, tt := range actsTests {
		t.Run(tt.name, func(t *testing.T) {
			if got := PreActs(page, tt.args.actions); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PreAct() = %v, want %v", got, tt.want)
			}
			time.Sleep(time.Second)
		})
	}
}
