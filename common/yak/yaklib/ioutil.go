package yaklib

import (
	"context"
	"io"
	"io/ioutil"
	"net"
	"yaklang/common/utils"
	"time"
)

var IoExports = map[string]interface{}{
	"ReadAll":  ioutil.ReadAll,
	"ReadFile": ioutil.ReadFile,
	"ReadEvery1s": func(c context.Context, reader io.Reader, f func([]byte) bool) {
		utils.ReadWithContextTickCallback(c, reader, f, 1*time.Second)
	},

	// 继承自 io
	"NopCloser":   ioutil.NopCloser,
	"Copy":        io.Copy,
	"CopyN":       io.CopyN,
	"Discard":     ioutil.Discard,
	"EOF":         io.EOF,
	"LimitReader": io.LimitReader,
	"MultiReader": io.MultiReader,
	//"NewSectionReader": io.NewSectionReader,
	"Pipe": io.Pipe,
	//"ReadFull":         io.ReadFull,
	//"ReadAtLeast":      io.ReadAtLeast,
	"TeeReader":   io.TeeReader,
	"WriteString": io.WriteString,
	"ReadStable": func(conn net.Conn, float float64) []byte {
		return utils.StableReader(conn, utils.FloatSecondDuration(float), 10*1024*1024)
	},
}
