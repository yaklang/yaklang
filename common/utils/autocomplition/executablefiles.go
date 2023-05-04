package autocomplition

import (
	"fmt"
	"github.com/c-bata/go-prompt"
	"io/ioutil"
	"os"
	"path"
	"strings"
)

func GetPathExecutableFile() []prompt.Suggest {
	paths := os.Getenv("PATH")
	pathDirs := strings.Split(paths, ":")

	targets := map[string]prompt.Suggest{}

	for _, pathDir := range pathDirs {
		pathDir = strings.TrimSpace(pathDir)

		files, err := ioutil.ReadDir(pathDir)
		if err != nil {
			continue
		}

		for _, file := range files {
			sug := fileInfoToPromptSuggest(file)
			absFile := path.Join(pathDir, file.Name())

			_, ok := targets[file.Name()]
			if ok {
				sug.Text = absFile
				targets[absFile] = sug
			} else {
				sug.Description = fmt.Sprintf("[%s] %s", sug.Description, absFile)
				targets[file.Name()] = sug
			}
		}
	}

	var sugs []prompt.Suggest
	for _, s := range targets {
		sugs = append(sugs, s)
	}
	return sugs
}

var (
	ExistedExecutableSuggestions = GetPathExecutableFile()
)
