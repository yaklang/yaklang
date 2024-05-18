package yakdiff

import (
	"fmt"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/yaklang/yaklang/common/utils"
)

func _defaultPatchHandler(commit *object.Commit, change *object.Change, patch *object.Patch) error {
	if patch == nil {
		return nil
	}

	for _, fp := range patch.FilePatches() {
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
			continue
		}

		fmt.Printf("-----------------------------------------------------\n"+
			"changes:[%-4s] | %v\n", fmt.Sprint(len(fp.Chunks())), filename)
		for _, chunked := range fp.Chunks() {
			verbose := chunked.Content()
			verbose = utils.ShrinkString(verbose, 64)

			fmt.Printf("  | %-67s |\n", verbose)
		}
	}

	return nil
}
