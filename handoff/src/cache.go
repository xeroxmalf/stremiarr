package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type CacheBackend interface {
	GetStreamCache(key string) (string, error)
	SetStreamCache(key string, data string, ttl time.Duration) error
	GetCatalogCache(key string) ([]byte, http.Header, int, error)
	SetCatalogCache(key string, body []byte, headers http.Header, statusCode int, ttl time.Duration) error
}

// ---------------------------
// REDIS BACKEND
// ---------------------------
type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(url string) *RedisCache {
	opt, err := redis.ParseURL(url)
	if err != nil {
		log.Fatalf("❌ Invalid Redis URL: %v", err)
	}
	client := redis.NewClient(opt)
	return &RedisCache{client: client}
}

func (r *RedisCache) GetStreamCache(key string) (string, error) {
	ctx := context.Background()
	return r.client.Get(ctx, "stream:"+key).Result()
}

func (r *RedisCache) SetStreamCache(key string, data string, ttl time.Duration) error {
	ctx := context.Background()
	return r.client.Set(ctx, "stream:"+key, data, ttl).Err()
}

func (r *RedisCache) GetCatalogCache(key string) ([]byte, http.Header, int, error) {
	ctx := context.Background()
	val, err := r.client.Get(ctx, "catalog:"+key).Result()
	if err != nil {
		return nil, nil, 0, err
	}

	var ce cacheEntry
	err = json.Unmarshal([]byte(val), &ce)
	if err != nil {
		return nil, nil, 0, err
	}
	return ce.Body, ce.Headers, ce.StatusCode, nil
}

func (r *RedisCache) SetCatalogCache(key string, body []byte, headers http.Header, statusCode int, ttl time.Duration) error {
	ctx := context.Background()
	ce := cacheEntry{
		Body:       body,
		Headers:    headers,
		StatusCode: statusCode,
		ExpiresAt:  time.Now().Add(ttl),
	}
	data, err := json.Marshal(ce)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, "catalog:"+key, data, ttl).Err()
}

// ---------------------------
// SQLITE & MEMORY BACKEND
// ---------------------------
type LocalCache struct{}

func (l *LocalCache) GetStreamCache(key string) (string, error) {
	var data string
	err := db.QueryRow("SELECT streams_json FROM stream_cache WHERE request_id = ?", key).Scan(&data)
	return data, err
}

func (l *LocalCache) SetStreamCache(key string, data string, ttl time.Duration) error {
	// Local Cache does not enforce TTL at insertion (handled by initDBMaintenance)
	_, err := db.Exec(`
        INSERT INTO stream_cache (request_id, streams_json, updated_at) 
        VALUES (?, ?, ?)
        ON CONFLICT(request_id) DO UPDATE SET 
            streams_json=excluded.streams_json, 
            updated_at=excluded.updated_at
    `, key, data, time.Now())
	return err
}

func (l *LocalCache) GetCatalogCache(key string) ([]byte, http.Header, int, error) {
	if entry, ok := catalogCache.Load(key); ok {
		ce := entry.(cacheEntry)
		if time.Now().Before(ce.ExpiresAt) {
			return ce.Body, ce.Headers, ce.StatusCode, nil
		}
		catalogCache.Delete(key)
	}
	return nil, nil, 0, redis.Nil
}

func (l *LocalCache) SetCatalogCache(key string, body []byte, headers http.Header, statusCode int, ttl time.Duration) error {
	catalogCache.Store(key, cacheEntry{
		Body:       body,
		Headers:    headers.Clone(),
		StatusCode: statusCode,
		ExpiresAt:  time.Now().Add(ttl),
	})
	return nil
}

// ---------------------------
// GLOBAL CACHE INSTANCE
// ---------------------------
var cache CacheBackend

func initCache() {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL != "" {
		log.Printf("📦 Initializing Redis Cache backend...")
		cache = NewRedisCache(redisURL)
	} else {
		log.Printf("📦 Initializing SQLite/Memory Cache backend...")
		cache = &LocalCache{}
	}
}

// --- CATALOG CACHING ---
type cacheEntry struct {
	Body       []byte
	Headers    http.Header
	StatusCode int
	ExpiresAt  time.Time
}

var catalogCache sync.Map

func initCatalogCache() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			deleted := 0
			catalogCache.Range(func(key, value interface{}) bool {
				ce := value.(cacheEntry)
				if now.After(ce.ExpiresAt) {
					catalogCache.Delete(key)
					deleted++
				}
				return true
			})
			if deleted > 0 {
				log.Printf("[Catalog] 🧹 Cleaned %d expired catalog cache entries", deleted)
			}
		}
	}()
}
