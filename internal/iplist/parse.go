package iplist

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxLineBytes = 1 << 20

var sourceTimePattern = regexp.MustCompile(`\d{4}-\d{2}-\d{2}(?:[T ]\d{2}:\d{2}:\d{2}(?:Z| ?UTC|[+-]\d{2}:?\d{2})?)?`)

// ParseOptions bounds the amount of untrusted input accepted by the builder.
type ParseOptions struct {
	MaxInputBytes int64
	MaxIPs        int
}

// ParseStats describes the source text before and after normalization.
type ParseStats struct {
	InputBytes      int64
	RawLines        int
	ValidLines      int
	UniqueIPs       int
	Duplicates      int
	SourceSHA256    string
	SourceUpdatedAt string
}

// ParseResult is a numerically sorted, duplicate-free IPv4 list.
type ParseResult struct {
	IPs   []uint32
	Stats ParseStats
}

type countingReader struct {
	r io.Reader
	n int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	r.n += int64(n)
	return n, err
}

// Parse reads the strict line-oriented source format described in FORMAT.md.
func Parse(r io.Reader, opts ParseOptions) (ParseResult, error) {
	if opts.MaxInputBytes <= 0 {
		return ParseResult{}, fmt.Errorf("max input bytes must be positive")
	}
	if opts.MaxIPs <= 0 {
		return ParseResult{}, fmt.Errorf("max IP count must be positive")
	}

	h := sha256.New()
	limited := io.LimitReader(r, opts.MaxInputBytes+1)
	counted := &countingReader{r: limited}
	scanner := bufio.NewScanner(io.TeeReader(counted, h))
	scanner.Buffer(make([]byte, 64*1024), maxLineBytes)

	seen := make(map[uint32]struct{})
	stats := ParseStats{}
	for scanner.Scan() {
		stats.RawLines++
		line := scanner.Text()
		if stats.RawLines == 1 {
			line = strings.TrimPrefix(line, "\ufeff")
		}
		if stats.SourceUpdatedAt == "" {
			stats.SourceUpdatedAt = sourceTimestamp(line)
		}
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		ip, err := ParseIPv4(line)
		if err != nil {
			return ParseResult{}, fmt.Errorf("line %d: %w", stats.RawLines, err)
		}
		stats.ValidLines++
		if _, ok := seen[ip]; ok {
			stats.Duplicates++
			continue
		}
		if len(seen) >= opts.MaxIPs {
			return ParseResult{}, fmt.Errorf("unique IP count exceeds limit %d", opts.MaxIPs)
		}
		seen[ip] = struct{}{}
	}
	if err := scanner.Err(); err != nil {
		return ParseResult{}, fmt.Errorf("read source: %w", err)
	}
	stats.InputBytes = counted.n
	stats.SourceSHA256 = hex.EncodeToString(h.Sum(nil))
	if counted.n > opts.MaxInputBytes {
		return ParseResult{}, fmt.Errorf("input size %d exceeds limit %d", counted.n, opts.MaxInputBytes)
	}
	if len(seen) == 0 {
		return ParseResult{}, fmt.Errorf("source contains no IPv4 addresses")
	}

	ips := make([]uint32, 0, len(seen))
	for ip := range seen {
		ips = append(ips, ip)
	}
	sort.Slice(ips, func(i, j int) bool { return ips[i] < ips[j] })
	stats.UniqueIPs = len(ips)
	return ParseResult{IPs: ips, Stats: stats}, nil
}

// ParseIPv4 accepts only canonical dotted-decimal IPv4 (no leading zeroes).
func ParseIPv4(s string) (uint32, error) {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return 0, fmt.Errorf("invalid IPv4 %q", s)
	}
	var result uint32
	for _, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return 0, fmt.Errorf("invalid IPv4 %q", s)
		}
		for i := range part {
			if part[i] < '0' || part[i] > '9' {
				return 0, fmt.Errorf("invalid IPv4 %q", s)
			}
		}
		n, err := strconv.Atoi(part)
		if err != nil || n > 255 {
			return 0, fmt.Errorf("invalid IPv4 %q", s)
		}
		result = result<<8 | uint32(n)
	}
	return result, nil
}

// IPv4String renders the canonical dotted-decimal form.
func IPv4String(ip uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip))
}

func sourceTimestamp(line string) string {
	lower := strings.ToLower(line)
	if !strings.Contains(lower, "updated") && !strings.Contains(lower, "modified") && !strings.Contains(lower, "generated") {
		return ""
	}
	for _, candidate := range sourceTimePattern.FindAllString(line, -1) {
		candidate = strings.Replace(candidate, " UTC", "Z", 1)
		if len(candidate) > 10 && candidate[10] == ' ' && strings.HasSuffix(candidate, "Z") {
			candidate = candidate[:10] + "T" + candidate[11:]
		}
		layouts := []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"}
		for _, layout := range layouts {
			if parsed, err := time.Parse(layout, candidate); err == nil {
				return parsed.UTC().Format(time.RFC3339)
			}
		}
	}
	return ""
}
