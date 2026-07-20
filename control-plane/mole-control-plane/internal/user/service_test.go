package user

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestNormalizeRegistration(t *testing.T) {
	username, email, password, err := normalizeRegistration(RegisterInput{
		Username: "  Alice_01 ",
		Email:    " ALICE@example.com ",
		Password: "a-secure-password",
	})
	if err != nil {
		t.Fatalf("normalize registration: %v", err)
	}
	if username != "alice_01" || email != "alice@example.com" || password != "a-secure-password" {
		t.Fatalf("unexpected normalized registration values: %q, %q, %q", username, email, password)
	}
}

func TestNormalizeRegistrationRejectsInvalidInput(t *testing.T) {
	tests := []RegisterInput{
		{Username: "ab", Email: "user@example.com", Password: "a-secure-password"},
		{Username: "valid-user", Email: "not-an-email", Password: "a-secure-password"},
		{Username: "valid-user", Email: "user@example.com", Password: "too-short"},
	}
	for _, input := range tests {
		if _, _, _, err := normalizeRegistration(input); err != ErrInvalidInput {
			t.Fatalf("expected invalid input for %+v, got %v", input, err)
		}
	}
}

func TestRandomToken(t *testing.T) {
	first, err := randomToken(32)
	if err != nil {
		t.Fatalf("generate first token: %v", err)
	}
	second, err := randomToken(32)
	if err != nil {
		t.Fatalf("generate second token: %v", err)
	}
	if first == second || len(first) != 43 {
		t.Fatalf("unexpected random tokens")
	}
}

func TestDummyPasswordHashUsesConfiguredCost(t *testing.T) {
	cost, err := bcrypt.Cost([]byte(dummyPasswordHash))
	if err != nil {
		t.Fatalf("inspect dummy password hash: %v", err)
	}
	if cost != bcryptCost {
		t.Fatalf("expected bcrypt cost %d, got %d", bcryptCost, cost)
	}
}
