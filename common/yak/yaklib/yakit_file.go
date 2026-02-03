package yaklib

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/yaklang/yaklang/common/utils"
)

type YakitFileAction struct {
	Action  string
	Message map[string]any
}

const (
	// Keep file-level action payload small to avoid hitting gRPC message size limits.
	// These actions are primarily used for logging/preview in yakit, not bulk transfer.
	yakitFileActionMaxContentBytes   = 8 * 1024
	yakitFileActionMaxMessagePreview = 512
	yakitFileActionMaxFindItems      = 64
	yakitFileActionMaxFindItemBytes  = 512
)

var (
	Read_Action   = "READ"
	Write_Action  = "WRITE"
	Create_Action = "CREATE"
	Delete_Action = "DELETE"
	Status_Action = "STATUS"
	Chmod_Action  = "CHMOD"
	Find_Action   = "FIND"
)

func shrinkBytesPreview(b []byte, max int) (preview string, truncated bool) {
	if max <= 0 {
		max = yakitFileActionMaxContentBytes
	}
	if len(b) <= max {
		return string(b), false
	}
	half := max / 2
	if half <= 0 {
		half = 1
	}
	buf := make([]byte, 0, max+3)
	buf = append(buf, b[:half]...)
	buf = append(buf, []byte("...")...)
	buf = append(buf, b[len(b)-half:]...)
	return string(buf), true
}

func shrinkStringPreview(s string, maxBytes int) (preview string, truncated bool) {
	if maxBytes <= 0 {
		maxBytes = yakitFileActionMaxFindItemBytes
	}
	if len(s) <= maxBytes {
		return s, false
	}
	half := maxBytes / 2
	if half <= 0 {
		half = 1
	}
	return s[:half] + "..." + s[len(s)-half:], true
}

func shrinkStringSlicePreview(in []string, maxItems int, maxItemBytes int) (out []string, truncated bool) {
	if maxItems <= 0 {
		maxItems = yakitFileActionMaxFindItems
	}
	if maxItemBytes <= 0 {
		maxItemBytes = yakitFileActionMaxFindItemBytes
	}

	n := len(in)
	if n > maxItems {
		n = maxItems
		truncated = true
	}

	out = make([]string, 0, n)
	for i := 0; i < n; i++ {
		p, t := shrinkStringPreview(in[i], maxItemBytes)
		if t {
			truncated = true
		}
		out = append(out, p)
	}
	return out, truncated
}

func FileReadAction(offset int, length int, unit string, content []byte) *YakitFileAction {
	preview, truncated := shrinkBytesPreview(content, yakitFileActionMaxContentBytes)
	preview = utils.ShrinkTextBlock(preview, yakitFileActionMaxContentBytes)
	return &YakitFileAction{
		Action: Read_Action,
		Message: map[string]any{
			"message":          fmt.Sprintf("read file with offset %d, length %d, unit %s", offset, length, unit),
			"offset":           offset,
			"length":           length,
			"unit":             unit,
			"content":          preview,
			"contentSize":      len(content),
			"contentTruncated": truncated,
		},
	}
}

func FileWriteAction(length int, mode string, content []byte) *YakitFileAction {
	preview, truncated := shrinkBytesPreview(content, yakitFileActionMaxContentBytes)
	preview = utils.ShrinkTextBlock(preview, yakitFileActionMaxContentBytes)
	return &YakitFileAction{
		Action: Write_Action,
		Message: map[string]any{
			"message":          fmt.Sprintf("write file with length %d, mode %s", length, mode),
			"length":           length,
			"mode":             mode, // append | cover
			"content":          preview,
			"contentSize":      len(content),
			"contentTruncated": truncated,
		},
	}
}

func FileCreateAction(isDir bool, chmodMode uint32) *YakitFileAction {
	return &YakitFileAction{
		Action: Create_Action,
		Message: map[string]any{
			"message":   fmt.Sprintf("create file, isDir: %t, chmodMode: %s", isDir, filemode.FileMode(chmodMode).String()),
			"isDir":     isDir,
			"chmodMode": filemode.FileMode(chmodMode).String(),
		},
	}
}

func FileDeleteAction(isDir bool) *YakitFileAction {
	return &YakitFileAction{
		Action: Delete_Action,
		Message: map[string]any{
			"message": fmt.Sprintf("delete file, isDir: %t", isDir),
			"isDir":   isDir,
		},
	}
}

func FileStatusAction(status any) *YakitFileAction {
	statusMap := make(map[string]any)
	fileInfo, ok := status.(os.FileInfo)
	if ok {
		statusMap["name"] = fileInfo.Name()
		statusMap["size"] = fileInfo.Size()
		statusMap["mode"] = fileInfo.Mode().String()
		statusMap["modTime"] = fileInfo.ModTime().String()
		statusMap["isDir"] = fileInfo.IsDir()
	} else {
		statusMap = utils.InterfaceToGeneralMap(status)
	}
	msgPreview := utils.ShrinkString(fmt.Sprintf("get file status: %+v", statusMap), yakitFileActionMaxMessagePreview)
	return &YakitFileAction{
		Action: Status_Action,
		Message: map[string]any{
			"message": msgPreview,
			"status":  statusMap,
		},
	}
}

func FileChmodAction(chmodMode uint32) *YakitFileAction {
	return &YakitFileAction{
		Action: Chmod_Action,
		Message: map[string]any{
			"message":   fmt.Sprintf("change file mode to %s", filemode.FileMode(chmodMode).String()),
			"chmodMode": filemode.FileMode(chmodMode).String(),
		},
	}
}

func FileFindAction(findMode string, findCondition string, findContent ...string) *YakitFileAction {
	findConditionPreview := utils.ShrinkString(findCondition, yakitFileActionMaxFindItemBytes)
	shrunkContent, contentTruncated := shrinkStringSlicePreview(findContent, yakitFileActionMaxFindItems, yakitFileActionMaxFindItemBytes)
	return &YakitFileAction{
		Action: Find_Action,
		Message: map[string]any{
			"message":          fmt.Sprintf("find file [mode:%s] [condition:%s]", findMode, findConditionPreview),
			"mode":             findMode, // name | content | all
			"content":          shrunkContent,
			"condition":        findConditionPreview,
			"contentTotal":     len(findContent),
			"contentTruncated": contentTruncated,
		},
	}
}

func (a *YakitFileAction) String() string {
	actionString, err := json.Marshal(a)
	if err != nil {
		return ""
	}
	return string(actionString)
}

func init() {
	YakitExports["fileReadAction"] = FileReadAction
	YakitExports["fileWriteAction"] = FileWriteAction
	YakitExports["fileCreateAction"] = FileCreateAction
	YakitExports["fileDeleteAction"] = FileDeleteAction
	YakitExports["fileStatusAction"] = FileStatusAction
	YakitExports["fileChmodAction"] = FileChmodAction
	YakitExports["fileFindAction"] = FileFindAction
}
