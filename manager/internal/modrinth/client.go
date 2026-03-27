package modrinth

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	BaseURL   = "https://api.modrinth.com/v2"
	UserAgent = "DemiMine/1.0 (https://github.com/demimine/demimine)"
	CacheTTL  = 10 * time.Minute
)

type Client struct {
	httpClient *http.Client
	cache      *cache
}

type cache struct {
	mu       sync.RWMutex
	items    map[string]*cacheItem
	setCount int64
}

type cacheItem struct {
	data      []byte
	expiresAt time.Time
}

const cacheSweepInterval = 100

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache: &cache{
			items: make(map[string]*cacheItem),
		},
	}
}

func (c *Client) get(endpoint string) ([]byte, error) {
	cacheKey := endpoint

	if cached := c.cache.get(cacheKey); cached != nil {
		return cached, nil
	}

	url := BaseURL + endpoint
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("modrinth api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("modrinth api error: %d - %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	c.cache.set(cacheKey, data, CacheTTL)
	return data, nil
}

func (c *cache) get(key string) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, exists := c.items[key]
	if !exists {
		return nil
	}
	if time.Now().After(item.expiresAt) {
		delete(c.items, key)
		return nil
	}
	return item.data
}

func (c *cache) set(key string, data []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[key] = &cacheItem{
		data:      data,
		expiresAt: time.Now().Add(ttl),
	}

	c.setCount++
	if c.setCount%cacheSweepInterval == 0 {
		now := time.Now()
		for k, item := range c.items {
			if now.After(item.expiresAt) {
				delete(c.items, k)
			}
		}
	}
}

func (c *Client) ClearCache() {
	c.cache.mu.Lock()
	defer c.cache.mu.Unlock()
	c.cache.items = make(map[string]*cacheItem)
}
