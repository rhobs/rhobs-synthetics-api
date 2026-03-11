package probestore

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/google/uuid"
	v1 "github.com/rhobs/rhobs-synthetics-api/pkg/apis/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	probeConfigMapNameFormat = "probe-config-%s"

	// lastReconciledLabelKey is the label RMO uses to stamp a heartbeat timestamp
	// on each probe ConfigMap during reconciliation.
	lastReconciledLabelKey = "last-reconciled"

	// defaultStaleProbeTTL is how long a probe can go without being reconciled
	// before the GC loop considers it stale and deletes it.
	// Override with PROBE_STALE_TTL env var (e.g., "15m", "1h").
	defaultStaleProbeTTL = 15 * time.Minute

	// unlabeledProbeTTL is how long a probe can exist without ever receiving
	// a last-reconciled heartbeat before GC deletes it. This catches probes
	// from non-RHOBS-enabled sectors that will never get heartbeats.
	unlabeledProbeTTL = 24 * time.Hour
)

// KubernetesProbeStore implements the ProbeStorage interface using Kubernetes ConfigMaps.
type KubernetesProbeStore struct {
	Client        kubernetes.Interface
	Namespace     string
	StaleProbeTTL time.Duration
}

// NewKubernetesProbeStore creates a new KubernetesProbeStore.
// The namespace existence is not checked here; it is assumed to exist.
// RBAC permissions for the service account only allow for namespaced resource access,
// so a cluster-level check for a namespace is not possible and also redundant.
func NewKubernetesProbeStore(ctx context.Context, client kubernetes.Interface, namespace string) (*KubernetesProbeStore, error) {
	ttl := defaultStaleProbeTTL
	if v := os.Getenv("PROBE_STALE_TTL"); v != "" {
		parsed, err := time.ParseDuration(v)
		if err != nil {
			log.Printf("Warning: invalid PROBE_STALE_TTL %q, using default %s: %v", v, defaultStaleProbeTTL, err)
		} else {
			ttl = parsed
			log.Printf("Using custom PROBE_STALE_TTL: %s", ttl)
		}
	}
	log.Printf("Initializing Kubernetes probe store in namespace %q (stale probe TTL: %s)", namespace, ttl)
	return &KubernetesProbeStore{
		Client:        client,
		Namespace:     namespace,
		StaleProbeTTL: ttl,
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

// GarbageCollectStaleProbes deletes probe ConfigMaps that are orphaned:
// 1. Probes with a last-reconciled timestamp older than StaleProbeTTL (default 15m)
// 2. Probes without a last-reconciled label that are older than 24 hours
//
// Case 1 catches probes for deleted clusters (RMO stops reconciling).
// Case 2 catches probes from non-RHOBS-enabled sectors that never get heartbeats.
func (k *KubernetesProbeStore) GarbageCollectStaleProbes(ctx context.Context) (int, error) {
	selector := fmt.Sprintf("%s=%s", baseAppLabelKey, baseAppLabelValue)
	configMaps, err := k.Client.CoreV1().ConfigMaps(k.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to list probe configmaps for GC: %w", err)
	}

	now := time.Now().UTC()
	deleted := 0

	for _, cm := range configMaps.Items {
		lastReconciledStr, ok := cm.Labels[lastReconciledLabelKey]
		if !ok {
			// No last-reconciled label -- check if the probe is old enough
			// to be considered abandoned (e.g., from a non-RHOBS-enabled sector
			// that will never get heartbeats).
			if !cm.CreationTimestamp.IsZero() && now.Sub(cm.CreationTimestamp.Time) > unlabeledProbeTTL {
				log.Printf("GC: deleting unlabeled probe configmap %s (created: %s, age: %s, no heartbeat ever received)",
					cm.Name, cm.CreationTimestamp.Format("20060102T150405Z"), now.Sub(cm.CreationTimestamp.Time).Round(time.Second))
				if err := k.Client.CoreV1().ConfigMaps(k.Namespace).Delete(ctx, cm.Name, metav1.DeleteOptions{}); err != nil {
					log.Printf("GC: failed to delete unlabeled probe configmap %s: %v", cm.Name, err)
					continue
				}
				deleted++
			}
			continue
		}

		lastReconciled, err := time.Parse("20060102T150405Z", lastReconciledStr)
		if err != nil {
			log.Printf("GC: could not parse last-reconciled label %q on configmap %s, skipping: %v", lastReconciledStr, cm.Name, err)
			continue
		}

		if now.Sub(lastReconciled) <= k.StaleProbeTTL {
			continue // still fresh
		}

		// Probe is stale, delete it
		log.Printf("GC: deleting stale probe configmap %s (last-reconciled: %s, age: %s)", cm.Name, lastReconciledStr, now.Sub(lastReconciled).Round(time.Second))
		err = k.Client.CoreV1().ConfigMaps(k.Namespace).Delete(ctx, cm.Name, metav1.DeleteOptions{})
		if err != nil {
			log.Printf("GC: failed to delete stale probe configmap %s: %v", cm.Name, err)
			continue
		}
		deleted++
	}

	return deleted, nil
}
