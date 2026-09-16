package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIEndToEnd(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.txt")
	binary := filepath.Join(dir, "candidate.bin")
	manifest := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(source, []byte("255.255.255.255\n1.2.3.4\n1.2.3.4\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if err := run([]string{"build", "--input", source, "--output", binary, "--manifest", manifest}, &out, &stderr); err != nil {
		t.Fatalf("build: %v (%s)", err, stderr.String())
	}
	out.Reset()
	if err := run([]string{"verify", "--input", binary, "--json"}, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"count": 2`) {
		t.Fatalf("verify output = %s", out.String())
	}
	out.Reset()
	if err := run([]string{"lookup", "--input", binary, "1.2.3.4", "1.2.3.5", "255.255.255.255"}, &out, &stderr); err != nil {
		t.Fatal(err)
	}
	want := "1.2.3.4\ttrue\n1.2.3.5\tfalse\n255.255.255.255\ttrue\n"
	if out.String() != want {
		t.Fatalf("lookup = %q", out.String())
	}
	if _, err := os.Stat(manifest); err != nil {
		t.Fatal(err)
	}
}

func TestChangeThresholds(t *testing.T) {
	if err := checkChange(100, 151, 50, 50); err == nil {
		t.Fatal("large increase accepted")
	}
	if err := checkChange(100, 49, 50, 50); err == nil {
		t.Fatal("large decrease accepted")
	}
	if err := checkChange(100, 150, 50, 50); err != nil {
		t.Fatal(err)
	}
}

func TestBuildRejectsEmptySourceWithoutCandidate(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "empty.txt")
	candidate := filepath.Join(dir, "candidate.bin")
	manifest := filepath.Join(dir, "candidate.manifest.json")
	for _, contents := range []string{"", "# upstream temporarily returned no addresses\n"} {
		if err := os.WriteFile(source, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		var out, stderr bytes.Buffer
		err := run([]string{"build", "--input", source, "--output", candidate,
			"--manifest", manifest}, &out, &stderr)
		if err == nil || !strings.Contains(err.Error(), "no IPv4 addresses") {
			t.Fatalf("empty source %q: error = %v", contents, err)
		}
		if _, err := os.Stat(candidate); !os.IsNotExist(err) {
			t.Fatalf("empty source created candidate: %v", err)
		}
		if _, err := os.Stat(manifest); !os.IsNotExist(err) {
			t.Fatalf("empty source created manifest: %v", err)
		}
	}
}
