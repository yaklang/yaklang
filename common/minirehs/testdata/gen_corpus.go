//go:build ignore

// gen_corpus.go 是一次性语料生成工具 (不参与正常构建, 用 //go:build ignore 隔离).
// 它从本地 yaklang 项目数据库 (default-yakit.db) 抽取真实 HTTP 流量, 把每条流量的
// request 与 response 报文写入 testdata/traffic_corpus.bin, 供 benchmark 复现使用.
//
// 用法 (需 CGO, 因为 go-sqlite3 是 cgo 驱动):
//
//	CGO_ENABLED=1 go run testdata/gen_corpus.go \
//	    -db ~/yakit-projects/default-yakit.db \
//	    -out testdata/traffic_corpus.bin -max 5242880
//
// 语料格式: 连续的 [4 字节小端长度][该长度的报文字节] 记录, 末尾无填充.
//
// 关键词: corpus, http_flows, traffic sample, benchmark data
package main

import (
	"database/sql"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"strconv"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	home, _ := os.UserHomeDir()
	dbPath := flag.String("db", home+"/yakit-projects/default-yakit.db", "yakit project sqlite db path")
	outPath := flag.String("out", "testdata/traffic_corpus.bin", "output corpus path")
	maxBytes := flag.Int("max", 5*1024*1024, "approximate max corpus size in bytes")
	flag.Parse()

	db, err := sql.Open("sqlite3", *dbPath+"?mode=ro")
	if err != nil {
		fmt.Fprintln(os.Stderr, "open db:", err)
		os.Exit(1)
	}
	defer db.Close()

	// 选择体量适中的报文, 避免单条超大报文主导语料; 按 id 顺序稳定可复现.
	rows, err := db.Query(`SELECT request, response FROM http_flows
		WHERE length(response) > 200 AND length(response) < 160000
		  AND length(request) > 80 AND length(request) < 80000
		ORDER BY id`)
	if err != nil {
		fmt.Fprintln(os.Stderr, "query:", err)
		os.Exit(1)
	}
	defer rows.Close()

	out, err := os.Create(*outPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "create out:", err)
		os.Exit(1)
	}
	defer out.Close()

	var total, records int
	writeRecord := func(b []byte) bool {
		if len(b) == 0 {
			return false
		}
		var hdr [4]byte
		binary.LittleEndian.PutUint32(hdr[:], uint32(len(b)))
		out.Write(hdr[:])
		out.Write(b)
		total += len(b) + 4
		records++
		return total >= *maxBytes
	}

	unquote := func(s string) []byte {
		if s == "" {
			return nil
		}
		if uq, err := strconv.Unquote(s); err == nil {
			return []byte(uq)
		}
		return []byte(s) // 已是原始字节时直接用
	}

	done := false
	for rows.Next() && !done {
		var req, rsp string
		if err := rows.Scan(&req, &rsp); err != nil {
			continue
		}
		if r := unquote(req); len(r) > 0 {
			if writeRecord(r) {
				done = true
			}
		}
		if done {
			break
		}
		if r := unquote(rsp); len(r) > 0 {
			if writeRecord(r) {
				done = true
			}
		}
	}

	fmt.Printf("corpus written: %s bytes=%d records=%d\n", *outPath, total, records)
}
