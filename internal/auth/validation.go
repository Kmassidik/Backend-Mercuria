package auth

import (
	"fmt"
	"regexp"
	"strings"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// ValidateEmail validates an email address
func ValidateEmail(email string) error {
	email = strings.TrimSpace(email)
	
	if email == "" {
		return fmt.Errorf("email is required")
	}

	if !emailRegex.MatchString(email) {
		return fmt.Errorf("invalid email format")
	}

	return nil
}

// ValidatePassword validates a password
func ValidatePassword(password string) error {
	if password == "" {
		return fmt.Errorf("password is required")
	}

	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	if len(password) > 72 {
		// bcrypt has a max length of 72 bytes
		return fmt.Errorf("password must be less than 72 characters")
	}

	return nil
}

// ValidateRegisterRequest validates a registration request
func ValidateRegisterRequest(req *RegisterRequest) error {
	if err := ValidateEmail(req.Email); err != nil {
		return err
	}

	if err := ValidatePassword(req.Password); err != nil {
		return err
	}

	// Normalize email
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	// Trim names
	req.FirstName = strings.TrimSpace(req.FirstName)
	req.LastName = strings.TrimSpace(req.LastName)

	return nil
}

// ValidateLoginRequest validates a login request
func ValidateLoginRequest(req *LoginRequest) error {
	if err := ValidateEmail(req.Email); err != nil {
		return err
	}

	if req.Password == "" {
		return fmt.Errorf("password is required")
	}

	// Normalize email
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	return nil
}