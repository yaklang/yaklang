package yakcmds

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/diff"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/samber/lo"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/yakgit"
	"github.com/yaklang/yaklang/common/utils/yakgit/yakdiff"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var GitCommands = []*cli.Command{
	{
		Name: "git-ai-commit",
		Action: func(c *cli.Context) error {
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			repo, err := git.PlainOpen(dir)
			if err != nil {
				return err
			}

			worktree, err := repo.Worktree()
			if err != nil {
				return utils.Wrap(err, "plain worktree failed")
			}

			headRef, err := repo.Head()
			if err != nil {
				return err
			}
			headCommit, err := repo.CommitObject(headRef.Hash())
			if err != nil {
				return utils.Wrap(err, `wtBase, err := repo.CommitObject(ref.Hash())`)
			}

			stats, _ := worktree.Status()
			if len(stats) > 0 {
				// submit and amend
				_, err = worktree.Add(".")
				if err != nil {
					return utils.Wrap(err, "worktree.Add failed")
				}
				commit, err := worktree.Commit("Auto Commit (Wait for amend)", &git.CommitOptions{
					All:       true,
					Author:    &headCommit.Author,
					Committer: &headCommit.Committer,
					Parents:   []plumbing.Hash{headCommit.ParentHashes[0]},
				})
				if err != nil {
					return utils.Wrap(err, "worktree.Commit failed")
				}
				headCommit, err = repo.CommitObject(commit)
				if err != nil {
					return utils.Wrap(err, `auto-commit changes failed`)
				}
				headRef, err = repo.Head()
				log.Infof("use head ref after commit")
			}

			// amend HEAD~1...HEAD
			headTree, err := headCommit.Tree()
			if err != nil {
				return utils.Wrap(err, `baseTree, err := wtBase.Tree()`)
			}

			parentBase, err := headCommit.Parent(0)
			if err != nil {
				return utils.Wrap(err, `wtBase.Parent(0) failed`)
			}
			parentTree, err := parentBase.Tree()
			if err != nil {
				return utils.Wrap(err, `fetch parent commit tree`)
			}

			changes, err := parentTree.Diff(headTree)
			if err != nil {
				return err
			}

			result := bytes.NewBuffer(nil)
			result.WriteString(`以下是 Git Diff` + "\r\n\r\n")
			for _, change := range changes {
				patch, err := change.Patch()
				if err != nil {
					continue
				}
				var buf bytes.Buffer
				buf.WriteString(change.String())
				buf.WriteByte('\n')
				raw := patch.String()
				if len(raw) > 409600 {
					raw = utils.ShrinkString(raw, 409600)
				}
				buf.WriteString(raw)
				fmt.Println(buf.String())
				io.Copy(result, &buf)
			}

			var finalMessage string
		GENERATE:
			for {
				log.Info("AI start to generate commit message(current)...")
				results, err := ai.FunctionCall(result.String(), map[string]any{
					"message":    "英文：适用于上述 git diff 的 commit message浓缩成一句话，标注 feature/bugfix/doc 之类的",
					"message_zh": `中文：要求含义同 message`,
				}, aispec.WithDebugStream(), aispec.WithType("openai"))
				if err != nil {
					log.Warnf("AI failed: %v", err)
					_, err := fmt.Scanf("start to retry? (enter to continue)")
					if err != nil {
						log.Warnf("retry failed: %v", err)
					}
					continue
				}

				commitMessage, ok := results["message"]
				if !ok {
					log.Infof("cannot generate ai commit message: ")
					_, err := fmt.Scanf("start to retry? (enter to continue)")
					if err != nil {
						log.Warnf("retry failed: %v", err)
					}
					continue
				}

				log.Infof("AI commit message: %v", commitMessage)
				commitMessageZh, _ := results["message_zh"]
				fmt.Println(commitMessageZh)
				// yes or retry?
				var result string
				fmt.Println()
				fmt.Println(`start to commit? (yes/y/t to commit, retry/r to retry, enter to continue): `)
				_, err = fmt.Scanf("%s", &result)
				if err != nil {
					return err
				}
				switch strings.TrimSpace(strings.ToLower(result)) {
				case "retry", "r":
					continue
				case "yes", "y", "t":
					finalMessage = codec.AnyToString(commitMessage)
					break GENERATE
				}
			}
			fmt.Println("start to commit...")
			commit, err := worktree.Commit(finalMessage, &git.CommitOptions{
				All:       true,
				Author:    &headCommit.Author,
				Committer: &headCommit.Committer,
				Parents:   []plumbing.Hash{headCommit.ParentHashes[0]},
			})
			if err != nil {
				return utils.Wrapf(err, "amend commit failed: %v", finalMessage)
			}
			_, err = repo.CommitObject(commit)
			if err != nil {
				return utils.Wrap(err, `auto-commit changes failed`)
			}
			return nil
		},
	},
	{
		Name:    "git-extract-fs",
		Aliases: []string{"gitefs"},
		Flags: []cli.Flag{
			cli.StringFlag{
				Name: "repository,repo,r", Usage: "Set Repository, default PWD",
			},
			cli.StringFlag{
				Name: "start", Usage: "start ref hash (range)",
			},
			cli.StringFlag{
				Name:  "end",
				Usage: "end ref hash(range)",
			},
			cli.StringFlag{
				Name:  "output",
				Usage: "output filename",
			},
		},
		Action: func(c *cli.Context) error {
			start := c.String("start")
			end := c.String("end")
			output := c.String("output")
			repos := c.String("repo")
			if repos == "" {
				pwd, err := os.Getwd()
				if err != nil {
					return utils.Wrap(err, "(emtpy repos parameter )get pwd failed")
				}
				repos = pwd
			}

			handleResult := func(i filesys_interface.FileSystem) {
				filesys.TreeView(i)
				suffix := ""
				if start == "" && end == "" {
					suffix = strings.Join(lo.Map(c.Args(), func(i string, _ int) string {
						if len(i) > 7 {
							return i[:7]
						}
						return i
					}), "-")
				} else if start == "" {
					suffix = end
					if len(suffix) > 7 {
						suffix = suffix[:7]
					}
				} else {
					if len(start) > 7 {
						start = start[:7]
					}
					if len(end) > 7 {
						end = end[:7]
					}
					suffix = start + "-" + end
				}
				fileName := fmt.Sprintf("commitfs-%v.zip", suffix)
				if output != "" && strings.HasSuffix(output, ".zip") {
					fileName = output
				}
				log.Infof("start to prepare writing zip file: %v", fileName)
				var buf bytes.Buffer
				zw := zip.NewWriter(&buf)
				filesys.SimpleRecursive(filesys.WithFileSystem(i), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
					fw, err := zw.Create(s)
					if err != nil {
						return err
					}
					raw, err := i.ReadFile(s)
					if err != nil {
						return err
					}
					_, err = fw.Write([]byte(raw))
					return nil
				}))
				zw.Flush()
				zw.Close()
				err := os.WriteFile(fileName, buf.Bytes(), 0644)
				if err != nil {
					log.Warnf("write zip failed: %v", err)
					return
				}
				log.Infof("write zip file: %v", fileName)
			}

			if start == "" && end == "" {
				args := c.Args()
				if len(args) <= 0 {
					return utils.Error("no start and end ref hash and args")
				}
				lfs, err := yakgit.FromCommits(repos, args...)
				if err != nil {
					return utils.Wrap(err, "fetch commits failed")
				}
				handleResult(lfs)
				return nil
			}

			if start == "" {
				lfs, err := yakgit.FromCommit(repos, end)
				if err != nil {
					return utils.Wrap(err, "fetch commit failed")
				}
				handleResult(lfs)
				return nil
			}

			if end == "" {
				end = yakgit.GetHeadHash(repos)
			}
			// start - n end
			lfs, err := yakgit.FromCommitRange(repos, start, end)
			if err != nil {
				return err
			}
			handleResult(lfs)
			return nil
		},
	},
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

						chunks := fp.Chunks()
						if len(chunks) <= 0 {
							continue
						}

						dstFile, err := os.ReadFile(filename)
						if err != nil {
							continue
						}
						editor := memedit.NewMemEditor(string(dstFile))

						editorOffset := 0
						for _, chunked := range chunks {
							verbose := chunked.Content()
							verbose = utils.ShrinkString(verbose, 64)

							var action string = " "
							var suffix string
							switch chunked.Type() {
							case diff.Equal: // eq
								action = " "
								editor.FindStringRangeIndexFirst(editorOffset, chunked.Content(), func(rangeIf *memedit.Range) {
									editorOffset = editor.GetOffsetByPosition(rangeIf.GetEnd())
								})
							case diff.Add: //
								action = "+"
								editor.FindStringRangeIndexFirst(editorOffset, chunked.Content(), func(rangeIf *memedit.Range) {
									editorOffset = editor.GetOffsetByPosition(rangeIf.GetEnd())
									suffix = ` (` + fmt.Sprintf(
										"%v:%v-%v:%v",
										rangeIf.GetStart().GetLine(), rangeIf.GetStart().GetColumn(),
										rangeIf.GetEnd().GetLine(), rangeIf.GetEnd().GetColumn(),
									) + `)`
								})
							case diff.Delete:
								action = "-"
							}
							fmt.Printf("  |%v%-67s |%v\n", action, verbose, suffix)
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
