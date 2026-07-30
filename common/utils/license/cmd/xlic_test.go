package main

import (
	"encoding/json"
	"testing"
)

func TestBuildEntitlementParams(t *testing.T) {
	params, err := buildEntitlementParams("hids, ssa")
	if err != nil {
		t.Fatalf("build params: %v", err)
	}
	var payload struct {
		Products []string `json:"products"`
		Version  int      `json:"version"`
	}
	if err := json.Unmarshal([]byte(params["entitlements"]), &payload); err != nil {
		t.Fatalf("unmarshal entitlements: %v", err)
	}
	if payload.Version != 1 || len(payload.Products) != 2 {
		t.Fatalf("unexpected entitlements: %+v", payload)
	}
}

func TestBuildEntitlementParamsRejectsUnknownProduct(t *testing.T) {
	if _, err := buildEntitlementParams("hids,unknown"); err == nil {
		t.Fatal("unknown product must be rejected")
	}
}
