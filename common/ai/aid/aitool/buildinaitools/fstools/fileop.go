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
		aitool.WithDescription("list files in directory or get file info"),
		aitool.WithStringParam("path", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			pathName := params.GetString("path")

			// Check if path exists first
			fileInfo, err := fsys.Stat(pathName)
			if err != nil {
				errMsg := utils.Errorf("stat %s: %v", pathName, err)
				stderr.Write([]byte("failed to stat path: " + pathName + "\n"))
				return nil, errMsg
			}

			// If it's a file, return file info directly
			if !fileInfo.IsDir() {
				var buf bytes.Buffer
				raw := map[string]any{
					"path":  pathName,
					"isDir": false,
					"type":  fileInfo.Mode().String(),
				}
				infoMap := make(map[string]any)
				raw["info"] = infoMap
				infoMap["name"] = fileInfo.Name()
				infoMap["size"] = fileInfo.Size()
				infoMap["mode"] = fileInfo.Mode()
				infoMap["modTime"] = fileInfo.ModTime()

				rawJSON, _ := json.Marshal(raw)
				buf.WriteString(string(rawJSON))
				buf.WriteString("\n")

				stdout.Write([]byte("file info retrieved: " + pathName + "\n"))
				return buf.String(), nil
			}

			// Otherwise, handle directory as before
			entries, err := fsys.ReadDir(pathName)
			if err != nil {
				errMsg := utils.Errorf("read dir %s: %v", pathName, err)
				stderr.Write([]byte("failed to read directory: " + pathName + "\n"))
				return nil, errMsg
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
			stdout.Write([]byte("listed " + utils.InterfaceToString(len(entries)) + " entries in directory: " + pathName + "\n"))
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
			aitool.WithParam_Default(20480),
			aitool.WithParam_Description("chunk size to read"),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			pathName := params.GetString("path")
			offset := params.GetInt("offset")
			chunkSize := params.GetInt("chunk_size")
			f, err := fsys.OpenFile(pathName, os.O_RDONLY, 0444)
			if err != nil {
				stderr.Write([]byte("failed to open file: " + pathName + "\n"))
				return nil, err
			}
			defer f.Close()

			// use seek
			if seeker, ok := f.(interface {
				Seek(offset int64, whence int) (int64, error)
			}); ok {
				_, err := seeker.Seek(int64(offset), io.SeekStart)
				if err != nil {
					stderr.Write([]byte("seek failed: " + err.Error() + "\n"))
				} else {
					// read chunk
					buf := make([]byte, chunkSize)
					raw, err := f.Read(buf)
					if err != nil && err != io.EOF {
						stderr.Write([]byte("read failed: " + err.Error() + "\n"))
					} else {
						content := string(buf[:raw])
						stdout.Write([]byte("read " + utils.InterfaceToString(raw) + " bytes from file: " + pathName + " (offset: " + utils.InterfaceToString(offset) + ")\n"))
						return content, nil
					}
				}
			}

			// if offset > 0
			if offset > 0 {
				_, err := io.CopyN(io.Discard, f, int64(offset))
				if err != nil {
					stderr.Write([]byte("discard failed: " + err.Error() + "\n"))
				} else {
					buf := make([]byte, chunkSize)
					raw, err := io.ReadFull(f, buf)
					if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
						stderr.Write([]byte("read failed: " + err.Error() + "\n"))
					} else {
						content := string(buf[:raw])
						stdout.Write([]byte("read " + utils.InterfaceToString(raw) + " bytes from file: " + pathName + " (offset: " + utils.InterfaceToString(offset) + ")\n"))
						return content, nil
					}
				}
			}

			// read chunk
			buf := make([]byte, chunkSize)
			raw, err := f.Read(buf)
			if err != nil && err != io.EOF {
				stderr.Write([]byte("read failed: " + err.Error() + "\n"))
			} else {
				content := string(buf[:raw])
				stdout.Write([]byte("read " + utils.InterfaceToString(raw) + " bytes from file: " + pathName + "\n"))
				return content, nil
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
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			pathName := params.GetString("path")
			err := fsys.Delete(pathName)
			if err != nil {
				stderr.Write([]byte("failed to remove file: " + pathName + "\n"))
				return nil, err
			}
			stdout.Write([]byte("successfully removed file: " + pathName + "\n"))
			return "success", nil
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
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			pathName := params.GetString("path")
			content := params.GetString("content")
			err := fsys.WriteFile(pathName, []byte(content), 0644)
			if err != nil {
				stderr.Write([]byte("failed to write file: " + pathName + "\n"))
				return nil, err
			}
			stdout.Write([]byte("successfully wrote " + utils.InterfaceToString(len(content)) + " bytes to file: " + pathName + "\n"))
			return "success", nil
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
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			src := params.GetString("src")
			dst := params.GetString("dst")
			r, err := fsys.OpenFile(src, os.O_RDONLY, 0444)
			if err != nil {
				stderr.Write([]byte("failed to open source file: " + src + "\n"))
				return nil, err
			}
			defer r.Close()
			writeFile, err := fsys.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
			if err != nil {
				stderr.Write([]byte("failed to open destination file: " + dst + "\n"))
				return nil, err
			}
			defer writeFile.Close()

			var bytesCopied int64
			if w, ok := writeFile.(io.Writer); ok {
				bytesCopied, err = io.Copy(w, r)
				if err != nil {
					stderr.Write([]byte("failed to copy file content\n"))
					return nil, err
				}
			} else {
				content, err := io.ReadAll(r)
				if err != nil {
					stderr.Write([]byte("failed to read source file\n"))
					return nil, err
				}
				err = fsys.WriteFile(dst, content, 0644)
				if err != nil {
					stderr.Write([]byte("failed to write destination file\n"))
					return nil, err
				}
				bytesCopied = int64(len(content))
			}

			stdout.Write([]byte("successfully copied " + utils.InterfaceToString(bytesCopied) + " bytes from " + src + " to " + dst + "\n"))
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
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			path := params.GetString("path")
			limit := params.GetInt("limit")
			offset := params.GetInt("offset")

			counter := int64(0)
			resultCount := int64(0)
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
						resultCount++
					}
					counter++
					return nil
				}),
			)
			if err != nil {
				stderr.Write([]byte("failed to traverse directory: " + path + "\n"))
				return nil, err
			}
			stdout.Write([]byte("listed " + utils.InterfaceToString(resultCount) + " items recursively from: " + path + " (offset: " + utils.InterfaceToString(offset) + ", limit: " + utils.InterfaceToString(limit) + ")\n"))
			return buf.String(), nil
		}),
	)
	if err != nil {
		log.Errorf("register tree tool: %v", err)
	}

	return factory.Tools(), nil
}
