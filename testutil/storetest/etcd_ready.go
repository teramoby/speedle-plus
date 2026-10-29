package storetest

import (
	"fmt"
	"net/http"
	"time"
)

// WaitForEtcdHTTP finishes gateway startup before TestMain calls m.Run.
// etcd's ReadyNotify can precede logrus.New in the HTTP gateway; m.Run's
// test2json redirection of os.Stderr would race with that logger's creation.
func WaitForEtcdHTTP() error {
	return waitForEtcdHTTP("http://localhost:2379/health", 10*time.Second)
}

func waitForEtcdHTTP(url string, timeout time.Duration) error {
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{}}
	defer client.CloseIdleConnections()
	deadline := time.Now().Add(timeout)
	var lastError error
	for {
		response, err := client.Get(url)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
			err = fmt.Errorf("unhealthy response: %s", response.Status)
		}
		lastError = err
		if remaining := time.Until(deadline); remaining > 0 {
			time.Sleep(min(50*time.Millisecond, remaining))
		} else {
			return fmt.Errorf("embedded etcd HTTP startup timed out: %w", lastError)
		}
	}
}
