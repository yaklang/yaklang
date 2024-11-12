package filesys

import (
	"fmt"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/filesys/treeview"
	"os"
)

func DumpTreeView(f fi.FileSystem) string {
	var arrs []string
	SimpleRecursive(WithFileSystem(f), WithStat(func(isDir bool, pathname string, info os.FileInfo) error {
		arrs = append(arrs, pathname)
		return nil
	}))
	return treeview.NewTreeView(arrs).Print()
}

func TreeView(f fi.FileSystem) {
	fmt.Println(DumpTreeView(f))
}
