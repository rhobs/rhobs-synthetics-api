package templates

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestTemplatesValidYAML(t *testing.T) {
	templateFiles := []string{
		"synthetics-api-template.yaml",
		"service-monitor-synthetics-api-template.yaml",
	}

	for _, file := range templateFiles {
		t.Run(file, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(".", file))
			if err != nil {
				t.Fatalf("Failed to read template file %s: %v", file, err)
			}

			var template map[string]interface{}
			if err := yaml.Unmarshal(content, &template); err != nil {
				t.Fatalf("Template %s is not valid YAML: %v", file, err)
			}

			// Verify it's an OpenShift template
			if apiVersion, ok := template["apiVersion"].(string); !ok || apiVersion != "template.openshift.io/v1" {
				t.Errorf("Template %s should have apiVersion 'template.openshift.io/v1', got %v", file, template["apiVersion"])
			}

			if kind, ok := template["kind"].(string); !ok || kind != "Template" {
				t.Errorf("Template %s should have kind 'Template', got %v", file, template["kind"])
			}

			// Verify it has objects
			if objects, ok := template["objects"].([]interface{}); !ok || len(objects) == 0 {
				t.Errorf("Template %s should have non-empty objects array", file)
			}
		})
	}
}

func TestSyntheticsAPITemplateStructure(t *testing.T) {
	content, err := os.ReadFile("synthetics-api-template.yaml")
	if err != nil {
		t.Fatalf("Failed to read synthetics-api-template.yaml: %v", err)
	}

	var template map[string]interface{}
	if err := yaml.Unmarshal(content, &template); err != nil {
		t.Fatalf("Template is not valid YAML: %v", err)
	}

	objects, ok := template["objects"].([]interface{})
	if !ok {
		t.Fatal("Template should have objects array")
	}

	expectedKinds := map[string]bool{
		"Service":        false,
		"ServiceAccount": false,
		"Deployment":     false,
		"NetworkPolicy":  false,
	}

	for _, obj := range objects {
		objMap, ok := obj.(map[string]interface{})
		if !ok {
			continue
		}

		if kind, ok := objMap["kind"].(string); ok {
			if _, expected := expectedKinds[kind]; expected {
				expectedKinds[kind] = true
			}
		}
	}

	for kind, found := range expectedKinds {
		if !found {
			t.Errorf("Expected to find %s object in template", kind)
		}
	}

	// Validate NetworkPolicy spec in detail
	for _, obj := range objects {
		objMap, ok := obj.(map[string]interface{})
		if !ok {
			continue
		}
		if kind, _ := objMap["kind"].(string); kind != "NetworkPolicy" {
			continue
		}

		spec, ok := objMap["spec"].(map[string]interface{})
		if !ok {
			t.Fatal("NetworkPolicy should have a spec")
		}

		podSelector, ok := spec["podSelector"].(map[string]interface{})
		if !ok {
			t.Fatal("NetworkPolicy should have podSelector")
		}
		matchLabels, ok := podSelector["matchLabels"].(map[string]interface{})
		if !ok || matchLabels["app.kubernetes.io/name"] != "synthetics-api" {
			t.Error("NetworkPolicy podSelector should target app.kubernetes.io/name: synthetics-api")
		}

		policyTypes, ok := spec["policyTypes"].([]interface{})
		if !ok || len(policyTypes) == 0 {
			t.Fatal("NetworkPolicy should have policyTypes")
		}
		foundIngress := false
		for _, pt := range policyTypes {
			if pt == "Ingress" {
				foundIngress = true
			}
		}
		if !foundIngress {
			t.Error("NetworkPolicy policyTypes should include Ingress")
		}

		ingressRules, ok := spec["ingress"].([]interface{})
		if !ok || len(ingressRules) == 0 {
			t.Fatal("NetworkPolicy should have ingress rules")
		}

		allowedSources := map[string]bool{
			"synthetics-agent": false,
			"rhobs-gateway":    false,
			"prometheus":       false,
		}
		foundE2EAgentSource := false
		for _, rule := range ingressRules {
			ruleMap, ok := rule.(map[string]interface{})
			if !ok {
				continue
			}

			ports, ok := ruleMap["ports"].([]interface{})
			if !ok || len(ports) == 0 {
				t.Error("Each NetworkPolicy ingress rule should specify ports")
				continue
			}
			ruleAllowsTCP8080 := false
			for _, p := range ports {
				portMap, ok := p.(map[string]interface{})
				if !ok {
					continue
				}
				if portMap["port"] != 8080 {
					t.Errorf("NetworkPolicy ingress port should be 8080, got %v", portMap["port"])
				}
				protocol, hasProtocol := portMap["protocol"]
				if hasProtocol && protocol != "TCP" {
					t.Errorf("NetworkPolicy ingress protocol should be TCP or omitted, got %v", protocol)
				}
				if portMap["port"] == 8080 && (!hasProtocol || protocol == "TCP") {
					ruleAllowsTCP8080 = true
				}
			}

			from, ok := ruleMap["from"].([]interface{})
			if !ok {
				continue
			}
			for _, f := range from {
				fMap, ok := f.(map[string]interface{})
				if !ok {
					continue
				}
				// Ensure no rule uses namespaceSelector: {} (allows all namespaces)
				if ns, exists := fMap["namespaceSelector"]; exists {
					nsMap, ok := ns.(map[string]interface{})
					if ok && len(nsMap) == 0 {
						t.Error("NetworkPolicy should not use namespaceSelector: {} (allows all namespaces)")
					}
				}
				if ps, ok := fMap["podSelector"].(map[string]interface{}); ok {
					if ml, ok := ps["matchLabels"].(map[string]interface{}); ok {
						if name, ok := ml["app.kubernetes.io/name"].(string); ok {
							allowedSources[name] = true
						}
						if app, ok := ml["app"].(string); ok && app == "synthetics-agent" {
							if ns, ok := fMap["namespaceSelector"].(map[string]interface{}); ok {
								if nsLabels, ok := ns["matchLabels"].(map[string]interface{}); ok && nsLabels["rhobs-e2e"] == "true" && ruleAllowsTCP8080 {
									foundE2EAgentSource = true
								}
							}
						}
					}
				}
			}
		}

		for source, found := range allowedSources {
			if !found {
				t.Errorf("NetworkPolicy should allow ingress from %s", source)
			}
		}
		if !foundE2EAgentSource {
			t.Error("NetworkPolicy should allow synthetics-agent pods from namespaces labelled rhobs-e2e=true on TCP port 8080")
		}
	}

	// Verify parameters
	params, ok := template["parameters"].([]interface{})
	if !ok || len(params) == 0 {
		t.Error("Template should have parameters")
	}

	// Verify IMAGE_DIGEST parameter is not present
	for _, param := range params {
		paramMap, ok := param.(map[string]interface{})
		if !ok {
			continue
		}
		if name, ok := paramMap["name"].(string); ok && name == "IMAGE_DIGEST" {
			t.Error("IMAGE_DIGEST parameter should not be present in template")
		}
	}

	// Verify NAMESPACE parameter is present
	namespaceFound := false
	imageTagFound := false
	for _, param := range params {
		paramMap, ok := param.(map[string]interface{})
		if !ok {
			continue
		}
		if name, ok := paramMap["name"].(string); ok {
			if name == "NAMESPACE" {
				namespaceFound = true
			}
			if name == "IMAGE_TAG" {
				imageTagFound = true
			}
		}
	}
	if !namespaceFound {
		t.Error("NAMESPACE parameter should be present in template")
	}
	if !imageTagFound {
		t.Error("IMAGE_TAG parameter should be present in template")
	}
}

func TestSyntheticsAPITemplateHAConfiguration(t *testing.T) {
	content, err := os.ReadFile("synthetics-api-template.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var template struct {
		Objects []struct {
			Kind string `yaml:"kind"`
			Spec struct {
				Replicas string `yaml:"replicas"`
				Strategy struct {
					RollingUpdate struct {
						MaxSurge       int `yaml:"maxSurge"`
						MaxUnavailable int `yaml:"maxUnavailable"`
					} `yaml:"rollingUpdate"`
				} `yaml:"strategy"`
				Template struct {
					Metadata struct {
						Labels map[string]string `yaml:"labels"`
					} `yaml:"metadata"`
					Spec struct {
						Affinity struct {
							PodAntiAffinity struct {
								Required []struct {
									LabelSelector struct {
										MatchLabels map[string]string `yaml:"matchLabels"`
									} `yaml:"labelSelector"`
									TopologyKey string `yaml:"topologyKey"`
								} `yaml:"requiredDuringSchedulingIgnoredDuringExecution"`
							} `yaml:"podAntiAffinity"`
						} `yaml:"affinity"`
					} `yaml:"spec"`
				} `yaml:"template"`
			} `yaml:"spec"`
		} `yaml:"objects"`
		Parameters []struct {
			Name  string `yaml:"name"`
			Value string `yaml:"value"`
		} `yaml:"parameters"`
	}
	if err := yaml.Unmarshal(content, &template); err != nil {
		t.Fatal(err)
	}
	replicaCountFound := false
	for _, p := range template.Parameters {
		if p.Name == "REPLICA_COUNT" {
			replicaCountFound = true
			if p.Value != "2" {
				t.Fatalf("REPLICA_COUNT default = %q, want 2", p.Value)
			}
		}
	}
	if !replicaCountFound {
		t.Fatal("REPLICA_COUNT parameter not found")
	}
	for _, object := range template.Objects {
		if object.Kind != "Deployment" {
			continue
		}
		if object.Spec.Replicas != "${{REPLICA_COUNT}}" {
			t.Errorf("replicas = %q, want parameterized count", object.Spec.Replicas)
		}
		if object.Spec.Strategy.RollingUpdate.MaxSurge != 0 || object.Spec.Strategy.RollingUpdate.MaxUnavailable != 1 {
			t.Error("HA rollout requires maxSurge=0 and maxUnavailable=1")
		}
		required := object.Spec.Template.Spec.Affinity.PodAntiAffinity.Required
		if len(required) != 1 || required[0].TopologyKey != "kubernetes.io/hostname" {
			t.Error("API replicas must have required hostname anti-affinity")
		} else {
			selector := required[0].LabelSelector.MatchLabels
			labels := object.Spec.Template.Metadata.Labels
			if len(selector) == 0 {
				t.Error("anti-affinity selector must select API pods")
			}
			for key, value := range selector {
				if labels[key] != value {
					t.Errorf("anti-affinity selector %q=%q does not match pod label", key, value)
				}
			}
		}
		return
	}
	t.Fatal("Deployment not found in API template")
}

func TestServiceMonitorTemplateStructure(t *testing.T) {
	content, err := os.ReadFile("service-monitor-synthetics-api-template.yaml")
	if err != nil {
		t.Fatalf("Failed to read service-monitor-synthetics-api-template.yaml: %v", err)
	}

	var template map[string]interface{}
	if err := yaml.Unmarshal(content, &template); err != nil {
		t.Fatalf("Template is not valid YAML: %v", err)
	}

	objects, ok := template["objects"].([]interface{})
	if !ok || len(objects) != 1 {
		t.Fatal("Template should have exactly one object")
	}

	serviceMonitor, ok := objects[0].(map[string]interface{})
	if !ok {
		t.Fatal("Object should be a map")
	}

	if kind, ok := serviceMonitor["kind"].(string); !ok || kind != "ServiceMonitor" {
		t.Errorf("Expected ServiceMonitor object, got %v", serviceMonitor["kind"])
	}

	if apiVersion, ok := serviceMonitor["apiVersion"].(string); !ok || apiVersion != "monitoring.coreos.com/v1" {
		t.Errorf("Expected apiVersion 'monitoring.coreos.com/v1', got %v", serviceMonitor["apiVersion"])
	}

	// Verify parameters
	params, ok := template["parameters"].([]interface{})
	if !ok || len(params) == 0 {
		t.Error("Template should have parameters")
	}

	namespaceFound := false
	imageTagFound := false
	for _, param := range params {
		paramMap, ok := param.(map[string]interface{})
		if !ok {
			continue
		}
		if name, ok := paramMap["name"].(string); ok {
			if name == "NAMESPACE" {
				namespaceFound = true
			}
			if name == "IMAGE_TAG" {
				imageTagFound = true
			}
			if name == "MONITORING_NAMESPACE" {
				t.Error("MONITORING_NAMESPACE parameter should not be present - should use NAMESPACE instead")
			}
		}
	}
	if !namespaceFound {
		t.Error("NAMESPACE parameter should be present in service monitor template")
	}
	if !imageTagFound {
		t.Error("IMAGE_TAG parameter should be present in service monitor template")
	}
}
