package iplist

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenLocalBounds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(path, []byte("1.2.3.4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenLocal(path, 3); err == nil {
		t.Fatal("oversized local file accepted")
	}
	r, err := OpenLocal(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
}

func TestDownloadRequiresHTTPSAndValidRetryCount(t *testing.T) {
	_, err := Download(context.Background(), "http://example.invalid/list", 100, time.Second, 0)
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("error = %v", err)
	}
	if _, err := Download(context.Background(), "https://example.invalid/list", 100, time.Second, 11); err == nil {
		t.Fatal("excess retry count accepted")
	}
}
