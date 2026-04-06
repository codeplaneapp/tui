package oauth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestToken_IsExpired_Fresh(t *testing.T) {
	tok := Token{
		ExpiresIn: 3600,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}
	assert.False(t, tok.IsExpired(), "token with ExpiresAt far in the future should not be expired")
}

func TestToken_IsExpired_Expired(t *testing.T) {
	tok := Token{
		ExpiresIn: 3600,
		ExpiresAt: time.Now().Add(-time.Hour).Unix(),
	}
	assert.True(t, tok.IsExpired(), "token with ExpiresAt in the past should be expired")
}

func TestToken_IsExpired_BufferZone(t *testing.T) {
	// A token that has ExpiresIn of 1000 seconds has a 10% buffer of 100s.
	// If ExpiresAt is 50 seconds from now, that falls within the 100s buffer
	// window, so it should be considered expired.
	tok := Token{
		ExpiresIn: 1000,
		ExpiresAt: time.Now().Add(50 * time.Second).Unix(),
	}
	assert.True(t, tok.IsExpired(), "token within 10%% buffer of its lifetime should be expired")
}

func TestToken_IsExpired_JustOutsideBuffer(t *testing.T) {
	// ExpiresIn = 1000s → 10% buffer = 100s.
	// ExpiresAt 200s from now → outside the buffer → not expired.
	tok := Token{
		ExpiresIn: 1000,
		ExpiresAt: time.Now().Add(200 * time.Second).Unix(),
	}
	assert.False(t, tok.IsExpired(), "token outside the 10%% buffer should not be expired")
}

func TestToken_SetExpiresAt(t *testing.T) {
	tok := Token{
		ExpiresIn: 3600,
	}
	before := time.Now().Unix()
	tok.SetExpiresAt()
	after := time.Now().Unix()

	// ExpiresAt should be roughly now + 3600 seconds.
	assert.GreaterOrEqual(t, tok.ExpiresAt, before+3600, "ExpiresAt should be at least now + ExpiresIn")
	assert.LessOrEqual(t, tok.ExpiresAt, after+3600, "ExpiresAt should be at most now + ExpiresIn")
}

func TestToken_SetExpiresIn(t *testing.T) {
	future := time.Now().Add(30 * time.Minute).Unix()
	tok := Token{
		ExpiresAt: future,
	}
	tok.SetExpiresIn()

	// Should be roughly 1800 seconds, give or take a second for test execution.
	assert.InDelta(t, 1800, tok.ExpiresIn, 2, "ExpiresIn should be approximately 1800 seconds")
}
