package auth

import (
	"testing"
)

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name    string
		email   string
		wantErr bool
	}{
		{
			name:    "valid email",
			email:   "user@example.com",
			wantErr: false,
		},
		{
			name:    "valid email with subdomain",
			email:   "user@mail.example.com",
			wantErr: false,
		},
		{
			name:    "empty email",
			email:   "",
			wantErr: true,
		},
		{
			name:    "missing @",
			email:   "userexample.com",
			wantErr: true,
		},
		{
			name:    "missing domain",
			email:   "user@",
			wantErr: true,
		},
		{
			name:    "missing local part",
			email:   "@example.com",
			wantErr: true,
		},
		{
			name:    "invalid format",
			email:   "not-an-email",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEmail(tt.email)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEmail() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name    string
		pass    string
		wantErr bool
	}{
		{
			name:    "valid password",
			pass:    "SecurePass123!",
			wantErr: false,
		},
		{
			name:    "minimum length password",
			pass:    "Pass123!",
			wantErr: false,
		},
		{
			name:    "too short",
			pass:    "Pass1!",
			wantErr: true,
		},
		{
			name:    "empty password",
			pass:    "",
			wantErr: true,
		},
		{
			name:    "very long password",
			pass:    string(make([]byte, 70)), // Changed from 80 to 70
			wantErr: false,
		},
		{
			name:    "too long password",
			pass:    string(make([]byte, 100)),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.pass)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePassword() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRegisterRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     RegisterRequest
		wantErr bool
	}{
		{
			name: "valid request",
			req: RegisterRequest{
				Email:     "user@example.com",
				Password:  "SecurePass123!",
				FirstName: "John",
				LastName:  "Doe",
			},
			wantErr: false,
		},
		{
			name: "valid request without names",
			req: RegisterRequest{
				Email:    "user@example.com",
				Password: "SecurePass123!",
			},
			wantErr: false,
		},
		{
			name: "invalid email",
			req: RegisterRequest{
				Email:    "invalid-email",
				Password: "SecurePass123!",
			},
			wantErr: true,
		},
		{
			name: "invalid password",
			req: RegisterRequest{
				Email:    "user@example.com",
				Password: "short",
			},
			wantErr: true,
		},
		{
			name: "empty email",
			req: RegisterRequest{
				Email:    "",
				Password: "SecurePass123!",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRegisterRequest(&tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRegisterRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}