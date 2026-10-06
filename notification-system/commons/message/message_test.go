package message

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// The files in testdata are the contract between the services. A service that is still
// running the previous version must be able to read what a new one writes, so change
// them, and the types, with that in mind.

// checkContract verifies that the golden file decodes to want with no field left over,
// and that want encodes back to the same JSON.
func checkContract[T any](t *testing.T, file string, want T) {
	t.Helper()
	golden, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}

	var got T
	dec := json.NewDecoder(bytes.NewReader(golden))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("Decoding %s into %T failed: %v", file, got, err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("Decoding %s returned unexpected diff (-want +got):\n%s", file, diff)
	}

	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Encoding %+v failed: %v", want, err)
	}
	var wantJSON, gotJSON any
	if err := json.Unmarshal(golden, &wantJSON); err != nil {
		t.Fatalf("Setup: %s is not JSON: %v", file, err)
	}
	if err := json.Unmarshal(encoded, &gotJSON); err != nil {
		t.Fatalf("Encoding %+v produced invalid JSON: %v", want, err)
	}
	if diff := cmp.Diff(wantJSON, gotJSON); diff != "" {
		t.Errorf("Encoding %T differs from %s (-want +got):\n%s", want, file, diff)
	}
}

func TestDeliveryContract(t *testing.T) {
	checkContract(t, "testdata/delivery.json", Delivery{
		MessageID: "019a2b3c-0000-7000-8000-000000000001:019a2b3c-0000-7000-8000-000000000002",
		JobID:     "019a2b3c-0000-7000-8000-000000000001",
		AppID:     "019a2b3c-0000-7000-8000-000000000003",
		Provider:  "twilio",
		Address:   "+5491100000000",
		Content:   Content{Title: "Your order", Body: "It is on its way."},
	})
}
