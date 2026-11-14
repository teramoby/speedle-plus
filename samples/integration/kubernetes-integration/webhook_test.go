package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Use the real client and JSON decoding against a loopback API, so upgrades
// exercise the resource paths and metadata consumed by webhook policies.
func TestKubernetesResourceMetadata(t *testing.T) {
	for _, resource := range []struct {
		name, kind, apiVersion, path string
	}{
		{"pods", "Pod", "v1", "/api/v1/namespaces/team/pods/example"},
		{"daemonsets", "DaemonSet", "apps/v1", "/apis/apps/v1/namespaces/team/daemonsets/example"},
		{"deployments", "Deployment", "apps/v1", "/apis/apps/v1/namespaces/team/deployments/example"},
		{"replicasets", "ReplicaSet", "apps/v1", "/apis/apps/v1/namespaces/team/replicasets/example"},
		{"replicationcontrollers", "ReplicationController", "v1", "/api/v1/namespaces/team/replicationcontrollers/example"},
		{"statefulsets", "StatefulSet", "apps/v1", "/apis/apps/v1/namespaces/team/statefulsets/example"},
		{"nodes", "Node", "v1", "/api/v1/nodes/example"},
		{"namespaces", "Namespace", "v1", "/api/v1/namespaces/example"},
	} {
		t.Run(resource.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != resource.path {
					t.Errorf("request: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"apiVersion":%q,"kind":%q,"metadata":{"name":"example","namespace":"team","labels":{"component":"etcd"},"ownerReferences":[{"apiVersion":"apps/v1","kind":"Deployment","name":"owner","uid":"owner-id"}]}}`, resource.apiVersion, resource.kind)
			}))
			defer server.Close()
			client, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			handler := &handlerImpl{kubeClientset: client}
			metadata, owners, err := handler.getResourceByName("team", resource.name, "example")
			if err != nil {
				t.Fatal(err)
			}
			if metadata.Name != "example" || metadata.Namespace != "team" || metadata.Labels["component"] != "etcd" ||
				len(owners) != 1 || owners[0].Kind != "Deployment" || owners[0].Name != "owner" || owners[0].UID != "owner-id" {
				t.Fatalf("metadata: %+v, owners: %+v", metadata, owners)
			}
		})
	}
}
