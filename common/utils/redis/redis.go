// Reference: https://github.com/astaxie/goredis
package redis

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"strconv"
	"strings"
	"time"
)

var zeroTime time.Time

type Client struct {
	conn    net.Conn
	timeout time.Duration
}

type RedisError string

func (err RedisError) Error() string { return "Redis Error: " + string(err) }

var doesNotExist = RedisError("Key does not exist")

func NewClient(conn net.Conn, timeout time.Duration) (c *Client) {
	c = &Client{
		conn:    conn,
		timeout: timeout,
	}
	return
}

// reads a bulk reply (i.e $5\r\nhello)
func readBulk(reader *bufio.Reader, head string) ([]byte, error) {
	var err error
	var data []byte

	if head == "" {
		head, err = reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
	}
	switch head[0] {
	case ':':
		data = []byte(strings.TrimSpace(head[1:]))

	case '$':
		size, err := strconv.Atoi(strings.TrimSpace(head[1:]))
		if err != nil {
			return nil, err
		}
		if size == -1 {
			return nil, doesNotExist
		}
		lr := io.LimitReader(reader, int64(size))
		data, err = ioutil.ReadAll(lr)
		if err == nil {
			// read end of line
			_, err = reader.ReadString('\n')
		}
	default:
		return nil, RedisError("Expecting Prefix '$' or ':'")
	}

	return data, err
}

func writeRequest(writer io.Writer, cmd string, args ...string) error {
	b := commandBytes(cmd, args...)
	_, err := writer.Write(b)
	return err
}

func commandBytes(cmd string, args ...string) []byte {
	cmdbuf := bytes.NewBufferString(fmt.Sprintf("*%d\r\n$%d\r\n%s\r\n", len(args)+1, len(cmd), cmd))
	for _, s := range args {
		cmdbuf.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(s), s))
	}
	return cmdbuf.Bytes()
}

func readResponse(reader *bufio.Reader) (interface{}, error) {
	var line string
	var err error

	// read until the first non-whitespace line
	for {
		line, err = reader.ReadString('\n')
		if len(line) == 0 || err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if len(line) > 0 {
			break
		}
	}

	if line[0] == '+' {
		return strings.TrimSpace(line[1:]), nil
	}

	if line[0] == '-' {
		errmesg := strings.TrimSpace(line[1:])
		return nil, RedisError(errmesg)
	}

	if line[0] == ':' {
		n, err := strconv.ParseInt(strings.TrimSpace(line[1:]), 10, 64)
		if err != nil {
			return nil, RedisError("Int reply is not a number")
		}
		return n, nil
	}

	if line[0] == '*' {
		size, err := strconv.Atoi(strings.TrimSpace(line[1:]))
		if err != nil {
			return nil, RedisError("MultiBulk reply expected a number")
		}
		if size <= 0 {
			return make([][]byte, 0), nil
		}
		res := make([][]byte, size)
		for i := 0; i < size; i++ {
			res[i], err = readBulk(reader, "")
			if err == doesNotExist {
				continue
			}
			if err != nil {
				return nil, err
			}
			// dont read end of line as might not have been bulk
		}
		return res, nil
	}
	return readBulk(reader, line)
}

func (client *Client) rawSend(c net.Conn, cmd []byte, timeouts ...time.Duration) (interface{}, error) {
	var timeout time.Duration = client.timeout
	if len(timeouts) > 0 {
		timeout = timeouts[0]
	}

	c.SetWriteDeadline(time.Now().Add(timeout))
	defer c.SetWriteDeadline(zeroTime)

	_, err := c.Write(cmd)
	if err != nil {
		return nil, err
	}

	c.SetReadDeadline(time.Now().Add(timeout))
	defer c.SetReadDeadline(zeroTime)

	reader := bufio.NewReader(c)
	data, err := readResponse(reader)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (client *Client) sendCommand(timeout time.Duration, cmd string, args ...string) (data interface{}, err error) {
	c := client.conn

	var b []byte

	b = commandBytes(cmd, args...)
	data, err = client.rawSend(c, b, timeout)
	// in case of "connection reset by peer" or "broken pipe" or "protected mode"
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "reset") || strings.Contains(errMsg, "broken") || strings.Contains(errMsg, "protected mode") {
			c.Close()
			return data, err
		}
	}

	return data, err
}

func (client *Client) sendCommands(cmdArgs <-chan []string, data chan<- interface{}) (err error) {
	c := client.conn
	var reader *bufio.Reader

	if err != nil {
		// Close client and synchronization issues are a nightmare to solve.
		c.Close()
		return err
	}

	reader = bufio.NewReader(c)

	// Ping first to verify connection is open
	err = writeRequest(c, "PING")
	if err != nil {
		return err
	}
	// Read Ping response
	pong, err := readResponse(reader)
	if pong != "PONG" {
		return RedisError("Unexpected response to PING.")
	}
	if err != nil {
		// Close client and synchronization issues are a nightmare to solve.
		c.Close()
		return err
	}

	errs := make(chan error)

	go func() {
		for cmdArg := range cmdArgs {
			err = writeRequest(c, cmdArg[0], cmdArg[1:]...)
			if err != nil {
				errs <- err
				break
			}
		}
		close(errs)
	}()

	go func() {
		for {
			response, err := readResponse(reader)
			if err != nil {
				errs <- err
				break
			}
			data <- response
		}
		close(errs)
	}()

	// Block until errs channel closes
	for e := range errs {
		err = e
	}

	return err
}

// General Commands
func (client *Client) Auth(password string) error {
	_, err := client.sendCommand(client.timeout, "AUTH", password)
	if err != nil {
		return err
	}

	return nil
}

func (client *Client) Ttl(key string) (int64, error) {
	res, err := client.sendCommand(client.timeout, "TTL", key)
	if err != nil {
		return -1, err
	}

	return res.(int64), nil
}

// String-related commands
func (client *Client) Set(key, val string) error {
	_, err := client.sendCommand(client.timeout, "SET", key, string(val))
	if err != nil {
		return err
	}

	return nil
}

func (client *Client) SetEx(key, val string, timeout time.Duration) error {
	_, err := client.sendCommand(timeout, "SET", key, string(val))
	if err != nil {
		return err
	}

	return nil
}

func (client *Client) Get(key string) ([]byte, error) {
	res, err := client.sendCommand(client.timeout, "GET", key)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return []byte(""), nil
	}
	data := res.([]byte)
	return data, nil
}

func (client *Client) GetEx(key string, timeout time.Duration) ([]byte, error) {
	res, err := client.sendCommand(timeout, "GET", key)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return []byte(""), nil
	}
	data := res.([]byte)
	return data, nil
}

// Container for messages received from publishers on channels that we're subscribed to.
type Message struct {
	ChannelMatched string
	Channel        string
	Message        []byte
}

// Subscribe to redis serve channels, this method will block until one of the sub/unsub channels are closed.
// There are two pairs of channels subscribe/unsubscribe & psubscribe/punsubscribe.
// The former does an exact match on the channel, the later uses glob patterns on the redis channels.
// Closing either of these channels will unblock this method call.
// Messages that are received are sent down the messages channel.
func (client *Client) Subscribe(subscribe <-chan string, unsubscribe <-chan string, psubscribe <-chan string, punsubscribe <-chan string, messages chan<- Message) error {
	cmds := make(chan []string, 0)
	data := make(chan interface{}, 0)

	go func() {
		for {
			var channel string
			var cmd string

			select {
			case channel = <-subscribe:
				cmd = "SUBSCRIBE"
			case channel = <-unsubscribe:
				cmd = "UNSUBSCRIBE"
			case channel = <-psubscribe:
				cmd = "PSUBSCRIBE"
			case channel = <-punsubscribe:
				cmd = "UNPSUBSCRIBE"

			}
			if channel == "" {
				break
			} else {
				cmds <- []string{cmd, channel}
			}
		}
		close(cmds)
		close(data)
	}()

	go func() {
		for response := range data {
			db := response.([][]byte)
			messageType := string(db[0])
			switch messageType {
			case "message":
				channel, message := string(db[1]), db[2]
				messages <- Message{channel, channel, message}
			case "subscribe":
				// Ignore
			case "unsubscribe":
				// Ignore
			case "pmessage":
				channelMatched, channel, message := string(db[1]), string(db[2]), db[3]
				messages <- Message{channelMatched, channel, message}
			case "psubscribe":
				// Ignore
			case "punsubscribe":
				// Ignore

			default:
				// log.Printf("Unknown message '%s'", messageType)
			}
		}
	}()

	err := client.sendCommands(cmds, data)

	return err
}

// Publish a message to a redis server.
func (client *Client) Publish(channel, val string) error {
	_, err := client.sendCommand(client.timeout, "PUBLISH", channel, val)
	if err != nil {
		return err
	}
	return nil
}

func (client *Client) Do(cmd string, args ...string) (interface{}, error) {
	return client.sendCommand(client.timeout, cmd, args...)
}

func (client *Client) Close() error {
	return client.conn.Close()
}
