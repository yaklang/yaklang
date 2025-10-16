package ssadb

import (
	"bytes"
	"fmt"
)

func (i *IrCode) VerboseString() string {
	buf := bytes.NewBufferString("")
	hashShort := i.SourceCodeHash
	if len(hashShort) > 5 {
		hashShort = hashShort[:5]
	}
	buf.WriteString(fmt.Sprintf("%5s:%-5s: %v - %v", hashShort, fmt.Sprint(i.CodeID), i.OpcodeName, i.ShortVerboseName))
	return buf.String()
}

func (i *IrCode) Show() {
	fmt.Println(i.VerboseString())
}
