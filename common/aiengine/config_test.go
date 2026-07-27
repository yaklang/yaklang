package aiengine

import "testing"

func TestWithStatelessSetsField(t *testing.T) {
	cfg := NewAIEngineConfig(WithStateless(true))
	if !cfg.Stateless {
		t.Fatal("WithStateless(true) did not set Stateless field")
	}
	cfg2 := NewAIEngineConfig(WithStateless(false))
	if cfg2.Stateless {
		t.Fatal("WithStateless(false) should leave Stateless false")
	}
}

func TestWithStatelessDefaultFalse(t *testing.T) {
	cfg := NewAIEngineConfig()
	if cfg.Stateless {
		t.Fatal("default AIEngineConfig should have Stateless=false")
	}
}
