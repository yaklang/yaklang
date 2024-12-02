package yakgit

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"os"
	"testing"
)

func TestFSConverter(t *testing.T) {
	fs, err := FromCommit("/Users/v1ll4n/Projects/yaklang", "f80b290a346dffaafb15964c4e10801066a8fccf")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(filesys.DumpTreeView(fs))

	fs, err = FromCommits("/Users/v1ll4n/Projects/yaklang", "f80b290a346dffaafb15964c4e10801066a8fccf", "54165a396a219d085980dca623ae1ff6582033ad")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(filesys.DumpTreeView(fs))

	fs, err = FromCommitRange("/Users/v1ll4n/Projects/yaklang", "54165a396a219d085980dca623ae1ff6582033ad", "f80b290a346dffaafb15964c4e10801066a8fccf")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(filesys.DumpTreeView(fs))

	filesys.SimpleRecursive(filesys.WithFileSystem(fs), filesys.WithFileStat(func(s string, info os.FileInfo) error {
		fmt.Println("--------------------------------")
		fmt.Println(s)
		raw, err := fs.ReadFile(s)
		if err != nil {
			return err
		}
		fmt.Println(string(raw))
		return nil
	}))
}
