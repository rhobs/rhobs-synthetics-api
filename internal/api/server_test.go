package api

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/rhobs/rhobs-synthetics-api/internal/probestore"
	v1 "github.com/rhobs/rhobs-synthetics-api/pkg/apis/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// mockProbeStore is a mock implementation of the ProbeStorage interface for testing.
type mockProbeStore struct {
	probes         map[uuid.UUID]v1.ProbeObject
	getProbeErr    error
	updateProbeErr error
}

// Enforce that mockProbeStore implements the ProbeStorage interface.
var _ probestore.ProbeStorage = (*mockProbeStore)(nil)

func (m *mockProbeStore) GetProbe(ctx context.Context, probeID uuid.UUID) (*v1.ProbeObject, error) {
	if m.getProbeErr != nil {
		return nil, m.getProbeErr
	}
	probe, ok := m.probes[probeID]
	if !ok {
		return nil, k8serrors.NewNotFound(schema.GroupResource{}, probeID.String())
	}
	return &probe, nil
}

func (m *mockProbeStore) UpdateProbe(ctx context.Context, probe v1.ProbeObject) (*v1.ProbeObject, error) {
	if m.updateProbeErr != nil {
		return nil, m.updateProbeErr
	}
	if _, ok := m.probes[probe.Id]; !ok {
		return nil, k8serrors.NewNotFound(schema.GroupResource{}, probe.Id.String())
	}
	m.probes[probe.Id] = probe
	return &probe, nil
}

func (m *mockProbeStore) ListProbes(ctx context.Context, selector string) ([]v1.ProbeObject, error) {
	return nil, errors.New("not implemented")
}
func (m *mockProbeStore) CreateProbe(ctx context.Context, probe v1.ProbeObject, urlHashString string) (*v1.ProbeObject, error) {
	return nil, errors.New("not implemented")
}
func (m *mockProbeStore) DeleteProbe(ctx context.Context, probeID uuid.UUID) error {
	return errors.New("not implemented")
}
func (m *mockProbeStore) ProbeWithURLHashExists(ctx context.Context, urlHashString string) (bool, error) {
	return false, errors.New("not implemented")
}

func TestUpdateProbe(t *testing.T) {
	probeID := uuid.New()
	initialProbe := v1.ProbeObject{
		Id:        probeID,
		StaticUrl: "https://example.com",
		Status:    v1.Pending,
	}
	newStatus := v1.Active

	testCases := []struct {
		name             string
		probeID          uuid.UUID
		reqBody          v1.UpdateProbeJSONRequestBody
		store            probestore.ProbeStorage
		expectedResponse v1.UpdateProbeResponseObject
		expectedErr      string
		postCheck        func(t *testing.T, store probestore.ProbeStorage)
	}{
		{
			name:    "successfully updates a probe",
			probeID: probeID,
			reqBody: v1.UpdateProbeJSONRequestBody{Status: &newStatus},
			store: &mockProbeStore{
				probes: map[uuid.UUID]v1.ProbeObject{probeID: initialProbe},
			},
			expectedResponse: v1.UpdateProbe200JSONResponse{
				Id:        probeID,
				StaticUrl: "https://example.com",
				Status:    v1.Active,
			},
			postCheck: func(t *testing.T, store probestore.ProbeStorage) {
				s := store.(*mockProbeStore)
				assert.Equal(t, newStatus, s.probes[probeID].Status)
			},
		},
		{
			name:    "returns 404 when probe does not exist",
			probeID: uuid.New(),
			reqBody: v1.UpdateProbeJSONRequestBody{Status: &newStatus},
			store:   &mockProbeStore{probes: map[uuid.UUID]v1.ProbeObject{}},
			expectedResponse: v1.UpdateProbe404JSONResponse{
				Warning: v1.WarningObject{Message: fmt.Sprintf("probe with ID %s not found", uuid.New().String())}, // Message is dynamic, we'll check the type
			},
		},
		{
			name:    "returns error when getting probe fails",
			probeID: probeID,
			reqBody: v1.UpdateProbeJSONRequestBody{Status: &newStatus},
			store: &mockProbeStore{
				probes:      map[uuid.UUID]v1.ProbeObject{probeID: initialProbe},
				getProbeErr: errors.New("generic get error"),
			},
			expectedErr: "failed to get probe from storage for update: generic get error",
		},
		{
			name:    "returns error when updating probe fails",
			probeID: probeID,
			reqBody: v1.UpdateProbeJSONRequestBody{Status: &newStatus},
			store: &mockProbeStore{
				probes:         map[uuid.UUID]v1.ProbeObject{probeID: initialProbe},
				updateProbeErr: errors.New("generic update error"),
			},
			expectedErr: "failed to update probe in storage: generic update error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			server := NewServer(tc.store)
			req := v1.UpdateProbeRequestObject{
				ProbeId: tc.probeID,
				Body:    &tc.reqBody,
			}

			// Act
			res, err := server.UpdateProbe(context.Background(), req)

			// Assert
			if tc.expectedErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErr)
			} else {
				require.NoError(t, err)
				if _, ok := res.(v1.UpdateProbe404JSONResponse); ok {
					require.IsType(t, tc.expectedResponse, res)
				} else {
					assert.Equal(t, tc.expectedResponse, res)
				}
			}

			if tc.postCheck != nil {
				tc.postCheck(t, tc.store)
			}
		})
	}
}
