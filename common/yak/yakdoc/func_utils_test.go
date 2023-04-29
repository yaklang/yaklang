package yakdoc

import (
	"fmt"
	"testing"
)

func ABC() {

}

func ABC2(i int, b interface{}) {

}

func ABC3(i int, b interface{}, c ...string) int {
	return 0
}

func ABC4(c ...string) {

}

func ABC5(c ...string) string {
	return ""
}

func ABC6(c ...string) (string, int) {
	return "", 1
}

func ABC7(c ...string) (ab string, _ int, err error) {
	return "", 1, nil
}

func TestFuncToFuncDecl(t *testing.T) {
	for index, i := range []interface{}{
		ABC, ABC2, ABC3, ABC4, ABC5,
		ABC6, ABC7,
	} {
		f := FuncToFuncDecl("test", fmt.Sprintf("ABC%d", index+1), i)
		fmt.Println(f.Decl)
		fmt.Println(f.VSCodeSnippets)
		println()
	}
}
