package stdio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"os/exec"
	"strings"
	"time"
)

const maxFrameSize = 64 << 20

var errWorkerProtocol = errors.New("invalid JSON-RPC frame from MCP worker")

type frame struct {
	data []byte
	err  error
}

func readFrames(ctx context.Context, r io.Reader, requireNewline bool) <-chan frame {
	frames := make(chan frame)
	go func() {
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 64<<10), maxFrameSize)
		if requireNewline {
			scanner.Split(func(data []byte, atEOF bool) (int, []byte, error) {
				if i := bytes.IndexByte(data, '\n'); i >= 0 {
					return i + 1, data[:i], nil
				}
				if atEOF && len(data) > 0 {
					return 0, nil, io.ErrUnexpectedEOF
				}
				return 0, nil, nil
			})
		}
		for scanner.Scan() {
			data := append(bytes.Clone(scanner.Bytes()), '\n')
			select {
			case frames <- frame{data: data}:
			case <-ctx.Done():
				return
			}
		}
		err := scanner.Err()
		if err == nil {
			err = io.EOF
		}
		select {
		case frames <- frame{err: err}:
		case <-ctx.Done():
		}
	}()
	return frames
}

type envelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`
}

type relay struct {
	output  io.Writer
	pending map[string]json.RawMessage
}

func idKey(id json.RawMessage) string {
	// Normalize equivalent string encodings without converting numeric IDs to
	// float64 (which would lose precision for large request IDs).
	var s string
	if json.Unmarshal(id, &s) == nil && len(id) > 0 && id[0] == '"' {
		encoded, _ := json.Marshal(s)
		return string(encoded)
	}
	number := string(bytes.TrimSpace(id))
	if number == "" || number == "null" {
		return number
	}
	// JSON encoders may rewrite 1.0 or 1e0 as 1. Compare the exact decimal
	// coefficient/exponent instead of float64, without expanding huge powers.
	mantissa, exponentText, hasExponent := strings.Cut(strings.ToLower(number), "e")
	exponent := new(big.Int)
	if hasExponent {
		if _, ok := exponent.SetString(exponentText, 10); !ok {
			return number
		}
	}
	negative := strings.HasPrefix(mantissa, "-")
	mantissa = strings.TrimPrefix(mantissa, "-")
	whole, fraction, _ := strings.Cut(mantissa, ".")
	digits := strings.TrimLeft(whole+fraction, "0")
	if digits == "" {
		return "0"
	}
	coefficient := strings.TrimRight(digits, "0")
	exponent.Add(exponent, big.NewInt(int64(len(digits)-len(coefficient)-len(fraction))))
	if negative {
		coefficient = "-" + coefficient
	}
	return coefficient + "e" + exponent.String()
}

func (r *relay) track(data []byte) {
	var message envelope
	if json.Unmarshal(data, &message) == nil && message.JSONRPC == "2.0" && message.Method != "" && len(message.ID) > 0 {
		r.pending[idKey(message.ID)] = message.ID
	}
}

func writeFrame(w io.Writer, data []byte) error {
	n, err := w.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return err
}

func (r *relay) forward(data []byte) error {
	var message envelope
	if err := json.Unmarshal(data, &message); err != nil || message.JSONRPC != "2.0" {
		return errWorkerProtocol
	}
	if message.Method == "" && (len(message.ID) == 0 || (len(message.Result) == 0) == (len(message.Error) == 0)) {
		return errWorkerProtocol
	}
	if err := writeFrame(r.output, data); err != nil {
		return fmt.Errorf("write MCP response: %w", err)
	}
	// Server requests and notifications must not consume client request IDs.
	if message.Method == "" {
		delete(r.pending, idKey(message.ID))
	}
	return nil
}

func (r *relay) fail(cause error, exitErr error) error {
	data := map[string]any{"reason": cause.Error()}
	var exited *exec.ExitError
	if errors.As(exitErr, &exited) {
		data["exitCode"] = exited.ExitCode()
	}
	for key, id := range r.pending {
		response, _ := json.Marshal(map[string]any{
			"jsonrpc": "2.0", "id": id,
			"error": map[string]any{"code": -32603, "message": "MCP worker failed", "data": data},
		})
		if err := writeFrame(r.output, append(response, '\n')); err != nil {
			return fmt.Errorf("report MCP worker failure: %w", err)
		}
		delete(r.pending, key)
	}
	return cause
}

// Run owns input until it returns, including closing it to unblock reads on
// worker failure or cancellation. cmd must re-execute the MCP entry point with
// its original arguments; its standard streams and worker environment are set
// here. Protocol writes are serialized and bounded by outputWriteTimeout, shortened
// on cancellation. Non-file output is owned by Run and its Close must unblock
// Write. File output is duplicated privately; logs is inherited by the worker.
func Run(ctx context.Context, cmd *exec.Cmd, input io.ReadCloser, output io.WriteCloser, logs *os.File) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer input.Close()
	writer, closeOutput, err := prepareOutput(output)
	if err != nil {
		return fmt.Errorf("prepare MCP output: %w", err)
	}
	defer closeOutput()
	reader, closeInput, err := prepareInput(input)
	if err != nil {
		return fmt.Errorf("prepare MCP input: %w", err)
	}
	defer closeInput()
	requests := readFrames(ctx, reader, false)
	r := &relay{output: &protocolWriter{ctx: ctx, output: writer}, pending: make(map[string]json.RawMessage)}
	var queued []byte
	select {
	case <-ctx.Done():
		return ctx.Err()
	case first := <-requests:
		if first.err != nil {
			if first.err == io.EOF {
				return nil
			}
			return first.err
		}
		queued = first.data
		r.track(queued)
	}
	// Starting on the first client frame lets initialization failures be returned
	// against initialize's ID instead of leaving the client with an unexplained EOF.
	p, err := startWorker(ctx, cmd, logs)
	if err != nil {
		return r.fail(err, err)
	}
	defer p.close()
	responses := readFrames(ctx, p.conn, true)
	outbound := make(chan []byte)
	writeErrors := make(chan error, 1)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case data, ok := <-outbound:
				if !ok {
					writeErrors <- p.conn.CloseWrite()
					return
				}
				if err := writeFrame(p.conn, data); err != nil {
					writeErrors <- err
					return
				}
			}
		}
	}()

	processDone := p.done
	var closing <-chan time.Time
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	beginClose := func() {
		if timer == nil {
			timer = time.NewTimer(shutdownTimeout)
			closing = timer.C
		}
	}
	var transportErr error
	for {
		incoming := requests
		var send chan []byte
		if queued != nil {
			incoming = nil
			send = outbound
		}
		select {
		case <-ctx.Done():
			return r.fail(ctx.Err(), nil)
		case <-closing:
			return r.fail(fmt.Errorf("MCP worker did not shut down within %s", shutdownTimeout), nil)
		case <-processDone:
			processDone = nil
			// Drain complete protocol frames before synthesizing errors. A child
			// may exit immediately after writing its final successful response.
			_ = p.conn.SetReadDeadline(time.Now().Add(shutdownTimeout))
		case send <- queued:
			queued = nil
		case request := <-incoming:
			if request.err != nil {
				if request.err != io.EOF {
					return r.fail(fmt.Errorf("read MCP client: %w", request.err), nil)
				}
				requests = nil
				close(outbound)
				beginClose()
				continue
			}
			queued = request.data
			r.track(queued)
		case err := <-writeErrors:
			writeErrors = nil
			if err != nil {
				transportErr = fmt.Errorf("write MCP worker: %w", err)
				beginClose()
			}
		case response := <-responses:
			if response.err == nil {
				if err := r.forward(response.data); err != nil {
					if errors.Is(err, errWorkerProtocol) {
						return r.fail(err, nil)
					}
					// A failed/partial client write cannot be repaired by appending
					// another JSON object to the same stream.
					return err
				}
				continue
			}
			// IPC EOF can precede Wait and the last stderr bytes. Bound the wait
			// if a worker closes its transport but fails to terminate.
			waitTimer := time.NewTimer(shutdownTimeout)
			select {
			case <-p.done:
			case <-ctx.Done():
				waitTimer.Stop()
				return r.fail(ctx.Err(), nil)
			case <-waitTimer.C:
				return r.fail(fmt.Errorf("MCP worker disconnected without exiting"), nil)
			}
			waitTimer.Stop()
			if p.waitErr != nil {
				return r.fail(fmt.Errorf("MCP worker exited: %w", p.waitErr), p.waitErr)
			}
			if response.err != io.EOF {
				return r.fail(fmt.Errorf("read MCP worker: %w", response.err), nil)
			}
			if transportErr != nil {
				return r.fail(transportErr, nil)
			}
			if len(r.pending) > 0 || requests != nil {
				return r.fail(fmt.Errorf("MCP worker exited before the client session ended"), nil)
			}
			return nil
		}
	}
}
