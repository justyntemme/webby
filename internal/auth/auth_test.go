package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashPassword(t *testing.T) {
	password := "testpassword123"

	hash, err := HashPassword(password)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, password, hash)
}

func TestCheckPassword(t *testing.T) {
	password := "testpassword123"
	hash, err := HashPassword(password)
	require.NoError(t, err)

	// Correct password
	assert.True(t, CheckPassword(password, hash))

	// Wrong password
	assert.False(t, CheckPassword("wrongpassword", hash))
}

func TestGenerateToken(t *testing.T) {
	userID := "test-user-id"
	username := "testuser"

	token, err := GenerateToken(userID, username)
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestValidateToken(t *testing.T) {
	userID := "test-user-id"
	username := "testuser"

	token, err := GenerateToken(userID, username)
	require.NoError(t, err)

	claims, err := ValidateToken(token)
	require.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)
	assert.Equal(t, username, claims.Username)
}

func TestValidateToken_Invalid(t *testing.T) {
	_, err := ValidateToken("invalid-token")
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidToken, err)
}

func TestRefreshToken(t *testing.T) {
	userID := "test-user-id"
	username := "testuser"

	token, err := GenerateToken(userID, username)
	require.NoError(t, err)

	newToken, err := RefreshToken(token)
	require.NoError(t, err)
	assert.NotEmpty(t, newToken)
	// Note: tokens generated in same second may be identical, so we just verify it's valid

	// Verify new token is valid
	claims, err := ValidateToken(newToken)
	require.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)
	assert.Equal(t, username, claims.Username)
}
