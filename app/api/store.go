package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schema string

var ErrNotFound = errors.New("not found")

type User struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

type Store struct {
	db *pgxpool.Pool
}

func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.db.Exec(ctx, schema)
	return err
}

func (s *Store) UpsertUser(ctx context.Context, u User) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO users (id, email, name, picture) VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET email = $2, name = $3, picture = $4`,
		u.ID, u.Email, u.Name, u.Picture)
	return err
}

func (s *Store) CreateSession(ctx context.Context, userID string, ttl time.Duration) (token string, err error) {
	token = randomToken()
	if _, err := s.db.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`); err != nil {
		return "", err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		hashToken(token), userID, time.Now().Add(ttl))
	return token, err
}

func (s *Store) UserBySession(ctx context.Context, token string) (User, error) {
	var u User
	err := s.db.QueryRow(ctx, `
		SELECT u.id, u.email, u.name, u.picture
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > now()`,
		hashToken(token)).Scan(&u.ID, &u.Email, &u.Name, &u.Picture)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	return u, err
}

func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hashToken(token))
	return err
}

func (s *Store) Items(ctx context.Context, userID string) (map[string][]json.RawMessage, error) {
	rows, err := s.db.Query(ctx, `SELECT collection, data FROM items WHERE user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := map[string][]json.RawMessage{}
	for rows.Next() {
		var collection string
		var data json.RawMessage
		if err := rows.Scan(&collection, &data); err != nil {
			return nil, err
		}
		items[collection] = append(items[collection], data)
	}
	return items, rows.Err()
}

func (s *Store) PutItem(ctx context.Context, userID, collection, id string, data json.RawMessage) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO items (user_id, collection, id, data) VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, collection, id) DO UPDATE SET data = $4, updated_at = now()`,
		userID, collection, id, data)
	return err
}

func (s *Store) DeleteItem(ctx context.Context, userID, collection, id string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM items WHERE user_id = $1 AND collection = $2 AND id = $3`,
		userID, collection, id)
	return err
}

type Asset struct {
	ID          string
	UserID      string
	ContentType string
	SizeBytes   int64
}

func (s *Store) AssetsSize(ctx context.Context, userID string) (int64, error) {
	var total int64
	err := s.db.QueryRow(ctx, `SELECT coalesce(sum(size_bytes), 0) FROM assets WHERE user_id = $1`, userID).Scan(&total)
	return total, err
}

func (s *Store) CreateAsset(ctx context.Context, a Asset) error {
	_, err := s.db.Exec(ctx, `INSERT INTO assets (id, user_id, content_type, size_bytes) VALUES ($1, $2, $3, $4)`,
		a.ID, a.UserID, a.ContentType, a.SizeBytes)
	return err
}

func (s *Store) Asset(ctx context.Context, userID, id string) (Asset, error) {
	a := Asset{ID: id, UserID: userID}
	err := s.db.QueryRow(ctx, `SELECT content_type, size_bytes FROM assets WHERE user_id = $1 AND id = $2`,
		userID, id).Scan(&a.ContentType, &a.SizeBytes)
	if errors.Is(err, pgx.ErrNoRows) {
		return Asset{}, ErrNotFound
	}
	return a, err
}

func (s *Store) DeleteAsset(ctx context.Context, userID, id string) (deleted bool, err error) {
	tag, err := s.db.Exec(ctx, `DELETE FROM assets WHERE user_id = $1 AND id = $2`, userID, id)
	return tag.RowsAffected() > 0, err
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
