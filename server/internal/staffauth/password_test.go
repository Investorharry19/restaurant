package staffauth

import "testing"

func TestHashPasswordUsesArgon2idAndVerifies(t *testing.T) {
	password := []byte("correct horse battery staple")
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if hash == string(password) || hash[:10] != "$argon2id$" {
		t.Fatalf("HashPassword() = %q, want an Argon2id encoding", hash)
	}
	if !VerifyPassword(hash, password) {
		t.Fatal("VerifyPassword() rejected the correct password")
	}
	if VerifyPassword(hash, []byte("incorrect password")) {
		t.Fatal("VerifyPassword() accepted an incorrect password")
	}
}

func TestHashPasswordRejectsWeakOrOversizedPasswords(t *testing.T) {
	for _, password := range [][]byte{[]byte("short"), make([]byte, 1025)} {
		if _, err := HashPassword(password); err == nil {
			t.Fatal("HashPassword() accepted invalid password length")
		}
	}
}

func TestVerifyPasswordRejectsUnexpectedParameters(t *testing.T) {
	if VerifyPassword("$argon2id$v=19$m=1,t=1,p=1$c2FsdA$aGFzaA", []byte("password")) {
		t.Fatal("VerifyPassword() accepted untrusted hashing parameters")
	}
}
