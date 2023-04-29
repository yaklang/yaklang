package dingrobot

import (
	"yaklang/common/log"
	"sync"
)

type SilenceDingRobot struct {
	Roboter
}

func NewSilenceDingRobot() *SilenceDingRobot {
	return &SilenceDingRobot{}
}

var (
	warningOnce = new(sync.Once)
)

func (s *SilenceDingRobot) GetCurrentWebHook() string {
	return ""
}

func warningSilence() {
	warningOnce.Do(func() {
		log.Warn("no ding robot set, if u want to notify ding group, use config.ding_robot[webhook,secret]")
	})
}

func (s *SilenceDingRobot) SendText(content string, atMobiles []string, isAtAll bool) error {
	warningSilence()
	return nil
}
func (s *SilenceDingRobot) SendLink(title, text, messageURL, picURL string) error {
	warningSilence()
	return nil
}
func (s *SilenceDingRobot) SendMarkdown(title, text string, atMobiles []string, isAtAll bool) error {
	warningSilence()
	return nil
}
func (s *SilenceDingRobot) SendActionCard(title, text, singleTitle, singleURL, btnOrientation, hideAvatar string) error {
	warningSilence()
	return nil
}
func (s *SilenceDingRobot) SetSecret(secret string) { return }
