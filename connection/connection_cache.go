package connection

import (
	"context"
	"fmt"
	"github.com/eko/gocache/v3/cache"
	"github.com/eko/gocache/v3/store"
	"github.com/gertd/go-pluralize"
	"log"
	"sync"
	"time"
)

// ConnectionCache is a simple cache wrapper - multiple connections use the same underlying cache (owned by the plugin)
// ConnectionCache modifies the cache keys to include the connection name and uses the underlying shared cache
type ConnectionCache struct {
	connectionName string
	cache          *cache.Cache[any]
	keys           map[string]struct{}
	keysLock       sync.Mutex
}

func NewConnectionCache(connectionName string, connectionCache *cache.Cache[any]) *ConnectionCache {
	return &ConnectionCache{
		connectionName: connectionName,
		cache:          connectionCache,
		keys:           make(map[string]struct{}),
	}
}

func (c *ConnectionCache) Set(ctx context.Context, key string, value interface{}) error {
	return c.SetWithTTL(ctx, key, value, 1*time.Hour)
}

func (c *ConnectionCache) SetWithTTL(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	// build a key which includes the connection name
	key = c.buildCacheKey(key)
	c.keysLock.Lock()
	c.keys[key] = struct{}{}
	c.keysLock.Unlock()

	err := c.cache.Set(ctx,
		key,
		value,
		// if ttl is zero there is no expiration
		store.WithExpiration(ttl),
		// put connection name in tags
		store.WithTags([]string{c.connectionName}),
	)

	// wait for value to pass through buffers (necessary for ristretto)
	time.Sleep(10 * time.Millisecond)

	return err
}

func (c *ConnectionCache) Get(ctx context.Context, key string) (interface{}, bool) {
	// build a key which includes the connection name
	key = c.buildCacheKey(key)
	item, err := c.cache.Get(ctx, key)
	success := err == nil

	return item, success
}

func (c *ConnectionCache) Delete(ctx context.Context, key string) {
	// build a key which includes the connection name
	key = c.buildCacheKey(key)

	c.cache.Delete(ctx, key)
}

// Clear deletes all cache items for this connection
func (c *ConnectionCache) Clear(ctx context.Context) {
	// TODO TACTICAL
	c.keysLock.Lock()
	log.Printf("[INFO] ConnectionCache clear for connection '%s', deleting %d %s",
		c.connectionName,
		len(c.keys),
		pluralize.NewClient().Pluralize("key", len(c.keys), false),
	)
	for key := range c.keys {
		c.cache.Delete(ctx, key)
	}
	c.keys = make(map[string]struct{})
	c.keysLock.Unlock()

	//c.cache.Invalidate(ctx, store.WithInvalidateTags([]string{c.connectionName}))
}

func (c *ConnectionCache) buildCacheKey(key string) string {
	return fmt.Sprintf("__connection_cache_key_%s__%s", c.connectionName, key)
}
