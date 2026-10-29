// Command watchcheck replaces a watched Speedle+ policy file the way editors
// and config tools do (write a temp file, rename it over the original) and
// reports whether the embedded evaluator survives and picks up the change.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/eval"
	_ "github.com/teramoby/speedle-plus/pkg/store/file"
)

func main() {
	mode := "rename"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}
	dir, _ := os.MkdirTemp("", "watchcheck-")
	p := filepath.Join(dir, "policy.spdl")
	orig := "[service.app]\n[policy]\nGRANT ROLE viewer read /data1\n[rolepolicy]\nGRANT USER bob viewer\n"
	os.WriteFile(p, []byte(orig), 0o644)
	ev, err := eval.NewFromFile(p, true)
	if err != nil {
		fmt.Println("load error:", err)
		os.Exit(2)
	}
	rc := ads.RequestContext{Subject: &ads.Subject{Principals: []*ads.Principal{{Type: "user", Name: "bob"}}},
		ServiceName: "app", Action: "write", Resource: "/data1"}
	updated := strings.Replace(orig, "[rolepolicy]", "GRANT ROLE viewer write /data1\n[rolepolicy]", 1)
	switch mode {
	case "rename": // atomic save: write temp, rename over
		tmp := p + ".tmp"
		os.WriteFile(tmp, []byte(updated), 0o644)
		os.Rename(tmp, p)
	case "inplace":
		os.WriteFile(p, []byte(updated), 0o644)
	}
	start := time.Now()
	for time.Since(start) < 5*time.Second {
		if ok, _, _ := ev.IsAllowed(rc); ok {
			fmt.Printf("mode=%s: survived, change visible after %v\n", mode, time.Since(start).Round(time.Millisecond))
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	fmt.Printf("mode=%s: survived, but change NOT visible after 5s\n", mode)
}
