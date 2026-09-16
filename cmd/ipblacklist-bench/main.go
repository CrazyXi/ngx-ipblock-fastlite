package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/netip"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type workerResult struct {
	Latencies []time.Duration
	Statuses  map[int]int
	Errors    int
	Closes    int
}

type report struct {
	URL              string      `json:"url"`
	Host             string      `json:"host"`
	ClientIP         string      `json:"client_ip,omitempty"`
	ClientIPs        []string    `json:"client_ips,omitempty"`
	Concurrency      int         `json:"concurrency"`
	RequestedFor     string      `json:"requested_duration"`
	ElapsedSeconds   float64     `json:"elapsed_seconds"`
	Requests         int         `json:"requests"`
	Errors           int         `json:"errors"`
	Unexpected       int         `json:"unexpected_statuses"`
	RequestsPerSec   float64     `json:"requests_per_second"`
	LatencyMeanMs    float64     `json:"latency_mean_ms"`
	LatencyP50Ms     float64     `json:"latency_p50_ms"`
	LatencyP95Ms     float64     `json:"latency_p95_ms"`
	LatencyP99Ms     float64     `json:"latency_p99_ms"`
	LatencyMaxMs     float64     `json:"latency_max_ms"`
	ExpectedClose    bool        `json:"expected_connection_close"`
	ConnectionCloses int         `json:"connection_closes"`
	ResponseStatuses map[int]int `json:"response_statuses"`
}

func main() {
	url := flag.String("url", "http://127.0.0.1:18081/protected", "target URL")
	host := flag.String("host", "blacklist.test", "HTTP Host header")
	clientIP := flag.String("client-ip", "", "optional CF-Connecting-IP header")
	clientIPList := flag.String("client-ips", "", "comma-separated valid IPs, rotated per client; exclusive with -client-ip")
	duration := flag.Duration("duration", 10*time.Second, "test duration")
	concurrency := flag.Int("concurrency", 32, "parallel clients")
	expected := flag.Int("expected-status", 204, "expected HTTP status")
	expectClose := flag.Bool("expect-close", false, "expect the server to close without an HTTP response (Nginx 444)")
	flag.Parse()
	clientIPs, err := parseClientIPs(*clientIP, *clientIPList)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if *duration <= 0 || *concurrency <= 0 || *concurrency > 4096 {
		fmt.Fprintln(os.Stderr, "duration and concurrency must be positive; concurrency must not exceed 4096")
		os.Exit(2)
	}

	transport := &http.Transport{
		MaxIdleConns:        *concurrency,
		MaxIdleConnsPerHost: *concurrency,
		MaxConnsPerHost:     *concurrency,
		IdleConnTimeout:     30 * time.Second,
	}
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()
	started := time.Now()
	results := make(chan workerResult, *concurrency)
	var wg sync.WaitGroup
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(workerIndex int) {
			defer wg.Done()
			local := workerResult{Statuses: make(map[int]int)}
			ipIndex := 0
			if len(clientIPs) > 0 {
				ipIndex = workerIndex % len(clientIPs)
			}
			for ctx.Err() == nil {
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, *url, nil)
				if err != nil {
					local.Errors++
					break
				}
				req.Host = *host
				if *clientIP != "" {
					req.Header.Set("CF-Connecting-IP", *clientIP)
				} else if len(clientIPs) > 0 {
					req.Header.Set("CF-Connecting-IP", clientIPs[ipIndex])
					ipIndex = (ipIndex + 1) % len(clientIPs)
				}
				requestStarted := time.Now()
				resp, err := client.Do(req)
				latency := time.Since(requestStarted)
				if err != nil {
					if *expectClose && ctx.Err() == nil && isExpectedClose(err) {
						local.Latencies = append(local.Latencies, latency)
						local.Closes++
					} else if ctx.Err() == nil {
						local.Errors++
					}
					continue
				}
				_, copyErr := io.Copy(io.Discard, resp.Body)
				closeErr := resp.Body.Close()
				if copyErr != nil || closeErr != nil {
					local.Errors++
				}
				local.Latencies = append(local.Latencies, latency)
				local.Statuses[resp.StatusCode]++
			}
			results <- local
		}(i)
	}
	wg.Wait()
	close(results)
	transport.CloseIdleConnections()
	elapsed := time.Since(started)

	all := make([]time.Duration, 0)
	statuses := make(map[int]int)
	errors := 0
	closes := 0
	for result := range results {
		all = append(all, result.Latencies...)
		errors += result.Errors
		closes += result.Closes
		for status, count := range result.Statuses {
			statuses[status] += count
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
	unexpected := 0
	for status, count := range statuses {
		if *expectClose || status != *expected {
			unexpected += count
		}
	}
	output := report{
		URL: *url, Host: *host, ClientIP: *clientIP, ClientIPs: clientIPs, Concurrency: *concurrency,
		RequestedFor: duration.String(), ElapsedSeconds: elapsed.Seconds(),
		Requests: len(all), Errors: errors, Unexpected: unexpected,
		ExpectedClose: *expectClose, ConnectionCloses: closes, ResponseStatuses: statuses,
	}
	if len(all) > 0 {
		var total time.Duration
		for _, latency := range all {
			total += latency
		}
		output.RequestsPerSec = float64(len(all)) / elapsed.Seconds()
		output.LatencyMeanMs = milliseconds(total / time.Duration(len(all)))
		output.LatencyP50Ms = milliseconds(percentile(all, 0.50))
		output.LatencyP95Ms = milliseconds(percentile(all, 0.95))
		output.LatencyP99Ms = milliseconds(percentile(all, 0.99))
		output.LatencyMaxMs = milliseconds(all[len(all)-1])
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if errors != 0 || unexpected != 0 {
		os.Exit(1)
	}
}

func parseClientIPs(single, list string) ([]string, error) {
	if list == "" {
		return nil, nil
	}
	if single != "" {
		return nil, errors.New("-client-ip and -client-ips are mutually exclusive")
	}
	ips := strings.Split(list, ",")
	for i, value := range ips {
		value = strings.TrimSpace(value)
		if _, err := netip.ParseAddr(value); err != nil {
			return nil, fmt.Errorf("invalid -client-ips address %q: %w", value, err)
		}
		ips[i] = value
	}
	return ips, nil
}

func isExpectedClose(err error) bool {
	if errors.Is(err, io.EOF) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "connection reset by peer") ||
		strings.Contains(message, "forcibly closed by the remote host")
}

func percentile(values []time.Duration, quantile float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	index := int(math.Ceil(float64(len(values))*quantile)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}

func milliseconds(value time.Duration) float64 {
	return float64(value) / float64(time.Millisecond)
}
