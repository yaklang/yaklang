package dap

import (
	"bufio"
	"io"
	"net"

	"github.com/google/go-dap"
	"github.com/yaklang/yaklang/common/log"
)

type DAPServer struct {
	listener net.Listener
	session  *DebugSession
	config   *DAPServerConfig
}

func (s *DAPServer) Start() {
	go func() {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.config.stopped:
			default:
				log.Errorf("Error accepting client connection: %v", err)
				s.config.triggerServerStop()
			}
			return
		}
		log.Infof("Accept connection from %v", conn.RemoteAddr())
		go s.handleConnection(conn)
	}()
}

func (s *DAPServer) Stop() {
	if s.listener != nil {
		s.listener.Close()
	}

	if s.session == nil {
		return
	}
	s.session.Close()
}

func NewDAPServer(config *DAPServerConfig) *DAPServer {
	log.Infof("Start DAP server at %s", config.listener.Addr())
	if config.stopped == nil {
		config.stopped = make(chan struct{})
	}

	server := &DAPServer{
		listener: config.listener,
		config:   config,
	}
	return server
}

func (s *DAPServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	// todo: add debugger to session
	session := &DebugSession{
		config: s.config,
		conn:   conn,
		rw:     bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn)),
	}
	s.session = session

	for {
		request, err := dap.ReadProtocolMessage(session.rw.Reader)
		if err != nil {
			defer s.config.triggerServerStop()

			if err == io.EOF {
				log.Infof("No more data to read: %v", err)
				break
			}
			if decodeErr, ok := err.(*dap.DecodeProtocolMessageFieldError); ok {
				// Send an error response to the users if we were unable to process the message.
				session.sendInternalErrorResponse(decodeErr.Seq, err.Error())
				continue
			} else {
				log.Errorf("DAP error: %v", err)
				break
			}
		}
		session.handleRequest(request)
		if _, ok := request.(*dap.DisconnectRequest); ok {
			return
		}
	}
}
