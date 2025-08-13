package probestore

import (
	"context"
	"os"
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