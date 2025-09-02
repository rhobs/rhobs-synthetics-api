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
