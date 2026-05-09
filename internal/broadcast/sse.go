package broadcast

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	nethttp "net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SSEEvent 是 SSE 广播事件的统一结构。
type SSEEvent struct {
	Topic   string `json:"topic"`
	Action  string `json:"action"` // update, create, delete
	ID      string `json:"id,omitempty"`
	Payload any    `json:"payload,omitempty"`
}

type client struct {
	id     string
	ip     string
	topics map[string]bool
	send   chan []byte
	active time.Time
}

// Hub 管理所有 SSE 连接和广播。
type Hub struct {
	mu           sync.RWMutex
	clients      map[string]*client
	topicClients map[string]map[string]*client // topic -> clientID -> client
	logger       *slog.Logger

	maxConn      int
	maxConnPerIP int
	timeout      time.Duration
	heartbeat    time.Duration
}

var defaultHub *Hub
var clientCounter int64

func init() {
	maxConn := 500
	if v := os.Getenv("SSE_MAX_CONNECTIONS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxConn = n
		}
	}
	maxPerIP := 10
	if v := os.Getenv("SSE_MAX_CONNECTIONS_PER_IP"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxPerIP = n
		}
	}
	timeout := 30 * time.Minute
	if v := os.Getenv("SSE_CONNECTION_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			timeout = d
		}
	}

	defaultHub = &Hub{
		clients:      make(map[string]*client),
		topicClients: make(map[string]map[string]*client),
		logger:       slog.Default(),
		maxConn:      maxConn,
		maxConnPerIP: maxPerIP,
		timeout:      timeout,
		heartbeat:    30 * time.Second,
	}

	go defaultHub.cleanupLoop()
}

// SetLogger 设置全局 Hub 的 logger。
func SetLogger(l *slog.Logger) {
	defaultHub.mu.Lock()
	defer defaultHub.mu.Unlock()
	defaultHub.logger = l
}

func (h *Hub) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		h.cleanup()
	}
}

func (h *Hub) cleanup() {
	h.mu.Lock()
	defer h.mu.Unlock()

	now := time.Now()
	for id, c := range h.clients {
		if now.Sub(c.active) > h.timeout {
			h.removeClientLocked(id)
		}
	}
}

func nextClientID() string {
	return fmt.Sprintf("c-%d-%d", time.Now().UnixNano(), atomic.AddInt64(&clientCounter, 1))
}

// Subscribe 建立 SSE 长连接。
// topics 通过 query 参数 ?topics=hosts,tasks 传递，支持多值和逗号分隔。
func (h *Hub) Subscribe(w nethttp.ResponseWriter, r *nethttp.Request) {
	// 解析 topics
	var topicSet []string
	for _, t := range r.URL.Query()["topics"] {
		for _, part := range strings.Split(t, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				topicSet = append(topicSet, part)
			}
		}
	}
	if len(topicSet) == 0 {
		nethttp.Error(w, `{"error":"topics required"}`, nethttp.StatusBadRequest)
		return
	}

	ip := clientIP(r)

	h.mu.Lock()

	if len(h.clients) >= h.maxConn {
		h.mu.Unlock()
		h.logger.Warn("sse: max connections reached, rejecting",
			"ip", ip, "max", h.maxConn)
		w.Header().Set("X-SSE-Fallback", "polling")
		nethttp.Error(w, `{"error":"sse unavailable, fallback to polling"}`, nethttp.StatusServiceUnavailable)
		return
	}

	ipCount := 0
	for _, c := range h.clients {
		if c.ip == ip {
			ipCount++
		}
	}
	if ipCount >= h.maxConnPerIP {
		h.mu.Unlock()
		h.logger.Warn("sse: max connections per IP reached, rejecting",
			"ip", ip, "max", h.maxConnPerIP)
		w.Header().Set("X-SSE-Fallback", "polling")
		nethttp.Error(w, `{"error":"sse unavailable, fallback to polling"}`, nethttp.StatusServiceUnavailable)
		return
	}

	clientID := nextClientID()
	c := &client{
		id:     clientID,
		ip:     ip,
		topics: make(map[string]bool),
		send:   make(chan []byte, 64),
		active: time.Now(),
	}
	for _, t := range topicSet {
		c.topics[t] = true
		if h.topicClients[t] == nil {
			h.topicClients[t] = make(map[string]*client)
		}
		h.topicClients[t][clientID] = c
	}
	h.clients[clientID] = c
	clientCount := len(h.clients)
	h.mu.Unlock()

	h.logger.Info("sse: client connected",
		"client_id", clientID,
		"ip", ip,
		"topics", topicSet,
		"total_clients", clientCount)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(nethttp.StatusOK)

	flusher, ok := w.(nethttp.Flusher)
	if !ok {
		h.logger.Error("sse: ResponseWriter does not support flushing")
		h.removeClient(clientID)
		return
	}

	fmt.Fprintf(w, "event: connected\ndata: %s\n\n", `{"status":"ok"}`)
	flusher.Flush()

	ctx := r.Context()
	heartbeat := time.NewTicker(h.heartbeat)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			h.removeClient(clientID)
			return
		case <-heartbeat.C:
			h.mu.RLock()
			if client, ok := h.clients[clientID]; ok {
				client.active = time.Now()
			}
			h.mu.RUnlock()
			fmt.Fprintf(w, ":heartbeat\n\n")
			flusher.Flush()
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			h.mu.RLock()
			if client, exists := h.clients[clientID]; exists {
				client.active = time.Now()
			}
			h.mu.RUnlock()
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

func (h *Hub) removeClient(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.removeClientLocked(id)
}

func (h *Hub) removeClientLocked(id string) {
	c, ok := h.clients[id]
	if !ok {
		return
	}
	close(c.send)
	delete(h.clients, id)
	for topic := range c.topics {
		if m, ok := h.topicClients[topic]; ok {
			delete(m, id)
			if len(m) == 0 {
				delete(h.topicClients, topic)
			}
		}
	}
	h.logger.Info("sse: client disconnected", "client_id", id, "ip", c.ip)
}

// Broadcast 向指定 topic 的所有订阅客户端广播轻量事件。
func (h *Hub) Broadcast(topic, action, id string) {
	event := SSEEvent{Topic: topic, Action: action, ID: id}
	data, err := json.Marshal(event)
	if err != nil {
		h.logger.Error("sse: marshal event failed", "error", err)
		return
	}

	h.mu.RLock()
	subscribers, ok := h.topicClients[topic]
	if !ok || len(subscribers) == 0 {
		h.mu.RUnlock()
		return
	}
	clients := make([]*client, 0, len(subscribers))
	for _, c := range subscribers {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		select {
		case c.send <- data:
		default:
			h.removeClient(c.id)
		}
	}
}

// BroadcastJSON 向指定 topic 广播任意 JSON payload。
func (h *Hub) BroadcastJSON(topic string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		h.logger.Error("sse: marshal payload failed", "error", err)
		return
	}

	h.mu.RLock()
	subscribers, ok := h.topicClients[topic]
	if !ok || len(subscribers) == 0 {
		h.mu.RUnlock()
		return
	}
	clients := make([]*client, 0, len(subscribers))
	for _, c := range subscribers {
		clients = append(clients, c)
	}
	h.mu.RUnlock()

	for _, c := range clients {
		select {
		case c.send <- data:
		default:
			h.removeClient(c.id)
		}
	}
}

// Stats 返回当前连接数和 topic 数。
func (h *Hub) Stats() (clients, topics int) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients), len(h.topicClients)
}

// 包级便捷函数

func Subscribe(w nethttp.ResponseWriter, r *nethttp.Request) {
	defaultHub.Subscribe(w, r)
}

// Broadcast 向指定 topic 广播轻量事件（topic + action + id）。
func Broadcast(topic, action, id string) {
	defaultHub.Broadcast(topic, action, id)
}

// BroadcastJSON 向指定 topic 广播任意 JSON payload。
func BroadcastJSON(topic string, payload any) {
	defaultHub.BroadcastJSON(topic, payload)
}

// Stats 返回当前连接统计。
func Stats() (clients, topics int) {
	return defaultHub.Stats()
}

func clientIP(r *nethttp.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i != -1 {
			return strings.TrimSpace(xff[:i])
		}
		return xff
	}
	if xri := r.Header.Get("X-Real-Ip"); xri != "" {
		return xri
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
