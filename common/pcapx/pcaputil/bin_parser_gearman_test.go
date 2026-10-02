package pcaputil

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func gearmanWire(response bool, kind uint32, data []byte) []byte {
	w := make([]byte, 12+len(data))
	copy(w, []byte{0, 'R', 'E', 'Q'})
	if response {
		w[3] = 'S'
	}
	binary.BigEndian.PutUint32(w[4:8], kind)
	binary.BigEndian.PutUint32(w[8:12], uint32(len(data)))
	copy(w[12:], data)
	return w
}

func TestGearmanBinaryAdmissionAndFields(t *testing.T) {
	for _, sample := range []struct {
		response bool
		kind     uint32
		wire     string
		field    string
	}{
		{false, 1, "lab.reverse", "Function"},
		{true, 8, "H:lab:7", "Job Handle"},
	} {
		fields, err := (&binGearman{}).consume(gearmanWire(sample.response, sample.kind, []byte(sample.wire)), 1<<20)
		require.NoError(t, err)
		require.Equal(t, sample.wire, fields[sample.field])
	}
	w := gearmanWire(false, 7, []byte("lab.reverse\x00\x00winlab"))
	require.Equal(t, ProbeAccept, probeGearman(w[:12], 64).Verdict)
	require.Equal(t, ProbeNeedMore, probeGearman(w[:3], 64).Verdict)
	fields, err := (&binGearman{}).consume(w, 1<<20)
	require.NoError(t, err)
	require.Equal(t, "SUBMIT_JOB", fields["Packet Name"])
	require.Equal(t, "lab.reverse", fields["Function"])
	require.Equal(t, []byte("winlab"), fields["Workload"])
	for _, bad := range [][]byte{
		func() []byte { x := bytes.Clone(w); x[1] = 'X'; return x }(),
		func() []byte { x := bytes.Clone(w); x[3] = 'S'; return x }(),
		func() []byte { x := bytes.Clone(w); x[7] = 0xff; return x }(),
		gearmanWire(false, 10, nil),
	} {
		require.NotEqualf(t, ProbeAccept, probeGearman(bad, 64).Verdict, "%x", bad)
	}
	_, err = (&binGearman{}).consume(w[:len(w)-1], 1<<20)
	require.Error(t, err)
	wrongLength := bytes.Clone(w)
	wrongLength[11]++
	_, err = (&binGearman{}).consume(wrongLength, 1<<20)
	require.Error(t, err)
}

func TestWinlab5013GearmanBinaryMessages(t *testing.T) {
	events := replayWinlab5013Protocols(t, "19-gearman.pcapng")
	var gearman []*ProtocolEvent
	for _, event := range events {
		if event.Protocol == "gearman" {
			require.Equal(t, "decoded", event.Status, "%s", event.Error)
			gearman = append(gearman, event)
		}
	}
	require.Len(t, gearman, 11)
	var seenSubmit, seenAssign, seenComplete bool
	for _, event := range gearman {
		switch event.Fields["Packet Name"] {
		case "SUBMIT_JOB":
			seenSubmit = event.Fields["Function"] == "lab.reverse" && bytes.Equal(event.Fields["Workload"].([]byte), []byte("winlab"))
		case "JOB_ASSIGN":
			seenAssign = event.Fields["Job Handle"] == "H:lab:7" && event.Fields["Function"] == "lab.reverse"
		case "WORK_COMPLETE":
			seenComplete = event.Fields["Job Handle"] == "H:lab:7" && bytes.Equal(event.Fields["Result"].([]byte), []byte("balniw"))
		}
	}
	require.True(t, seenSubmit)
	require.True(t, seenAssign)
	require.True(t, seenComplete)
}
