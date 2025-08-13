package probestore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	v1 "github.com/rhobs/rhobs-synthetics-api/pkg/apis/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalProbeStore_ErrorHandling(t *testing.T) {
	ctx := context.Background()

	t.Run("NewLocalProbeStoreWithDir with empty string defaults to local dir", func(t *testing.T) {
		// Create temp dir first to avoid creating default 'data' dir
		tempDir, err := os.MkdirTemp("", "probe-store-test-*")
		require.NoError(t, err)
		defer func() { _ = os.RemoveAll(tempDir) }()

		// Test with empty string (should use default)
		store, err := NewLocalProbeStoreWithDir("")
		require.NoError(t, err)
		assert.NotNil(t, store)
		// Clean up the default directory if it was created
		defer func() { _ = os.RemoveAll("data") }()
	})

	t.Run("corrupted probe file handling in ListProbes", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "probe-store-test-*")
		require.NoError(t, err)
		defer func() { _ = os.RemoveAll(tempDir) }()

		store, err := NewLocalProbeStoreWithDir(tempDir)
		require.NoError(t, err)

		// Create a valid probe
		validProbe := createTestProbe(uuid.New())
		_, err = store.CreateProbe(ctx, validProbe, "valid-hash")
		require.NoError(t, err)

		// Create a corrupted probe file
		corruptedFile := filepath.Join(tempDir, uuid.New().String()+".json")
		err = os.WriteFile(corruptedFile, []byte("{invalid json"), 0644)
		require.NoError(t, err)

		// ListProbes should still work and return the valid probe
		probes, err := store.ListProbes(ctx, baseAppLabelKey+"="+baseAppLabelValue)
		require.NoError(t, err)
		assert.Len(t, probes, 1)
		assert.Equal(t, validProbe.Id, probes[0].Id)
	})

	t.Run("unreadable file handling in ListProbes", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "probe-store-test-*")
		require.NoError(t, err)
		defer func() { _ = os.RemoveAll(tempDir) }()

		store, err := NewLocalProbeStoreWithDir(tempDir)
		require.NoError(t, err)

		// Create a valid probe
		validProbe := createTestProbe(uuid.New())
		_, err = store.CreateProbe(ctx, validProbe, "valid-hash")
		require.NoError(t, err)

		// Create an unreadable file (this may not work on all systems)
		unreadableFile := filepath.Join(tempDir, uuid.New().String()+".json")
		err = os.WriteFile(unreadableFile, []byte(`{"valid":"json"}`), 0000)
		if err == nil {
			defer func() { _ = os.Chmod(unreadableFile, 0644) }() // Restore permissions for cleanup
		}

		// ListProbes should still work
		probes, err := store.ListProbes(ctx, baseAppLabelKey+"="+baseAppLabelValue)
		require.NoError(t, err)
		assert.Len(t, probes, 1)
		assert.Equal(t, validProbe.Id, probes[0].Id)
	})

	t.Run("probe with nil labels in ListProbes", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "probe-store-test-*")
		require.NoError(t, err)
		defer func() { _ = os.RemoveAll(tempDir) }()

		store, err := NewLocalProbeStoreWithDir(tempDir)
		require.NoError(t, err)

		// Create a probe with nil labels
		probe := v1.ProbeObject{
			Id:        uuid.New(),
			StaticUrl: "http://example.com/nil-labels",
			Status:    v1.Pending,
			Labels:    nil, // Explicitly nil
		}

		// Manually write the probe file without going through CreateProbe
		// (which would add system labels)
		probeFile := filepath.Join(tempDir, probe.Id.String()+".json")
		probeData := `{"id":"` + probe.Id.String() + `","static_url":"http://example.com/nil-labels","status":"pending"}`
		err = os.WriteFile(probeFile, []byte(probeData), 0644)
		require.NoError(t, err)

		// ListProbes should handle nil labels gracefully
		probes, err := store.ListProbes(ctx, "") // Empty selector matches all
		require.NoError(t, err)
		assert.Len(t, probes, 1)
		assert.Equal(t, probe.Id, probes[0].Id)
	})

	t.Run("invalid label selector in ListProbes", func(t *testing.T) {
		tempDir, err := os.MkdirTemp("", "probe-store-test-*")
		require.NoError(t, err)
		defer func() { _ = os.RemoveAll(tempDir) }()

		store, err := NewLocalProbeStoreWithDir(tempDir)
		require.NoError(t, err)

		// Try to list with invalid selector
		_, err = store.ListProbes(ctx, "invalid selector syntax !@#$%")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse label selector")
	})
}