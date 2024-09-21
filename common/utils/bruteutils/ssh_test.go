package bruteutils

import (
	"github.com/yaklang/yaklang/common/log"
	"golang.org/x/crypto/ssh"
	"io"
	"testing"
	"time"
)

func TestSSHClientConnecting(t *testing.T) {
	t.Skip()

	client, err := sshDial(`tcp`, "xxx:22", &ssh.ClientConfig{
		User:            `admin`,
		Auth:            []ssh.AuthMethod{ssh.Password("admin@123")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		BannerCallback: func(message string) error {
			return nil
		},
		HostKeyAlgorithms: []string{"ssh-rsa", "ssh-dss", "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521", "ssh-ed25519"},
		Timeout:           10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	log.Info("client is fetched")

	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}

	log.Infof("start to fetch stdin")
	in, err := session.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}

	err = session.RequestPty("xtermtty1", 80, 40, ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	})
	if err != nil {
		t.Fatal(err)
	}
	log.Infof("session id: %#v", string(client.SessionID()))

	log.Info("start to write ?\r\n")
	_, err = in.Write([]byte("?\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)

	// Read from the session's stdout
	out, err := session.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, 1024)
	n, err := out.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	log.Infof("Received: %s", string(buf[:n]))

	session.Close()
	client.Close()
}
