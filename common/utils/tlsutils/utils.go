package tlsutils

import (
	"bufio"
	"bytes"
	"yaklang/common/utils"
	"yaklang/common/yak/yaklib/codec"
)

func SplitBlock(raw []byte, blockSize int) ([]string, error) {
	scanner := bufio.NewScanner(bytes.NewBuffer(raw))
	scanner.Split(bufio.ScanBytes)

	var results []string
	var buff []byte
	for scanner.Scan() {
		buff = append(buff, scanner.Bytes()...)

		if len(buff) > blockSize {
			return nil, utils.Errorf("BUG for tlsutil.SplitBlock, split err")
		}

		if len(buff) == blockSize {
			results = append(results, codec.EncodeToHex(buff))
			buff = nil
		}
	}

	if len(buff) > blockSize {
		return nil, utils.Errorf("BUG for tlsutil.SplitBlock, split err")
	}

	if len(buff) > 0 {
		results = append(results, codec.EncodeToHex(buff))
	}
	return results, nil
}

func MergeBlock(raw []string) ([]byte, error) {
	var buffer []byte
	for _, r := range raw {
		results, err := codec.DecodeHex(r)
		if err != nil {
			return nil, err
		}
		buffer = append(buffer, results...)
	}
	return buffer, nil
}
