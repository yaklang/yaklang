package yakcmds

import (
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var JavaDecompilerCommand = &cli.Command{
	Name:    "java-decompiler",
	Usage:   `Java Decompiler in Thirdparty Implemented`,
	Aliases: []string{"jd"},
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "jar,j",
			Usage: "--jar <jar file> to decompile",
		},
		cli.StringFlag{
			Name:  "output,o",
			Usage: "output directory",
		},
	},
	Action: func(c *cli.Context) error {
		jarPath := c.String("jar")
		jarfs, err := javaclassparser.NewJarFSFromLocal(jarPath)
		if err != nil {
			return err
		}

		if utils.GetFirstExistedFile(jarPath) == "" {
			return utils.Errorf("jar file not existed: %v", jarPath)
		}

		compiledBase := c.String("output")
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
				raw, err := jarfs.ReadFile(s)
				if err != nil {
					return err
				}
				os.WriteFile(target, raw, 0755)
				return nil
			}

			raw, err := jarfs.ReadFile(s)
			if err != nil {
				log.Warnf("jarfs.ReadFile (Decompiler) failed: %v", err)
				raw, err := jarfs.ZipFS.ReadFile(s)
				if err != nil {
					return utils.Wrap(err, "jarfs.ZipFS.ReadFile failed")
				}
				return os.WriteFile(target, raw, 0755)
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
	},
}
