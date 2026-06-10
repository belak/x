package pass

import (
	"testing"

	"github.com/alecthomas/assert/v2"
)

// djangoHash is a Django-format argon2id hash of "hunter2", used to verify
// compatibility with Django's hash format.
const djangoHash = "argon2$argon2id$v=19$m=102400,t=2,p=8$2Edzkzz0lFpqNgSJVwwnPA$Lfy0zIbXPqhjiFLb9eBr3UOA37z77D9+lXA4CnE7mzc"

func TestArgon2id_HashVerify(t *testing.T) {
	h := Argon2id{Memory: 8 * 1024, Iterations: 1, Parallelism: 1}

	hash, err := h.Hash("password")
	assert.NoError(t, err)

	assert.NoError(t, h.Verify(hash, "password"))

	err = h.Verify(hash, "wrong")
	assert.IsError(t, err, ErrMismatch)
}

func TestArgon2id_Identify(t *testing.T) {
	h := Argon2id{}

	assert.True(t, h.Identify("$argon2id$v=19$m=65536,t=1,p=4$fakesalt$fakehash"))
	assert.True(t, h.Identify(djangoHash))
	assert.False(t, h.Identify("$2a$10$fakehash"))
	assert.False(t, h.Identify("$argon2i$v=19$m=65536,t=1,p=4$fakesalt$fakehash"))
}

func TestArgon2id_NeedsUpdate(t *testing.T) {
	h := Argon2id{}

	t.Run("fresh hash", func(t *testing.T) {
		hash, err := h.Hash("password")
		assert.NoError(t, err)
		assert.False(t, h.NeedsUpdate(hash))
	})

	t.Run("different params", func(t *testing.T) {
		hash, err := h.Hash("password")
		assert.NoError(t, err)
		// Hash used default params; this hasher wants different memory.
		other := Argon2id{Memory: 32 * 1024}
		assert.True(t, other.NeedsUpdate(hash))
	})

	t.Run("django format always needs update", func(t *testing.T) {
		assert.True(t, h.NeedsUpdate(djangoHash))
	})
}

func TestArgon2id_Django(t *testing.T) {
	h := Argon2id{}

	t.Run("verify wrong password returns ErrMismatch not format error", func(t *testing.T) {
		err := h.Verify(djangoHash, "wrong")
		assert.IsError(t, err, ErrMismatch)
	})
}

func TestBcrypt_HashVerify(t *testing.T) {
	h := Bcrypt{Cost: 4}

	hash, err := h.Hash("password")
	assert.NoError(t, err)

	assert.NoError(t, h.Verify(hash, "password"))

	err = h.Verify(hash, "wrong")
	assert.IsError(t, err, ErrMismatch)
}

func TestBcrypt_Identify(t *testing.T) {
	h := Bcrypt{}

	assert.True(t, h.Identify("$2a$10$fakehash"))
	assert.True(t, h.Identify("$2b$10$fakehash"))
	assert.False(t, h.Identify("$argon2id$v=19$fakehash"))
}

func TestBcrypt_NeedsUpdate(t *testing.T) {
	h := Bcrypt{Cost: 4}

	t.Run("fresh hash", func(t *testing.T) {
		hash, err := h.Hash("password")
		assert.NoError(t, err)
		assert.False(t, h.NeedsUpdate(hash))
	})

	t.Run("cost below minimum", func(t *testing.T) {
		hash, err := h.Hash("password")
		assert.NoError(t, err)
		// Hash was cost 4; this hasher requires cost 10.
		other := Bcrypt{Cost: 10, MinCost: 10}
		assert.True(t, other.NeedsUpdate(hash))
	})
}

func TestContext_Hash(t *testing.T) {
	ctx := NewTestContext()

	hash, err := ctx.Hash("password")
	assert.NoError(t, err)
	assert.True(t, Argon2id{}.Identify(hash))
}

func TestContext_Verify(t *testing.T) {
	ctx := NewTestContext()

	t.Run("argon2id hash", func(t *testing.T) {
		hash, _ := Argon2id{}.Hash("password")
		assert.NoError(t, ctx.Verify(hash, "password"))
	})

	t.Run("bcrypt hash", func(t *testing.T) {
		hash, _ := Bcrypt{Cost: 4}.Hash("password")
		assert.NoError(t, ctx.Verify(hash, "password"))
	})

	t.Run("mismatch", func(t *testing.T) {
		hash, _ := ctx.Hash("password")
		assert.IsError(t, ctx.Verify(hash, "wrong"), ErrMismatch)
	})

	t.Run("unknown format", func(t *testing.T) {
		assert.IsError(t, ctx.Verify("$unknown$hash", "password"), ErrUnknownHash)
	})
}

func TestContext_NeedsUpdate(t *testing.T) {
	ctx := NewTestContext()

	t.Run("fresh default hash", func(t *testing.T) {
		hash, _ := ctx.Hash("password")
		assert.False(t, ctx.NeedsUpdate(hash))
	})

	t.Run("deprecated scheme", func(t *testing.T) {
		hash, _ := Bcrypt{Cost: 4}.Hash("password")
		assert.True(t, ctx.NeedsUpdate(hash))
	})

	t.Run("django format", func(t *testing.T) {
		assert.True(t, ctx.NeedsUpdate(djangoHash))
	})

	t.Run("unknown format", func(t *testing.T) {
		assert.False(t, ctx.NeedsUpdate("$unknown$hash"))
	})
}

func TestRFC9106Params(t *testing.T) {
	t.Run("low memory identify and verify", func(t *testing.T) {
		hash, err := RFC9106LowMemory.Hash("password")
		assert.NoError(t, err)
		assert.True(t, RFC9106LowMemory.Identify(hash))
		assert.NoError(t, RFC9106LowMemory.Verify(hash, "password"))
		assert.False(t, RFC9106LowMemory.NeedsUpdate(hash))
	})

	t.Run("high memory needs update from low memory hash", func(t *testing.T) {
		hash, err := RFC9106LowMemory.Hash("password")
		assert.NoError(t, err)
		assert.True(t, RFC9106HighMemory.NeedsUpdate(hash))
	})
}

func TestContext_DummyVerify(t *testing.T) {
	ctx := NewTestContext()

	// Should not panic and should complete without error regardless of input.
	ctx.DummyVerify("any password")
	ctx.DummyVerify("")
	ctx.DummyVerify("password")

	// Second call reuses the cached dummy hash (sync.Once).
	ctx.DummyVerify("another call")
}

func TestNewDefaultContext(t *testing.T) {
	ctx := NewDefaultContext()

	hash, err := ctx.Hash("password")
	assert.NoError(t, err)

	assert.NoError(t, ctx.Verify(hash, "password"))
	assert.IsError(t, ctx.Verify(hash, "wrong"), ErrMismatch)
	assert.False(t, ctx.NeedsUpdate(hash))

	// DummyVerify should not panic.
	ctx.DummyVerify("password")
}
