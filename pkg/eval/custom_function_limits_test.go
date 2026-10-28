package eval

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/teramoby/speedle-plus/api/ext"
	"github.com/teramoby/speedle-plus/api/pms"
)

func TestCustomerFunctionResponseIsBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"result": "%s"}`, strings.Repeat("x", maxCustomerFunctionResponseBytes))
	}))
	defer server.Close()
	cf := &pms.Function{Name: "big", FuncURL: server.URL}
	if _, err := CallCustomerFunction(cf, &ext.CustomerFunctionRequest{}); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("got %v, want a size error", err)
	}
}

func TestCustomerFunctionResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result": 42}`)
	}))
	defer server.Close()
	cf := &pms.Function{Name: "answer", FuncURL: server.URL}
	if got, err := CallCustomerFunction(cf, &ext.CustomerFunctionRequest{}); err != nil || got != float64(42) {
		t.Errorf("got %v, %v; want 42", got, err)
	}
}

func TestFuncResultCacheIsBounded(t *testing.T) {
	cache := &FuncResultCache{Results: map[string]FuncResult{}}
	cf := &pms.Function{Name: "f", ResultCachable: true}
	for i := 0; i < maxFuncResultCacheEntries+10; i++ {
		cache.AddToCache(fmt.Sprintf("f(%d)", i), cf, i)
	}
	if len(cache.Results) != maxFuncResultCacheEntries {
		t.Errorf("cache holds %d entries, want %d", len(cache.Results), maxFuncResultCacheEntries)
	}
	cache.AddToCache("f(0)", cf, "updated")
	if got := cache.ReadFromCache("f(0)", cf); got != "updated" {
		t.Errorf("an existing entry was not updated when the cache is full: %v", got)
	}
}
