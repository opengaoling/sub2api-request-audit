package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const (
	fingerprintKeyPrefix   = "fingerprint:"
	globalFingerprintKey   = "global_fingerprint"
	fingerprintTTL         = 7 * 24 * time.Hour // 7天，配合每24小时懒续期可保持活跃账号永不过期
	maskedSessionKeyPrefix = "masked_session:"
	maskedSessionTTL       = 15 * time.Minute
)

// fingerprintKey generates the Redis key for account fingerprint cache.
func fingerprintKey(accountID int64) string {
	return fmt.Sprintf("%s%d", fingerprintKeyPrefix, accountID)
}

// maskedSessionKey generates the Redis key for masked session ID cache.
func maskedSessionKey(accountID int64) string {
	return fmt.Sprintf("%s%d", maskedSessionKeyPrefix, accountID)
}

type identityCache struct {
	rdb *redis.Client
}

func NewIdentityCache(rdb *redis.Client) service.IdentityCache {
	return &identityCache{rdb: rdb}
}

func (c *identityCache) GetFingerprint(ctx context.Context, accountID int64) (*service.Fingerprint, error) {
	key := fingerprintKey(accountID)
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	var fp service.Fingerprint
	if err := json.Unmarshal([]byte(val), &fp); err != nil {
		return nil, err
	}
	return &fp, nil
}

func (c *identityCache) SetFingerprint(ctx context.Context, accountID int64, fp *service.Fingerprint) error {
	key := fingerprintKey(accountID)
	val, err := json.Marshal(fp)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, key, val, fingerprintTTL).Err()
}

func (c *identityCache) ListFingerprints(ctx context.Context) ([]*service.Fingerprint, error) {
	var cursor uint64
	result := make([]*service.Fingerprint, 0)
	for {
		keys, next, err := c.rdb.Scan(ctx, cursor, fingerprintKeyPrefix+"*", 100).Result()
		if err != nil {
			return nil, err
		}
		if len(keys) > 0 {
			values, err := c.rdb.MGet(ctx, keys...).Result()
			if err != nil {
				return nil, err
			}
			for _, value := range values {
				text, ok := value.(string)
				if !ok || text == "" {
					continue
				}
				var fp service.Fingerprint
				if json.Unmarshal([]byte(text), &fp) == nil {
					result = append(result, &fp)
				}
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return result, nil
}

func (c *identityCache) GetGlobalFingerprint(ctx context.Context) (*service.Fingerprint, error) {
	value, err := c.rdb.Get(ctx, globalFingerprintKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	var fp service.Fingerprint
	if err := json.Unmarshal([]byte(value), &fp); err != nil {
		return nil, err
	}
	return &fp, nil
}

func (c *identityCache) SetGlobalFingerprint(ctx context.Context, fp *service.Fingerprint) error {
	if fp == nil {
		return c.rdb.Del(ctx, globalFingerprintKey).Err()
	}
	value, err := json.Marshal(fp)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, globalFingerprintKey, value, 0).Err()
}

func (c *identityCache) GetMaskedSessionID(ctx context.Context, accountID int64) (string, error) {
	key := maskedSessionKey(accountID)
	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", err
	}
	return val, nil
}

func (c *identityCache) SetMaskedSessionID(ctx context.Context, accountID int64, sessionID string) error {
	key := maskedSessionKey(accountID)
	return c.rdb.Set(ctx, key, sessionID, maskedSessionTTL).Err()
}
