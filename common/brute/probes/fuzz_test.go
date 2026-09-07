package probes_test

import (
	"testing"

	"github.com/yaklang/yaklang/common/brute/probes/mongodb"
	"github.com/yaklang/yaklang/common/brute/probes/mssql"
	"github.com/yaklang/yaklang/common/brute/probes/mysql"
	"github.com/yaklang/yaklang/common/brute/probes/postgres"
)

// FuzzMySQLGreeting 模糊 MySQL Initial Handshake 解析器：不得 panic。
func FuzzMySQLGreeting(f *testing.F) {
	f.Add([]byte{10, 'm', 'y', 's', 'q', 'l', 0, 1, 0, 0, 0, 1, 2, 3, 4, 5, 6, 7, 8, 0, 0})
	f.Add([]byte{9})
	f.Add([]byte{10})
	f.Add([]byte{})
	f.Add([]byte{10, 'x', 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = mysql.ParseGreetingForFuzz(data)
	})
}

// FuzzBSONDecode 模糊 BSON 解码器：不得 panic。
func FuzzBSONDecode(f *testing.F) {
	f.Add([]byte{5, 0, 0, 0, 0})
	f.Add([]byte{16, 0, 0, 0, 0x02, 'a', 0, 2, 0, 0, 0, 'b', 0, 0})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 1})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = mongodb.DecodeDForFuzz(data)
	})
}

// FuzzPGErrorResponse 模糊 PostgreSQL ErrorResponse 解析。
func FuzzPGErrorResponse(f *testing.F) {
	f.Add([]byte{'S', 'F', 'A', 'T', 'A', 'L', 0, 'C', '2', '8', 'P', '0', '1', 0, 'M', 'm', 's', 'g', 0, 0})
	f.Add([]byte{0})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		_ = postgres.ParseErrorResponseForFuzz(data)
	})
}

// FuzzMSSQLPrelogin 模糊 TDS PRELOGIN 解析。
func FuzzMSSQLPrelogin(f *testing.F) {
	f.Add([]byte{0, 0, 26, 0, 6, 1, 0, 32, 0, 1, 0xff})
	f.Add([]byte{0xff})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = mssql.ParsePreloginForFuzz(data)
	})
}

// FuzzMSSQLTokens 模糊 TDS 响应 token 流解析。
func FuzzMSSQLTokens(f *testing.F) {
	f.Add([]byte{0xAA, 4, 0, 0x48, 0, 0, 0, 1, 14, 2, 0, 'o', 'k', 0, 0})
	f.Add([]byte{0xFD})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _, _ = mssql.ParseTokensForFuzz(data)
	})
}
