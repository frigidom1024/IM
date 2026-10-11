package api

import (
	"context"
	"time"
)

type DependencyCheck func(context.Context) error
type RouterOption func(*routerOptions)
type routerOptions struct {
	timeout time.Duration
	checks  map[string]DependencyCheck
}

// WithDependencyChecks adds bounded dependency checks. A nil check explicitly
// marks an optional dependency disabled. Existing NewRouter callers remain valid.
func WithDependencyChecks(timeout time.Duration, checks map[string]DependencyCheck) RouterOption {
	return func(o *routerOptions) {
		if timeout <= 0 {
			timeout = 2 * time.Second
		}
		o.timeout = timeout
		o.checks = make(map[string]DependencyCheck, len(checks))
		for name, check := range checks {
			o.checks[name] = check
		}
	}
}
func (o *routerOptions) dependencies(parent context.Context) (map[string]string, bool) {
	ctx, cancel := context.WithTimeout(parent, o.timeout)
	defer cancel()
	states := make(map[string]string, len(o.checks))
	pending := 0
	type result struct {
		name string
		err  error
	}
	results := make(chan result, len(o.checks))
	for name, check := range o.checks {
		if check == nil {
			states[name] = "disabled"
			continue
		}
		states[name] = "unavailable"
		pending++
		go func(name string, check DependencyCheck) { results <- result{name, check(ctx)} }(name, check)
	}
	ready := true
	for pending > 0 {
		select {
		case r := <-results:
			pending--
			if r.err == nil {
				states[r.name] = "ok"
			} else {
				ready = false
			}
		case <-ctx.Done():
			return states, false
		}
	}
	return states, ready
}
