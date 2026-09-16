package auth

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestListenerCallback(t *testing.T) {
	l, err := StartListener()
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	uri := l.RedirectURI()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		resp, err := http.Get(uri + "?code=abc%2F1&state=xyz")
		if err != nil {
			errCh <- err
			return
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			errCh <- errStatus(resp.StatusCode)
			return
		}
		if len(body) == 0 {
			errCh <- errStatus(0)
			return
		}
		errCh <- nil
	}()

	cb, err := l.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cb.Code != "abc/1" && cb.Code != "abc%2F1" {
		// net/http Query unescapes; expect abc/1
		if cb.Code != "abc/1" {
			t.Fatalf("code = %q", cb.Code)
		}
	}
	if cb.State != "xyz" {
		t.Fatalf("state = %q", cb.State)
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func errStatus(n int) error {
	return &statusError{n}
}

type statusError struct{ n int }

func (e *statusError) Error() string { return http.StatusText(e.n) }
