package probestore

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	v1 "github.com/rhobs/rhobs-synthetics-api/pkg/apis/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

// Helper function to create a test probe
func createTestProbe(id uuid.UUID) v1.ProbeObject {
	if id == (uuid.UUID{}) {
		id = uuid.New()
	}
	return v1.ProbeObject{
		Id:        id,
		StaticUrl: "http://example.com/test",
		Status:    v1.Pending,
		Labels:    &v1.LabelsSchema{"env": "test"},
	}
}

func TestNewLocalProbeStore(t *testing.T) {
	// Clean up any existing default directory
	if _, err := os.Stat(localProbeStoreDir); err == nil {
		defer os.RemoveAll(localProbeStoreDir) //nolint:errcheck
	}

	store, err := NewLocalProbeStore()

	require.NoError(t, err)
	assert.NotNil(t, store)
	assert.Equal(t, localProbeStoreDir, store.Directory)

	// Verify directory was created
	info, err := os.Stat(localProbeStoreDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestNewLocalProbeStoreWithDir(t *testing.T) {
	testCases := []struct {
		name        string
		dataDir     string
		expectErr   bool
		expectedDir string
	}{
		{
			name:        "creates store with custom directory",
			dataDir:     "/tmp/test-probes-custom",
			expectErr:   false,
			expectedDir: "/tmp/test-probes-custom",
		},
		{
			name:        "falls back to default when empty string provided",
			dataDir:     "",
			expectErr:   false,
			expectedDir: localProbeStoreDir,
		},
		{
			name:      "fails when directory is not writable",
			dataDir:   "/root/test-probes", // Assuming this won't be writable
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Clean up before and after test
			if tc.dataDir != "" && tc.dataDir != localProbeStoreDir {
				defer os.RemoveAll(tc.dataDir) //nolint:errcheck
			}
			if tc.expectedDir == localProbeStoreDir {
				defer os.RemoveAll(localProbeStoreDir) //nolint:errcheck
			}

			store, err := NewLocalProbeStoreWithDir(tc.dataDir)

			if tc.expectErr {
				require.Error(t, err)
				assert.Nil(t, store)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, store)
				assert.Equal(t, tc.expectedDir, store.Directory)

				// Verify directory exists and is writable
				info, err := os.Stat(tc.expectedDir)
				require.NoError(t, err)
				assert.True(t, info.IsDir())
			}
		})
	}
}

func TestLocalProbeStore_CreateProbe(t *testing.T) {
	ctx := context.Background()
	probeID := uuid.New()

	testCases := []struct {
		name      string
		probe     v1.ProbeObject
		urlHash   string
		expectErr bool
		postCheck func(t *testing.T, store *LocalProbeStore, createdProbe *v1.ProbeObject)
	}{
		{
			name: "successfully creates a probe",
			probe: v1.ProbeObject{
				Id:        probeID,
				StaticUrl: "http://example.com/test",
				Status:    v1.Pending,
				Labels:    &v1.LabelsSchema{"env": "test"},
			},
			urlHash:   "test-hash-123",
			expectErr: false,
			postCheck: func(t *testing.T, store *LocalProbeStore, createdProbe *v1.ProbeObject) {
				// Verify system labels were added
				assert.Equal(t, baseAppLabelValue, (*createdProbe.Labels)[baseAppLabelKey])
				assert.Equal(t, "test-hash-123", (*createdProbe.Labels)[probeURLHashLabelKey])
				assert.Equal(t, string(v1.Pending), (*createdProbe.Labels)[probeStatusLabelKey])

				// Test file was actually created
				expectedFile := filepath.Join(store.Directory, probeID.String()+".json")
				_, err := os.Stat(expectedFile)
				assert.NoError(t, err, "Probe file should exist")
			},
		},
		{
			name: "error creating probe with empty ID",
			probe: v1.ProbeObject{
				StaticUrl: "http://example.com/test",
				Status:    v1.Pending,
			},
			urlHash:   "test-hash",
			expectErr: true,
		},
		{
			name: "error creating probe with empty URL hash",
			probe: v1.ProbeObject{
				Id:        uuid.New(),
				StaticUrl: "http://example.com/test",
				Status:    v1.Pending,
			},
			urlHash:   "",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			tempDir, err := os.MkdirTemp("", "probe-store-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir) //nolint:errcheck

			store, err := NewLocalProbeStoreWithDir(tempDir)
			require.NoError(t, err)

			// Act
			createdProbe, err := store.CreateProbe(ctx, tc.probe, tc.urlHash)

			// Assert
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.probe.Id, createdProbe.Id)
				assert.Equal(t, tc.probe.StaticUrl, createdProbe.StaticUrl)
			}

			if tc.postCheck != nil {
				tc.postCheck(t, store, createdProbe)
			}
		})
	}
}

func TestLocalProbeStore_UpdateProbe(t *testing.T) {
	ctx := context.Background()
	probeID := uuid.New()
	initialProbe := v1.ProbeObject{
		Id:        probeID,
		StaticUrl: "http://example.com/initial",
		Status:    v1.Pending,
		Labels:    &v1.LabelsSchema{"env": "test"},
	}

	testCases := []struct {
		name          string
		probeToUpdate v1.ProbeObject
		setupProbe    bool
		expectErr     bool
		postCheck     func(t *testing.T, store *LocalProbeStore, result *v1.ProbeObject)
	}{
		{
			name: "successfully updates a probe",
			probeToUpdate: func() v1.ProbeObject {
				p := initialProbe
				p.Status = v1.Active
				p.Labels = &v1.LabelsSchema{"env": "test", "new": "label"}
				return p
			}(),
			setupProbe: true,
			expectErr:  false,
			postCheck: func(t *testing.T, store *LocalProbeStore, result *v1.ProbeObject) {
				assert.Equal(t, v1.Active, result.Status)
				assert.Equal(t, "label", (*result.Labels)["new"])
				// Verify system labels are preserved
				assert.Equal(t, baseAppLabelValue, (*result.Labels)[baseAppLabelKey])
				assert.Equal(t, string(v1.Active), (*result.Labels)[probeStatusLabelKey])
			},
		},
		{
			name:          "error updating non-existent probe",
			probeToUpdate: v1.ProbeObject{Id: uuid.New()},
			setupProbe:    false,
			expectErr:     true,
		},
		{
			name: "error updating probe with empty ID",
			probeToUpdate: v1.ProbeObject{
				StaticUrl: "http://example.com/test",
				Status:    v1.Active,
			},
			setupProbe: false,
			expectErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			tempDir, err := os.MkdirTemp("", "probe-store-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir) //nolint:errcheck

			store, err := NewLocalProbeStoreWithDir(tempDir)
			require.NoError(t, err)

			if tc.setupProbe {
				_, err = store.CreateProbe(ctx, initialProbe, "test-hash-123")
				require.NoError(t, err)
			}

			// Act
			result, err := store.UpdateProbe(ctx, tc.probeToUpdate)

			// Assert
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.probeToUpdate.Id, result.Id)
			}

			if tc.postCheck != nil {
				tc.postCheck(t, store, result)
			}
		})
	}
}

func TestLocalProbeStore_ProbeWithURLHashExists(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name        string
		hashToCheck string
		setupProbes []struct {
			probe   v1.ProbeObject
			urlHash string
		}
		expectedExists bool
	}{
		{
			name:           "hash does not exist when no probes",
			hashToCheck:    "non-existent-hash",
			setupProbes:    nil,
			expectedExists: false,
		},
		{
			name:        "hash exists after creating probe",
			hashToCheck: "existing-hash",
			setupProbes: []struct {
				probe   v1.ProbeObject
				urlHash string
			}{
				{
					probe: v1.ProbeObject{
						Id:        uuid.New(),
						StaticUrl: "http://example.com/test",
						Status:    v1.Pending,
					},
					urlHash: "existing-hash",
				},
			},
			expectedExists: true,
		},
		{
			name:        "different hash does not exist",
			hashToCheck: "different-hash",
			setupProbes: []struct {
				probe   v1.ProbeObject
				urlHash string
			}{
				{
					probe: v1.ProbeObject{
						Id:        uuid.New(),
						StaticUrl: "http://example.com/test",
						Status:    v1.Pending,
					},
					urlHash: "existing-hash",
				},
			},
			expectedExists: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			tempDir, err := os.MkdirTemp("", "probe-store-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir) //nolint:errcheck

			store, err := NewLocalProbeStoreWithDir(tempDir)
			require.NoError(t, err)

			for _, setup := range tc.setupProbes {
				_, err = store.CreateProbe(ctx, setup.probe, setup.urlHash)
				require.NoError(t, err)
			}

			// Act
			exists, err := store.ProbeWithURLHashExists(ctx, tc.hashToCheck)

			// Assert
			require.NoError(t, err)
			assert.Equal(t, tc.expectedExists, exists)
		})
	}
}

func TestLocalProbeStore_ListProbes(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name        string
		setupProbes []struct {
			probe   v1.ProbeObject
			urlHash string
		}
		selector        string
		expectedCount   int
		expectedProbeID uuid.UUID
	}{
		{
			name: "list all probes with base selector",
			setupProbes: []struct {
				probe   v1.ProbeObject
				urlHash string
			}{
				{
					probe: v1.ProbeObject{
						Id:        uuid.New(),
						StaticUrl: "http://example.com/1",
						Status:    v1.Active,
						Labels:    &v1.LabelsSchema{"env": "prod"},
					},
					urlHash: "hash1",
				},
				{
					probe: v1.ProbeObject{
						Id:        uuid.New(),
						StaticUrl: "http://example.com/2",
						Status:    v1.Pending,
						Labels:    &v1.LabelsSchema{"env": "test"},
					},
					urlHash: "hash2",
				},
			},
			selector:      fmt.Sprintf("%s=%s", baseAppLabelKey, baseAppLabelValue),
			expectedCount: 2,
		},
		{
			name: "filter probes by environment label",
			setupProbes: []struct {
				probe   v1.ProbeObject
				urlHash string
			}{
				{
					probe: v1.ProbeObject{
						Id:        uuid.New(),
						StaticUrl: "http://example.com/1",
						Status:    v1.Active,
						Labels:    &v1.LabelsSchema{"env": "prod"},
					},
					urlHash: "hash1",
				},
				{
					probe: v1.ProbeObject{
						Id:        uuid.New(),
						StaticUrl: "http://example.com/2",
						Status:    v1.Pending,
						Labels:    &v1.LabelsSchema{"env": "test"},
					},
					urlHash: "hash2",
				},
			},
			selector:        "env=prod",
			expectedCount:   1,
			expectedProbeID: uuid.UUID{}, // Will be set dynamically
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			tempDir, err := os.MkdirTemp("", "probe-store-test-*")
			require.NoError(t, err)
			defer os.RemoveAll(tempDir) //nolint:errcheck

			store, err := NewLocalProbeStoreWithDir(tempDir)
			require.NoError(t, err)

			var prodProbeID uuid.UUID
			for _, setup := range tc.setupProbes {
				_, err = store.CreateProbe(ctx, setup.probe, setup.urlHash)
				require.NoError(t, err)

				// Track the prod probe ID for validation
				if setup.probe.Labels != nil && (*setup.probe.Labels)["env"] == "prod" {
					prodProbeID = setup.probe.Id
				}
			}

			// Act
			probes, err := store.ListProbes(ctx, tc.selector)

			// Assert
			require.NoError(t, err)
			assert.Len(t, probes, tc.expectedCount)

			// If filtering for prod environment, verify the correct probe is returned
			if tc.selector == "env=prod" && tc.expectedCount == 1 {
				assert.Equal(t, prodProbeID, probes[0].Id)
			}
		})
	}
}

func TestLocalProbeStore_AdditionalErrorHandling(t *testing.T) {
	ctx := context.Background()

	t.Run("ListProbes with directory scan errors", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "probe-store-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir) //nolint:errcheck

		store, err := NewLocalProbeStoreWithDir(tempDir)
		require.NoError(t, err)

		// Create a file with invalid JSON
		invalidJSONFile := filepath.Join(tempDir, "invalid.json")
		err = os.WriteFile(invalidJSONFile, []byte("{invalid json"), 0644)
		require.NoError(t, err)

		// Create a valid probe for comparison
		validProbe := v1.ProbeObject{
			Id:        uuid.New(),
			StaticUrl: "http://example.com/valid",
			Status:    v1.Active,
		}
		_, err = store.CreateProbe(ctx, validProbe, "valid-hash")
		require.NoError(t, err)

		// ListProbes should skip the invalid file but still return valid probes
		probes, err := store.ListProbes(ctx, fmt.Sprintf("%s=%s", baseAppLabelKey, baseAppLabelValue))
		require.NoError(t, err)
		assert.Len(t, probes, 1)
		assert.Equal(t, validProbe.Id, probes[0].Id)
	})

	t.Run("ProbeWithURLHashExists with malformed files", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "probe-store-test-*")
		require.NoError(t, err)
		defer os.RemoveAll(tempDir) //nolint:errcheck

		store, err := NewLocalProbeStoreWithDir(tempDir)
		require.NoError(t, err)

		// Create a file with invalid JSON
		invalidJSONFile := filepath.Join(tempDir, "invalid.json")
		err = os.WriteFile(invalidJSONFile, []byte("{invalid json"), 0644)
		require.NoError(t, err)

		// Should not find hash in malformed file
		exists, err := store.ProbeWithURLHashExists(ctx, "some-hash")
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestLocalProbeStore_GetProbe(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name        string
		setupProbe  bool
		probeID     uuid.UUID
		expectErr   bool
		checkErr    func(t *testing.T, err error)
		expectedURL string
	}{
		{
			name:        "successfully gets existing probe",
			setupProbe:  true,
			probeID:     uuid.New(),
			expectErr:   false,
			expectedURL: "http://example.com/test",
		},
		{
			name:       "error getting non-existent probe",
			setupProbe: false,
			probeID:    uuid.New(),
			expectErr:  true,
			checkErr: func(t *testing.T, err error) {
				assert.True(t, k8serrors.IsNotFound(err), "expected a 'not found' error")
			},
		},
		{
			name:       "error with empty probe ID",
			setupProbe: false,
			probeID:    uuid.UUID{},
			expectErr:  true,
			checkErr: func(t *testing.T, err error) {
				assert.True(t, k8serrors.IsNotFound(err), "expected a 'not found' error")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			tempDir, err := os.MkdirTemp("", "probe-store-test-*")
			require.NoError(t, err)
			defer func() { _ = os.RemoveAll(tempDir) }()

			store, err := NewLocalProbeStoreWithDir(tempDir)
			require.NoError(t, err)

			if tc.setupProbe {
				// Create a probe first
				probe := createTestProbe(tc.probeID)
				_, err = store.CreateProbe(ctx, probe, "test-hash")
				require.NoError(t, err)
			}

			// Act
			result, err := store.GetProbe(ctx, tc.probeID)

			// Assert
			if tc.expectErr {
				require.Error(t, err)
				assert.Nil(t, result)
				if tc.checkErr != nil {
					tc.checkErr(t, err)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, tc.probeID, result.Id)
				assert.Equal(t, tc.expectedURL, result.StaticUrl)
				assert.Equal(t, v1.Pending, result.Status)
			}
		})
	}
}

func TestLocalProbeStore_DeleteProbe(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name         string
		probeStatus  v1.StatusSchema
		probeID      uuid.UUID
		expectErr    bool
		checkDeleted bool // true if probe should be completely deleted
		checkErr     func(t *testing.T, err error)
	}{
		{
			name:         "successfully deletes pending probe immediately",
			probeStatus:  v1.Pending,
			probeID:      uuid.New(),
			expectErr:    false,
			checkDeleted: true,
		},
		{
			name:         "successfully sets active probe to terminating",
			probeStatus:  v1.Active,
			probeID:      uuid.New(),
			expectErr:    false,
			checkDeleted: false,
		},
		{
			name:         "successfully deletes failed probe immediately",
			probeStatus:  v1.Failed,
			probeID:      uuid.New(),
			expectErr:    false,
			checkDeleted: true,
		},
		{
			name:         "handles already terminating probe gracefully",
			probeStatus:  v1.Terminating,
			probeID:      uuid.New(),
			expectErr:    false,
			checkDeleted: false,
		},
		{
			name:        "error deleting non-existent probe",
			probeStatus: "", // not used
			probeID:     uuid.New(),
			expectErr:   true,
			checkErr: func(t *testing.T, err error) {
				assert.True(t, k8serrors.IsNotFound(err), "expected a 'not found' error")
			},
		},
		{
			name:        "error with empty probe ID",
			probeStatus: "", // not used
			probeID:     uuid.UUID{},
			expectErr:   true,
			checkErr: func(t *testing.T, err error) {
				assert.Contains(t, err.Error(), "probe ID cannot be empty")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			tempDir, err := os.MkdirTemp("", "probe-store-test-*")
			require.NoError(t, err)
			defer func() { _ = os.RemoveAll(tempDir) }()

			store, err := NewLocalProbeStoreWithDir(tempDir)
			require.NoError(t, err)

			// Create probe if needed (skip for error cases)
			if tc.probeStatus != "" {
				probe := v1.ProbeObject{
					Id:        tc.probeID,
					StaticUrl: "http://example.com/test",
					Status:    tc.probeStatus,
					Labels:    &v1.LabelsSchema{"env": "test"},
				}
				_, err = store.CreateProbe(ctx, probe, "test-hash")
				require.NoError(t, err)
			}

			// Act
			err = store.DeleteProbe(ctx, tc.probeID)

			// Assert
			if tc.expectErr {
				require.Error(t, err)
				if tc.checkErr != nil {
					tc.checkErr(t, err)
				}
			} else {
				require.NoError(t, err)

				if tc.checkDeleted {
					// Verify the probe was completely deleted
					_, err = store.GetProbe(ctx, tc.probeID)
					require.Error(t, err, "Probe should be deleted")
					assert.True(t, k8serrors.IsNotFound(err), "expected a 'not found' error for deleted probe")
				} else {
					// Verify the probe still exists but status was updated appropriately
					probe, err := store.GetProbe(ctx, tc.probeID)
					require.NoError(t, err, "Probe should still exist")

					switch tc.probeStatus {
					case v1.Active:
						assert.Equal(t, v1.Terminating, probe.Status, "Active probe should be set to terminating")
					case v1.Terminating:
						assert.Equal(t, v1.Terminating, probe.Status, "Terminating probe should remain terminating")
					}
				}
			}
		})
	}
}
