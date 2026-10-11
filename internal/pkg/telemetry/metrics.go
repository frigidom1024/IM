// Package telemetry exports process-local Prometheus metrics with fixed labels.
package telemetry

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
)

var Default = New()
var bounds = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}

type httpKey struct {
	Method, Route string
	Status        int
}
type observation struct {
	Count   uint64
	Sum     float64
	Buckets []uint64
}
type Collector struct {
	mu             sync.Mutex
	http           map[httpKey]*observation
	auth           map[string]uint64
	uploadFailures uint64
	connections    int64
	users          map[string]int
	messages       map[string]uint64
	dbStats        func() sql.DBStats
	redisStats     func() RedisStats
}

type RedisStats struct {
	Total, Idle            uint32
	Hits, Misses, Timeouts uint32
}

func (c *Collector) SetRedisStats(provider func() RedisStats) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.redisStats = provider
}

func New() *Collector {
	return &Collector{http: make(map[httpKey]*observation), auth: make(map[string]uint64), users: make(map[string]int), messages: make(map[string]uint64)}
}
func (c *Collector) ObserveHTTP(method, route string, status int, seconds float64) {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
	default:
		method = "OTHER"
	}
	if route == "" {
		route = "unmatched"
	}
	if status < 100 || status > 599 {
		status = 500
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := httpKey{method, route, status}
	o := c.http[key]
	if o == nil {
		o = &observation{Buckets: make([]uint64, len(bounds))}
		c.http[key] = o
	}
	o.Count++
	o.Sum += seconds
	for i, b := range bounds {
		if seconds <= b {
			o.Buckets[i]++
		}
	}
}
func (c *Collector) Auth(transport string, success bool) {
	if transport != "ws" {
		transport = "http"
	}
	result := "failure"
	if success {
		result = "success"
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.auth[transport+":"+result]++
}
func (c *Collector) UploadFailure() { c.mu.Lock(); c.uploadFailures++; c.mu.Unlock() }

// User IDs are held solely for calculating the unique-users gauge; never labels.
func (c *Collector) Connected(user string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connections++
	c.users[user]++
}
func (c *Collector) Disconnected(user string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.users[user] > 0 {
		c.connections--
		c.users[user]--
		if c.users[user] == 0 {
			delete(c.users, user)
		}
	}
}
func (c *Collector) MessagePersisted(success bool) {
	result := "persisted"
	if !success {
		result = "persistence_failure"
	}
	c.mu.Lock()
	c.messages[result]++
	c.mu.Unlock()
}
func (c *Collector) SetDBStats(stats func() sql.DBStats) {
	c.mu.Lock()
	c.dbStats = stats
	c.mu.Unlock()
}
func (c *Collector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	c.Write(w)
}
func (c *Collector) Write(w io.Writer) {
	c.mu.Lock()
	var buffer bytes.Buffer
	output := w
	w = &buffer
	defer func() { c.mu.Unlock(); _, _ = output.Write(buffer.Bytes()) }()
	fmt.Fprintln(w, "# HELP gochat_http_requests_total Completed HTTP requests.\n# TYPE gochat_http_requests_total counter\n# HELP gochat_http_request_duration_seconds HTTP request duration.\n# TYPE gochat_http_request_duration_seconds histogram")
	keys := make([]httpKey, 0, len(c.http))
	for k := range c.http {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i]) < fmt.Sprint(keys[j]) })
	for _, k := range keys {
		o := c.http[k]
		labels := fmt.Sprintf("method=%q,route=%q,status=%q", k.Method, k.Route, fmt.Sprint(k.Status))
		fmt.Fprintf(w, "gochat_http_requests_total{%s} %d\n", labels, o.Count)
		for i, b := range bounds {
			fmt.Fprintf(w, "gochat_http_request_duration_seconds_bucket{%s,le=%q} %d\n", labels, fmt.Sprint(b), o.Buckets[i])
		}
		fmt.Fprintf(w, "gochat_http_request_duration_seconds_bucket{%s,le=\"+Inf\"} %d\ngochat_http_request_duration_seconds_sum{%s} %g\ngochat_http_request_duration_seconds_count{%s} %d\n", labels, o.Count, labels, o.Sum, labels, o.Count)
	}
	fmt.Fprintln(w, "# HELP gochat_auth_total Authentication outcomes.\n# TYPE gochat_auth_total counter")
	for _, t := range []string{"http", "ws"} {
		for _, result := range []string{"success", "failure"} {
			fmt.Fprintf(w, "gochat_auth_total{transport=%q,result=%q} %d\n", t, result, c.auth[t+":"+result])
		}
	}
	fmt.Fprintf(w, "# HELP gochat_upload_failures_total Failed uploads.\n# TYPE gochat_upload_failures_total counter\ngochat_upload_failures_total %d\n# HELP gochat_ws_connections Active WebSocket connections.\n# TYPE gochat_ws_connections gauge\ngochat_ws_connections %d\n# HELP gochat_ws_users Connected unique users.\n# TYPE gochat_ws_users gauge\ngochat_ws_users %d\n", c.uploadFailures, c.connections, len(c.users))
	fmt.Fprintln(w, "# HELP gochat_messages_total Message persistence outcomes.\n# TYPE gochat_messages_total counter")
	for _, result := range []string{"persisted", "persistence_failure"} {
		fmt.Fprintf(w, "gochat_messages_total{result=%q} %d\n", result, c.messages[result])
	}
	if c.redisStats != nil {
		s := c.redisStats()
		fmt.Fprintf(w, "# HELP gochat_redis_connections Redis pool connections.\n# TYPE gochat_redis_connections gauge\ngochat_redis_connections{state=\"total\"} %d\ngochat_redis_connections{state=\"idle\"} %d\n", s.Total, s.Idle)
		fmt.Fprintf(w, "# HELP gochat_redis_pool_total Redis connection pool outcomes.\n# TYPE gochat_redis_pool_total counter\ngochat_redis_pool_total{result=\"hit\"} %d\ngochat_redis_pool_total{result=\"miss\"} %d\ngochat_redis_pool_total{result=\"timeout\"} %d\n", s.Hits, s.Misses, s.Timeouts)
	}
	if c.dbStats != nil {
		s := c.dbStats()
		for _, g := range []struct {
			name  string
			value int
		}{{"open", s.OpenConnections}, {"in_use", s.InUse}, {"idle", s.Idle}, {"max_open", s.MaxOpenConnections}} {
			fmt.Fprintf(w, "# HELP gochat_db_connections_%s Database pool connections.\n# TYPE gochat_db_connections_%s gauge\ngochat_db_connections_%s %d\n", g.name, g.name, g.name, g.value)
		}
		fmt.Fprintf(w, "# HELP gochat_db_wait_total Database pool waits.\n# TYPE gochat_db_wait_total counter\ngochat_db_wait_total %d\n# HELP gochat_db_wait_duration_seconds_total Time waiting for database connections.\n# TYPE gochat_db_wait_duration_seconds_total counter\ngochat_db_wait_duration_seconds_total %g\n", s.WaitCount, s.WaitDuration.Seconds())
	}
}
