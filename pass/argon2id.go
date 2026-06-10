package pass

import (
	"fmt"
	"strings"

	"github.com/alexedwards/argon2id"
)

// RFC9106LowMemory is the Argon2id parameter set from RFC 9106's second
// recommendation, for environments where memory is constrained to ~64 MiB.
// This is the default used by pass.Default.
var RFC9106LowMemory = Argon2id{
	Memory:      1 << 16, // 64 MiB
	Iterations:  3,
	Parallelism: 4,
	KeyLen:      32,
	SaltLen:     16,
}

// RFC9106HighMemory is the Argon2id parameter set from RFC 9106's first
// recommendation, for environments where memory is not at a premium.
var RFC9106HighMemory = Argon2id{
	Memory:      1 << 21, // 2 GiB
	Iterations:  1,
	Parallelism: 4,
	KeyLen:      32,
	SaltLen:     16,
}

// Argon2id hashes passwords using the argon2id algorithm. The zero value uses
// the defaults from github.com/alexedwards/argon2id (64 MiB memory, 1
// iteration, runtime.NumCPU() parallelism, 32-byte key, 16-byte salt).
//
// Django's "argon2$argon2id$..." hash format is accepted for verification and
// always triggers NeedsUpdate so hashes are migrated to the canonical format
// on next login.
type Argon2id struct {
	Memory      uint32 // kibibytes; 0 uses default (65536)
	Iterations  uint32 // 0 uses default (1)
	Parallelism uint8  // 0 uses default (runtime.NumCPU())
	KeyLen      uint32 // 0 uses default (32)
	SaltLen     uint32 // 0 uses default (16)
}

func (a Argon2id) params() *argon2id.Params {
	p := *argon2id.DefaultParams
	if a.Memory != 0 {
		p.Memory = a.Memory
	}
	if a.Iterations != 0 {
		p.Iterations = a.Iterations
	}
	if a.Parallelism != 0 {
		p.Parallelism = a.Parallelism
	}
	if a.KeyLen != 0 {
		p.KeyLength = a.KeyLen
	}
	if a.SaltLen != 0 {
		p.SaltLength = a.SaltLen
	}
	return &p
}

// normalize converts Django's "argon2$argon2id$..." format to the standard
// PHC format "$argon2id$..." by stripping the leading "argon2" prefix.
func (Argon2id) normalize(hash string) string {
	return strings.TrimPrefix(hash, "argon2")
}

// Identify reports whether hash was produced by argon2id, including Django's
// "argon2$argon2id$..." format.
func (Argon2id) Identify(hash string) bool {
	return strings.HasPrefix(hash, "$argon2id$") ||
		strings.HasPrefix(hash, "argon2$argon2id$")
}

// Hash hashes password using argon2id with the configured parameters.
func (a Argon2id) Hash(password string) (string, error) {
	hash, err := argon2id.CreateHash(password, a.params())
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}
	return hash, nil
}

// Verify checks password against hash, accepting both standard and Django
// formats.
func (a Argon2id) Verify(hash, password string) error {
	match, err := argon2id.ComparePasswordAndHash(password, a.normalize(hash))
	if err != nil {
		return fmt.Errorf("verifying password: %w", err)
	}
	if !match {
		return ErrMismatch
	}
	return nil
}

// NeedsUpdate reports whether hash should be rehashed. Returns true for
// Django-format hashes (to migrate to canonical format) or if the stored
// parameters differ from the current configuration.
func (a Argon2id) NeedsUpdate(hash string) bool {
	if strings.HasPrefix(hash, "argon2$") {
		return true
	}
	params, _, _, err := argon2id.DecodeHash(hash)
	if err != nil {
		return false
	}
	current := a.params()
	return params.Memory != current.Memory ||
		params.Iterations != current.Iterations ||
		params.Parallelism != current.Parallelism ||
		params.KeyLength != current.KeyLength
}
