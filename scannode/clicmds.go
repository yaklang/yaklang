package scannode

import (
	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/yak/distyak"
)

// DistYakCommand preserves the shared runner and adds node-owned process
// lifetime supervision when the node supplies a parent pipe.
var DistYakCommand = func() cli.Command {
	command := distyak.DistYakCommand
	action := command.Action
	command.Action = func(c *cli.Context) error {
		stopLifetime, err := watchScriptParentLifetime()
		if err != nil {
			return err
		}
		defer stopLifetime()
		return cli.HandleAction(action, c)
	}
	return command
}()
