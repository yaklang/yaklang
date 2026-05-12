package yakcmds

import (
	"fmt"
	"os"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/utils"
)

var ExportBuiltinRulesCommand = &cli.Command{
	Name:  "export-builtin-rules",
	Usage: "Export yaklang builtin SyntaxFlow rules as a ZIP archive",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "output,o",
			Usage: "Output ZIP file path (default: stdout)",
		},
		cli.BoolFlag{
			Name:  "verbose,v",
			Usage: "Show progress",
		},
	},
	Action: func(c *cli.Context) error {
		outputPath := c.String("output")
		verbose := c.Bool("verbose")

		var w *os.File
		if outputPath == "" || outputPath == "-" {
			w = os.Stdout
		} else {
			var err error
			w, err = os.Create(outputPath)
			if err != nil {
				return utils.Wrapf(err, "create output file: %s", outputPath)
			}
			defer w.Close()
		}

		var notify func(float64, string)
		if verbose {
			notify = func(progress float64, ruleName string) {
				fmt.Fprintf(os.Stderr, "[%3.0f%%] %s\n", progress*100, ruleName)
			}
		}

		if err := sfbuildin.ExportBuiltinRulesToArchive(w, notify); err != nil {
			return utils.Wrap(err, "export builtin rules failed")
		}

		if outputPath != "" && outputPath != "-" {
			log.Infof("Exported builtin rules to %s", outputPath)
		}
		return nil
	},
}
