package sshutil

import (
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/ssh"
	"testing"
	"time"
)

func TestLoginExec(t *testing.T) {
	test := assert.New(t)

	config := &ssh.ClientConfig{
		User:            "ubuntu",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
	}
	config.Auth = []ssh.AuthMethod{ssh.Password("ubuntu")}

	client, err := ssh.Dial("tcp", "172.16.86.130:22", config)
	if err != nil {
		test.Nil(err)
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		test.Nil(err)
		return
	}

	err = session.Run("echo 123123123")
	if err != nil {
		test.Nil(err)
		return
	}
}
