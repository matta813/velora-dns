package auth

import "testing"

func TestArgon2IDPasswordHash(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "correct horse battery staple" || !VerifyPassword(hash, "correct horse battery staple") || VerifyPassword(hash, "wrong password value") {
		t.Fatal("password hash verification failed")
	}
	if VerifyPassword("malformed", "correct horse battery staple") {
		t.Fatal("malformed hash accepted")
	}
}

func TestInputValidation(t *testing.T) {
	if ValidateUsername("valid.user") != nil || ValidateUsername("bad user") == nil {
		t.Fatal("username validation")
	}
	if ValidatePassword("long enough password") != nil || ValidatePassword("short") == nil {
		t.Fatal("password validation")
	}
	for _, role := range []Role{Admin, Operator, Viewer} {
		if !ValidateRole(role) {
			t.Fatal(role)
		}
	}
	if ValidateRole("owner") {
		t.Fatal("unknown role")
	}
}
