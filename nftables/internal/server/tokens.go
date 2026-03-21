package server

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

const (
	// DefaultTokenExpiry is the default lifetime for join tokens.
	DefaultTokenExpiry = 24 * time.Hour
	// tokenBytes is the number of random bytes used to generate a token.
	tokenBytes = 32
)

// Token represents a join token stored in the database.
type Token struct {
	Hash      string     `json:"token_hash"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	UsedBy    string     `json:"used_by,omitempty"`
}

// GenerateToken creates a new single-use join token.
// It returns the raw token string (shown to user once) and stores only the SHA256 hash.
func (s *Store) GenerateToken(expiry time.Duration) (string, time.Time, error) {
	if expiry <= 0 {
		expiry = DefaultTokenExpiry
	}

	// Generate random token
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, fmt.Errorf("generate random token: %w", err)
	}
	rawHex := hex.EncodeToString(raw)

	// Hash for storage
	hash := hashToken(rawHex)
	expiresAt := time.Now().UTC().Add(expiry)

	_, err := s.db.Exec(
		"INSERT INTO tokens (token_hash, expires_at) VALUES (?, ?)",
		hash, expiresAt,
	)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("store token: %w", err)
	}

	return rawHex, expiresAt, nil
}

// ValidateAndConsumeToken checks a raw token against stored hashes.
// If valid (exists, unused, not expired), it marks the token as used and returns nil.
// The usedBy parameter records which host consumed the token.
func (s *Store) ValidateAndConsumeToken(rawToken, usedBy string) error {
	hash := hashToken(rawToken)

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var expiresAt time.Time
	var usedAt sql.NullTime
	err = tx.QueryRow(
		"SELECT expires_at, used_at FROM tokens WHERE token_hash = ?",
		hash,
	).Scan(&expiresAt, &usedAt)
	if err == sql.ErrNoRows {
		return fmt.Errorf("invalid token")
	}
	if err != nil {
		return fmt.Errorf("query token: %w", err)
	}

	if usedAt.Valid {
		return fmt.Errorf("token already used")
	}

	if time.Now().UTC().After(expiresAt) {
		return fmt.Errorf("token expired")
	}

	// Mark as used
	_, err = tx.Exec(
		"UPDATE tokens SET used_at = CURRENT_TIMESTAMP, used_by = ? WHERE token_hash = ?",
		usedBy, hash,
	)
	if err != nil {
		return fmt.Errorf("consume token: %w", err)
	}

	return tx.Commit()
}

// ListTokens returns all tokens (for admin inspection).
func (s *Store) ListTokens() ([]Token, error) {
	rows, err := s.db.Query(
		"SELECT token_hash, created_at, expires_at, used_at, used_by FROM tokens ORDER BY created_at DESC",
	)
	if err != nil {
		return nil, fmt.Errorf("list tokens: %w", err)
	}
	defer rows.Close()

	var tokens []Token
	for rows.Next() {
		var t Token
		var usedAt sql.NullTime
		var usedBy sql.NullString
		if err := rows.Scan(&t.Hash, &t.CreatedAt, &t.ExpiresAt, &usedAt, &usedBy); err != nil {
			return nil, fmt.Errorf("scan token: %w", err)
		}
		if usedAt.Valid {
			t.UsedAt = &usedAt.Time
		}
		if usedBy.Valid {
			t.UsedBy = usedBy.String
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// RevokeToken deletes a token by its hash.
func (s *Store) RevokeToken(tokenHash string) error {
	result, err := s.db.Exec("DELETE FROM tokens WHERE token_hash = ?", tokenHash)
	if err != nil {
		return fmt.Errorf("revoke token: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// hashToken computes the SHA256 hex digest of a raw token string.
func hashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}
