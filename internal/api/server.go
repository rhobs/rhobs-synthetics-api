package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"

	"github.com/google/uuid"
	v1 "github.com/rhobs/rhobs-synthetics-api/pkg/apis/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
)

type Server struct {
	KubeClient kubernetes.Interface
	Namespace  string
}

func NewServer(client kubernetes.Interface, namespace string) Server {
	return Server{
		KubeClient: client,
		Namespace:  namespace,
	}
}

// (GET /metrics/probes)
func (s Server) ListProbes(ctx context.Context, request v1.ListProbesRequestObject) (v1.ListProbesResponseObject, error) {
	// Start with the base selector for our app
	baseSelector := "app=rhobs-synthetics"
	finalSelector := baseSelector

	// If the user provided a selector, validate and append it
	if request.Params.LabelSelector != nil && *request.Params.LabelSelector != "" {
		userSelector := *request.Params.LabelSelector
		// Validate the user-provided selector syntax
		_, err := labels.Parse(userSelector)
		if err != nil {
			return v1.ListProbes400JSONResponse{
				Error: &struct {
					Code    *int32  `json:"code,omitempty"`
					Message *string `json:"message,omitempty"`
				}{
					Code:    int32p(400),
					Message: stringp(fmt.Sprintf("invalid label_selector: %v", err)),
				},
			}, nil
		}
		finalSelector = fmt.Sprintf("%s,%s", baseSelector, userSelector)
	}

	configMaps, err := s.KubeClient.CoreV1().ConfigMaps(s.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: finalSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list config maps: %w", err)
	}

	probes := []v1.ProbeObject{}
	for _, cm := range configMaps.Items {
		probe := v1.ProbeObject{}
		err := json.Unmarshal([]byte(cm.Data["probe-config.json"]), &probe)
		if err != nil {
			log.Printf("Error unmarshaling probe from configmap %s: %v", cm.Name, err)
			continue // Or handle error more gracefully
		}
		probes = append(probes, probe)
	}

	return v1.ListProbes200JSONResponse(v1.ProbesArrayResponse{Probes: probes}), nil
}

// (GET /metrics/probes/{probe_id})
func (s Server) GetProbeById(ctx context.Context, request v1.GetProbeByIdRequestObject) (v1.GetProbeByIdResponseObject, error) {
	configMapName := fmt.Sprintf("probe-config-%s", request.ProbeId)
	cm, err := s.KubeClient.CoreV1().ConfigMaps(s.Namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return v1.GetProbeById404JSONResponse{
				Error: &struct {
					Code    *int32  `json:"code,omitempty"`
					Message *string `json:"message,omitempty"`
				}{
					Code:    int32p(404),
					Message: stringp(fmt.Sprintf("probe with ID %s not found", request.ProbeId)),
				},
			}, nil
		}
		return nil, fmt.Errorf("failed to get config map: %w", err)
	}

	probe := v1.ProbeObject{}
	err = json.Unmarshal([]byte(cm.Data["probe-config.json"]), &probe)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal probe from configmap: %w", err)
	}

	return v1.GetProbeById200JSONResponse(probe), nil
}

// (POST /metrics/probes)
func (s Server) CreateProbe(ctx context.Context, request v1.CreateProbeRequestObject) (v1.CreateProbeResponseObject, error) {
	// Validate that a probe with this static_url doesn't already exist
	urlHash := sha256.Sum256([]byte(request.Body.StaticUrl))
	// A SHA256 hash is 64 characters, but Kubernetes labels are limited to 63.
	// Truncating the hash is safe enough for this purpose.
	urlHashString := hex.EncodeToString(urlHash[:])[:63]
	hashLabelSelector := fmt.Sprintf("rhobs-synthetics/static-url-hash=%s", urlHashString)

	existingProbes, err := s.KubeClient.CoreV1().ConfigMaps(s.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: hashLabelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing probes: %w", err)
	}

	if len(existingProbes.Items) > 0 {
		return v1.CreateProbe409JSONResponse{
			Error: &struct {
				Code    *int32  `json:"code,omitempty"`
				Message *string `json:"message,omitempty"`
			}{
				Code:    int32p(409),
				Message: stringp(fmt.Sprintf("a probe for static_url %q already exists", request.Body.StaticUrl)),
			},
		}, nil
	}

	probeID := uuid.New()

	// Prepare the probe object that will be stored
	probeToStore := v1.ProbeObject{
		Id:        probeID,
		StaticUrl: request.Body.StaticUrl,
		Labels:    request.Body.Labels,
	}

	// Convert payload to JSON for storing in the configmap
	payloadBytes, err := json.Marshal(probeToStore)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	configMapName := fmt.Sprintf("probe-config-%s", probeID)

	// Use labels from the request, and add our own app label
	cmLabels := make(map[string]string)
	if request.Body.Labels != nil {
		for k, v := range *request.Body.Labels {
			cmLabels[k] = v
		}
	}
	cmLabels["app"] = "rhobs-synthetics"
	cmLabels["rhobs-synthetics/static-url-hash"] = urlHashString

	// Create the config map
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: s.Namespace,
			Labels:    cmLabels,
		},
		Data: map[string]string{
			"probe-config.json": string(payloadBytes),
		},
	}

	// Create the config map in Kubernetes
	_, err = s.KubeClient.CoreV1().ConfigMaps(s.Namespace).Create(ctx, configMap, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create config map: %w", err)
	}

	log.Printf("Successfully created probe and config map for probe ID: %s", probeID)
	return v1.CreateProbe201JSONResponse(probeToStore), nil
}

// (DELETE /metrics/probes/{probe_id})
func (s Server) DeleteProbe(ctx context.Context, request v1.DeleteProbeRequestObject) (v1.DeleteProbeResponseObject, error) {
	configMapName := fmt.Sprintf("probe-config-%s", request.ProbeId)
	err := s.KubeClient.CoreV1().ConfigMaps(s.Namespace).Delete(ctx, configMapName, metav1.DeleteOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return v1.DeleteProbe404JSONResponse{
				Error: &struct {
					Code    *int32  `json:"code,omitempty"`
					Message *string `json:"message,omitempty"`
				}{
					Code:    int32p(404),
					Message: stringp(fmt.Sprintf("probe with ID %s not found", request.ProbeId)),
				},
			}, nil
		}
		return nil, fmt.Errorf("failed to delete config map: %w", err)
	}

	log.Printf("Successfully deleted probe for probe ID: %s", request.ProbeId)
	return v1.DeleteProbe204Response{}, nil
}

// helper function to create a pointer to a string
func stringp(s string) *string {
	return &s
}

// helper function to create a pointer to an int32
func int32p(i int32) *int32 {
	return &i
}
