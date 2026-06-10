// Package pass provides multi-scheme password hashing with transparent
// migration support. Argon2id is the default; bcrypt is accepted for
// migration from existing stores. Django's "argon2$argon2id$..." hash
// format is also supported.
//
// Typical login flow:
//
//	pc := pass.NewDefaultContext()
//
//	if err := pc.Verify(stored, input); err != nil {
//	    return err
//	}
//	if pc.NeedsUpdate(stored) {
//	    if newHash, err := pc.Hash(input); err == nil {
//	        _ = db.UpdatePasswordHash(ctx, userID, newHash)
//	    }
//	}
package pass

import (
	"errors"
	"sync"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// ErrMismatch is returned by Verify when the password does not match the hash.
var ErrMismatch = errors.New("password does not match")

// ErrUnknownHash is returned by Verify when no registered hasher recognizes
// the hash format.
var ErrUnknownHash = errors.New("unrecognized hash format")

// Hasher is implemented by each supported password hashing algorithm.
type Hasher interface {
	// Hash produces an encoded hash string for password.
	Hash(password string) (string, error)
	// Verify checks password against hash. Returns ErrMismatch on wrong
	// password or ErrUnknownHash if the format is not recognized.
	Verify(hash, password string) error
	// Identify reports whether this hasher produced hash.
	Identify(hash string) bool
	// NeedsUpdate reports whether hash should be rehashed with current
	// parameters.
	NeedsUpdate(hash string) bool
}

// Context holds an ordered list of hashers. The first hasher is the default
// for new hashes; the rest are accepted for verification but treated as
// deprecated (NeedsUpdate returns true for any hash they identify).
type Context struct {
	hashers   []Hasher
	dummyOnce sync.Once
	dummyHash string
}

// NewContext returns a Context using the given hashers. The first hasher is
// used for new hashes; remaining hashers are treated as deprecated.
func NewContext(hashers ...Hasher) *Context {
	return &Context{hashers: hashers}
}

// Hash hashes password using the default (first) hasher.
func (c *Context) Hash(password string) (string, error) {
	return c.hashers[0].Hash(password)
}

// Verify checks password against hash, dispatching to whichever registered
// hasher identifies the hash format. Returns ErrUnknownHash if none match.
func (c *Context) Verify(hash, password string) error {
	for _, h := range c.hashers {
		if h.Identify(hash) {
			return h.Verify(hash, password)
		}
	}
	return ErrUnknownHash
}

// DummyVerify performs a full hash verification against a pre-computed dummy
// hash to prevent timing-based user enumeration. Call it in the "user not
// found" branch of your authentication flow so that missing-user lookups take
// the same time as wrong-password lookups:
//
//	user, err := db.GetUser(ctx, username)
//	if errors.Is(err, db.ErrNotFound) {
//	    pass.DummyVerify(password)
//	    return ErrInvalidCredentials
//	}
//
// The dummy hash is computed lazily on the first call using the default
// hasher's parameters.
func (c *Context) DummyVerify(password string) {
	c.dummyOnce.Do(func() {
		// Null bytes are not valid UTF-8 and won't appear in any real
		// password, so this hash can never match user input.
		c.dummyHash, _ = c.hashers[0].Hash("\x00\x00\x00\x00\x00\x00\x00\x00")
	})
	_ = c.hashers[0].Verify(c.dummyHash, password)
}

// NeedsUpdate reports whether hash should be rehashed. Returns true if the
// hash belongs to a non-default hasher or if the default hasher's own
// NeedsUpdate reports true.
func (c *Context) NeedsUpdate(hash string) bool {
	for i, h := range c.hashers {
		if h.Identify(hash) {
			return i != 0 || h.NeedsUpdate(hash)
		}
	}
	return false
}

// NewTestContext returns a Context with minimal-cost hashers for use in tests.
// The parameters are intentionally weak — do not use in production.
func NewTestContext() *Context {
	return NewContext(
		Argon2id{Memory: 8 * 1024, Iterations: 1, Parallelism: 1},
		Bcrypt{Cost: bcrypt.MinCost},
	)
}

// NewDefaultContext returns a Context suitable for most applications. Outside
// of tests it uses RFC 9106 low-memory Argon2id as the primary hasher with
// bcrypt accepted for migration. Under go test it delegates to NewTestContext
// so that test suites do not pay full hashing costs.
func NewDefaultContext() *Context {
	if testing.Testing() {
		return NewTestContext()
	}
	return NewContext(RFC9106LowMemory, Bcrypt{})
}
