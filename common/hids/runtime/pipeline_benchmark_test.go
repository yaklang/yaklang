//go:build hids && linux

package runtime

import (
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
)

func BenchmarkPipelinePrepareProcessExecSnapshotDisabled(b *testing.B) {
	disabled := false
	pipeline := newPipelineFromSpec(nil, model.DesiredSpec{
		Mode: model.ModeObserve,
		Collectors: model.Collectors{
			Process: model.CollectorSpec{
				Enabled: true,
				Backend: model.CollectorBackendEBPF,
			},
		},
		Reporting: model.ReportingPolicy{
			EmitSnapshotObservations: &disabled,
		},
	})

	base := time.Unix(1_700_000_000, 0).UTC()
	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		event := model.Event{
			Type:      model.EventTypeProcessExec,
			Source:    "ebpf.process",
			Timestamp: base.Add(time.Duration(index) * time.Millisecond),
			Tags:      []string{"process", "ebpf"},
			Process: &model.Process{
				PID:                 1000 + index,
				ParentPID:           1,
				Name:                "bash",
				Username:            "root",
				Image:               "/bin/bash",
				Command:             "/bin/bash -lc whoami",
				ParentName:          "systemd",
				BootID:              "boot-bench",
				StartTimeUnixMillis: base.Add(time.Duration(index) * time.Millisecond).UnixMilli(),
			},
		}
		event = pipeline.prepareEvent(event)
		_ = shouldPublishObservation(event, pipeline.emitSnapshots)
	}
}

func BenchmarkPipelinePrepareNetworkLifecycleSnapshotDisabled(b *testing.B) {
	disabled := false
	pipeline := newPipelineFromSpec(nil, model.DesiredSpec{
		Mode: model.ModeObserve,
		Collectors: model.Collectors{
			Process: model.CollectorSpec{
				Enabled: true,
				Backend: model.CollectorBackendEBPF,
			},
			Network: model.CollectorSpec{
				Enabled: true,
				Backend: model.CollectorBackendEBPF,
			},
		},
		Reporting: model.ReportingPolicy{
			EmitSnapshotObservations: &disabled,
		},
	})

	base := time.Unix(1_700_000_000, 0).UTC()
	b.ReportAllocs()
	b.ResetTimer()

	for index := 0; index < b.N; index++ {
		pid := 2000 + index
		fd := index + 3
		connectAt := base.Add(time.Duration(index) * time.Second)

		connect := model.Event{
			Type:      model.EventTypeNetworkConnect,
			Source:    "ebpf.network",
			Timestamp: connectAt,
			Tags:      []string{"network", "ebpf", "outbound"},
			Process: &model.Process{
				PID:                 pid,
				ParentPID:           1,
				Name:                "curl",
				Username:            "root",
				Image:               "/usr/bin/curl",
				Command:             "curl https://example.com",
				ParentName:          "systemd",
				BootID:              "boot-bench",
				StartTimeUnixMillis: connectAt.UnixMilli(),
			},
			Network: &model.Network{
				Protocol:      "tcp",
				SourceAddress: "10.0.0.5",
				SourcePort:    40000 + index%2000,
				DestAddress:   "1.1.1.1",
				DestPort:      443,
				Direction:     "outbound",
			},
			Data: map[string]any{
				"fd": fd,
			},
		}
		connect = pipeline.prepareEvent(connect)
		_ = shouldPublishObservation(connect, pipeline.emitSnapshots)

		closeEvent := model.Event{
			Type:      model.EventTypeNetworkClose,
			Source:    "ebpf.network",
			Timestamp: connectAt.Add(5 * time.Minute),
			Tags:      []string{"network", "ebpf", "close"},
			Process: &model.Process{
				PID: pid,
			},
			Network: &model.Network{
				ConnectionState: "closed",
			},
			Data: map[string]any{
				"fd": fd,
			},
		}
		closeEvent = pipeline.prepareEvent(closeEvent)
		_ = shouldPublishObservation(closeEvent, pipeline.emitSnapshots)
	}
}
