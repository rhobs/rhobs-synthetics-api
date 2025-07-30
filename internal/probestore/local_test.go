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
)

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
