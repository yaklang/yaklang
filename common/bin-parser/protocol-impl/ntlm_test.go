package protocol_impl

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"testing"
)

func TestNtlmMessage(t *testing.T) {
	testData := []byte("test")
	negMsg := NewNegotiateMessage()
	testField := negMsg.NewField(testData)
	negMsg.DomainNameFields = testField
	negPayload, err := negMsg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	negMagRes, err := ParseNegotiateMessage(negPayload)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, testData, negMagRes.DomainNameFields.Value())

	authMsg := NewAuthenticationMessage()
	authMsg.DomainNameFields = authMsg.NewField(testData)
	authPayload, err := authMsg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	authMagRes, err := ParseAuthenticationMessage(authPayload)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, testData, authMagRes.DomainNameFields.Value())

	chMsg := NewChallengeMessage()
	chMsg.TargetNameFields = chMsg.NewField(testData)
	chPayload, err := chMsg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	chRes, err := ParseChallengeMessage(chPayload)
	assert.Equal(t, testData, chRes.TargetNameFields.Value())
}
func TestCalcHash(t *testing.T) {
	password := "123456"
	user := "guest"
	domain := "yaklang"

	ntV1 := NTOWFv1(password, user, domain)
	assert.Equal(t, "32ed87bdb5fdc5e9cba88547376818d4", codec.EncodeToHex(ntV1))

	lmV1 := LMOWFv1(password, user, domain)
	assert.Equal(t, "44efce164ab921caaad3b435b51404ee", codec.EncodeToHex(lmV1))

	ntV2 := NTOWFv2(password, user, domain)
	assert.Equal(t, "d98dbd85b34eaf3566b3bfd92e499e99", codec.EncodeToHex(ntV2))

	lmV2 := LMOWFv2(password, user, domain)
	assert.Equal(t, "d98dbd85b34eaf3566b3bfd92e499e99", codec.EncodeToHex(lmV2))

	timestamp, _ := codec.DecodeHex("022f70f8238bda01")
	serverName, _ := codec.DecodeHex("02001e0069005a003700770034006e00310069006f0075006d003600340035005a0001001e0069005a003700770034006e00310069006f0075006d003600340035005a0004001e0069005a003700770034006e00310069006f0075006d003600340035005a0003001e0069005a003700770034006e00310069006f0075006d003600340035005a0007000800022f70f8238bda0100000000")
	clientChallenge, _ := codec.DecodeHex("594d646174713673")
	serverChallenge, _ := codec.DecodeHex("ee1ac04dce23620f")
	ntV2, _ = codec.DecodeHex("6c15c767e9ce82c75bfe9d3974c5c971")
	lmV2, _ = codec.DecodeHex("6c15c767e9ce82c75bfe9d3974c5c971")
	netNtV2, netlLmV2, sessionBaseKey := NetNTLMv2(ntV2, lmV2, serverChallenge, clientChallenge, timestamp, serverName)
	assert.Equal(t, "433bab4d00dfbb69f6d0a009008884500101000000000000022f70f8238bda01594d6461747136730000000002001e0069005a003700770034006e00310069006f0075006d003600340035005a0001001e0069005a003700770034006e00310069006f0075006d003600340035005a0004001e0069005a003700770034006e00310069006f0075006d003600340035005a0003001e0069005a003700770034006e00310069006f0075006d003600340035005a0007000800022f70f8238bda010000000000000000", codec.EncodeToHex(netNtV2))
	assert.Equal(t, "ac16b0f6ebda537a213197fc5396770a594d646174713673", codec.EncodeToHex(netlLmV2))
	assert.Equal(t, "b8cd8533ce80af70e48d81d3f387879e", codec.EncodeToHex(sessionBaseKey))

	// TODO: test for NetNTLMv1
}
