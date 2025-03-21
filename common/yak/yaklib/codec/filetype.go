package codec

import "github.com/h2non/filetype"

var javaClassType = filetype.NewType("class", "application/java-class")

func javaClassTypeMatcher(buf []byte) bool {
	return len(buf) >= 4 && buf[0] == 0xAC && buf[1] == 0xED &&
		buf[2] == 0x00 && buf[3] == 0x05
}

func init() {
	filetype.AddMatcher(javaClassType, javaClassTypeMatcher)
}
