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

	// lastReconciledKey is the key used to stamp a heartbeat timestamp on each
	// probe ConfigMap during reconciliation. Stored as an annotation (not a label)
	// to avoid Prometheus metric label churn.
	lastReconciledKey = "last-reconciled"

	// defaultStaleProbeTTL is how long a probe can go without being reconciled
	// before the GC loop considers it stale and deletes it.
	// Override with PROBE_STALE_TTL env var (e.g., "15m", "1h").
	defaultStaleProbeTTL = 15 * time.Minute

	// defaultNoHeartbeatProbeTTL is how long a probe can exist without ever
	// receiving a last-reconciled heartbeat before GC deletes it.
	// Override with PROBE_UNLABELED_TTL env var (e.g., "24h", "48h").
	defaultNoHeartbeatProbeTTL = 24 * time.Hour
)

// KubernetesProbeStore implements the ProbeStorage interface using Kubernetes ConfigMaps.
type KubernetesProbeStore struct {
	Client             kubernetes.Interface
	Namespace          string
	StaleProbeTTL      time.Duration
	NoHeartbeatProbeTTL time.Duration
}

// NewKubernetesProbeStore creates a new KubernetesProbeStore.
// The namespace existence is not checked here; it is assumed to exist.
// RBAC permissions for the service account only allow for namespaced resource access,
// so a cluster-level check for a namespace is not possible and also redundant.
func NewKubernetesProbeStore(ctx context.Context, client kubernetes.Interface, namespace string) (*KubernetesProbeStore, error) {
	staleTTL := defaultStaleProbeTTL
	if v := os.Getenv("PROBE_STALE_TTL"); v != "" {
		parsed, err := time.ParseDuration(v)
		if err != nil {
			log.Printf("Warning: invalid PROBE_STALE_TTL %q, using default %s: %v", v, defaultStaleProbeTTL, err)
		} else {
			staleTTL = parsed
			log.Printf("Using custom PROBE_STALE_TTL: %s", staleTTL)
		}
	}
	noHeartbeatTTL := defaultNoHeartbeatProbeTTL
	if v := os.Getenv("PROBE_UNLABELED_TTL"); v != "" {
		parsed, err := time.ParseDuration(v)
		if err != nil {
			log.Printf("Warning: invalid PROBE_UNLABELED_TTL %q, using default %s: %v", v, defaultNoHeartbeatProbeTTL, err)
		} else {
			noHeartbeatTTL = parsed
			log.Printf("Using custom PROBE_UNLABELED_TTL: %s", noHeartbeatTTL)
		}
	}
	log.Printf("Initializing Kubernetes probe store in namespace %q (stale TTL: %s, no-heartbeat TTL: %s)", namespace, staleTTL, noHeartbeatTTL)
	return &KubernetesProbeStore{
		Client:             client,
		Namespace:          namespace,
		StaleProbeTTL:      staleTTL,
		NoHeartbeatProbeTTL: noHeartbeatTTL,
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
	cmAnnotations := make(map[string]string)
	if probe.Labels != nil {
		for key, val := range *probe.Labels {
			if key == lastReconciledKey {
				cmAnnotations[key] = val
			} else {
				cmLabels[key] = val
			}
		}
	}
	// Add our base app label from the constant
	cmLabels[baseAppLabelKey] = baseAppLabelValue
	cmLabels[probeURLHashLabelKey] = urlHashString
	cmLabels[probeStatusLabelKey] = string(probe.Status)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        configMapName,
			Namespace:   k.Namespace,
			Labels:      cmLabels,
			Annotations: cmAnnotations,
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

	// Update labels and annotations, ensuring base labels are preserved.
	// last-reconciled goes to annotations to avoid Prometheus label churn.
	if cm.Labels == nil {
		cm.Labels = make(map[string]string)
	}
	if cm.Annotations == nil {
		cm.Annotations = make(map[string]string)
	}
	if probe.Labels != nil {
		for key, val := range *probe.Labels {
			if key == lastReconciledKey {
				cm.Annotations[key] = val
			} else {
				cm.Labels[key] = val
			}
		}
	}
	// Migrate: remove last-reconciled from labels if it was there before
	delete(cm.Labels, lastReconciledKey)
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
	// Exclude probes in terminating or failed status -- these are effectively
	// inactive and should not block creation of a new probe for the same URL.
	for _, cm := range existingProbes.Items {
		status := cm.Labels[probeStatusLabelKey]
		if status != string(v1.Terminating) && status != string(v1.Failed) {
			return true, nil
		}
	}
	return false, nil
}

// GarbageCollectStaleProbes deletes probe ConfigMaps that are orphaned:
// 1. Probes with a last-reconciled timestamp older than StaleProbeTTL (default 15m)
// 2. Probes without a last-reconciled heartbeat that are older than NoHeartbeatProbeTTL
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
		// Check annotations first (current), fall back to labels (pre-migration)
		lastReconciledStr, ok := cm.Annotations[lastReconciledKey]
		if !ok {
			lastReconciledStr, ok = cm.Labels[lastReconciledKey]
		}
		if !ok {
			// No heartbeat at all -- check if the probe is old enough
			// to be considered abandoned (e.g., from a non-RHOBS-enabled sector
			// that will never get heartbeats).
			if !cm.CreationTimestamp.IsZero() && now.Sub(cm.CreationTimestamp.Time) > k.NoHeartbeatProbeTTL {
				if err := k.transitionToTerminating(ctx, &cm, "no heartbeat ever received"); err != nil {
					log.Printf("GC: failed to transition no-heartbeat probe %s to terminating: %v", cm.Name, err)
					continue
				}
				deleted++
			}
			continue
		}

		lastReconciled, err := time.Parse("20060102T150405Z", lastReconciledStr)
		if err != nil {
			log.Printf("GC: could not parse last-reconciled %q on configmap %s, skipping: %v", lastReconciledStr, cm.Name, err)
			continue
		}

		if now.Sub(lastReconciled) <= k.StaleProbeTTL {
			continue // still fresh
		}

		// Probe is stale -- transition to terminating so the agent can clean up the Probe CR
		if err := k.transitionToTerminating(ctx, &cm, fmt.Sprintf("stale heartbeat %s", lastReconciledStr)); err != nil {
			log.Printf("GC: failed to transition stale probe %s to terminating: %v", cm.Name, err)
			continue
		}
		deleted++
	}

	return deleted, nil
}

// transitionToTerminating sets a probe's status to terminating instead of deleting
// it directly. This allows the synthetics-agent to see the terminating probe and
// clean up the corresponding Probe CR on the backplane/cell before the probe is
// fully removed from the API.
func (k *KubernetesProbeStore) transitionToTerminating(ctx context.Context, cm *corev1.ConfigMap, reason string) error {
	currentStatus := cm.Labels[probeStatusLabelKey]
	if currentStatus == string(v1.Terminating) {
		// Already terminating -- delete it (agent had its chance)
		log.Printf("GC: deleting already-terminating probe %s (%s)", cm.Name, reason)
		return k.Client.CoreV1().ConfigMaps(k.Namespace).Delete(ctx, cm.Name, metav1.DeleteOptions{})
	}

	// Transition to terminating
	cm.Labels[probeStatusLabelKey] = string(v1.Terminating)
	_, err := k.Client.CoreV1().ConfigMaps(k.Namespace).Update(ctx, cm, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update status to terminating: %w", err)
	}
	log.Printf("GC: transitioned probe %s to terminating (%s)", cm.Name, reason)
	return nil
}
