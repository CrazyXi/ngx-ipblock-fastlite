package main

import (
	"errors"
	"io"
	"testing"
	"time"
)

func TestPercentileAndMilliseconds(t *testing.T) {
	values := []time.Duration{time.Millisecond, 2 * time.Millisecond, 10 * time.Millisecond}
	if got := percentile(values, 0.50); got != 2*time.Millisecond {
		t.Fatalf("p50 = %v", got)
	}
	if got := milliseconds(percentile(values, 0.99)); got != 10 {
		t.Fatalf("p99 = %v", got)
	}
}

func TestExpectedClose(t *testing.T) {
	if !isExpectedClose(io.EOF) || !isExpectedClose(errors.New("connection reset by peer")) {
		t.Fatal("expected close was not recognized")
	}
	if isExpectedClose(errors.New("connection refused")) {
		t.Fatal("unrelated network error was accepted")
	}
}

func TestParseClientIPs(t *testing.T) {
	for _, input := range []string{"bad", "192.0.2.1,", "192.0.2.1,,::1"} {
		if _, err := parseClientIPs("", input); err == nil {
			t.Fatalf("accepted %q", input)
		}
	}
	if _, err := parseClientIPs("192.0.2.1", "::1"); err == nil {
		t.Fatal("accepted conflicting flags")
	}
	if got, err := parseClientIPs("invalid-for-functional-test", ""); err != nil || got != nil {
		t.Fatal(got, err)
	}
	got, err := parseClientIPs("", "192.0.2.1, ::1 ")
	if err != nil || len(got) != 2 || got[0] != "192.0.2.1" || got[1] != "::1" {
		t.Fatal(got, err)
	}
}
