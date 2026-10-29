package docscheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

// Decode the actual bilingual manifest against current Kubernetes types. This
// checks the documentation contract; admission/registry/rollout need a cluster.
func TestDeploymentManifest(t *testing.T) {
	var english string
	for _, locale := range []string{"", ".zh"} {
		data, err := os.ReadFile(filepath.Join(root(), "docs/hugo/content/docs/deployment"+locale+".md"))
		if err != nil {
			t.Fatal(err)
		}
		match := regexp.MustCompile("(?s)<!-- deployment-template: kubernetes -->\\s*```yaml\\n(.*?)\\n```").FindSubmatch(data)
		if match == nil {
			t.Fatal("Missing Kubernetes example")
		}
		text := string(match[1])
		if locale == "" {
			english = text
		} else if text != english {
			t.Fatal("Translated manifest differs")
		}
		parts := strings.Split(text, "\n---\n")
		if len(parts) != 2 {
			t.Fatal("Expected Deployment and Service")
		}
		decode := func(source string, target any) {
			raw, err := yaml.YAMLToJSONStrict([]byte(source))
			if err != nil {
				t.Fatal(err)
			}
			decoder := json.NewDecoder(strings.NewReader(string(raw)))
			decoder.DisallowUnknownFields()
			if err := decoder.Decode(target); err != nil {
				t.Fatal(err)
			}
		}
		var deployment appsv1.Deployment
		var service corev1.Service
		decode(parts[0], &deployment)
		decode(parts[1], &service)
		if deployment.APIVersion != "apps/v1" || service.APIVersion != "v1" {
			t.Fatal("Obsolete API version")
		}
		if !reflect.DeepEqual(deployment.Spec.Selector.MatchLabels, deployment.Spec.Template.Labels) || !reflect.DeepEqual(service.Spec.Selector, deployment.Spec.Template.Labels) {
			t.Fatal("Selector does not match pods")
		}
		if *deployment.Spec.Replicas != 1 || deployment.Spec.Strategy.Type != appsv1.RecreateDeploymentStrategyType || service.Spec.Type != corev1.ServiceTypeClusterIP {
			t.Fatal("File-store development topology changed")
		}
		pod := deployment.Spec.Template.Spec
		if len(pod.InitContainers) != 1 || len(pod.Containers) != 2 || len(pod.Volumes) != 1 || pod.Volumes[0].EmptyDir == nil {
			t.Fatal("Missing shared policy initialization/storage")
		}
		for _, c := range pod.Containers {
			if len(c.VolumeMounts) != 1 || c.VolumeMounts[0].MountPath != "/data" || c.ReadinessProbe == nil || c.ReadinessProbe.TCPSocket == nil {
				t.Fatal("Missing shared store or readiness probe")
			}
			if len(c.Ports) != 1 || c.ReadinessProbe.TCPSocket.Port.IntVal != c.Ports[0].ContainerPort {
				t.Fatal("Readiness probe checks the wrong port")
			}
			if !strings.Contains(strings.Join(c.Args, " "), "--filestore-loc /data/policies.json") {
				t.Fatal("PMS/ADS must share the same file")
			}
		}
	}
}
