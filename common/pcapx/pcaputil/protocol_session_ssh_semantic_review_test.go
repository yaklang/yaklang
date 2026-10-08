package pcaputil

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestProtocolSessionSSHPrelineRoleTaxonomy(t *testing.T) {
	for _, known := range []bool{false, true} {
		for _, preDir := range []int{0, 1} {
			for _, deferred := range []bool{false, true} {
				for _, size := range []int{0, 1, 7} {
					t.Run(fmt.Sprintf("known=%t/dir=%d/deferred=%t/size=%d", known, preDir, deferred, size), func(t *testing.T) {
						opts := []ProtocolSessionOption{WithSessionTransport("tcp")}
						if known {
							opts = append(opts, WithSessionClientDirection(0))
						}
						s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), opts...)
						require.NoError(t, err)
						s.(*captureSession).f.a.config.Deferred = deferred
						require.Nil(t, s.Feed(1-preDir, time.Unix(1, 0), sshIdent("SSH-2.0-ReviewPeer")).Err)
						wire := []byte("Maintenance notice\r\nSSH-2.0-ReviewEndpoint\r\n")
						var es []*ProtocolEvent
						var first *ProtocolError
						for w := wire; len(w) > 0; {
							n := len(w)
							if size > 0 {
								n = min(n, size)
							}
							r := s.Feed(preDir, time.Unix(1, 0), w[:n])
							if first == nil && r.Err != nil && r.Err.Kind != ErrNeedMore {
								first = r.Err
							}
							es = append(es, r.Events...)
							w = w[n:]
						}
						if !known {
							require.NotNil(t, first)
							require.Equal(t, ErrContextRequired, first.Kind)
						} else if preDir == 0 {
							require.NotNil(t, first)
							require.Equal(t, ErrMalformedMessage, first.Kind)
						} else {
							require.Nil(t, first)
							require.Len(t, es, 2)
							want := []map[string]any{{"Packet Name": "pre-identification line", "Text": "Maintenance notice", "Line Number": 1, "Context Level": "observed-server"}, {"Packet Name": "identification", "Identification": "SSH-2.0-ReviewEndpoint", "Version": "2.0", "Context Level": "observed"}}
							raw := [][]byte{[]byte("Maintenance notice\r\n"), sshIdent("SSH-2.0-ReviewEndpoint")}
							for i, e := range es {
								require.Equal(t, raw[i], e.Raw)
								assertSemanticReviewJSON(t, want[i], e.Session)
								fields, err := e.GetFields()
								require.NoError(t, err)
								assertSemanticReviewJSON(t, want[i], fields)
							}
						}
						s.Close("eof")
						require.Zero(t, s.Stats().BufferedBytes)
					})
				}
			}
		}
	}
}

type semanticReviewSSHConn struct {
	net.Conn
	dir                       int
	mu                        *sync.Mutex
	steps                     *[]sessionStep
	serverCipher              chan struct{}
	serverNewKeys, clientHeld bool
}

func (c *semanticReviewSSHConn) Write(w []byte) (int, error) {
	newkeys := len(w) >= 6 && w[5] == 21 && w[0] == 0 && w[1] == 0 && w[2] == 0
	if c.dir == 0 && newkeys && !c.clientHeld {
		c.clientHeld = true
		select {
		case <-c.serverCipher:
		case <-time.After(5 * time.Second):
			return 0, fmt.Errorf("server encrypted EXT_INFO not observed")
		}
	}
	c.mu.Lock()
	*c.steps = append(*c.steps, sessionStep{c.dir, bytes.Clone(w)})
	c.mu.Unlock()
	n, err := c.Conn.Write(w)
	if c.dir == 1 {
		if c.serverNewKeys {
			select {
			case <-c.serverCipher:
			default:
				close(c.serverCipher)
			}
		}
		if newkeys {
			c.serverNewKeys = true
		}
	}
	return n, err
}

// Both fixed-version endpoint implementations must complete KEX. Only delivery
// of client NEWKEYS is delayed; the server follows RFC 8308 section 2.4.
// Ephemeral private keys never leave this function or enter fixture artifacts.
func semanticReviewSSHTranscript(t *testing.T) []sessionStep {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	client, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	defer client.Close()
	server, err := ln.Accept()
	require.NoError(t, err)
	defer server.Close()
	require.NoError(t, client.SetDeadline(time.Now().Add(8*time.Second)))
	require.NoError(t, server.SetDeadline(time.Now().Add(8*time.Second)))
	var mu sync.Mutex
	var steps []sessionStep
	ready := make(chan struct{})
	cc := &semanticReviewSSHConn{Conn: client, dir: 0, mu: &mu, steps: &steps, serverCipher: ready}
	sc := &semanticReviewSSHConn{Conn: server, dir: 1, mu: &mu, steps: &steps, serverCipher: ready}
	config := ssh.Config{KeyExchanges: []string{"diffie-hellman-group14-sha256"}, Ciphers: []string{"aes128-ctr"}, MACs: []string{"hmac-sha2-256"}}
	_, key, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(key)
	require.NoError(t, err)
	cfg := &ssh.ServerConfig{Config: config, NoClientAuth: true}
	cfg.AddHostKey(signer)
	done := make(chan error, 1)
	go func() {
		conn, _, _, e := ssh.NewServerConn(sc, cfg)
		if conn != nil {
			defer conn.Close()
		}
		done <- e
	}()
	conn, _, _, err := ssh.NewClientConn(cc, "review.invalid", &ssh.ClientConfig{Config: config, User: "review", HostKeyCallback: ssh.InsecureIgnoreHostKey(), HostKeyAlgorithms: []string{"ssh-ed25519"}})
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, <-done)
	mu.Lock()
	defer mu.Unlock()
	return append([]sessionStep(nil), steps...)
}

func TestProtocolSessionSSHDirectionKeysPreservePeerNEWKEYS(t *testing.T) {
	steps := semanticReviewSSHTranscript(t)
	for _, deferred := range []bool{false, true} {
		for mode := 0; mode < 12; mode++ {
			t.Run(fmt.Sprintf("deferred=%t/mode=%d", deferred, mode), func(t *testing.T) {
				s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(0))
				require.NoError(t, err)
				s.(*captureSession).f.a.config.Deferred = deferred
				var es []*ProtocolEvent
				for _, st := range steps {
					for w := st.wire; len(w) > 0; {
						n := len(w)
						if mode == 1 {
							n = 1
						} else if mode > 1 {
							n = min(n, mode*3+1)
						}
						r := s.Feed(st.dir, time.Unix(1, 0), w[:n])
						es = append(es, r.Events...)
						w = w[n:]
					}
				}
				var newkeys []*ProtocolEvent
				var opaque *ProtocolEvent
				for _, e := range es {
					if e.Session["Packet Name"] == "NEWKEYS" {
						newkeys = append(newkeys, e)
					}
					if e.Direction == 1 && e.sessionError != nil && e.sessionError.Kind == ErrEncrypted {
						opaque = e
					}
				}
				require.Len(t, newkeys, 2, "opposite direction must retain its old-key plaintext phase")
				require.NotNil(t, opaque)
				require.Equal(t, 1, newkeys[0].Direction)
				require.Equal(t, 0, newkeys[1].Direction)
				// Literal RFC4253 message 21 oracle; ciphertext and private keys are not goldens.
				for i, e := range newkeys {
					require.Equal(t, byte(21), e.Raw[5])
					require.Equal(t, int(binary.BigEndian.Uint32(e.Raw[:4]))+4, len(e.Raw))
					want := map[string]any{"Packet Name": "NEWKEYS", "Message Number": 21, "Context Level": "observed", "Identification": "SSH-2.0-Go"}
					if i == 1 {
						want["Encrypted"] = true
						want["Protocol Transition"] = "ssh->encrypted"
					}
					assertSemanticReviewJSON(t, want, e.Session)
					require.Contains(t, []string{"decoded", "deferred"}, e.Status)
					require.Empty(t, e.Error)
					_, err = e.GetFields()
					require.NoError(t, err)
				}
				require.Less(t, opaque.ID, newkeys[1].ID)
				s.Close("eof")
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}

func TestProtocolSessionSSHResourceAfterOpaqueStillFailsConnection(t *testing.T) {
	steps := semanticReviewSSHTranscript(t)
	s, err := NewProtocolSessionWithOptions(ParserBudget{MaxFrameBytes: 4096, MaxMessageBytes: 8192}, WithSessionTransport("tcp"), WithSessionClientDirection(0))
	require.NoError(t, err)
	opaque := false
	for _, st := range steps {
		r := s.Feed(st.dir, time.Unix(1, 0), st.wire)
		if r.Err != nil && r.Err.Kind == ErrEncrypted {
			opaque = true
			break
		}
		require.Nil(t, r.Err)
	}
	require.True(t, opaque)
	// A valid SSH declared packet size exceeding this caller's frame budget.
	header := make([]byte, 5)
	binary.BigEndian.PutUint32(header, 5000)
	header[4] = 4
	r := s.Feed(0, time.Unix(1, 0), header)
	require.NotNil(t, r.Err)
	require.Equal(t, ErrResourceExceeded, r.Err.Kind)
	cs := s.(*captureSession)
	require.Nil(t, cs.f.ssh)
	require.True(t, cs.f.directions[0].stopped)
	require.True(t, cs.f.directions[1].stopped)
	require.Empty(t, s.Feed(1, time.Unix(1, 0), sshNewKeys()).Events)
	s.Close("eof")
	require.Zero(t, s.Stats().BufferedBytes)
}
