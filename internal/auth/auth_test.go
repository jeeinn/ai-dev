package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashPassword(t *testing.T) {
	hash, err := HashPassword("testpassword")
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, "testpassword", hash)
}

func TestCheckPassword(t *testing.T) {
	hash, err := HashPassword("testpassword")
	require.NoError(t, err)

	// Correct password
	assert.True(t, CheckPassword("testpassword", hash))

	// Wrong password
	assert.False(t, CheckPassword("wrongpassword", hash))
}

func TestJWTGenerateAndValidate(t *testing.T) {
	manager := NewJWTManager("test-secret", 1*time.Hour)

	// Generate token
	token, err := manager.GenerateToken(1, "testuser", "admin", false)
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	// Validate token
	claims, err := manager.ValidateToken(token)
	require.NoError(t, err)
	assert.Equal(t, int64(1), claims.UserID)
	assert.Equal(t, "testuser", claims.Username)
	assert.Equal(t, "admin", claims.Role)
}

func TestJWTExpiredToken(t *testing.T) {
	// Create manager with very short expiration
	manager := NewJWTManager("test-secret", 1*time.Millisecond)

	// Generate token
	token, err := manager.GenerateToken(1, "testuser", "admin", false)
	require.NoError(t, err)

	// Wait for token to expire
	time.Sleep(10 * time.Millisecond)

	// Validate token should fail
	_, err = manager.ValidateToken(token)
	assert.Error(t, err)
}

func TestJWTInvalidToken(t *testing.T) {
	manager := NewJWTManager("test-secret", 1*time.Hour)

	// Invalid token
	_, err := manager.ValidateToken("invalid-token")
	assert.Error(t, err)
}

func TestJWTWrongSecret(t *testing.T) {
	manager1 := NewJWTManager("secret1", 1*time.Hour)
	manager2 := NewJWTManager("secret2", 1*time.Hour)

	// Generate with secret1
	token, err := manager1.GenerateToken(1, "testuser", "admin", false)
	require.NoError(t, err)

	// Validate with secret2 should fail
	_, err = manager2.ValidateToken(token)
	assert.Error(t, err)
}

func TestPasswordMultipleHashes(t *testing.T) {
	// Same password should produce different hashes (due to salt)
	hash1, _ := HashPassword("password")
	hash2, _ := HashPassword("password")
	assert.NotEqual(t, hash1, hash2)

	// But both should validate
	assert.True(t, CheckPassword("password", hash1))
	assert.True(t, CheckPassword("password", hash2))
}
