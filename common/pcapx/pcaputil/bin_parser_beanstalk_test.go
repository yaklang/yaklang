package pcaputil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBeanstalkPutAdmissionAndBodyFraming(t *testing.T) {
	good := []byte("put 1024 0 60 12\r\nhello-winlab\r\n")
	require.Equal(t, ProbeAccept, probeBeanstalk(good[:18], 64).Verdict)
	for _, bad := range [][]byte{
		[]byte("put x 0 60 12\r\n"),
		[]byte("put 1024 0 60 1048577\r\n"),
		[]byte("put 1024 -1 60 12\r\n"),
		[]byte("PUT 1024 0 60 12\r\n"),
		[]byte("put 1024 0 60 12\n"),
	} {
		require.NotEqualf(t, ProbeAccept, probeBeanstalk(bad, 64).Verdict, "%q", bad)
	}
	flow := &binFlow{a: &binParser{config: BinParserConfig{MaxMessageBytes: 1 << 20}}, beanstalk: &binBeanstalk{client: 0}}
	_, _, err := flow.frameBeanstalk(0, good[:len(good)-1])
	require.NoError(t, err) // incomplete body waits; it is not misframed.
	_, _, err = flow.frameBeanstalk(0, []byte("put 1024 0 60 12\r\nhello-winlab!!"))
	require.ErrorContains(t, err, "delimiter")
	fields, err := (&binBeanstalk{client: 0}).consume(0, good, 16)
	require.NoError(t, err)
	require.Equal(t, uint64(1024), fields["Priority"])
	require.Equal(t, uint64(60), fields["TTR"])
	require.Equal(t, []byte("hello-winlab"), fields["Body"])
}

func TestWinlab5013BeanstalkSession(t *testing.T) {
	events := replayWinlab5013Protocols(t, "20-beanstalkd.pcapng")
	var matched []*ProtocolEvent
	for _, event := range events {
		if event.Protocol == "beanstalkd" {
			require.Equal(t, "decoded", event.Status, "%s", event.Error)
			matched = append(matched, event)
		}
	}
	require.Len(t, matched, 6)
	require.Equal(t, "PUT", matched[0].Fields["Packet Name"])
	require.Equal(t, []byte("hello-winlab"), matched[0].Fields["Body"])
	require.Equal(t, "7", matched[1].Fields["Job ID"])
	require.Equal(t, "RESERVED", matched[3].Fields["Packet Name"])
	require.Equal(t, []byte("hello-winlab"), matched[3].Fields["Body"])
	require.Equal(t, "DELETED", matched[5].Fields["Packet Name"])
}
