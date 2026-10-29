// Command loadgen is a closed-loop HTTP load generator for authorization
// endpoints. Each of -c workers sends one request at a time over a kept-alive
// connection for -d, after a -warmup that is not recorded.
//
// Requests pick a random role i in [1, n] and a random user of that role;
// 80% ask for /books/bookI (allowed), 20% for another role's book (denied),
// so no two consecutive requests are the same.
//
//	loadgen -target thin    -url http://127.0.0.1:9000/check
//	loadgen -target opa     -url http://127.0.0.1:8181/v1/data/authz/allow
//	loadgen -target speedle -url http://127.0.0.1:6734/authz-check/v1/is-allowed
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func body(target string, user, res string) []byte {
	switch target {
	case "opa":
		return []byte(fmt.Sprintf(`{"input":{"user":%q,"action":"read","resource":%q}}`, user, res))
	case "speedle":
		return []byte(fmt.Sprintf(`{"subject":{"principals":[{"type":"user","name":%q}]},"serviceName":"app","resource":%q,"action":"read"}`, user, res))
	default:
		return []byte(fmt.Sprintf(`{"user":%q,"action":"read","resource":%q}`, user, res))
	}
}

// allowed extracts the decision from each server's response format.
func allowed(target string, b []byte) (bool, error) {
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		return false, err
	}
	key := "allowed"
	if target == "opa" {
		key = "result"
	}
	ok, isBool := v[key].(bool)
	if !isBool {
		return false, fmt.Errorf("no boolean %q in %s", key, b)
	}
	return ok, nil
}

func pct(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	i := int(p / 100 * float64(len(sorted)-1))
	return sorted[i]
}

func main() {
	target := flag.String("target", "thin", "thin | opa | speedle")
	url := flag.String("url", "http://127.0.0.1:9000/check", "endpoint")
	kind := flag.String("kind", "plain", "plain | manyusers")
	srvPID := flag.Int("srv-pid", 0, "server PID for Linux CPU measurement")
	n := flag.Int("n", 10000, "number of roles in the policy set")
	c := flag.Int("c", 16, "concurrent workers")
	d := flag.Duration("d", 20*time.Second, "measured duration")
	warm := flag.Duration("warmup", 3*time.Second, "unrecorded warmup")
	label := flag.String("label", "", "row label")
	flag.Parse()
	if *n < 2 || *c < 1 || *d <= 0 || *warm < 0 || (*kind != "plain" && *kind != "manyusers") {
		fmt.Fprintln(os.Stderr, "invalid parameters")
		os.Exit(2)
	}
	ticks := clockTicks()

	tr := &http.Transport{MaxIdleConns: *c * 2, MaxIdleConnsPerHost: *c * 2, IdleConnTimeout: time.Minute}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	var (
		wg        sync.WaitGroup
		recording atomic.Bool
		stop      atomic.Bool
		errs      atomic.Int64
		wrong     atomic.Int64
		mu        sync.Mutex
		all       []time.Duration
	)
	for w := 0; w < *c; w++ {
		wg.Add(1)
		go func(seed uint64) {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(seed, seed*7+1))
			lat := make([]time.Duration, 0, 1<<16)
			for !stop.Load() {
				i := rng.IntN(*n) + 1
				user := fmt.Sprintf("user%d-%d", i, rng.IntN(10)+1)
				want := rng.IntN(10) < 8
				res := fmt.Sprintf("/books/book%d", i)
				if !want {
					res = fmt.Sprintf("/books/book%d", i%*n+1)
				}
				if *kind == "manyusers" {
					r := (i-1)%10 + 1
					if !want {
						r = r%10 + 1
					}
					user = fmt.Sprintf("user%d", i)
					res = fmt.Sprintf("/books/book%d-%d", r, rng.IntN(10)+1)
				}
				measured := recording.Load()
				t0 := time.Now()
				resp, err := client.Post(*url, "application/json", bytes.NewReader(body(*target, user, res)))
				var got bool
				if err == nil {
					b, readErr := io.ReadAll(resp.Body)
					resp.Body.Close()
					if readErr != nil {
						err = readErr
					} else if resp.StatusCode != 200 {
						err = fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
					} else {
						got, err = allowed(*target, b)
					}
				}
				dt := time.Since(t0)
				if !measured || !recording.Load() {
					continue
				}
				if err != nil {
					if errs.Add(1) == 1 {
						fmt.Fprintln(os.Stderr, "first error:", err)
					}
					continue
				}
				if got != want {
					wrong.Add(1)
				}
				lat = append(lat, dt)
			}
			mu.Lock()
			all = append(all, lat...)
			mu.Unlock()
		}(uint64(w + 1))
	}
	time.Sleep(*warm)
	srv0, gen0 := cpuTime(*srvPID), cpuTime(os.Getpid())
	start := time.Now()
	recording.Store(true)
	time.Sleep(*d)
	recording.Store(false)
	elapsed := time.Since(start)
	srv1, gen1 := cpuTime(*srvPID), cpuTime(os.Getpid())
	stop.Store(true)
	wg.Wait()

	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
	us := func(x time.Duration) string { return fmt.Sprintf("%.0f", float64(x.Nanoseconds())/1000) }
	var maxLatency time.Duration
	if len(all) > 0 {
		maxLatency = all[len(all)-1]
	}
	fmt.Printf("| %s | %d | %.0f | %s | %s | %s | %s | %s | %d | %d | %s | %s |\n", *label, *c,
		float64(len(all))/elapsed.Seconds(), us(pct(all, 50)), us(pct(all, 90)), us(pct(all, 99)),
		us(pct(all, 99.9)), us(maxLatency), errs.Load(), wrong.Load(), cores(srv0, srv1, ticks, elapsed), cores(gen0, gen1, ticks, elapsed))
	if len(all) == 0 || errs.Load() > 0 || wrong.Load() > 0 {
		os.Exit(1)
	}
}

func clockTicks() float64 {
	if runtime.GOOS != "linux" {
		return 0
	}
	b, err := exec.Command("getconf", "CLK_TCK").Output()
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
	return v
}

// /proc stat comm may contain spaces and ')': split after the final ')'.
func cpuTime(pid int) int64 {
	if runtime.GOOS != "linux" || pid <= 0 {
		return -1
	}
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return -1
	}
	end := strings.LastIndexByte(string(b), ')')
	if end < 0 {
		return -1
	}
	fields := strings.Fields(string(b[end+1:]))
	if len(fields) < 13 {
		return -1
	}
	u, e1 := strconv.ParseInt(fields[11], 10, 64)
	s, e2 := strconv.ParseInt(fields[12], 10, 64)
	if e1 != nil || e2 != nil {
		return -1
	}
	return u + s
}
func cores(a, b int64, ticks float64, elapsed time.Duration) string {
	if a < 0 || b < a || ticks <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.3f", float64(b-a)/ticks/elapsed.Seconds())
}
