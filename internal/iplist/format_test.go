package iplist

import (
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteVerifyReadAndBoundaries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "list.bin")
	ips := []uint32{0, 1, 0x7fffffff, 0x80000000, 0xfffffffe, 0xffffffff}
	built := time.Unix(1_789_444_800, 0).UTC()
	meta, err := WriteBinary(path, ips, built)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Count != uint64(len(ips)) || meta.FileSize != int64(HeaderSize)+int64(len(ips))*4 {
		t.Fatalf("metadata = %+v", meta)
	}
	got, verified, err := ReadAll(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	if verified.DataSHA256 != meta.DataSHA256 || len(got) != len(ips) {
		t.Fatalf("verify/read mismatch: %+v %#v", verified, got)
	}
	for i := range ips {
		if got[i] != ips[i] {
			t.Fatalf("got[%d]=%x want=%x", i, got[i], ips[i])
		}
	}
}

func TestVerifyRejectsCorruptions(t *testing.T) {
	base := makeValidBytes(t)
	tests := map[string]func([]byte) []byte{
		"magic":         func(b []byte) []byte { b[0] ^= 1; return b },
		"version":       func(b []byte) []byte { binary.LittleEndian.PutUint32(b[8:12], 2); return b },
		"header length": func(b []byte) []byte { binary.LittleEndian.PutUint32(b[12:16], 63); return b },
		"count":         func(b []byte) []byte { binary.LittleEndian.PutUint64(b[16:24], 99); return b },
		"digest":        func(b []byte) []byte { b[32] ^= 1; return b },
		"truncated":     func(b []byte) []byte { return b[:len(b)-1] },
		"unsorted": func(b []byte) []byte {
			binary.LittleEndian.PutUint32(b[64:68], 3)
			binary.LittleEndian.PutUint32(b[68:72], 2)
			fixDigest(b)
			return b
		},
		"duplicate": func(b []byte) []byte {
			binary.LittleEndian.PutUint32(b[68:72], 1)
			fixDigest(b)
			return b
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			data := append([]byte(nil), base...)
			path := filepath.Join(t.TempDir(), "bad.bin")
			if err := os.WriteFile(path, mutate(data), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := VerifyFile(path, 10); err == nil {
				t.Fatal("corrupt file accepted")
			}
		})
	}
}

func TestWriteRejectsEmptyUnsortedAndExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "list.bin")
	if _, err := WriteBinary(path, nil, time.Now()); err == nil {
		t.Fatal("empty list accepted")
	}
	if _, err := WriteBinary(path, []uint32{2, 1}, time.Now()); err == nil {
		t.Fatal("unsorted list accepted")
	}
	if _, err := WriteBinary(path, []uint32{1}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteBinary(path, []uint32{1}, time.Now()); err == nil {
		t.Fatal("existing output replaced")
	}
}

func makeValidBytes(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "valid.bin")
	if _, err := WriteBinary(path, []uint32{1, 2, 3}, time.Unix(1000, 0)); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func fixDigest(b []byte) {
	sum := sha256.Sum256(b[64:])
	copy(b[32:64], sum[:])
}

func TestIPv4StringRoundTrip(t *testing.T) {
	for _, ip := range []uint32{0, 0x7fffffff, 0x80000000, 0xffffffff} {
		raw := IPv4String(ip)
		got, err := ParseIPv4(raw)
		if err != nil || got != ip {
			t.Fatalf("round-trip %x via %s: %x %v", ip, raw, got, err)
		}
	}
	if !strings.Contains(IPv4String(0x01020304), "1.2.3.4") {
		t.Fatal("unexpected render")
	}
}
