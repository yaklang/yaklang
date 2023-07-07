package consts

import (
	"fmt"
	"testing"
)

func TestGetDefaultBaseHomeDir(t *testing.T) {
	println(GetDefaultBaseHomeDir())
}

func TestGetVulinboxPath(t *testing.T) {
	a := GetVulinboxPath()
	fmt.Println(a)

}
