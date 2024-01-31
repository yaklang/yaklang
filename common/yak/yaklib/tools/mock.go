package tools

import (
	"bufio"
	"bytes"
	"context"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net"
	"strings"
)

type mockRedisServer struct {
	needPasswd   bool
	passwd       string
	data         map[string]string
	connAuthInfo map[net.Conn]bool
}

func DebugMockRedis(ctx context.Context, needPasswd bool, passwd ...string) (string, int) {
	mockServer := &mockRedisServer{
		needPasswd:   needPasswd,
		data:         make(map[string]string),
		connAuthInfo: make(map[net.Conn]bool),
	}
	if needPasswd && len(passwd) > 0 {
		mockServer.passwd = passwd[0]
	}
	return utils.DebugMockTCPHandlerFuncContext(ctx, mockServer.redisHandler)
}

func (s *mockRedisServer) redisHandler(ctx context.Context, lis net.Listener, conn net.Conn) {
	var bufReader = bufio.NewReader(conn)
	var readData = make(chan []byte, 10)

	go func() { // write Data
		for datum := range readData {
			cmdArr := bytes.Split(datum, []byte(" "))
			if len(cmdArr) <= 0 {
				continue
			}
			cmd := strings.ToUpper(string(cmdArr[0]))
			if s.needPasswd && !s.connAuthInfo[conn] && cmd != "AUTH" {
				conn.Write([]byte("-NOAUTH Authentication required.\r\n"))
				continue
			}
			switch cmd {
			case "AUTH":
				if len(cmdArr) <= 1 {
					conn.Write([]byte("-ERR wrong number of arguments for 'auth' command\r\n"))
					continue
				}
				if string(cmdArr[1]) != s.passwd {
					conn.Write([]byte("-ERR invalid password\r\n"))
					continue
				}
				s.connAuthInfo[conn] = true
				conn.Write([]byte("+OK\r\n"))
			case "SET":
				if len(cmdArr) <= 2 {
					conn.Write([]byte("-ERR wrong number of arguments for 'set' command\r\n"))
					continue
				}
				s.data[string(cmdArr[1])] = string(cmdArr[2])
				conn.Write([]byte("+OK\r\n"))
			case "GET":
				if len(cmdArr) <= 1 {
					conn.Write([]byte("-ERR wrong number of arguments for 'get' command\r\n"))
					continue
				}
				if v, ok := s.data[string(cmdArr[1])]; ok {
					conn.Write([]byte("$" + codec.Itoa(len(v)) + "\r\n" + v + "\r\n"))
				} else {
					conn.Write([]byte("$-1\r\n"))
				}
			}
		}
	}()

	for { // reade Data
		select {
		case <-ctx.Done():
			break
		default:
		}
		data, err := readRedisData(bufReader)
		if err != nil {
			close(readData)
			return
		}
		readData <- data
	}
}

func readRedisData(bufReader *bufio.Reader) ([]byte, error) {
	line, _, err := bufReader.ReadLine()
	if err != nil {
		return nil, err
	}
	if len(line) <= 0 {
		return nil, utils.Error("empty line")
	}

	controlChar := line[0]
	switch controlChar {
	case '+':
		return bytes.TrimPrefix(line, []byte("+")), nil
	case '-':
		return bytes.TrimPrefix(line, []byte("-")), nil
	case ':':
		return bytes.TrimPrefix(line, []byte(":")), nil
	case '$':
		length := codec.Atoi(string(bytes.TrimPrefix(line, []byte("$"))))
		data := make([]byte, length)
		_, err := bufReader.Read(data)
		if err != nil {
			return nil, err
		}
		bufReader.Discard(2) // discard \r\n
		return data, nil
	case '*':
		length := codec.Atoi(string(bytes.TrimPrefix(line, []byte("*"))))
		BulkDataArr := [][]byte{}
		for i := 0; i < length; i++ {
			data, err := readRedisData(bufReader)
			if err != nil {
				break
			}
			BulkDataArr = append(BulkDataArr, data)
		}
		return bytes.Join(BulkDataArr, []byte(" ")), nil
	default:
		return nil, utils.Errorf("unknown control char: %v", controlChar)
	}
}
