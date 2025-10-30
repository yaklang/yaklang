package aid

//
//import (
//	"bytes"
//	"context"
//	"io"
//	"testing"
//)
//
//func TestRiskControl_Enabled(t *testing.T) {
//	// Test nil riskControl
//	var rc *riskControl
//	if rc.enabled() {
//		t.Fatal("nil riskControl should not be enabled")
//	}
//
//	// Test riskControl with nil callback
//	rc = &riskControl{}
//	if rc.enabled() {
//		t.Fatal("riskControl with nil callback should not be enabled")
//	}
//
//	// Test enabled riskControl
//	rc = &riskControl{
//		callback: func(*Config, context.Context, io.Reader) *RiskControlResult {
//			return &RiskControlResult{}
//		},
//	}
//	if !rc.enabled() {
//		t.Fatal("riskControl with valid callback should be enabled")
//	}
//}
//
//func TestRiskControl_SetCallback(t *testing.T) {
//	rc := &riskControl{}
//
//	// Test setting callback
//	callback := func(*Config, context.Context, io.Reader) *RiskControlResult {
//		return &RiskControlResult{}
//	}
//	rc.setCallback(callback)
//
//	if rc.callback == nil {
//		t.Fatal("callback should be set")
//	}
//}
//
//func TestRiskControl_DoRiskControl(t *testing.T) {
//	tests := []struct {
//		name           string
//		rc             *riskControl
//		config         *Config
//		reader         io.Reader
//		expectedResult *RiskControlResult
//	}{
//		{
//			name:   "nil riskControl",
//			rc:     nil,
//			config: &Config{},
//			reader: bytes.NewReader([]byte("test")),
//			expectedResult: &RiskControlResult{
//				Skipped: true,
//				Score:   0,
//				Reason:  "not enabled",
//			},
//		},
//		{
//			name:   "nil callback",
//			rc:     &riskControl{},
//			config: &Config{},
//			reader: bytes.NewReader([]byte("test")),
//			expectedResult: &RiskControlResult{
//				Skipped: true,
//				Score:   0,
//				Reason:  "not enabled (no aid forge set)",
//			},
//		},
//		{
//			name: "valid callback",
//			rc: &riskControl{
//				callback: func(*Config, context.Context, io.Reader) *RiskControlResult {
//					return &RiskControlResult{
//						Skipped: false,
//						Score:   0.8,
//						Reason:  "test reason",
//					}
//				},
//			},
//			config: &Config{},
//			reader: bytes.NewReader([]byte("test")),
//			expectedResult: &RiskControlResult{
//				Skipped: false,
//				Score:   0.8,
//				Reason:  "test reason",
//			},
//		},
//	}
//
//	for _, tt := range tests {
//		t.Run(tt.name, func(t *testing.T) {
//			result := tt.rc.doRiskControl(tt.config, context.Background(), tt.reader)
//			if result.Skipped != tt.expectedResult.Skipped {
//				t.Errorf("expected Skipped %v, got %v", tt.expectedResult.Skipped, result.Skipped)
//			}
//			if result.Score != tt.expectedResult.Score {
//				t.Errorf("expected Score %v, got %v", tt.expectedResult.Score, result.Score)
//			}
//			if result.Reason != tt.expectedResult.Reason {
//				t.Errorf("expected Reason %v, got %v", tt.expectedResult.Reason, result.Reason)
//			}
//		})
//	}
//}
