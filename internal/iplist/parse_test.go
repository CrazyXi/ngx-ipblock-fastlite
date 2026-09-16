package iplist

import (
	"strings"
	"testing"
)

func TestParseNormalizesAndSorts(t *testing.T) {
	source := "\ufeff# Updated 2026-09-15 06:19:02 UTC\r\n255.255.255.255 # end\r\n1.2.3.4\r\n128.0.0.0\r\n1.2.3.4\r\n0.0.0.0\r\n"
	got, err := Parse(strings.NewReader(source), ParseOptions{MaxInputBytes: 1024, MaxIPs: 10})
	if err != nil {
		t.Fatal(err)
	}
	want := []uint32{0, 0x01020304, 0x80000000, 0xffffffff}
	if len(got.IPs) != len(want) {
		t.Fatalf("IPs = %#v", got.IPs)
	}
	for i := range want {
		if got.IPs[i] != want[i] {
			t.Fatalf("IPs[%d] = %x, want %x", i, got.IPs[i], want[i])
		}
	}
	if got.Stats.Duplicates != 1 || got.Stats.ValidLines != 5 || got.Stats.SourceUpdatedAt != "2026-09-15T06:19:02Z" {
		t.Fatalf("stats = %+v", got.Stats)
	}
}

func TestParseIPv4Strict(t *testing.T) {
	valid := map[string]uint32{
		"0.0.0.0": 0, "127.255.255.255": 0x7fffffff,
		"128.0.0.0": 0x80000000, "255.255.255.255": 0xffffffff,
	}
	for raw, want := range valid {
		got, err := ParseIPv4(raw)
		if err != nil || got != want {
			t.Errorf("ParseIPv4(%q) = %x, %v", raw, got, err)
		}
	}
	invalid := []string{"", "1.2.3", "1.2.3.4.5", "01.2.3.4", "1.2.3.256", "1.2.3.-1", "0x1.2.3.4", "1.2.3.4:80", "1.2.3.4 5.6.7.8", "1.2.3.0/24", "::1", " 1.2.3.4"}
	for _, raw := range invalid {
		if _, err := ParseIPv4(raw); err == nil {
			t.Errorf("ParseIPv4(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestParseRejectsInvalidLineWithNumber(t *testing.T) {
	_, err := Parse(strings.NewReader("1.2.3.4\n1.2.3.0/24\n"), ParseOptions{MaxInputBytes: 1024, MaxIPs: 10})
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseRejectsEmptyAndLimits(t *testing.T) {
	if _, err := Parse(strings.NewReader("# only comments\n"), ParseOptions{MaxInputBytes: 100, MaxIPs: 10}); err == nil {
		t.Fatal("empty source accepted")
	}
	if _, err := Parse(strings.NewReader("1.2.3.4\n"), ParseOptions{MaxInputBytes: 3, MaxIPs: 10}); err == nil {
		t.Fatal("oversized source accepted")
	}
	if _, err := Parse(strings.NewReader("1.2.3.4\n1.2.3.5\n"), ParseOptions{MaxInputBytes: 100, MaxIPs: 1}); err == nil {
		t.Fatal("too many addresses accepted")
	}
}
