package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type clientFingerprintRepository struct {
	db *sql.DB
}

func NewClientFingerprintRepository(db *sql.DB) service.ClientFingerprintRepository {
	return &clientFingerprintRepository{db: db}
}

func (r *clientFingerprintRepository) Upsert(ctx context.Context, fingerprint service.CapturedFingerprint) error {
	headers, err := json.Marshal(fingerprint.Headers)
	if err != nil {
		return fmt.Errorf("marshal client fingerprint headers: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO client_request_fingerprints (
			platform, fingerprint_hash, headers, user_agent, capture_count, first_seen_at, last_seen_at
		) VALUES ($1, $2, $3, $4, 1, NOW(), NOW())
		ON CONFLICT (platform, fingerprint_hash) DO UPDATE SET
			headers = EXCLUDED.headers,
			user_agent = EXCLUDED.user_agent,
			capture_count = client_request_fingerprints.capture_count + 1,
			last_seen_at = NOW()
	`, fingerprint.Platform, fingerprint.ID, headers, fingerprint.UserAgent)
	if err != nil {
		return fmt.Errorf("upsert client fingerprint: %w", err)
	}
	return nil
}

func (r *clientFingerprintRepository) List(ctx context.Context, platform string, limit int) ([]service.CapturedFingerprint, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT fingerprint_hash, platform, headers, user_agent, capture_count, first_seen_at, last_seen_at
		FROM client_request_fingerprints
		WHERE platform = $1
		ORDER BY last_seen_at DESC
		LIMIT $2
	`, platform, limit)
	if err != nil {
		return nil, fmt.Errorf("list client fingerprints: %w", err)
	}
	defer rows.Close()

	result := make([]service.CapturedFingerprint, 0)
	for rows.Next() {
		fingerprint, scanErr := scanCapturedFingerprint(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, fingerprint)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate client fingerprints: %w", err)
	}
	return result, nil
}

func (r *clientFingerprintRepository) Get(ctx context.Context, platform, id string) (*service.CapturedFingerprint, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT fingerprint_hash, platform, headers, user_agent, capture_count, first_seen_at, last_seen_at
		FROM client_request_fingerprints
		WHERE platform = $1 AND fingerprint_hash = $2
	`, platform, id)
	fingerprint, err := scanCapturedFingerprint(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &fingerprint, nil
}

type fingerprintScanner interface {
	Scan(dest ...any) error
}

func scanCapturedFingerprint(scanner fingerprintScanner) (service.CapturedFingerprint, error) {
	var row service.CapturedFingerprint
	var headers []byte
	if err := scanner.Scan(&row.ID, &row.Platform, &headers, &row.UserAgent, &row.CaptureCount, &row.FirstSeenAt, &row.LastSeenAt); err != nil {
		return row, err
	}
	if err := json.Unmarshal(headers, &row.Headers); err != nil {
		return row, fmt.Errorf("unmarshal client fingerprint headers: %w", err)
	}
	return row, nil
}
