package probestore

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/google/uuid"
	v1 "github.com/rhobs/rhobs-synthetics-api/pkg/apis/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	probeConfigMapNameFormat = "probe-config-%s"
)

// KubernetesProbeStore implements the ProbeStorage interface using Kubernetes ConfigMaps.
type KubernetesProbeStore struct {
	Client    kubernetes.Interface
	Namespace string
}

// NewKubernetesProbeStore creates a new KubernetesProbeStore.
// The namespace existence is not checked here; it is assumed to exist.
// RBAC permissions for the service account only allow for namespaced resource access,
// so a cluster-level check for a namespace is not possible and also redundant.
func NewKubernetesProbeStore(ctx context.Context, client kubernetes.Interface, namespace string) (*KubernetesProbeStore, error) {
	log.Printf("Initializing Kubernetes probe store in namespace %q", namespace)
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
	cmLabels[probeStatusLabelKey] = string(probe.Status)

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

	// TODO: Tune logging level for this
	log.Printf("Created probe %s with URL hash %s", probe.Id.String(), urlHashString)
	return &probe, nil
}

func (k *KubernetesProbeStore) UpdateProbe(ctx context.Context, probe v1.ProbeObject) (*v1.ProbeObject, error) {
	configMapName := fmt.Sprintf(probeConfigMapNameFormat, probe.Id)

	// We need to fetch the existing ConfigMap to get its resource version for the update.
	cm, err := k.Client.CoreV1().ConfigMaps(k.Namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		return nil, err // Let the caller handle not found errors
	}

	// Marshal the updated probe object
	payloadBytes, err := json.Marshal(probe)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal updated payload: %w", err)
	}

	// Update the data
	cm.Data["probe-config.json"] = string(payloadBytes)

	// Update the labels, ensuring our base labels are preserved
	if cm.Labels == nil {
		cm.Labels = make(map[string]string)
	}
	if probe.Labels != nil {
		for key, val := range *probe.Labels {
			cm.Labels[key] = val
		}
	}
	cm.Labels[baseAppLabelKey] = baseAppLabelValue
	cm.Labels[probeStatusLabelKey] = string(probe.Status)

	updatedCM, err := k.Client.CoreV1().ConfigMaps(k.Namespace).Update(ctx, cm, metav1.UpdateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to update configmap %s: %w", configMapName, err)
	}

	// Return the fully updated probe object
	var finalProbe v1.ProbeObject
	if err := json.Unmarshal([]byte(updatedCM.Data["probe-config.json"]), &finalProbe); err != nil {
		return nil, fmt.Errorf("failed to unmarshal probe from updated configmap: %w", err)
	}

	// TODO: Tune logging level for this
	log.Printf("Updated probe %s", probe.Id.String())
	return &finalProbe, nil
}

func (k *KubernetesProbeStore) DeleteProbe(ctx context.Context, probeID uuid.UUID) error {
	configMapName := fmt.Sprintf(probeConfigMapNameFormat, probeID)

	// TODO: Tune logging level for this
	log.Printf("Deleted probe %s", probeID.String())
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
