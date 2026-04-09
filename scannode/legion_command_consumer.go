package scannode

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/yaklang/yaklang/common/log"
)

const commandPollInterval = time.Second

type commandConsumer struct {
	sessionID string
	cancel    context.CancelFunc
	conn      *nats.Conn
	sub       *nats.Subscription
}

func (b *legionJobBridge) Run(ctx context.Context) {
	ticker := time.NewTicker(commandPollInterval)
	defer ticker.Stop()
	defer b.stopConsumer()
	defer b.publisher.Close()
	defer b.capabilityPublisher.Close()

	for {
		if ctx.Err() != nil {
			return
		}
		b.syncConsumer(ctx)

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (b *legionJobBridge) syncConsumer(parent context.Context) {
	session, ok := b.agent.node.GetSessionState()
	if !ok {
		b.stopConsumer()
		return
	}

	b.mu.Lock()
	current := b.consumer
	b.mu.Unlock()
	if current != nil && current.sessionID == session.SessionID {
		return
	}

	b.stopConsumer()
	consumer, err := b.startConsumer(parent, session.NATSURL, session.SessionID, session.CommandSubject)
	if err != nil {
		log.Errorf("start legion command consumer failed: %v", err)
		return
	}

	b.mu.Lock()
	b.consumer = consumer
	b.mu.Unlock()
}

func (b *legionJobBridge) startConsumer(
	parent context.Context,
	natsURL string,
	sessionID string,
	commandSubject string,
) (*commandConsumer, error) {
	conn, err := nats.Connect(natsURL, nats.Name("yak-node-commands-"+b.agent.node.NodeId))
	if err != nil {
		return nil, fmt.Errorf("connect command nats: %w", err)
	}
	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("build command jetstream context: %w", err)
	}

	subscription, err := js.PullSubscribe(
		commandSubjectWildcard(commandSubject),
		consumerNameForNode(b.agent.node.NodeId),
		nats.BindStream(legionCommandStream),
		nats.ManualAck(),
		nats.AckExplicit(),
		nats.MaxAckPending(64),
	)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("pull subscribe commands: %w", err)
	}

	ctx, cancel := context.WithCancel(parent)
	consumer := &commandConsumer{
		sessionID: sessionID,
		cancel:    cancel,
		conn:      conn,
		sub:       subscription,
	}
	go b.consumeLoop(ctx, consumer)
	log.Infof("started legion command consumer: node_id=%s session_id=%s", b.agent.node.NodeId, sessionID)
	return consumer, nil
}

func (b *legionJobBridge) stopConsumer() {
	b.mu.Lock()
	consumer := b.consumer
	b.consumer = nil
	b.mu.Unlock()
	if consumer == nil {
		return
	}

	consumer.cancel()
	if consumer.sub != nil {
		_ = consumer.sub.Unsubscribe()
	}
	if consumer.conn != nil {
		consumer.conn.Close()
	}
}

func (b *legionJobBridge) consumeLoop(ctx context.Context, consumer *commandConsumer) {
	for {
		if ctx.Err() != nil {
			return
		}

		messages, err := consumer.sub.Fetch(4, nats.MaxWait(time.Second))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) || ctx.Err() != nil {
				continue
			}
			if isCommandConsumerResetError(err) {
				log.Errorf(
					"legion command consumer became invalid: node_id=%s session_id=%s err=%v diagnosis=%q",
					b.agent.node.NodeId,
					consumer.sessionID,
					err,
					"another process may be running with the same node_id, or the platform session/consumer was replaced",
				)
				b.stopConsumer()
				return
			}
			log.Errorf(
				"fetch legion commands failed: node_id=%s session_id=%s err=%v",
				b.agent.node.NodeId,
				consumer.sessionID,
				err,
			)
			continue
		}
		for _, message := range messages {
			if err := b.handleMessage(ctx, message); err != nil {
				log.Errorf("handle legion command failed: %v", err)
				_ = message.Nak()
				continue
			}
			_ = message.Ack()
		}
	}
}

func isCommandConsumerResetError(err error) bool {
	return errors.Is(err, nats.ErrConsumerDeleted) ||
		errors.Is(err, nats.ErrNoResponders) ||
		errors.Is(err, nats.ErrConnectionClosed) ||
		errors.Is(err, nats.ErrDisconnected) ||
		errors.Is(err, nats.ErrBadSubscription) ||
		errors.Is(err, nats.ErrSubscriptionClosed)
}

func (b *legionJobBridge) handleMessage(
	ctx context.Context,
	message *nats.Msg,
) error {
	switch {
	case strings.HasSuffix(message.Subject, "."+legionCommandDispatch):
		return b.handleDispatch(ctx, message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandCancel):
		return b.handleCancel(message.Data)
	case strings.HasSuffix(message.Subject, "."+legionCommandCapabilityApply):
		return b.handleCapabilityApply(ctx, message.Data)
	default:
		return fmt.Errorf("unsupported legion command subject: %s", message.Subject)
	}
}

func consumerNameForNode(nodeID string) string {
	var builder strings.Builder
	builder.WriteString("legion-node-")
	for _, r := range nodeID {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		default:
			builder.WriteByte('-')
		}
	}
	return builder.String()
}
