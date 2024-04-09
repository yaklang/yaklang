package protocol_impl

import "testing"

func TestNtlm(t *testing.T) {
	negMsg := NegotiateMessage{}
	_, err := negMsg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	authMsg := AuthenticationMessage{}
	_, err = authMsg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	chMsg := ChallengeMessage{}
	_, err = chMsg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
}
