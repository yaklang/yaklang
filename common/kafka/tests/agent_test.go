package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/kafka"
	"testing"
)

func TestManager(t *testing.T) {
	request := kafka.NewTaskRequest("", "", []byte(kafka.NewTaskRequestMessage(kafka.Script, uuid.NewString(), nil).String()))
	marshal, err := json.Marshal(request)
	fmt.Println(string(marshal), err)
}
func TestAgent(t *testing.T) {
	ctx, _ := context.WithCancel(context.Background())
	manager := kafka.NewManager(uuid.NewString(), "127.0.0.1:9092")
	err := manager.Start(ctx)
	require.NoError(t, err)
}
