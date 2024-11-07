package yakcmds

import (
	"errors"
	"github.com/samber/lo"
	"github.com/segmentio/ksuid"
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
			return errors.New("only one of --jar and --jar-directory can be set")
		}
		if !c.IsSet("jar") && !c.IsSet("jar-directory") {
			return errors.New("one of --jar and --jar-directory must be set")
		}
		var jars []string
		var handledClass []string
		if c.IsSet("jar") {
			jarPath := c.String("jar")
			jarPaths := strings.Split(jarPath, ",")
			for _, jar := range jarPaths {
				jars = append(jars, jar)
			}
		} else {
			dirMode := c.String("jar-directory")
			err := filesys.Recursive(dirMode, filesys.WithFileStat(func(s string, info fs.FileInfo) error {
				if strings.HasSuffix(s, ".jar") {
					jars = append(jars, s)
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
		jars = lo.Filter(jars, func(jar string, _ int) bool {
			jar = strings.TrimSpace(jar)
			if utils.GetFirstExistedFile(jar) != "" {
				log.Infof("find jar: %v", jar)
				return true
			}
			log.Warnf("jar file not existed: %v", jar)
			return false
		})

		if len(jars) > 1 {
			for _, jarPath := range jars {
				err := jarAction(true, jarPath, c)
				if err != nil {
					log.Warnf("jarAction failed: %v", err)
				}
			}
		} else if len(jars) == 1 {
			return jarAction(false, jars[0], c)
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

		raw, err := jarfs.ReadFile(s)
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
