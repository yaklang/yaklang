package fstools

import (
	"bytes"
	"encoding/json"
	"io"
	"os"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/filesys"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

func CreateSystemFSTools() ([]*aitool.Tool, error) {
	tools, err := CreateFSOperator(filesys.NewLocalFs())
	if err != nil {
		return nil, utils.Errorf("create fs operator: %v", err)
	}
	return tools, nil
}

func CreateFSOperator(fsys filesys_interface.FileSystem) ([]*aitool.Tool, error) {
	if fsys == nil || utils.IsNil(fsys) {
		return nil, utils.Errorf("fsys is nil")
	}

	var err error
	factory := aitool.NewFactory()
	err = factory.RegisterTool("ls",
		aitool.WithDescription("list files in directory"),
		aitool.WithStringParam("path", aitool.WithParam_Required(true)),
		aitool.WithCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			pathName := params.GetString("path")
			entries, err := fsys.ReadDir(pathName)
			if err != nil {
				return nil, utils.Errorf("read dir %s: %v", pathName, err)
			}
			var buf bytes.Buffer
			for _, entry := range entries {
				raw := map[string]any{
					"path":  entry.Name(),
					"isDir": entry.IsDir(),
					"type":  entry.Type().String(),
				}
				infoMap := make(map[string]any)
				raw["info"] = infoMap
				info, err := entry.Info()
				if err != nil {
					infoMap["_err"] = err.Error()
				} else {
					infoMap["name"] = info.Name()
					infoMap["size"] = info.Size()
					infoMap["mode"] = info.Mode()
					infoMap["modTime"] = info.ModTime()
				}
				rawJSON, _ := json.Marshal(raw)
				buf.WriteString(string(rawJSON))
				if len(rawJSON) > 0 {
					buf.WriteString("\n")
				}
			}
			return buf.String(), nil
		}),
	)
	if err != nil {
		log.Errorf("register ls tool: %v", err)
	}
	err = factory.RegisterTool(
		"read_file",
		aitool.WithDescription("read file content, considering the context size, adjust chunk and offset size"),
		aitool.WithStringParam("path",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("file path"),
		),
		aitool.WithIntegerParam("offset",
			aitool.WithParam_Default(0),
			aitool.WithParam_Description("offset to start reading"),
		),
		aitool.WithIntegerParam("chunk_size",
			aitool.WithParam_Default(2048),
			aitool.WithParam_Description("chunk size to read"),
		),
		aitool.WithCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			pathName := params.GetString("path")
			offset := params.GetInt("offset")
			chunkSize := params.GetInt("chunk_size")
			f, err := fsys.OpenFile(pathName, os.O_RDONLY, 0444)
			if err != nil {
				return nil, err
			}
			defer f.Close()

			// use seek
			if seeker, ok := f.(interface {
				Seek(offset int64, whence int) (int64, error)
			}); ok {
				_, err := seeker.Seek(int64(offset), io.SeekStart)
				if err != nil {
					os.Stderr.WriteString("seek failed: " + err.Error())
				} else {
					// read chunk
					buf := make([]byte, chunkSize)
					raw, err := f.Read(buf)
					if err != nil {
						os.Stderr.WriteString("read failed: " + err.Error())
					} else {
						return string(buf[:raw]), nil
					}
				}
			}

			// if offset > 0
			if offset > 0 {
				_, err := io.CopyN(io.Discard, f, int64(offset))
				if err != nil {
					os.Stderr.WriteString("discard failed: " + err.Error())
				} else {
					buf := make([]byte, chunkSize)
					raw, err := io.ReadFull(f, buf)
					if err != nil {
						os.Stderr.WriteString("read failed: " + err.Error())
					} else {
						return string(buf[:raw]), nil
					}
				}
			}

			// read chunk
			buf := make([]byte, chunkSize)
			raw, err := f.Read(buf)
			if err != nil {
				os.Stderr.WriteString("read failed: " + err.Error())
			} else {
				return string(buf[:raw]), nil
			}
			return nil, nil
		}),
	)
	if err != nil {
		log.Errorf("register read_file tool: %v", err)
	}

	err = factory.RegisterTool(
		"remove_file",
		aitool.WithDescription("remove file"),
		aitool.WithStringParam("path",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("file path"),
		),
		aitool.WithCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			pathName := params.GetString("path")
			err := fsys.Delete(pathName)
			if err != nil {
				return nil, err
			}
			return nil, nil
		}),
	)
	if err != nil {
		log.Errorf("register remove_file tool: %v", err)
	}

	err = factory.RegisterTool(
		"write_file",
		aitool.WithDescription("write file content"),
		aitool.WithStringParam("path",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("file path"),
		),
		aitool.WithStringParam("content",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("file content"),
		),
		aitool.WithCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			pathName := params.GetString("path")
			content := params.GetString("content")
			err := fsys.WriteFile(pathName, []byte(content), 0644)
			if err != nil {
				return nil, err
			}
			return nil, nil
		}),
	)
	if err != nil {
		log.Errorf("register write_file tool: %v", err)
	}
	err = factory.RegisterTool(
		"copy_file",
		aitool.WithDescription("copy file"),
		aitool.WithStringParam("src",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("source file path"),
		),
		aitool.WithStringParam("dst",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("destination file path"),
		),
		aitool.WithCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			src := params.GetString("src")
			dst := params.GetString("dst")
			r, err := fsys.OpenFile(src, os.O_RDONLY, 0444)
			if err != nil {
				return nil, err
			}
			defer r.Close()
			writeFile, err := fsys.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
			if err != nil {
				return nil, err
			}
			defer writeFile.Close()

			if w, ok := writeFile.(io.Writer); ok {
				_, err = io.Copy(w, r)
				if err != nil {
					return nil, err
				}
			} else {
				content, err := io.ReadAll(r)
				if err != nil {
					return nil, err
				}
				err = fsys.WriteFile(dst, content, 0644)
				if err != nil {
					return nil, err
				}
			}

			return "success", nil
		}),
	)
	if err != nil {
		log.Errorf("register copy_file tool: %v", err)
	}

	err = factory.RegisterTool(
		"tree",
		aitool.WithDescription("list files in directory recursively"),
		aitool.WithStringParam("path", aitool.WithParam_Required(true)),
		aitool.WithIntegerParam("limit", aitool.WithParam_Required(true), aitool.WithParam_Default(20)),
		aitool.WithIntegerParam("offset", aitool.WithParam_Default(0)),
		aitool.WithCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			path := params.GetString("path")
			limit := params.GetInt("limit")
			offset := params.GetInt("offset")

			counter := int64(0)
			var buf bytes.Buffer
			err := filesys.Recursive(
				path,
				filesys.WithFileSystem(fsys),
				filesys.WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
					if counter >= offset {
						if counter >= offset+limit {
							return utils.Error("more than limit")
						}
						raw := map[string]any{
							"path":  pathname,
							"isDir": isDir,
							"type":  info.Mode().String(),
						}
						infoMap := make(map[string]any)
						raw["info"] = infoMap
						infoMap["name"] = info.Name()
						infoMap["size"] = info.Size()
						infoMap["mode"] = info.Mode()
						infoMap["modTime"] = info.ModTime()
						rawJSON, _ := json.Marshal(raw)
						buf.WriteString(string(rawJSON))
						if len(rawJSON) > 0 {
							buf.WriteString("\n")
						}
					}
					counter++
					return nil
				}),
			)
			if err != nil {
				return nil, err
			}
			return buf.String(), nil
		}),
	)
	if err != nil {
		log.Errorf("register tree tool: %v", err)
	}

	return factory.Tools(), nil
}
