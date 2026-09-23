package agent

import (
	"container/list"
	"sync"
)

// thumbnailCacheBytes bounds the in-memory thumbnail cache. Entries are keyed
// by path and resource version, so any change to the file misses the cache;
// nothing is written to disk.
const (
	thumbnailCacheBytes    = 8 << 20
	thumbnailCacheMaxEntry = 512 << 10
)

type thumbnailCacheEntry struct {
	key         string
	content     []byte
	contentType string
}

type thumbnailCache struct {
	mu      sync.Mutex
	limit   int
	size    int
	order   *list.List
	entries map[string]*list.Element
}

func newThumbnailCache(limit int) *thumbnailCache {
	return &thumbnailCache{limit: limit, order: list.New(), entries: make(map[string]*list.Element)}
}

func thumbnailCacheKey(path, version string) string { return version + "\x00" + path }

func (c *thumbnailCache) get(key string) ([]byte, string, bool) {
	if c == nil {
		return nil, "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	element := c.entries[key]
	if element == nil {
		return nil, "", false
	}
	c.order.MoveToFront(element)
	entry := element.Value.(*thumbnailCacheEntry)
	return entry.content, entry.contentType, true
}

func (c *thumbnailCache) put(key string, content []byte, contentType string) {
	if c == nil || len(content) == 0 || len(content) > thumbnailCacheMaxEntry || len(content) > c.limit {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if element := c.entries[key]; element != nil {
		c.size -= len(element.Value.(*thumbnailCacheEntry).content)
		c.order.Remove(element)
		delete(c.entries, key)
	}
	stored := append([]byte(nil), content...)
	c.entries[key] = c.order.PushFront(&thumbnailCacheEntry{key: key, content: stored, contentType: contentType})
	c.size += len(stored)
	for c.size > c.limit {
		oldest := c.order.Back()
		entry := oldest.Value.(*thumbnailCacheEntry)
		c.order.Remove(oldest)
		delete(c.entries, entry.key)
		c.size -= len(entry.content)
	}
}
