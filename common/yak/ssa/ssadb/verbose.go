package ssadb

import (
	"bytes"
	"fmt"
)

func (i *IrCode) VerboseString() string {
	buf := bytes.NewBufferString("")
	buf.WriteString(fmt.Sprintf("%-5s: %v - %v", fmt.Sprint(i.ID), i.OpcodeName, i.ShortVerboseName))
	return buf.String()
}

func (i *IrCode) Show() {
	fmt.Println(i.VerboseString())
}
