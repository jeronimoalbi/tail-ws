package broadcast

import (
	"sync"

	"github.com/gorilla/websocket"
)

// NewConnections create a new Websocket connections registry.
func NewConnections() *Connections {
	return &Connections{
		registry: make(map[*websocket.Conn]struct{}),
	}
}

// Connections keeps track of active Websocket connections.
type Connections struct {
	mu       sync.RWMutex
	registry map[*websocket.Conn]struct{}
}

// IsEmpty checks if there are registered connections.
func (c *Connections) IsEmpty() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.registry) == 0
}

// Add adds a new Websocket connection to the registry.
func (c *Connections) Add(ws *websocket.Conn) {
	c.mu.Lock()
	c.registry[ws] = struct{}{}
	c.mu.Unlock()
}

// Delete removes a Websocket connection from the registry.
// Connections are closed after being removed.
func (c *Connections) Delete(ws *websocket.Conn) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.registry, ws)
	return ws.Close()
}

// Close closes all connections.
func (c *Connections) Close() {
	c.Iter(func(ws *websocket.Conn) bool {
		ws.Close()
		return true
	})
}

// Iter allows iterating the current connections.
// Iteration stops when when false is returned.
func (c *Connections) Iter(fn func(*websocket.Conn) bool) {
	c.mu.RLock()
	for ws := range c.registry {
		if !fn(ws) {
			return
		}
	}
	c.mu.RUnlock()
}
