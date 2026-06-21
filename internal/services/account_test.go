package services

import "testing"

func TestAccountDeletionConfirmedRequiresExactDeletePhrase(t *testing.T) {
	if !AccountDeletionConfirmed("DELETE") {
		t.Fatal("expected DELETE to confirm account deletion")
	}
	for _, value := range []string{"delete", " DELETE ", "yes", ""} {
		if AccountDeletionConfirmed(value) {
			t.Fatalf("expected %q not to confirm account deletion", value)
		}
	}
}
