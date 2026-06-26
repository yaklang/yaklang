//go:build cgo && minirehs_mvs_amalg

package minirehs

// 本文件仅在 `minirehs_mvs_amalg` 构建标签下参与编译, 作用是给 cgo 注入
// -DMVS_USE_AMALGAMATION, 让 mvs_cgo.go 的预处理改为编入单文件 amalgamation 产物
// (native/mvscan/amalgamation/mvscan.c) 而非多文件源. 这样可用同一套差分/oracle 测试
// 直接验证发行单文件的运行期行为 (而不仅是能否编译).
//
// 用法: go test -tags 'minirehs_mvs minirehs_mvs_amalg' ./common/minirehs/...
//
// 关键词: mvscan, amalgamation, cgo, build tag, runtime verification

/*
#cgo CFLAGS: -DMVS_USE_AMALGAMATION=1
*/
import "C"
