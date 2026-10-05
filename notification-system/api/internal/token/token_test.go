package token

import "testing"

func mustNewSigner(t *testing.T, secret string) *Signer {
	t.Helper()
	s, err := NewSigner(secret)
	if err != nil {
		t.Fatalf("Setup: NewSigner(%q) failed: %v", secret, err)
	}
	return s
}

func TestIssueThenVerify(t *testing.T) {
	s := mustNewSigner(t, "secret")
	const appID = "app-1"

	tok, err := s.Issue(appID)
	if err != nil {
		t.Fatalf("Issue(%q) failed: %v", appID, err)
	}
	got, err := s.Verify(tok)
	if err != nil || got != appID {
		t.Errorf("Verify(Issue(%q)) = %q, %v, want %q, nil", appID, got, err, appID)
	}
}

func TestVerifyRejects(t *testing.T) {
	s := mustNewSigner(t, "secret")
	valid, err := s.Issue("app-1")
	if err != nil {
		t.Fatalf("Setup: Issue failed: %v", err)
	}
	foreign, err := mustNewSigner(t, "other secret").Issue("app-1")
	if err != nil {
		t.Fatalf("Setup: Issue failed: %v", err)
	}

	tests := []struct {
		name string
		tok  string
	}{
		{name: "empty", tok: ""},
		{name: "garbage", tok: "not-a-token"},
		{name: "tampered", tok: valid + "x"},
		{name: "signed with another secret", tok: foreign},
		// A token whose header claims {"alg":"none"} and so carries no signature.
		{name: "unsigned", tok: "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiJhcHAtMSJ9."},
	}
	for _, test := range tests {
		if got, err := s.Verify(test.tok); err == nil {
			t.Errorf("Verify(%s token) = %q, nil, want an error", test.name, got)
		}
	}
}

func TestNewSignerRejectsEmptySecret(t *testing.T) {
	if _, err := NewSigner(""); err == nil {
		t.Error(`NewSigner("") = nil error, want an error`)
	}
}
