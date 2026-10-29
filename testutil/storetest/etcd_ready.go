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
	client := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{}}
	defer client.CloseIdleConnections()
	response, err := client.Get("http://localhost:2379/health")
	if err != nil {
		return fmt.Errorf("embedded etcd HTTP startup failed: %w", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("embedded etcd is unhealthy: %s", response.Status)
	}
	return nil
}
