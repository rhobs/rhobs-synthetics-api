package probestore

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/google/uuid"
	v1 "github.com/rhobs/rhobs-synthetics-api/pkg/apis/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// NewKubernetesProbeStore creates a new KubernetesProbeStore and verifies the namespace exists.
func NewKubernetesProbeStore(ctx context.Context, client kubernetes.Interface, namespace string) (*KubernetesProbeStore, error) {
	log.Printf("Verifying that namespace %q exists...", namespace)
	_, err := client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, fmt.Errorf("namespace %q does not exist", namespace)
		}
		return nil, fmt.Errorf("failed to verify if namespace %q exists: %w", namespace, err)
	}
	log.Printf("Namespace %q verified.", namespace)

	return &KubernetesProbeStore{
		Client:    client,
		Namespace: namespace,
	}, nil
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
