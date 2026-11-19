// Local-only 90% miss / 10% listed-source benchmark. One connection per attempt.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func attempt(source net.IP) string {
	d := net.Dialer{Timeout: 200 * time.Millisecond, LocalAddr: &net.TCPAddr{IP: source}}
	c, err := d.Dial("tcp4", "127.0.0.1:18080")
	if err != nil {
		if e, ok := err.(net.Error); ok && e.Timeout() {
			return "connect_timeout"
		}
		return "dial_error"
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err = io.WriteString(c, "GET / HTTP/1.1\r\nHost: bench\r\nConnection: close\r\n\r\n"); err != nil {
		return "write_error"
	}
	b := bufio.NewReader(c)
	if _, err = b.Peek(1); err == io.EOF {
		return "empty_close"
	} else if err != nil {
		return "read_error"
	}
	resp, err := http.ReadResponse(b, nil)
	if err != nil {
		return "invalid_response"
	}
	_, err = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if err != nil {
		return "truncated"
	}
	return strconv.Itoa(resp.StatusCode)
}

func main() {
	if len(os.Args) != 3 {
		panic("mixedclient SAMPLE_COUNT COMMA_SEPARATED_HIT_SOURCE_IPS")
	}
	n, err := strconv.Atoi(os.Args[1])
	if err != nil || n <= 0 || n%320 != 0 {
		panic("count must be a positive multiple of 320")
	}
	var hits []net.IP
	for _, s := range strings.Split(os.Args[2], ",") {
		ip := net.ParseIP(s).To4()
		if ip == nil {
			panic("bad source")
		}
		hits = append(hits, ip)
	}
	type counts map[string]int
	done := make(chan counts, 32)
	start := time.Now()
	for w := 0; w < 32; w++ {
		go func(w int) {
			out := counts{}
			for j := 0; j < n/32; j++ {
				src := net.IPv4(127, 0, 0, byte(1+(j+w)%16))
				group := "miss/"
				if (j+w)%10 == 0 {
					src = hits[(j/10+w)%len(hits)]
					group = "listed/"
				}
				out[group+attempt(src)]++
			}
			done <- out
		}(w)
	}
	all := counts{}
	for w := 0; w < 32; w++ {
		for k, v := range <-done {
			all[k] += v
		}
	}
	elapsed := time.Since(start).Seconds()
	b, err := json.Marshal(map[string]any{"attempts": n, "elapsed_s": elapsed, "rps": float64(n) / elapsed, "counts": all})
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
}
