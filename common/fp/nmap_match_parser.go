package fp

import (
	"bufio"
	"bytes"
	"github.com/pkg/errors"
)

type DataBlock struct {
	Name    string
	Content []byte
	Option  []byte
}

type Rule struct {
	Type        string
	ServiceName string
	DataBlocks  map[byte]*DataBlock
	CpeBlocks   []*DataBlock
}

const (
	delimiter = ' '
)

const (
	statInit                = 0
	statStart               = 1
	statInData              = 2
	statStartData           = 3
	statSetDataDelimiter    = 4
	statSetDataBlockOptions = 5
)

func parseMatchRule(line []byte) (*Rule, error) {
	scanner := bufio.NewScanner(bytes.NewBuffer(line))
	scanner.Split(bufio.ScanBytes)

	var (
		dataBlockNameBuffer []byte
		ch                  byte
		state               int
		dataDelimiter       byte
		currentDataBlock    = &DataBlock{}
		cpeBlocks           []*DataBlock
	)

	rule := &Rule{
		DataBlocks: map[byte]*DataBlock{},
	}

	for scanner.Scan() {
		ch = scanner.Bytes()[0]

		switch state {
		case statSetDataBlockOptions:
			if delimiter == ch {
				state = statStartData
				continue
			}

			currentDataBlock.Option = append(currentDataBlock.Option, ch)
			continue
		case statInData:
			if ch == dataDelimiter {
				state = statSetDataBlockOptions
				continue
			}

			currentDataBlock.Content = append(currentDataBlock.Content, ch)
			continue
		case statSetDataDelimiter:
			dataDelimiter = ch
			state = statInData
			continue
		case statStartData:
			// p/v/i/h/o/d/cpe
			if bytes.Contains([]byte{'p', 'v', 'i', 'h', 'o', 'd', 'm'}, []byte{ch}) && len(dataBlockNameBuffer) <= 0 {
				state = statSetDataDelimiter
				currentDataBlock = &DataBlock{Name: string(ch)}
				rule.DataBlocks[ch] = currentDataBlock
				continue
			}

			if len(dataBlockNameBuffer) > 4 {
				return nil, errors.Errorf("parse data block failed, buffer failed: %s", string(append(dataBlockNameBuffer, ch)))
			}

			dataBlockNameBuffer = append(dataBlockNameBuffer, ch)
			if len(dataBlockNameBuffer) >= 4 {
				if len(dataBlockNameBuffer) == 4 {
					if string(dataBlockNameBuffer) == "cpe:" {
						state = statSetDataDelimiter
						currentDataBlock = &DataBlock{Name: "cpe"}
						dataBlockNameBuffer = []byte{}
						cpeBlocks = append(cpeBlocks, currentDataBlock)
						continue
					}
				} else {
					dataBlockNameBuffer = []byte{}
				}
			} else {
				continue
			}

		case statInit:
			if delimiter == ch {
				if rule.Type == "match" || rule.Type == "softmatch" {
					state = statStart
					continue
				}
				return nil, errors.New("first pattern failed")
			}

			rule.Type += string(ch)
			continue
		case statStart:
			if delimiter == ch {
				state = statStartData
				continue
			}

			rule.ServiceName += string(ch)
			continue
		default:

		}
	}

	rule.CpeBlocks = cpeBlocks
	return rule, nil
}
