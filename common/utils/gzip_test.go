package utils

import (
	"fmt"
	"io/ioutil"
	"testing"
)

func TestIsGzipBytes(t *testing.T) {
	// raw, err = file.ReadFile("/Users/v1ll4n/Project/palm/data/nmap-service-probes.txt")
	//die(err)
	//
	//bytes, err = gzip.Compress(raw)
	//die(err)
	//
	//a, err = gzip.Decompress(bytes)
	//die(err)
	//
	//if len(raw) != len(a) {
	//    panic(1)
	//}
	raw, err := ioutil.ReadFile("/Users/v1ll4n/Project/palm/data/nmap-service-probes.txt")
	if err != nil {
		panic(err)
	}

	bytes, err := GzipCompress(raw)
	if err != nil {
		panic(err)
	}
	if !IsGzip(bytes) {
		panic("no gzip")
	}

	data, err := GzipDeCompress(bytes)
	println(fmt.Sprintf("len(de-compress) => %v", len(data)))
	println(fmt.Sprintf("len(origin) => %v", len(raw)))
	if err != nil {
		panic(err)
	}

	if len(data) != len(raw) {
		panic(111)
	}
}
