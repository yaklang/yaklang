//go:build !no_language
// +build !no_language

package yakcmds

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/samber/lo"
	"github.com/segmentio/ksuid"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var JavaDecompilerSelfChecking = &cli.Command{
	Name:    "java-decompiler-self-checking",
	Usage:   `use 'cd /tmp/error-jdsc && echo "Syntax Error: $(ls syntax-error*.class 2>/dev/null | wc -l), Decompile Error: $(ls decompile-err*.class 2>/dev/null | wc -l)" && cd -' to check quick! compile with yak jdsc --output /tmp/error-jdsc`,
	Aliases: []string{"jdsc"},
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "error-output,output",
			Value: "decompiler-self-checking-failed-files",
		},
		cli.BoolFlag{
			Name: "verbose,v",
		},
	},
	Action: func(c *cli.Context) error {
		// search .m2 directory
		homeDir, err := filepath.Abs(utils.GetHomeDirDefault("."))
		if err != nil {
			return utils.Error(err)
		}
		m2Dir := filepath.Join(homeDir, ".m2")

		outputDir := c.String("output")
		outputDir, err = filepath.Abs(outputDir)
		if err != nil {
			return utils.Error(err)
		}
		os.MkdirAll(outputDir, 0755)

		swg := utils.NewSizedWaitGroup(100)

		var visitedCompiledError sync.Map

		var decompilerFinishedCount *int64 = new(int64)
		var decompilerFailedCount *int64 = new(int64)
		var decompilerSyntaxErrorCount *int64 = new(int64)

		go func() {
			var lastFinished, lastFailed, lastSyntax int64
			for {
				time.Sleep(time.Second)
				finished := atomic.LoadInt64(decompilerFinishedCount)
				failed := atomic.LoadInt64(decompilerFailedCount)
				syntax := atomic.LoadInt64(decompilerSyntaxErrorCount)

				if finished != lastFinished || failed != lastFailed || syntax != lastSyntax {
					total := finished + failed + syntax
					var failedPercent, syntaxPercent float64
					if total > 0 {
						failedPercent = float64(failed) / float64(total) * 100
						syntaxPercent = float64(syntax) / (float64(total) - float64(failed)) * 100
					}
					log.Infof("Decompiler Status - Total: %v, Success: %v, Failed: %v (%.1f%%), Syntax Errors: %v (%.1f%%)",
						total, finished, failed, failedPercent, syntax, syntaxPercent)
					lastFinished = finished
					lastFailed = failed
					lastSyntax = syntax
				}
			}
		}()

		handle := func(s string, raw []byte) error {
			hash := codec.Sha256(raw)
			if len(hash) > 24 {
				hash = hash[:24]
			}
			results, err := javaclassparser.Decompile(raw)
			if err != nil {
				atomic.AddInt64(decompilerFailedCount, 1)
				errHash := codec.Sha256([]byte(err.Error()))
				if _, ok := visitedCompiledError.Load(errHash); ok {
					return nil
				} else {
					visitedCompiledError.Store(errHash, struct{}{})
				}
				// decompiler error
				//          "syntax-error--
				fileName := "decompile-err-" + hash
				originCls := filepath.Join(outputDir, fileName+".class")
				os.WriteFile(originCls, raw, 0755)

				if c.Bool("verbose") {
					log.Errorf("javaclassparser.Decompile failed: %v", err)
				}
				return nil
			}
			atomic.AddInt64(decompilerFinishedCount, 1)

			//vfs := filesys.NewVirtualFs()
			//vfs.AddFile("origin.java", results)

			_, err = java2ssa.Frontend(results)
			if err != nil {
				atomic.AddInt64(decompilerSyntaxErrorCount, 1)
				if c.Bool("verbose") {
					log.Errorf("java2ssa.Frontend failed: %v", err)
				}
				fileName := "syntax-error--" + hash
				originCls := filepath.Join(outputDir, fileName+".class")
				target := filepath.Join(outputDir, fileName+".java")
				os.WriteFile(originCls, raw, 0755)
				os.WriteFile(target, []byte(results), 0755)
				return nil
			}
			return nil
		}
		filesys.Recursive(m2Dir, filesys.WithFileStat(func(s string, info fs.FileInfo) error {
			if filepath.Ext(s) != ".jar" {
				return nil
			}

			if c.Bool("verbose") {
				log.Infof("start to decompile %v", s)
			}
			zfs, err := filesys.NewZipFSFromLocal(s)
			if err != nil {
				return err
			}
			filesys.SimpleRecursive(filesys.WithFileSystem(zfs), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
				if zfs.Ext(s) != ".class" {
					return nil
				}

				if strings.Contains(filepath.Base(s), "package-info.class") {
					return nil
				}

				raw, err := zfs.ReadFile(s)
				if err != nil {
					if c.Bool("verbose") {
						log.Error(err)
					}
					return nil
				}

				swg.Add()
				go func() {
					defer swg.Done()
					err := handle(s, raw)
					if err != nil {
						if c.Bool("verbose") {
							log.Errorf("handle failed: %v", err)
						}
					}
				}()
				return nil
			}))
			return nil
		}))
		swg.Wait()
		return nil
	},
}

var JavaDecompilerCommand = &cli.Command{
	Name:    "java-decompiler",
	Usage:   `Java Decompiler in Thirdparty Implemented`,
	Aliases: []string{"jd"},
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "jar,j,input,in",
			Usage: "--input <jar/class/zip/war file> to decompile",
		},
		cli.StringFlag{
			Name:  "jar-directory,jardir,dir",
			Usage: "--jar-directory <jar directory> to decompile",
		},
		cli.StringFlag{
			Name:  "output,o",
			Usage: "output directory",
		},
		cli.StringFlag{
			Name:  "error-output,e",
			Usage: "mirror error output file",
		},
		cli.BoolFlag{
			Name: "quiet,q",
		},
	},
	Action: func(c *cli.Context) error {
		if c.Bool("quiet") {
			log.SetLevel(log.WarnLevel)
		}
		if c.IsSet("jar") && c.IsSet("jar-directory") {
			return errors.New("only one of --input and --jar-directory can be set")
		}
		if !c.IsSet("jar") && !c.IsSet("jar-directory") {
			return errors.New("one of --jar and --jar-directory must be set")
		}
		var inputs []string
		var handledClass []string
		if c.IsSet("jar") {
			inputPaths := c.String("jar")
			inputPathList := strings.Split(inputPaths, ",")
			for _, jar := range inputPathList {
				inputs = append(inputs, jar)
			}
		} else {
			dirMode := c.String("jar-directory")
			err := filesys.Recursive(dirMode, filesys.WithFileStat(func(s string, info fs.FileInfo) error {
				if strings.HasSuffix(s, ".jar") {
					inputs = append(inputs, s)
					return nil
				}
				if strings.HasSuffix(s, ".class") {
					target := strings.TrimSuffix(s, ".class") + ".java"
					if res, _ := utils.PathExists(target); res {
						log.Infof("%v is decompiled, skip", s)
						handledClass = append(handledClass, target)
						return nil
					}
					log.Infof("start to decompile %v", s)
					raw, err := os.ReadFile(s)
					if err != nil {
						return err
					}
					err = classDecompiler(raw, target)
					if err != nil {
						return err
					}
				}
				return nil
			}))
			if err != nil {
				return err
			}
		}
		inputs = lo.Filter(inputs, func(jar string, _ int) bool {
			jar = strings.TrimSpace(jar)
			if utils.GetFirstExistedFile(jar) != "" {
				log.Infof("find jar: %v", jar)
				return true
			}
			log.Warnf("jar file not existed: %v", jar)
			return false
		})

		if len(inputs) > 1 {
			for _, jarPath := range inputs {
				err := jarAction(true, jarPath, c)
				if err != nil {
					log.Warnf("jarAction failed: %v", err)
				}
			}
		} else if len(inputs) == 1 {
			return jarAction(false, inputs[0], c)
		} else {
			if len(handledClass) > 0 {
				log.Infof("compile %v java class files", len(handledClass))
				return nil
			}
			return utils.Errorf("no jar file found")
		}
		return nil
	},
}

func classDecompiler(raw []byte, targetFile string) error {
	obj, err := javaclassparser.Parse(raw)
	if err != nil {
		return err
	}
	decompilerStr, err := obj.Dump()
	if err != nil {
		return utils.Wrap(err, "javaclassparser.Parse(raw).Dump() failed")
	}
	return os.WriteFile(targetFile, []byte(decompilerStr), 0755)
}

func jarAction(multiMode bool, jarPath string, c *cli.Context) error {
	if filepath.Ext(jarPath) == ".class" {
		log.Infof("start to decompile %v", jarPath)
		target := strings.TrimSuffix(jarPath, ".class") + ".java"
		raw, err := os.ReadFile(jarPath)
		if err != nil {
			return err
		}
		err = classDecompiler(raw, target)
		if err != nil {
			return err
		}
		return nil
	}

	jarfs, err := javaclassparser.NewJarFSFromLocal(jarPath)
	if err != nil {
		return err
	}

	if utils.GetFirstExistedFile(jarPath) == "" {
		return utils.Errorf("jar file not existed: %v", jarPath)
	}

	compiledBase := c.String("output")
	if multiMode {
		compiledBase = ""
	}
	if compiledBase == "" {
		var notJar bool
		var jarName string
		_, jarName = filepath.Split(jarPath)
		compiledBase, notJar = strings.CutSuffix(jarName, ".jar")
		if !notJar {
			compiledBase = jarName
		}
	}
	compiledBase, err = filepath.Abs(compiledBase)
	if err != nil {
		return utils.Wrap(err, "filepath.Abs failed")
	}
	if utils.GetFirstExistedPath(compiledBase) == "" {
		log.Info("output directory not existed, create it, os.MkdirAll")
		err := os.MkdirAll(compiledBase, 0755)
		if err != nil {
			return utils.Wrap(err, "os.MkdirAll failed")
		}
	}
	if utils.GetFirstExistedPath(compiledBase) == "" {
		return utils.Errorf("output directory not existed")
	}

	failedDir := c.String("error-output")
	if multiMode {
		failedDir = ""
	}
	if failedDir == "" {
		dirName, _ := filepath.Split(compiledBase)
		failedDir = filepath.Join(dirName, "compiling-failed-files")
	}
	err = os.MkdirAll(failedDir, 0755)
	if err != nil {
		return utils.Wrap(err, "os.MkdirAll failed for failedDir")
	}

	log.Info("start to recursive jarfs")
	err = filesys.Recursive(".", filesys.WithFileSystem(jarfs), filesys.WithStat(func(isDir bool, s string, info fs.FileInfo) error {
		target := filepath.Join(compiledBase, s)
		if isDir {
			log.Infof("create dir: %v", target)
			err := os.MkdirAll(target, 0755)
			if err != nil {
				log.Warnf("os.MkdirAll failed: %v", err)
				return err
			}
			return nil
		}

		if jarfs.Ext(s) != ".class" {
			raw, err := jarfs.ZipFS.ReadFile(s)
			if err != nil {
				return err
			}
			return os.WriteFile(target, raw, 0755)
		}

		finished := utils.NewBool(false)
		go func() {
			time.Sleep(5 * time.Second)
			if finished.IsSet() {
				return
			}
			originFileRaw, _ := jarfs.ZipFS.ReadFile(s)
			log.Warnf("Jarpath: %v", jarPath)
			log.Warnf("Decompiler for %v is too slow, maybe it's a big(%v) class or bug here", s, utils.ByteSize(uint64(len(originFileRaw))))
			// saving to failed dir (block-slow)
			fileName := filepath.Base(s)
			fileName = strings.TrimSuffix(fileName, ".class")
			fileName = "decompiler-block-" + fileName + "-" + ksuid.New().String() + ".class"
			mirrorFailedFile := filepath.Join(failedDir, fileName)
			log.Warnf("write failed file: %v", mirrorFailedFile)
			if err := os.WriteFile(mirrorFailedFile, originFileRaw, 0755); err != nil {
				log.Errorf("os.WriteFile failed: %v", err)
			}
		}()
		raw, err := jarfs.ReadFile(s)
		finished.Set()
		if err != nil {
			log.Warnf("jarfs.ReadFile (Decompiler) failed: %v", err)
			raw, err := jarfs.ZipFS.ReadFile(s)
			if err != nil {
				return utils.Wrap(err, "jarfs.ZipFS.ReadFile failed")
			}
			fileName := filepath.Base(s)
			fileName = strings.TrimSuffix(fileName, ".class")
			fileName = "decompiler-err-" + fileName + "-" + ksuid.New().String() + ".class"
			mirrorFailedFile := filepath.Join(failedDir, fileName)
			log.Warnf("write failed file: %v", mirrorFailedFile)
			if err := os.WriteFile(mirrorFailedFile, raw, 0755); err != nil {
				return err
			}
			if err := os.WriteFile(target, raw, 0755); err != nil {
				return err
			}
			return nil
		}

		// fix file
		after := s
		if result, ok := strings.CutSuffix(after, ".class"); ok {
			after = result + ".java"
		} else {
			after = after + ".java"
		}
		target = filepath.Join(compiledBase, after)
		log.Infof("write file: %v", target)
		return os.WriteFile(target, raw, 0755)
	}))
	if err != nil {
		return err
	}
	return nil
}
