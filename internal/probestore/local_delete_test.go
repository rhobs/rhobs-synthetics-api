package probestore

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
)

func TestLocalProbeStore_DeleteProbe(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name       string
		setupProbe bool
		probeID    uuid.UUID
		expectErr  bool
		checkErr   func(t *testing.T, err error)
	}{
		{
			name:       "successfully deletes existing probe",
			setupProbe: true,
			probeID:    uuid.New(),
			expectErr:  false,
		},
		{
			name:       "error deleting non-existent probe",
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

			if tc.setupProbe {
				// Create a probe first
				probe := createTestProbe(tc.probeID)
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
				// Verify the probe was actually deleted
				_, err = store.GetProbe(ctx, tc.probeID)
				assert.True(t, k8serrors.IsNotFound(err))
			}
		})
	}
}