package kafka

import (
	"context"
	"fmt"
	"github.com/google/uuid"
)

var (
	groupPrefix string = "palm-group"
)

type KafkaTopic struct {
	Public bool //是否广播，如果广播就需要使用不同的消费者组
	Name   Topic

	//设置某个topic是否可写
	IsWrite bool
}

func newkafkaTopic(public bool, name Topic, isWrite bool) *KafkaTopic {
	return &KafkaTopic{
		Public:  public,
		Name:    name,
		IsWrite: isWrite,
	}
}
func NewPublicTopic(name Topic) *KafkaTopic {
	return newkafkaTopic(true, name, false)
}
func NewChannelTopic(name Topic) *KafkaTopic {
	return newkafkaTopic(false, name, false)
}
func NewWriterTopic(name Topic) *KafkaTopic {
	return newkafkaTopic(false, name, true)
}
func getTopicsFromAgentType(agentType AgentType) []*KafkaTopic {
	topics := []*KafkaTopic{NewPublicTopic(ManagerTopic)}
	switch agentType {
	case ScanAgent:
		topics = append(topics, NewChannelTopic(TaskTopic))
	}
	return topics
}

// topicManager去指定reader，所以我们通过Manager去生成reader
type TopicManager struct {
	topics    map[string]*KafkaTopic
	fetchName func() string
	typ       AgentType
}

// NewTopicManager 根据TopicManager生成
func NewTopicManager() *TopicManager {
	return &TopicManager{
		topics: make(map[string]*KafkaTopic),
		fetchName: func() string {
			return uuid.NewString()
		},
	}
}
func newTopicManagerFromAgentType(agentType AgentType) *TopicManager {
	manager := NewTopicManager()
	for _, topic := range getTopicsFromAgentType(agentType) {
		manager.registerTopic(topic)
	}
	return manager
}

func (t *TopicManager) registerTopic(topic *KafkaTopic) {
	t.topics[string(topic.Name)] = topic
}

// GenerateTopicReader 通过topicManager去生成AgentReader
func (t *TopicManager) GenerateTopicReader(ctx context.Context, address string, opts ...ReaderOptions) []*AgentReader[*Request] {
	var readers []*AgentReader[*Request]
	for _, topic := range t.topics {
		if !topic.IsWrite {
			if topic.Public {
				readers = append(readers, NewReader[*Request](ctx, address, fmt.Sprintf("%s-%s", groupPrefix, t.fetchName()), string(topic.Name), opts...))
			} else {
				readers = append(readers, NewReader[*Request](ctx, address, fmt.Sprintf("%s-%s", groupPrefix, string(topic.Name)), string(topic.Name), opts...))
			}
		}
	}
	return readers
}
