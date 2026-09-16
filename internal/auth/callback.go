package auth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

const (
	callbackHTML = `<!doctype html><html><head><meta charset="utf-8"><title>Signed in</title></head>
<body style="font-family:system-ui;padding:2rem">
<h1>Signed in</h1><p>You can close this tab and return to the terminal.</p>
</body></html>`
	defaultWait = 5 * time.Minute
)

// Callback is the OAuth redirect payload.
type Callback struct {
	Code  string
	State string
}

// Listener is a one-shot 127.0.0.1 OAuth redirect server.
type Listener struct {
	ln   net.Listener
	port int
}

// StartListener binds 127.0.0.1:0.
func StartListener() (*Listener, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("callback listen: %w", err)
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok || addr.Port == 0 {
		_ = ln.Close()
		return nil, fmt.Errorf("callback listen: no port")
	}
	return &Listener{ln: ln, port: addr.Port}, nil
}

// Close stops the listener.
func (l *Listener) Close() error {
	if l == nil || l.ln == nil {
		return nil
	}
	return l.ln.Close()
}

// RedirectURI is http://127.0.0.1:{port}/callback.
func (l *Listener) RedirectURI() string {
	return fmt.Sprintf("http://127.0.0.1:%d/callback", l.port)
}

// Wait blocks until a valid callback or ctx/timeout.
func (l *Listener) Wait(ctx context.Context) (Callback, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultWait)
		defer cancel()
	}

	type result struct {
		cb  Callback
		err error
	}
	ch := make(chan result, 1)
	srv := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/callback" {
				http.NotFound(w, r)
				return
			}
			q := r.URL.Query()
			code := q.Get("code")
			state := q.Get("state")
			if code == "" || state == "" {
				http.Error(w, "missing code or state", http.StatusBadRequest)
				ch <- result{err: fmt.Errorf("callback: missing code or state")}
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(callbackHTML))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			ch <- result{cb: Callback{Code: code, State: state}}
		}),
	}

	go func() {
		err := srv.Serve(l.ln)
		if err != nil && err != http.ErrServerClosed {
			select {
			case ch <- result{err: err}:
			default:
			}
		}
	}()

	select {
	case <-ctx.Done():
		_ = srv.Shutdown(context.Background())
		return Callback{}, fmt.Errorf("timed out waiting for browser callback")
	case r := <-ch:
		ctxShut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctxShut)
		return r.cb, r.err
	}
}
