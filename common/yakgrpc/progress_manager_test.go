package yakgrpc

import (
	"context"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
	"time"
)

func TestGRPC_SaveCancelSimpleDetect(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()

	target1 := utils.RandStringBytes(10)
	target2 := utils.RandStringBytes(10)
	taskPrefix := utils.RandStringBytes(5)

	record := &ypb.LastRecord{
		LastRecordPtr:        0,
		Percent:              0,
		YakScriptOnlineGroup: "",
		ExtraInfo:            "",
	}
	param := &ypb.StartBruteParams{
		Type:                       "",
		Targets:                    "",
		TargetFile:                 "",
		ReplaceDefaultUsernameDict: false,
		ReplaceDefaultPasswordDict: false,
		Usernames:                  nil,
		UsernameFile:               "",
		Passwords:                  nil,
		PasswordFile:               "",
		Prefix:                     nil,
		Timeout:                    0,
		Concurrent:                 0,
		Retry:                      0,
		TargetTaskConcurrent:       0,
		OkToStop:                   false,
		DelayMin:                   0,
		DelayMax:                   0,
		PluginScriptName:           "",
	}

	var data = []*ypb.RecordPortScanRequest{
		{
			PortScanRequest: &ypb.PortScanRequest{
				Targets:  target1,
				TaskName: taskPrefix + "1",
			},
		},
		{
			PortScanRequest: &ypb.PortScanRequest{
				Targets:  target1,
				TaskName: taskPrefix + "2",
			},
		},
		{
			PortScanRequest: &ypb.PortScanRequest{
				Targets:  target2,
				TaskName: taskPrefix + "3",
			},
		},
		{
			PortScanRequest: &ypb.PortScanRequest{
				Targets:  target2,
				TaskName: taskPrefix + "4",
			},
		},
	}

	for _, datum := range data {
		datum.StartBruteParams = param
		datum.LastRecord = record
		_, err = client.SaveCancelSimpleDetect(ctx, datum)
		if err != nil {
			t.Fatal(err)
		}
	}

	// test fuzzy task name
	task, err := client.QuerySimpleDetectUnfinishedTask(ctx, &ypb.QueryUnfinishedTaskRequest{
		Filter: &ypb.UnfinishedTaskFilter{
			TaskName: taskPrefix,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	require.Equal(t, 4, len(task.GetTasks()))

	// test fuzzy target
	task, err = client.QuerySimpleDetectUnfinishedTask(ctx, &ypb.QueryUnfinishedTaskRequest{
		Filter: &ypb.UnfinishedTaskFilter{
			Target: target1[2:6],
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	require.Equal(t, 2, len(task.GetTasks()))
}
