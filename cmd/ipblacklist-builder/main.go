package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"ipblacklist/internal/iplist"
)

const (
	defaultMaxBytes = int64(100 * 1024 * 1024)
	defaultMaxIPs   = uint64(2_000_000)
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		usage(stderr)
		return errors.New("a subcommand is required")
	}
	switch args[0] {
	case "build":
		return runBuild(args[1:], stdout, stderr)
	case "verify":
		return runVerify(args[1:], stdout, stderr)
	case "lookup":
		return runLookup(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	default:
		usage(stderr)
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "Usage: ipblacklist-builder <build|verify|lookup> [options]")
}

func runBuild(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "", "local TXT input path")
	url := fs.String("url", "", "HTTPS TXT source URL")
	output := fs.String("output", "", "new candidate binary path")
	manifestPath := fs.String("manifest", "", "new JSON manifest path (default: <output>.manifest.json)")
	maxBytes := fs.Int64("max-input-bytes", defaultMaxBytes, "maximum source bytes")
	maxIPs := fs.Uint64("max-ips", defaultMaxIPs, "maximum unique IPv4 count")
	timeout := fs.Duration("timeout", 30*time.Second, "HTTP request timeout")
	retries := fs.Int("retries", 2, "additional HTTP attempts")
	previous := fs.String("previous", "", "previous binary for change threshold checks")
	maxIncrease := fs.Float64("max-increase-percent", -1, "maximum increase versus previous; negative disables")
	maxDecrease := fs.Float64("max-decrease-percent", -1, "maximum decrease versus previous; negative disables")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if (*input == "") == (*url == "") {
		return errors.New("exactly one of --input or --url is required")
	}
	if *output == "" {
		return errors.New("--output is required")
	}
	if *maxIPs == 0 || *maxIPs > uint64(^uint(0)>>1) {
		return errors.New("--max-ips is invalid for this platform")
	}
	if *manifestPath == "" {
		*manifestPath = *output + ".manifest.json"
	}

	var source io.ReadCloser
	var sourceName string
	var err error
	if *input != "" {
		source, err = iplist.OpenLocal(*input, *maxBytes)
		sourceName = *input
	} else {
		source, err = iplist.Download(context.Background(), *url, *maxBytes, *timeout, *retries)
		sourceName = *url
	}
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	parsed, parseErr := iplist.Parse(source, iplist.ParseOptions{MaxInputBytes: *maxBytes, MaxIPs: int(*maxIPs)})
	closeErr := source.Close()
	if parseErr != nil {
		return parseErr
	}
	if closeErr != nil {
		return fmt.Errorf("close source: %w", closeErr)
	}
	if *previous != "" {
		old, err := iplist.VerifyFile(*previous, *maxIPs)
		if err != nil {
			return fmt.Errorf("verify previous binary: %w", err)
		}
		if err := checkChange(old.Count, uint64(len(parsed.IPs)), *maxIncrease, *maxDecrease); err != nil {
			return err
		}
	}

	builtAt := time.Now().UTC().Truncate(time.Second)
	meta, err := iplist.WriteBinary(*output, parsed.IPs, builtAt)
	if err != nil {
		return err
	}
	manifest := iplist.Manifest{
		Format: iplist.Magic, FormatVersion: iplist.Version, Source: sourceName,
		SourceSHA256: parsed.Stats.SourceSHA256, SourceBytes: parsed.Stats.InputBytes,
		SourceLines: parsed.Stats.RawLines, ValidLines: parsed.Stats.ValidLines,
		UniqueIPs: parsed.Stats.UniqueIPs, Duplicates: parsed.Stats.Duplicates,
		SourceUpdatedAt: parsed.Stats.SourceUpdatedAt, BuiltAt: meta.BuiltAt,
		BuiltAtUnix: meta.BuiltAtUnix, DataSHA256: meta.DataSHA256,
		BinarySHA256: meta.BinarySHA256, BinaryBytes: meta.FileSize,
	}
	if err := iplist.WriteManifest(*manifestPath, manifest); err != nil {
		return fmt.Errorf("write manifest (binary remains a valid candidate at %s): %w", *output, err)
	}
	fmt.Fprintf(stdout, "built %s: %d unique IPv4, %d duplicates removed, data_sha256=%s\n", *output, meta.Count, parsed.Stats.Duplicates, meta.DataSHA256)
	fmt.Fprintf(stdout, "manifest %s\n", *manifestPath)
	return nil
}

func checkChange(oldCount, newCount uint64, maxIncrease, maxDecrease float64) error {
	if oldCount == 0 {
		return errors.New("previous list is empty")
	}
	change := (float64(newCount) - float64(oldCount)) * 100 / float64(oldCount)
	if maxIncrease >= 0 && change > maxIncrease {
		return fmt.Errorf("list increase %.2f%% exceeds threshold %.2f%% (%d -> %d)", change, maxIncrease, oldCount, newCount)
	}
	if maxDecrease >= 0 && -change > maxDecrease {
		return fmt.Errorf("list decrease %.2f%% exceeds threshold %.2f%% (%d -> %d)", -change, maxDecrease, oldCount, newCount)
	}
	return nil
}

func runVerify(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "", "binary path")
	maxIPs := fs.Uint64("max-ips", defaultMaxIPs, "maximum allowed IPv4 count")
	asJSON := fs.Bool("json", false, "emit JSON")
	field := fs.String("field", "", "emit one field: data-sha256, binary-sha256, count, built-at-unix")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *input == "" {
		return errors.New("--input is required")
	}
	meta, err := iplist.VerifyFile(*input, *maxIPs)
	if err != nil {
		return err
	}
	if *field != "" {
		switch *field {
		case "data-sha256":
			fmt.Fprintln(stdout, meta.DataSHA256)
		case "binary-sha256":
			fmt.Fprintln(stdout, meta.BinarySHA256)
		case "count":
			fmt.Fprintln(stdout, meta.Count)
		case "built-at-unix":
			fmt.Fprintln(stdout, meta.BuiltAtUnix)
		default:
			return fmt.Errorf("unknown field %q", *field)
		}
		return nil
	}
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(meta)
	}
	fmt.Fprintf(stdout, "valid: version=%d count=%d built_at=%s data_sha256=%s bytes=%d\n", meta.Version, meta.Count, meta.BuiltAt, meta.DataSHA256, meta.FileSize)
	return nil
}

func runLookup(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("lookup", flag.ContinueOnError)
	fs.SetOutput(stderr)
	input := fs.String("input", "", "binary path")
	maxIPs := fs.Uint64("max-ips", defaultMaxIPs, "maximum allowed IPv4 count")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *input == "" || fs.NArg() == 0 {
		return errors.New("--input and at least one IPv4 argument are required")
	}
	ips, _, err := iplist.ReadAll(*input, *maxIPs)
	if err != nil {
		return err
	}
	for _, raw := range fs.Args() {
		ip, err := iplist.ParseIPv4(raw)
		if err != nil {
			return err
		}
		found := sort.Search(len(ips), func(i int) bool { return ips[i] >= ip })
		fmt.Fprintf(stdout, "%s\t%t\n", raw, found < len(ips) && ips[found] == ip)
	}
	return nil
}
