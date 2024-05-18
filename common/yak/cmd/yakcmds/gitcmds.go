package yakcmds

import (
	"context"
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/yakgit/yakdiff"
	"path/filepath"
	"strings"
)

var GitCommands = []*cli.Command{
	{
		Name: "git-diff",
		Flags: []cli.Flag{
			cli.StringFlag{Name: "repository,repo,r", Usage: `Setting Repository, default PWD`},
			cli.StringFlag{Name: "target", Usage: "Target Commit (Tag or Hash)"},
			cli.StringFlag{Name: "base", Usage: `Git Base Commit (Hash/Tag/Branch), default HEAD`, Value: "HEAD"},
			cli.StringFlag{
				Name:  "exclude-filenames",
				Usage: "ignore some filenames like protobuf or some...",
				Value: "*.pb.go,*.png,*.gif,*.jpg,*.jpeg," +
					"*_test.go,*.sum,bindata.*," +
					"*_lexer.go,*_parser.go,*.interp,*.tokens," +
					"*.yakdoc.yaml,embed.go,*.min.js,*.min.css"},
			//cli.StringFlag{Name: "exclude-keywords", Value: ""},
			cli.BoolFlag{Name: "debug", Usage: "Debug will show more log info"},
		},
		Action: func(c *cli.Context) error {
			debug := c.Bool("debug")

			var repoPath string = "."
			if ret := c.String("repository"); ret != "" {
				repoPath = ret
			}

			if debug {
				log.Infof("start to open plain repo: %v", repoPath)
			}
			repo, err := git.PlainOpen(repoPath)
			if err != nil {
				return err
			}

			var base = c.String("base")
			if base == "" || base == "HEAD" {
				if debug {
					log.Info("base: start to check HEAD hash...")
				}
				ref, err := repo.Head()
				if err != nil {
					return err
				}
				base = ref.Hash().String()
				if debug {
					log.Infof("found HEAD hash: %v", base)
				}
			}

			var target = c.String("target")
			if target == "" || target == "HEAD" {
				if debug {
					log.Info("target: start to check HEAD hash...")
				}
				ref, err := repo.Head()
				if err != nil {
					return err
				}
				target = ref.Hash().String()
				if debug {
					log.Infof("found HEAD hash: %v", base)
				}
			}

			excludeFiles := utils.PrettifyListFromStringSplitEx(c.String("exclude-filenames"))
			shouldFocus := func(pathName string) bool {
				_, filename := filepath.Split(pathName)
				if utils.MatchAnyOfGlob(filename, excludeFiles...) {
					return false
				}
				return true
			}

			if base == target {
				return utils.Errorf("base and target is the same hash(tag): %v", base)
			}

			err = yakdiff.GitHashDiffContext(
				context.Background(), repo,
				base, target,
				func(commit *object.Commit, change *object.Change, patch *object.Patch) error {
					if patch == nil {
						fmt.Println(patch.String())
						return nil
					}

					action, err := change.Action()
					if err != nil {
						log.Warnf("Change Action fetch failed: %s", err)
						return nil
					}

					fmt.Println(`---------------------------------------------------------------------------`)
					//for _, i := range patch.Stats() {
					//	fmt.Print(i.String())
					//}
					stats := patch.Stats()
					patches := patch.FilePatches()

					if len(patches) <= 0 {
						fmt.Println(patch.String())
						return nil
					}

					showStats := true
					if len(stats) != len(patches) {
						showStats = false
					}
					for idx, fp := range patches {
						fromFile, toFile := fp.Files()
						if fromFile == nil && toFile == nil {
							fmt.Println(fp)
							continue
						}

						var filename string
						if toFile == nil {
							filename = fromFile.Path()
						} else {
							filename = toFile.Path()
						}

						if fp.IsBinary() {
							if debug {
								log.Infof("skip binary change: %v", filename)
							}
							continue
						}

						if !shouldFocus(filename) {
							if debug {
								log.Infof("ignore file: %s by user config", filename)
							}
							continue
						}

						if showStats {
							fmt.Printf("%v: | %v\n", action, strings.TrimSpace(stats[idx].String()))
						} else {
							fmt.Printf("%v: | %v\n", action, filename)
						}
						for _, chunked := range fp.Chunks() {
							verbose := chunked.Content()
							verbose = utils.ShrinkString(verbose, 64)

							var action string = " "
							switch chunked.Type() {
							case diff.Equal: // eq
								action = " "
							case diff.Add: //
								action = "+"
							case diff.Delete:
								action = "-"
							}
							fmt.Printf("  |%v%-67s |\n", action, verbose)
						}
					}

					return nil
				},
			)
			if err != nil {
				return err
			}
			return nil
		},
	},
}
