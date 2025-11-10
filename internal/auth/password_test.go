package auth

import (
	"testing"
)

func TestHashPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{
			name:     "valid password",
			password: "SecurePass123!",
			wantErr:  false,
		},
		{
			name:     "empty password should fail",
			password: "",
			wantErr:  true,
		},
		{
			name:     "very long password",
			password: string(make([]byte, 70)), 
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := HashPassword(tt.password)
			if (err != nil) != tt.wantErr {
				t.Errorf("HashPassword() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Hash should not be empty
				if hash == "" {
					t.Error("HashPassword() returned empty hash")
				}

				// Hash should not equal password
				if hash == tt.password {
					t.Error("HashPassword() hash equals plain password")
				}

				// Should be able to verify the hash
				if !VerifyPassword(hash, tt.password) {
					t.Error("VerifyPassword() failed for correct password")
				}

				// Wrong password should fail
				if VerifyPassword(hash, "wrongpassword") {
					t.Error("VerifyPassword() succeeded for wrong password")
				}
			}
		})
	}
}

func TestVerifyPassword(t *testing.T) {
	password := "TestPassword123!"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	tests := []struct {
		name     string
		hash     string
		password string
		want     bool
	}{
		{
			name:     "correct password",
			hash:     hash,
			password: password,
			want:     true,
		},
		{
			name:     "wrong password",
			hash:     hash,
			password: "WrongPassword",
			want:     false,
		},
		{
			name:     "empty password",
			hash:     hash,
			password: "",
			want:     false,
		},
		{
			name:     "invalid hash",
			hash:     "invalid-hash",
			password: password,
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VerifyPassword(tt.hash, tt.password)
			if got != tt.want {
				t.Errorf("VerifyPassword() = %v, want %v", got, tt.want)
			}
		})
	}
}