package autocomplition

import (
	"fmt"
	"github.com/c-bata/go-prompt"
	"io/ioutil"
	"os"
	utils2 "yaklang/common/utils"
	"path"
	"strings"
)

func fileInfoToPromptSuggest(file os.FileInfo) prompt.Suggest {
	var (
		data, desc string
	)

	data = file.Name()
	desc = fmt.Sprintf("size: %v IsDir: %v mode: %v",
		utils2.ByteCountDecimal(file.Size()),
		file.IsDir(),
		file.Mode().String(),
	)
	return prompt.Suggest{
		Text:        data,
		Description: desc,
	}
}

func GetWorkDirSuggestions(raw string) []prompt.Suggest {
	if raw == "" {
		raw = "."
	}

	files, err := ioutil.ReadDir(raw)
	if err != nil {
		raw = path.Dir(raw)

		files, _ = ioutil.ReadDir(raw)
	}

	var sugs []prompt.Suggest
	for _, file := range files {
		var (
			data = path.Join(raw, file.Name())
		)
		if file.IsDir() {
			if !strings.HasSuffix(data, "/") {
				data += "/"
			}
		}

		suggest := fileInfoToPromptSuggest(file)
		suggest.Text = data
		sugs = append(sugs, suggest)
	}

	return sugs
}
