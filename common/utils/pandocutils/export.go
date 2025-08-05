package pandocutils

import (
	"context"
	"fmt"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"path/filepath"
)

// pandoc.SimpleConvertMarkdownFileToDocxContext can convert markdown to docx file
//
// example:
// ```
// md := "filename.md"
// ctx := context.Background()
// result, err := pandoc.SimpleConvertMarkdownFileToDocxContext(ctx, md)
// if err != nil { die(err) }
// // println("Converted file path:", result)
// ```
func _simpleConvertMarkdownFileToDocxContext(ctx context.Context, md string) (string, error) {
	filename := fmt.Sprintf("md2docx-%v.docx", ksuid.New().String())
	dirname := filepath.Join(consts.GetDefaultYakitBaseTempDir(), "pandoc-output")
	os.MkdirAll(dirname, os.ModePerm)
	pathName := filepath.Join(dirname, filename)
	err := SimpleCovertMarkdownToDocx(ctx, md, pathName)
	if err != nil {
		return "", fmt.Errorf("failed to convert markdown to docx: %w", err)
	}
	if !utils.FileExists(pathName) {
		return "", utils.Errorf("output file does not exist after conversion: %s", pathName)
	}
	return pathName, nil
}

// pandoc.SimpleConvertMarkdownFileToDocx can convert markdown to docx file
//
// example:
// ```
// md := "filename.md"
// outputFile, err = pandoc.SimpleConvertMarkdownFileToDocx(md)
// ```
func _simpleConvertMarkdownFileToDocx(md string) (string, error) {
	return _simpleConvertMarkdownFileToDocxContext(context.Background(), md)
}

var Exports = map[string]any{
	"SimpleConvertMarkdownFileToDocxContext": _simpleConvertMarkdownFileToDocxContext,
	"SimpleConvertMarkdownFileToDocx":        _simpleConvertMarkdownFileToDocx,
	"SimpleCoverMD2Word":                     SimpleCovertMarkdownToDocx,
}
