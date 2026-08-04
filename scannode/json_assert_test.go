package scannode

import (
	"encoding/json"
	"reflect"
	"testing"
)

func assertJSONEqual(t *testing.T, got []byte, want string) {
	t.Helper()

	var gotValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("unmarshal got json: %v", err)
	}

	var wantValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("unmarshal want json: %v", err)
	}

	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("unexpected json payload: got=%s want=%s", string(got), want)
	}
}
