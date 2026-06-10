package pass

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// Bcrypt hashes passwords using bcrypt. The zero value uses
// bcrypt.DefaultCost (10) for both hashing and the minimum acceptable cost.
type Bcrypt struct {
	Cost    int // 0 uses bcrypt.DefaultCost
	MinCost int // 0 uses Cost; hashes below this trigger NeedsUpdate
}

func (b Bcrypt) cost() int {
	if b.Cost == 0 {
		return bcrypt.DefaultCost
	}
	return b.Cost
}

func (b Bcrypt) minCost() int {
	if b.MinCost == 0 {
		return b.cost()
	}
	return b.MinCost
}

// Identify reports whether hash is a bcrypt hash.
func (Bcrypt) Identify(hash string) bool {
	return strings.HasPrefix(hash, "$2")
}

// Hash hashes password using bcrypt with the configured cost.
func (b Bcrypt) Hash(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), b.cost())
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}
	return string(hash), nil
}

// Verify checks password against hash.
func (b Bcrypt) Verify(hash, password string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if errors.Is(err, bcrypt.ErrMismatchedHashAndPassword) {
		return ErrMismatch
	}
	if err != nil {
		return fmt.Errorf("verifying password: %w", err)
	}
	return nil
}

// NeedsUpdate reports whether hash was hashed with a cost below MinCost.
func (b Bcrypt) NeedsUpdate(hash string) bool {
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		return false
	}
	return cost < b.minCost()
}
