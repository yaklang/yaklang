package filesys

import (
	"fmt"
	"os"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/filesys/treeview"
)

func DumpTreeView(f fi.FileSystem) string {
	var arrs []string
	SimpleRecursive(WithFileSystem(f), WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
		arrs = append(arrs, pathname)
		return nil
	}))
	return treeview.NewTreeView(arrs).Print()
}

func DumpTreeViewWithLimits(f fi.FileSystem, maxDepth, maxLines int) string {
	return treeview.NewTreeViewFromFSWithLimits(f, ".", maxDepth, maxLines).Print()
}

func TreeView(f fi.FileSystem) {
	fmt.Println(DumpTreeView(f))
}
