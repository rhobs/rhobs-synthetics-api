package probestore

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	v1 "github.com/rhobs/rhobs-synthetics-api/pkg/apis/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	localProbeStoreDir = "data"
)

// LocalProbeStore implements the ProbeStorage interface using the local filesystem.
// It stores each probe as a separate JSON file in a directory.
type LocalProbeStore struct {
	Directory string
}

// NewLocalProbeStore creates a new LocalProbeStore with the default data directory.
func NewLocalProbeStore() (*LocalProbeStore, error) {
	return NewLocalProbeStoreWithDir(localProbeStoreDir)
}

// NewLocalProbeStoreWithDir creates a new LocalProbeStore with a custom directory.
func NewLocalProbeStoreWithDir(dataDir string) (*LocalProbeStore, error) {
	if dataDir == "" {
		dataDir = localProbeStoreDir // fallback to default
	}

	// Check if directory exists
	if _, err := os.Stat(dataDir); err != nil {
		if os.IsNotExist(err) {
			// Directory doesn't exist, create it
			if err := os.MkdirAll(dataDir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create probe store directory: %w", err)
			}
			log.Printf("Created local probe store directory %q", dataDir)
		} else {
			// Some other error occurred while checking
			return nil, fmt.Errorf("failed to check probe store directory: %w", err)
		}
	} else {
		log.Printf("Using existing local probe store directory %q", dataDir)
	}

	// Validate that the directory is writable
	testFile := filepath.Join(dataDir, ".write_test")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return nil, fmt.Errorf("probe store directory is not writable: %w", err)
	}
	os.Remove(testFile) //nolint:errcheck

	return &LocalProbeStore{Directory: dataDir}, nil
}

// ListProbes lists all probes that match the given label selector.
func (l *LocalProbeStore) ListProbes(ctx context.Context, selector string) ([]v1.ProbeObject, error) {
	sel, err := labels.Parse(selector)
	if err != nil {
		return nil, fmt.Errorf("failed to parse label selector: %w", err)
	}

	probes := []v1.ProbeObject{}
	var skippedFiles []string

	walkErr := filepath.WalkDir(l.Directory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("error accessing path %s: %w", path, err)
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("Warning: Error reading probe file %s: %v", path, err)
			skippedFiles = append(skippedFiles, path)
			return nil // Continue walking, but track skipped files
		}

		var probe v1.ProbeObject
		if err := json.Unmarshal(data, &probe); err != nil {
			log.Printf("Warning: Error unmarshaling probe from file %s: %v", path, err)
			skippedFiles = append(skippedFiles, path)
			return nil // Continue walking, but track skipped files
		}

		// Handle nil labels gracefully
		probeLabels := labels.Set{}
		if probe.Labels != nil {
			probeLabels = labels.Set(*probe.Labels)
		}

		if sel.Matches(probeLabels) {
			probes = append(probes, probe)
		}

		return nil
	})

	if walkErr != nil {
		return nil, fmt.Errorf("error walking probe store directory: %w", walkErr)
	}

	if len(skippedFiles) > 0 {
		log.Printf("Warning: Skipped %d corrupted or unreadable probe files", len(skippedFiles))
	}

	return probes, nil
}

// GetProbe retrieves a single probe by its ID.
func (l *LocalProbeStore) GetProbe(ctx context.Context, probeID uuid.UUID) (*v1.ProbeObject, error) {
	filePath := filepath.Join(l.Directory, probeID.String()+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, k8serrors.NewNotFound(schema.GroupResource{Group: "rhobs-synthetics", Resource: "probes"}, probeID.String())
		}
		return nil, fmt.Errorf("failed to read probe file: %w", err)
	}

	var probe v1.ProbeObject
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("failed to unmarshal probe: %w", err)
	}

	return &probe, nil
}

// CreateProbe creates a new probe, storing it as a JSON file.
func (l *LocalProbeStore) CreateProbe(ctx context.Context, probe v1.ProbeObject, urlHashString string) (*v1.ProbeObject, error) {
	// Validate input
	if probe.Id == (uuid.UUID{}) {
		return nil, fmt.Errorf("probe ID cannot be empty")
	}
	if urlHashString == "" {
		return nil, fmt.Errorf("URL hash cannot be empty")
	}

	// Check for existing probe with same URL hash
	exists, err := l.ProbeWithURLHashExists(ctx, urlHashString)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing probe with URL hash: %w", err)
	}
	if exists {
		return nil, k8serrors.NewAlreadyExists(schema.GroupResource{Group: "rhobs-synthetics", Resource: "probes"}, "probe with same static_url")
	}

	// Initialize labels if nil and add system labels
	if probe.Labels == nil {
		probe.Labels = &v1.LabelsSchema{}
	}
	(*probe.Labels)[probeURLHashLabelKey] = urlHashString
	(*probe.Labels)[baseAppLabelKey] = baseAppLabelValue
	(*probe.Labels)[probeStatusLabelKey] = string(probe.Status)

	filePath := filepath.Join(l.Directory, probe.Id.String()+".json")

	// Check if file already exists
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		return nil, k8serrors.NewAlreadyExists(schema.GroupResource{Group: "rhobs-synthetics", Resource: "probes"}, probe.Id.String())
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(probe, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal probe: %w", err)
	}

	// Write file atomically by writing to temp file then renaming
	tempPath := filePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write probe file: %w", err)
	}

	if err := os.Rename(tempPath, filePath); err != nil {
		os.Remove(tempPath) //nolint:errcheck
		return nil, fmt.Errorf("failed to finalize probe file: %w", err)
	}

	// TODO: Tune logging level for this
	log.Printf("Created probe %s with URL hash %s", probe.Id.String(), urlHashString)
	return &probe, nil
}

// UpdateProbe updates an existing probe's JSON file.
func (l *LocalProbeStore) UpdateProbe(ctx context.Context, probe v1.ProbeObject) (*v1.ProbeObject, error) {
	// Validate input
	if probe.Id == (uuid.UUID{}) {
		return nil, fmt.Errorf("probe ID cannot be empty")
	}

	filePath := filepath.Join(l.Directory, probe.Id.String()+".json")

	// Check if probe exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, k8serrors.NewNotFound(schema.GroupResource{Group: "rhobs-synthetics", Resource: "probes"}, probe.Id.String())
	}

	// Read existing probe to preserve certain labels (like URL hash)
	existingProbe, err := l.GetProbe(ctx, probe.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to read existing probe: %w", err)
	}

	// Ensure system labels are preserved/updated
	if probe.Labels == nil {
		probe.Labels = &v1.LabelsSchema{}
	}
	(*probe.Labels)[baseAppLabelKey] = baseAppLabelValue
	(*probe.Labels)[probeStatusLabelKey] = string(probe.Status)

	// Preserve URL hash from existing probe if not explicitly set
	if existingProbe.Labels != nil {
		if urlHash, exists := (*existingProbe.Labels)[probeURLHashLabelKey]; exists {
			if _, hasNewHash := (*probe.Labels)[probeURLHashLabelKey]; !hasNewHash {
				(*probe.Labels)[probeURLHashLabelKey] = urlHash
			}
		}
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(probe, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal updated probe: %w", err)
	}

	// Write file atomically
	tempPath := filePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write updated probe file: %w", err)
	}

	if err := os.Rename(tempPath, filePath); err != nil {
		os.Remove(tempPath) //nolint:errcheck
		return nil, fmt.Errorf("failed to finalize updated probe file: %w", err)
	}

	// TODO: Tune logging level for this
	log.Printf("Updated probe %s", probe.Id.String())
	return &probe, nil
}

// DeleteProbe deletes a probe's JSON file.
func (l *LocalProbeStore) DeleteProbe(ctx context.Context, probeID uuid.UUID) error {
	// Validate input
	if probeID == (uuid.UUID{}) {
		return fmt.Errorf("probe ID cannot be empty")
	}

	filePath := filepath.Join(l.Directory, probeID.String()+".json")

	// Check if file exists before attempting deletion
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return k8serrors.NewNotFound(schema.GroupResource{Group: "rhobs-synthetics", Resource: "probes"}, probeID.String())
	}

	// Attempt to delete the file
	err := os.Remove(filePath)
	if err != nil {
		return fmt.Errorf("failed to delete probe file: %w", err)
	}

	// TODO: Tune logging level for this
	log.Printf("Deleted probe %s", probeID.String())
	return nil
}

// ProbeWithURLHashExists checks if a probe with the given URL hash already exists.
// This is optimized to stop at the first match rather than scanning all files.
func (l *LocalProbeStore) ProbeWithURLHashExists(ctx context.Context, urlHashString string) (bool, error) {
	var found bool
	walkErr := filepath.WalkDir(l.Directory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("Warning: Error reading probe file %s: %v", path, err)
			return nil // Continue walking
		}

		var probe v1.ProbeObject
		if err := json.Unmarshal(data, &probe); err != nil {
			log.Printf("Warning: Error unmarshaling probe from file %s: %v", path, err)
			return nil // Continue walking
		}

		// Check if this probe has the URL hash we're looking for
		if probe.Labels != nil {
			if hashValue, exists := (*probe.Labels)[probeURLHashLabelKey]; exists && hashValue == urlHashString {
				found = true
				return filepath.SkipAll // Stop walking, we found what we need
			}
		}

		return nil
	})

	if walkErr != nil {
		return false, fmt.Errorf("error checking for existing probe with URL hash: %w", walkErr)
	}

	return found, nil
}
