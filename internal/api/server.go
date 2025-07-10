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

const (
	probeConfigMapNameFormat = "probe-config-%s"
	baseAppLabelKey          = "app"
	baseAppLabelValue        = "rhobs-synthetics-probe"
	probeURLHashLabelKey     = "rhobs-synthetics/static-url-hash"
)

// ProbeStorage defines the interface for storing and retrieving probes.
type ProbeStorage interface {
	ListProbes(ctx context.Context, selector string) ([]v1.ProbeObject, error)
	GetProbe(ctx context.Context, probeID uuid.UUID) (*v1.ProbeObject, error)
	CreateProbe(ctx context.Context, probe v1.ProbeObject, urlHashString string) (*v1.ProbeObject, error)
	DeleteProbe(ctx context.Context, probeID uuid.UUID) error
	ProbeWithURLHashExists(ctx context.Context, urlHashString string) (bool, error)
}

// KubernetesProbeStore implements the ProbeStorage interface using Kubernetes ConfigMaps.
type KubernetesProbeStore struct {
	Client    kubernetes.Interface
	Namespace string
}

// NewKubernetesProbeStore creates a new KubernetesProbeStore.
func NewKubernetesProbeStore(client kubernetes.Interface, namespace string) *KubernetesProbeStore {
	return &KubernetesProbeStore{
		Client:    client,
		Namespace: namespace,
	}
}

func (k *KubernetesProbeStore) ListProbes(ctx context.Context, selector string) ([]v1.ProbeObject, error) {
	configMaps, err := k.Client.CoreV1().ConfigMaps(k.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list config maps: %w", err)
	}

	probes := []v1.ProbeObject{}
	for _, cm := range configMaps.Items {
		probe := v1.ProbeObject{}
		if probeData, ok := cm.Data["probe-config.json"]; ok {
			err := json.Unmarshal([]byte(probeData), &probe)
			if err != nil {
				log.Printf("Error unmarshaling probe from configmap %s: %v", cm.Name, err)
				continue // Or handle error more gracefully
			}
			probes = append(probes, probe)
		}
	}
	return probes, nil
}

func (k *KubernetesProbeStore) GetProbe(ctx context.Context, probeID uuid.UUID) (*v1.ProbeObject, error) {
	configMapName := fmt.Sprintf(probeConfigMapNameFormat, probeID)
	cm, err := k.Client.CoreV1().ConfigMaps(k.Namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		return nil, err // Pass the error up, including not found errors
	}

	probe := &v1.ProbeObject{}
	err = json.Unmarshal([]byte(cm.Data["probe-config.json"]), probe)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal probe from configmap: %w", err)
	}
	return probe, nil
}

func (k *KubernetesProbeStore) CreateProbe(ctx context.Context, probe v1.ProbeObject, urlHashString string) (*v1.ProbeObject, error) {
	payloadBytes, err := json.Marshal(probe)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	configMapName := fmt.Sprintf(probeConfigMapNameFormat, probe.Id)
	cmLabels := make(map[string]string)
	if probe.Labels != nil {
		for key, val := range *probe.Labels {
			cmLabels[key] = val
		}
	}
	// Add our base app label from the constant
	cmLabels[baseAppLabelKey] = baseAppLabelValue
	cmLabels[probeURLHashLabelKey] = urlHashString

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: k.Namespace,
			Labels:    cmLabels,
		},
		Data: map[string]string{
			"probe-config.json": string(payloadBytes),
		},
	}

	_, err = k.Client.CoreV1().ConfigMaps(k.Namespace).Create(ctx, configMap, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	return &probe, nil
}

func (k *KubernetesProbeStore) DeleteProbe(ctx context.Context, probeID uuid.UUID) error {
	configMapName := fmt.Sprintf(probeConfigMapNameFormat, probeID)
	return k.Client.CoreV1().ConfigMaps(k.Namespace).Delete(ctx, configMapName, metav1.DeleteOptions{})
}

func (k *KubernetesProbeStore) ProbeWithURLHashExists(ctx context.Context, urlHashString string) (bool, error) {
	hashLabelSelector := fmt.Sprintf("%s=%s", probeURLHashLabelKey, urlHashString)
	existingProbes, err := k.Client.CoreV1().ConfigMaps(k.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: hashLabelSelector,
	})
	if err != nil {
		return false, fmt.Errorf("failed to check for existing probes: %w", err)
	}
	return len(existingProbes.Items) > 0, nil
}

// Server is the main API server object.
type Server struct {
	Store ProbeStorage
}

// NewServer creates a new API server.
func NewServer(store ProbeStorage) Server {
	return Server{
		Store: store,
	}
}

// (GET /metrics/probes)
func (s Server) ListProbes(ctx context.Context, request v1.ListProbesRequestObject) (v1.ListProbesResponseObject, error) {
	baseSelector := fmt.Sprintf("%s=%s", baseAppLabelKey, baseAppLabelValue)
	finalSelector := baseSelector

	// If the user provided a selector, validate and append it
	if request.Params.LabelSelector != nil && *request.Params.LabelSelector != "" {
		userSelector := *request.Params.LabelSelector
		// Validate the user-provided selector syntax
		_, err := labels.Parse(userSelector)
		if err != nil {
			return v1.ListProbes400JSONResponse{
				Error: v1.ErrorObject{
					Message: fmt.Sprintf("invalid label_selector: %v", err),
				},
			}, nil
		}
		finalSelector = fmt.Sprintf("%s,%s", baseSelector, userSelector)
	}

	probes, err := s.Store.ListProbes(ctx, finalSelector)
	if err != nil {
		return nil, fmt.Errorf("failed to list probes from storage: %w", err)
	}

	return v1.ListProbes200JSONResponse(v1.ProbesArrayResponse{Probes: probes}), nil
}

// (GET /metrics/probes/{probe_id})
func (s Server) GetProbeById(ctx context.Context, request v1.GetProbeByIdRequestObject) (v1.GetProbeByIdResponseObject, error) {
	probe, err := s.Store.GetProbe(ctx, request.ProbeId)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return v1.GetProbeById404JSONResponse{
				Warning: v1.WarningObject{
					Message: fmt.Sprintf("probe with ID %s not found", request.ProbeId),
				},
			}, nil
		}
		return nil, fmt.Errorf("failed to get probe from storage: %w", err)
	}

	return v1.GetProbeById200JSONResponse(*probe), nil
}

// (POST /metrics/probes)
func (s Server) CreateProbe(ctx context.Context, request v1.CreateProbeRequestObject) (v1.CreateProbeResponseObject, error) {
	urlHash := sha256.Sum256([]byte(request.Body.StaticUrl))
	urlHashString := hex.EncodeToString(urlHash[:])[:63]

	exists, err := s.Store.ProbeWithURLHashExists(ctx, urlHashString)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing probes: %w", err)
	}

	if exists {
		return v1.CreateProbe409JSONResponse{
			Error: v1.ErrorObject{
				Message: fmt.Sprintf("a probe for static_url %q already exists", request.Body.StaticUrl),
			},
		}, nil
	}

	probeToStore := v1.ProbeObject{
		Id:        uuid.New(),
		StaticUrl: request.Body.StaticUrl,
		Labels:    request.Body.Labels,
	}

	createdProbe, err := s.Store.CreateProbe(ctx, probeToStore, urlHashString)
	if err != nil {
		return v1.CreateProbe500JSONResponse{
			Error: v1.ErrorObject{
				Message: fmt.Sprintf("failed to create probe: %v", err),
			},
		}, nil
	}

	log.Printf("Successfully created probe and config map for probe ID: %s", createdProbe.Id)
	return v1.CreateProbe201JSONResponse(*createdProbe), nil
}

// (DELETE /metrics/probes/{probe_id})
func (s Server) DeleteProbe(ctx context.Context, request v1.DeleteProbeRequestObject) (v1.DeleteProbeResponseObject, error) {
	err := s.Store.DeleteProbe(ctx, request.ProbeId)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return v1.DeleteProbe404JSONResponse{
				Warning: v1.WarningObject{
					Message: fmt.Sprintf("probe with ID %s not found", request.ProbeId),
				},
			}, nil
		}
		return nil, fmt.Errorf("failed to delete probe from storage: %w", err)
	}

	log.Printf("Successfully deleted probe for probe ID: %s", request.ProbeId)
	return v1.DeleteProbe204Response{}, nil
}
