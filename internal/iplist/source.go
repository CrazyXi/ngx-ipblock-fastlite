package iplist

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"
)

// OpenLocal opens a bounded local source after checking its reported size.
func OpenLocal(path string, maxBytes int64) (io.ReadCloser, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if st.Size() > maxBytes {
		_ = f.Close()
		return nil, fmt.Errorf("input size %d exceeds limit %d", st.Size(), maxBytes)
	}
	return f, nil
}

// Download performs a certificate-verified HTTPS GET with a timeout and bounded retries.
func Download(ctx context.Context, rawURL string, maxBytes int64, timeout time.Duration, retries int) (io.ReadCloser, error) {
	if retries < 0 || retries > 10 {
		return nil, fmt.Errorf("retries must be between 0 and 10")
	}
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" {
				return fmt.Errorf("redirect target must use HTTPS")
			}
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, err
		}
		if req.URL.Scheme != "https" {
			return nil, fmt.Errorf("source URL must use HTTPS")
		}
		req.Header.Set("User-Agent", "ipblacklist-builder/1")
		resp, err := client.Do(req)
		if err == nil {
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				if value := resp.Header.Get("Content-Length"); value != "" {
					if n, parseErr := strconv.ParseInt(value, 10, 64); parseErr == nil && n > maxBytes {
						_ = resp.Body.Close()
						return nil, fmt.Errorf("remote content length %d exceeds limit %d", n, maxBytes)
					}
				}
				return resp.Body, nil
			}
			lastErr = fmt.Errorf("HTTP status %s", resp.Status)
			_ = resp.Body.Close()
			if resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
				return nil, lastErr
			}
		} else {
			lastErr = err
		}
		if attempt < retries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt+1) * 250 * time.Millisecond):
			}
		}
	}
	return nil, fmt.Errorf("download failed after %d attempt(s): %w", retries+1, lastErr)
}
