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

	// Get the existing ConfigMap to check its current status
	cm, err := k.Client.CoreV1().ConfigMaps(k.Namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if err != nil {
		return err // Pass the error up, including not found errors
	}

	// Unmarshal the existing probe object to check its status
	probe := &v1.ProbeObject{}
	err = json.Unmarshal([]byte(cm.Data["probe-config.json"]), probe)
	if err != nil {
		return fmt.Errorf("failed to unmarshal probe from configmap %s: %w", configMapName, err)
	}

	// Handle deletion based on current probe status
	switch probe.Status {
	case v1.Pending:
		// Probe was never picked up by an agent, delete immediately
		err = k.DeleteProbeStorage(ctx, probeID)
		if err != nil {
			return fmt.Errorf("failed to delete pending probe %s: %w", probeID.String(), err)
		}
		log.Printf("Deleted pending probe %s immediately (never processed by agent)", probeID.String())
		return nil

	case v1.Active:
		// Probe is active, set to terminating and wait for agent cleanup
		probe.Status = v1.Terminating

		// Marshal the updated probe object
		payloadBytes, err := json.Marshal(probe)
		if err != nil {
			return fmt.Errorf("failed to marshal updated payload: %w", err)
		}

		// Update the ConfigMap data
		cm.Data["probe-config.json"] = string(payloadBytes)

		// Update the status label
		if cm.Labels == nil {
			cm.Labels = make(map[string]string)
		}
		cm.Labels[probeStatusLabelKey] = string(v1.Terminating)

		// Update the ConfigMap instead of deleting it
		_, err = k.Client.CoreV1().ConfigMaps(k.Namespace).Update(ctx, cm, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update configmap %s to terminating status: %w", configMapName, err)
		}

		log.Printf("Set active probe %s status to terminating (waiting for agent cleanup)", probeID.String())
		return nil

	case v1.Terminating:
		// Already terminating, no action needed
		log.Printf("Probe %s is already in terminating state", probeID.String())
		return nil

	case v1.Failed:
		// Failed probe, delete immediately as agent likely won't process it
		err = k.DeleteProbeStorage(ctx, probeID)
		if err != nil {
			return fmt.Errorf("failed to delete failed probe %s: %w", probeID.String(), err)
		}
		log.Printf("Deleted failed probe %s immediately", probeID.String())
		return nil

	default:
		// Unknown status, treat as pending and delete immediately
		err = k.DeleteProbeStorage(ctx, probeID)
		if err != nil {
			return fmt.Errorf("failed to delete probe %s with unknown status %s: %w", probeID.String(), probe.Status, err)
		}
		log.Printf("Deleted probe %s with unknown status %s immediately", probeID.String(), probe.Status)
		return nil
	}
}

func (k *KubernetesProbeStore) DeleteProbeStorage(ctx context.Context, probeID uuid.UUID) error {
	configMapName := fmt.Sprintf(probeConfigMapNameFormat, probeID)

	// TODO: Tune logging level for this
	log.Printf("Deleting probe configmap: %s", probeID.String())
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
