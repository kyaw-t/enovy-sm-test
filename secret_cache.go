package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
)

// SecretCache fetches and caches a secret from AWS Secrets Manager.
// It refreshes automatically when the TTL expires.
type SecretCache struct {
	client   *secretsmanager.Client
	secretID string
	jsonKey  string // key to extract from the secret JSON, e.g. "api-key"
	ttl      time.Duration

	mu        sync.RWMutex
	value     string
	fetchedAt time.Time
}

func NewSecretCache(client *secretsmanager.Client, secretID, jsonKey string, ttl time.Duration) *SecretCache {
	return &SecretCache{
		client:   client,
		secretID: secretID,
		jsonKey:  jsonKey,
		ttl:      ttl,
	}
}

// Get returns the cached secret value, refreshing if the TTL has expired.
func (sc *SecretCache) Get(ctx context.Context) (string, error) {
	sc.mu.RLock()
	if sc.value != "" && time.Since(sc.fetchedAt) < sc.ttl {
		val := sc.value
		sc.mu.RUnlock()
		return val, nil
	}
	sc.mu.RUnlock()

	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Double-check after acquiring write lock
	if sc.value != "" && time.Since(sc.fetchedAt) < sc.ttl {
		return sc.value, nil
	}

	result, err := sc.client.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(sc.secretID),
	})
	if err != nil {
		// If we have a stale value, return it rather than failing
		if sc.value != "" {
			return sc.value, nil
		}
		return "", fmt.Errorf("failed to fetch secret %s: %w", sc.secretID, err)
	}

	secretStr := aws.ToString(result.SecretString)

	// If a JSON key is specified, extract it
	if sc.jsonKey != "" {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(secretStr), &parsed); err != nil {
			return "", fmt.Errorf("failed to parse secret JSON: %w", err)
		}
		val, ok := parsed[sc.jsonKey]
		if !ok {
			return "", fmt.Errorf("key %q not found in secret JSON", sc.jsonKey)
		}
		secretStr = fmt.Sprintf("%v", val)
	}

	sc.value = secretStr
	sc.fetchedAt = time.Now()
	return sc.value, nil
}
