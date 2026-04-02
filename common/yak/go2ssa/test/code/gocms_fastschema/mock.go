package mailer

import (
	"io"
	"net"
	"strconv"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

type MockSMTPServer struct {
	Host    string
	Port    int
	Close   func() error
	Backend *Backend
}

func CreateMockSMTPServer(addresses ...string) (_ *MockSMTPServer, err error) {
	addresses = append(addresses, "127.0.0.1:0")
	netServer, err := net.Listen("tcp", addresses[0])
	if err != nil {
		return nil, err
	}

	netServerAddress := netServer.Addr().String()
	host, portStr, _ := net.SplitHostPort(netServerAddress)
	port, _ := strconv.Atoi(portStr)
	mockSMTPServer := &MockSMTPServer{
		Host:    host,
		Port:    port,
		Backend: &Backend{},
	}

	smtpServer := smtp.NewServer(mockSMTPServer.Backend)
	smtpServer.Addr = netServerAddress
	smtpServer.Domain = mockSMTPServer.Host
	smtpServer.AllowInsecureAuth = true
	mockSMTPServer.Close = smtpServer.Close

	go func() {
		if err := smtpServer.Serve(netServer); err != nil {
			panic(err)
		}
	}()

	return mockSMTPServer, nil
}

var _ smtp.Session = (*Session)(nil)
var _ smtp.AuthSession = (*Session)(nil)
var _ smtp.Backend = (*Backend)(nil)

type Backend struct {
	Usernames []string
	Passwords []string
	Froms     []string
	Rcpts     []string
	Messages  [][]byte
}

func (b *Backend) NewSession(_ *smtp.Conn) (smtp.Session, error) {
	return &Session{b}, nil
}

type Session struct{ *Backend }

func (s *Session) AuthMechanisms() []string {
	return []string{sasl.Plain}
}

func (s *Session) Auth(mech string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(identity, username, password string) error {
		s.Usernames = append(s.Usernames, username)
		s.Passwords = append(s.Passwords, password)
		return nil
	}), nil
}

func (s *Session) Mail(from string, _ *smtp.MailOptions) error {
	s.Froms = append(s.Froms, from)
	return nil
}

func (s *Session) Rcpt(to string, _ *smtp.RcptOptions) error {
	s.Rcpts = append(s.Rcpts, to)
	return nil
}

func (s *Session) Data(r io.Reader) error {
	if b, err := io.ReadAll(r); err != nil {
		return err
	} else {
		s.Messages = append(s.Messages, b)
	}
	return nil
}

func (*Session) Reset() {}

func (*Session) Logout() error { return nil }
