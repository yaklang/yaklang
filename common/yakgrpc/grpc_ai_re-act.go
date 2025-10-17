package yakgrpc

import (
	"context"
	"github.com/google/uuid"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/rag"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) StartAIReAct(stream ypb.Yak_StartAIReActServer) error {
	firstMsg, err := stream.Recv()
	if err != nil {
		log.Errorf("recv re-act first config msg failed: %v", err)
		return utils.Errorf("recv first mgs failed: %v", err)
	}

	if !firstMsg.IsStart {
		log.Errorf("recv re-act first config msg is invalid: %v", firstMsg)
		return utils.Error("first msg is not a start/config message, set IsStart to true")
	}

	startParams := firstMsg.Params

	baseCtx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	inputEvent := make(chan *ypb.AIInputEvent, 1000)

	optsFromStartParams := aireact.ConvertYPBAIStartParamsToReActConfig(startParams)

	var currentCoordinatorId = startParams.CoordinatorId
	_ = currentCoordinatorId
	var coordinatorIdOnce = new(sync.Once)
	feedback := func(e *schema.AiOutputEvent) {
		if e.Timestamp <= 0 {
			e.Timestamp = time.Now().Unix() // fallback
		}
		if e.CoordinatorId != "" {
			coordinatorIdOnce.Do(func() {
				currentCoordinatorId = e.CoordinatorId
			})
		}
		err := stream.Send(e.ToGRPC())
		if err != nil {
			log.Errorf("send re-act event to stream failed: %v", err)
		}
	}

	persistentSession := uuid.NewString() // default to random session id

	var reActOptions = []aireact.Option{
		aireact.WithEventHandler(func(e *schema.AiOutputEvent) {
			feedback(e)
		}),
		aireact.WithEventInputChan(inputEvent),
		aireact.WithContext(baseCtx),
		aireact.WithBuiltinTools(),
		aireact.WithAICallback(aicommon.AIChatToAICallbackType(ai.Chat)),
		aireact.WithEnhanceKnowledgeManager(rag.NewRagEnhanceKnowledgeManager()),
		aireact.WithPersistentSessionId(persistentSession),
	}
	reActOptions = append(reActOptions, optsFromStartParams...)

	reAct, err := aireact.NewReAct(reActOptions...)
	if err != nil {
		log.Errorf("create re-act failed: %v", err)
		return utils.Errorf("create re-act instance failed: %v", err)
	}
	_ = reAct // ensure reAct is not nil
	for {
		select {
		case <-baseCtx.Done():
			log.Info("AIReAct stream context done, stopping re-act")
			return nil
		default:
			// continue processing
		}

		msg, err := stream.Recv()
		if err != nil {
			log.Errorf("recv re-act msg failed: %v", err)
			continue
		}
		select {
		case <-baseCtx.Done():
			log.Info("AIReAct stream context done, stopping re-act")
			return nil
		case inputEvent <- msg:
		}
	}
}
