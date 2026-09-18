package dns

import "testing"

func TestTSIGStoreValidatesAndRemovesKeys(t *testing.T) {
	store := NewTSIGStore()
	if err := store.AddKey("transfer-key", "hmac-sha256", "c2VjcmV0"); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.GetKey("transfer-key."); !ok {
		t.Fatal("configured key not found by canonical name")
	}
	if !store.RemoveKey("transfer-key") || store.RemoveKey("transfer-key") {
		t.Fatal("remove result did not reflect key existence")
	}
	if err := store.AddKey("bad", "unsupported", "c2VjcmV0"); err == nil {
		t.Fatal("unsupported algorithm accepted")
	}
	if err := store.AddKey("bad", "hmac-sha256", ""); err == nil {
		t.Fatal("empty secret accepted")
	}
}
