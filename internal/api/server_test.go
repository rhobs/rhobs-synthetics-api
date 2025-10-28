package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
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
	probes                    map[uuid.UUID]v1.ProbeObject
	getProbeErr               error
	updateProbeErr            error
	listProbesErr             error
	createProbeErr            error
	deleteProbeErr            error
	probeWithURLHashExistsErr error
	urlHashes                 map[string]bool
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
	if m.listProbesErr != nil {
		return nil, m.listProbesErr
	}
	var res []v1.ProbeObject
	for _, p := range m.probes {
		res = append(res, p)
	}
	return res, nil
}

func (m *mockProbeStore) CreateProbe(ctx context.Context, probe v1.ProbeObject, urlHashString string) (*v1.ProbeObject, error) {
	if m.createProbeErr != nil {
		return nil, m.createProbeErr
	}
	if m.probes == nil {
		m.probes = make(map[uuid.UUID]v1.ProbeObject)
	}
	if m.urlHashes == nil {
		m.urlHashes = make(map[string]bool)
	}
	m.probes[probe.Id] = probe
	m.urlHashes[urlHashString] = true
	return &probe, nil
}

func (m *mockProbeStore) DeleteProbe(ctx context.Context, probeID uuid.UUID) error {
	if m.deleteProbeErr != nil {
		return m.deleteProbeErr
	}
	if _, ok := m.probes[probeID]; !ok {
		return k8serrors.NewNotFound(schema.GroupResource{}, probeID.String())
	}
	// Set status to terminating instead of deleting
	probe := m.probes[probeID]
	probe.Status = v1.Terminating
	m.probes[probeID] = probe
	return nil
}

func (m *mockProbeStore) DeleteProbeStorage(ctx context.Context, probeID uuid.UUID) error {
	if m.deleteProbeErr != nil {
		return m.deleteProbeErr
	}
	if _, ok := m.probes[probeID]; !ok {
		return k8serrors.NewNotFound(schema.GroupResource{}, probeID.String())
	}
	delete(m.probes, probeID)
	return nil
}

func (m *mockProbeStore) ProbeWithURLHashExists(ctx context.Context, urlHashString string) (bool, error) {
	if m.probeWithURLHashExistsErr != nil {
		return false, m.probeWithURLHashExistsErr
	}
	_, exists := m.urlHashes[urlHashString]
	return exists, nil
}

func TestListProbes(t *testing.T) {
	probe1ID := uuid.New()
	probe2ID := uuid.New()
	probes := []v1.ProbeObject{
		{Id: probe1ID, StaticUrl: "https://example.com/1"},
		{Id: probe2ID, StaticUrl: "https://example.com/2"},
	}

	testCases := []struct {
		name             string
		params           v1.ListProbesParams
		store            probestore.ProbeStorage
		expectedResponse v1.ListProbesResponseObject
		expectedErr      string
	}{
		{
			name:   "successfully lists probes",
			params: v1.ListProbesParams{},
			store: &mockProbeStore{
				probes: map[uuid.UUID]v1.ProbeObject{
					probe1ID: probes[0],
					probe2ID: probes[1],
				},
			},
			expectedResponse: v1.ListProbes200JSONResponse(v1.ProbesArrayResponse{Probes: probes}),
		},
		{
			name:   "returns 400 for invalid label selector",
			params: v1.ListProbesParams{LabelSelector: func() *string { s := "invalid selector"; return &s }()},
			store:  &mockProbeStore{},
			expectedResponse: v1.ListProbes400JSONResponse{
				Error: v1.ErrorObject{Message: "invalid label_selector: unable to parse requirement: found 'invalid', expected: identifier, '!', 'in', 'notin', '=', '==', '!='"},
			},
		},
		{
			name:   "successfully lists probes with valid label selector",
			params: v1.ListProbesParams{LabelSelector: func() *string { s := "env=prod"; return &s }()},
			store: &mockProbeStore{
				probes: map[uuid.UUID]v1.ProbeObject{
					probe1ID: probes[0],
				},
			},
			expectedResponse: v1.ListProbes200JSONResponse(v1.ProbesArrayResponse{Probes: []v1.ProbeObject{probes[0]}}),
		},
		{
			name:   "returns error when listing fails",
			params: v1.ListProbesParams{},
			store: &mockProbeStore{
				listProbesErr: errors.New("generic list error"),
			},
			expectedErr: "failed to list probes from storage: generic list error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := NewServer(tc.store)
			req := v1.ListProbesRequestObject{Params: tc.params}

			res, err := server.ListProbes(context.Background(), req)

			if tc.expectedErr != "" {
				require.Error(t, err)
				assert.EqualError(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
				if resp400, ok := res.(v1.ListProbes400JSONResponse); ok {
					assert.True(t, strings.HasPrefix(resp400.Error.Message, "invalid label_selector:"))
				} else if resp200, ok := res.(v1.ListProbes200JSONResponse); ok {
					expectedResp, expectedOk := tc.expectedResponse.(v1.ListProbes200JSONResponse)
					require.True(t, expectedOk)
					assert.ElementsMatch(t, expectedResp.Probes, resp200.Probes)
				} else {
					assert.Equal(t, tc.expectedResponse, res)
				}
			}
		})
	}
}

func TestGetProbeById(t *testing.T) {
	probeID := uuid.New()
	probe := v1.ProbeObject{Id: probeID, StaticUrl: "https://example.com"}

	testCases := []struct {
		name             string
		probeID          uuid.UUID
		store            probestore.ProbeStorage
		expectedResponse v1.GetProbeByIdResponseObject
		expectedErr      string
	}{
		{
			name:             "successfully gets a probe",
			probeID:          probeID,
			store:            &mockProbeStore{probes: map[uuid.UUID]v1.ProbeObject{probeID: probe}},
			expectedResponse: v1.GetProbeById200JSONResponse(probe),
		},
		{
			name:             "returns 404 when probe not found",
			probeID:          uuid.New(),
			store:            &mockProbeStore{probes: map[uuid.UUID]v1.ProbeObject{}},
			expectedResponse: v1.GetProbeById404JSONResponse{},
		},
		{
			name:        "returns error when getting fails",
			probeID:     probeID,
			store:       &mockProbeStore{getProbeErr: errors.New("generic get error")},
			expectedErr: "failed to get probe from storage: generic get error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := NewServer(tc.store)
			req := v1.GetProbeByIdRequestObject{ProbeId: tc.probeID}

			res, err := server.GetProbeById(context.Background(), req)

			if tc.expectedErr != "" {
				require.Error(t, err)
				assert.EqualError(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
				if _, ok := res.(v1.GetProbeById404JSONResponse); ok {
					assert.IsType(t, tc.expectedResponse, res)
				} else {
					assert.Equal(t, tc.expectedResponse, res)
				}
			}
		})
	}
}

func TestCreateProbe(t *testing.T) {
	newURL := "https://example.com/new"
	urlHashBytes := sha256.Sum256([]byte(newURL))
	urlHashString := hex.EncodeToString(urlHashBytes[:])[:63]

	testCases := []struct {
		name             string
		reqBody          v1.CreateProbeJSONRequestBody
		store            probestore.ProbeStorage
		expectedResponse v1.CreateProbeResponseObject
		expectedErr      string
	}{
		{
			name:             "successfully creates a probe",
			reqBody:          v1.CreateProbeJSONRequestBody{StaticUrl: newURL},
			store:            &mockProbeStore{},
			expectedResponse: v1.CreateProbe201JSONResponse{},
		},
		{
			name:             "returns 409 when url hash exists",
			reqBody:          v1.CreateProbeJSONRequestBody{StaticUrl: newURL},
			store:            &mockProbeStore{urlHashes: map[string]bool{urlHashString: true}},
			expectedResponse: v1.CreateProbe409JSONResponse{},
		},
		{
			name:    "returns error when checking url hash fails",
			reqBody: v1.CreateProbeJSONRequestBody{StaticUrl: newURL},
			store: &mockProbeStore{
				probeWithURLHashExistsErr: errors.New("generic hash check error"),
			},
			expectedErr: "failed to check for existing probes: generic hash check error",
		},
		{
			name:    "returns error when creating probe fails",
			reqBody: v1.CreateProbeJSONRequestBody{StaticUrl: newURL},
			store: &mockProbeStore{
				createProbeErr: errors.New("generic create error"),
			},
			expectedResponse: v1.CreateProbe500JSONResponse{Error: v1.ErrorObject{Message: "failed to create probe: generic create error"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := NewServer(tc.store)
			req := v1.CreateProbeRequestObject{Body: &tc.reqBody}

			res, err := server.CreateProbe(context.Background(), req)

			if tc.expectedErr != "" {
				require.Error(t, err)
				assert.EqualError(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
				assert.IsType(t, tc.expectedResponse, res)
				if resp201, ok := res.(v1.CreateProbe201JSONResponse); ok {
					assert.Equal(t, newURL, resp201.StaticUrl)
				}
			}
		})
	}
}

func TestDeleteProbe(t *testing.T) {
	probeID := uuid.New()

	testCases := []struct {
		name             string
		probeID          uuid.UUID
		store            probestore.ProbeStorage
		expectedResponse v1.DeleteProbeResponseObject
		expectedErr      string
	}{
		{
			name:             "successfully deletes a probe",
			probeID:          probeID,
			store:            &mockProbeStore{probes: map[uuid.UUID]v1.ProbeObject{probeID: {}}},
			expectedResponse: v1.DeleteProbe204Response{},
		},
		{
			name:             "returns 404 when probe not found",
			probeID:          uuid.New(),
			store:            &mockProbeStore{probes: map[uuid.UUID]v1.ProbeObject{}},
			expectedResponse: v1.DeleteProbe404JSONResponse{},
		},
		{
			name:        "returns error when deleting fails",
			probeID:     probeID,
			store:       &mockProbeStore{deleteProbeErr: errors.New("generic delete error")},
			expectedErr: "failed to delete probe from storage: generic delete error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := NewServer(tc.store)
			req := v1.DeleteProbeRequestObject{ProbeId: tc.probeID}

			res, err := server.DeleteProbe(context.Background(), req)

			if tc.expectedErr != "" {
				require.Error(t, err)
				assert.EqualError(t, err, tc.expectedErr)
			} else {
				require.NoError(t, err)
				assert.IsType(t, tc.expectedResponse, res)
			}
		})
	}
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
			name:    "allows status field updates (RMO can set terminating, agents can set active/failed)",
			probeID: probeID,
			reqBody: v1.UpdateProbeJSONRequestBody{Status: &newStatus},
			store: &mockProbeStore{
				probes: map[uuid.UUID]v1.ProbeObject{probeID: initialProbe},
			},
			expectedResponse: v1.UpdateProbe200JSONResponse{
				Id:        probeID,
				StaticUrl: "https://example.com",
				Status:    newStatus,
			},
		},
		{
			name:    "returns 404 when probe does not exist (testing with labels)",
			probeID: uuid.New(),
			reqBody: v1.UpdateProbeJSONRequestBody{Labels: &v1.LabelsSchema{"environment": "test"}},
			store:   &mockProbeStore{probes: map[uuid.UUID]v1.ProbeObject{}},
			expectedResponse: v1.UpdateProbe404JSONResponse{
				Warning: v1.WarningObject{Message: fmt.Sprintf("probe with ID %s not found", uuid.New().String())}, // Message is dynamic, we'll check the type
			},
		},
		{
			name:    "returns error when getting probe fails",
			probeID: probeID,
			reqBody: v1.UpdateProbeJSONRequestBody{Labels: &v1.LabelsSchema{"environment": "test"}},
			store: &mockProbeStore{
				probes:      map[uuid.UUID]v1.ProbeObject{probeID: initialProbe},
				getProbeErr: errors.New("generic get error"),
			},
			expectedErr: "failed to get probe from storage for update: generic get error",
		},
		{
			name:    "returns error when updating probe fails",
			probeID: probeID,
			reqBody: v1.UpdateProbeJSONRequestBody{Labels: &v1.LabelsSchema{"environment": "test"}},
			store: &mockProbeStore{
				probes:         map[uuid.UUID]v1.ProbeObject{probeID: initialProbe},
				updateProbeErr: errors.New("generic update error"),
			},
			expectedErr: "failed to update probe in storage: generic update error",
		},
		{
			name:    "successfully deletes probe when status set to deleted",
			probeID: probeID,
			reqBody: v1.UpdateProbeJSONRequestBody{Status: &[]v1.StatusSchema{v1.Deleted}[0]},
			store: &mockProbeStore{
				probes: map[uuid.UUID]v1.ProbeObject{
					probeID: {Id: probeID, StaticUrl: "https://example.com", Status: v1.Terminating},
				},
			},
			expectedResponse: v1.UpdateProbe200JSONResponse{
				Id:        probeID,
				StaticUrl: "https://example.com",
				Status:    v1.Deleted,
			},
			postCheck: func(t *testing.T, store probestore.ProbeStorage) {
				// Verify the probe was actually deleted from the store
				s := store.(*mockProbeStore)
				_, exists := s.probes[probeID]
				assert.False(t, exists, "Probe should have been actually deleted from store")
			},
		},
		{
			name:    "successfully updates user labels",
			probeID: probeID,
			reqBody: v1.UpdateProbeJSONRequestBody{Labels: &v1.LabelsSchema{"environment": "prod", "team": "sre"}},
			store: &mockProbeStore{
				probes: map[uuid.UUID]v1.ProbeObject{probeID: initialProbe},
			},
			expectedResponse: v1.UpdateProbe200JSONResponse{
				Id:        probeID,
				StaticUrl: "https://example.com",
				Status:    v1.Pending,
				Labels:    &v1.LabelsSchema{"environment": "prod", "team": "sre"},
			},
			postCheck: func(t *testing.T, store probestore.ProbeStorage) {
				s := store.(*mockProbeStore)
				labels := s.probes[probeID].Labels
				assert.NotNil(t, labels)
				assert.Equal(t, "prod", (*labels)["environment"])
				assert.Equal(t, "sre", (*labels)["team"])
			},
		},
		{
			name:    "returns 403 when trying to create protected label: app",
			probeID: probeID,
			reqBody: v1.UpdateProbeJSONRequestBody{Labels: &v1.LabelsSchema{"app": "malicious-app"}},
			store: &mockProbeStore{
				probes: map[uuid.UUID]v1.ProbeObject{probeID: initialProbe},
			},
			expectedResponse: v1.UpdateProbe403JSONResponse{
				Error: v1.ErrorObject{Message: "creation of system-managed label 'app' is forbidden"},
			},
		},
		{
			name:    "returns 403 when trying to create protected label: rhobs-synthetics/status",
			probeID: probeID,
			reqBody: v1.UpdateProbeJSONRequestBody{Labels: &v1.LabelsSchema{"rhobs-synthetics/status": "hacked"}},
			store: &mockProbeStore{
				probes: map[uuid.UUID]v1.ProbeObject{probeID: initialProbe},
			},
			expectedResponse: v1.UpdateProbe403JSONResponse{
				Error: v1.ErrorObject{Message: "creation of system-managed label 'rhobs-synthetics/status' is forbidden"},
			},
		},
		{
			name:    "returns 403 when trying to create protected label: rhobs-synthetics/static-url-hash",
			probeID: probeID,
			reqBody: v1.UpdateProbeJSONRequestBody{Labels: &v1.LabelsSchema{"rhobs-synthetics/static-url-hash": "fakehash"}},
			store: &mockProbeStore{
				probes: map[uuid.UUID]v1.ProbeObject{probeID: initialProbe},
			},
			expectedResponse: v1.UpdateProbe403JSONResponse{
				Error: v1.ErrorObject{Message: "creation of system-managed label 'rhobs-synthetics/static-url-hash' is forbidden"},
			},
		},
		{
			name:    "returns 403 when trying to modify protected label: private",
			probeID: probeID,
			reqBody: v1.UpdateProbeJSONRequestBody{Labels: &v1.LabelsSchema{"private": ""}},
			store: &mockProbeStore{
				probes: map[uuid.UUID]v1.ProbeObject{probeID: initialProbe},
			},
			expectedResponse: v1.UpdateProbe403JSONResponse{
				Error: v1.ErrorObject{Message: "creation of system-managed label 'private' is forbidden"},
			},
		},
		{
			name:    "allows status updates with labels (RMO can set terminating, agents can set active/failed)",
			probeID: probeID,
			reqBody: v1.UpdateProbeJSONRequestBody{
				Status: &newStatus,
				Labels: &v1.LabelsSchema{"environment": "prod"},
			},
			store: &mockProbeStore{
				probes: map[uuid.UUID]v1.ProbeObject{probeID: initialProbe},
			},
			expectedResponse: v1.UpdateProbe200JSONResponse{
				Id:        probeID,
				StaticUrl: "https://example.com",
				Status:    newStatus,
				Labels:    &v1.LabelsSchema{"environment": "prod"},
			},
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

func Test_validateProtectedLabels(t *testing.T) {
	tests := []struct {
		name      string
		old       v1.LabelsSchema
		new       v1.LabelsSchema
		expectErr bool
	}{
		{
			name:      "label 'app' is protected",
			old:       v1.LabelsSchema{baseAppLabelKey: "test"},
			new:       v1.LabelsSchema{baseAppLabelKey: "bad"},
			expectErr: true,
		},
		{
			name:      "label 'rhobs-synthetics/status' is protected",
			old:       v1.LabelsSchema{probeStatusLabelKey: "test"},
			new:       v1.LabelsSchema{probeStatusLabelKey: "bad"},
			expectErr: true,
		},
		{
			name:      "label 'rhobs-synthetics/static-url-hash' is protected",
			old:       v1.LabelsSchema{probeURLHashLabelKey: "test"},
			new:       v1.LabelsSchema{probeURLHashLabelKey: "bad"},
			expectErr: true,
		},
		{
			name:      "label 'private' is protected",
			old:       v1.LabelsSchema{privateProbeLabelKey: "test"},
			new:       v1.LabelsSchema{privateProbeLabelKey: "bad"},
			expectErr: true,
		},
		{
			name:      "protected labels cannot be set if unset",
			old:       v1.LabelsSchema{},
			new:       v1.LabelsSchema{privateProbeLabelKey: "bad"},
			expectErr: true,
		},
		{
			name:      "no error if protected label is unchanged",
			old:       v1.LabelsSchema{privateProbeLabelKey: "test"},
			new:       v1.LabelsSchema{privateProbeLabelKey: "test"},
			expectErr: false,
		},
		{
			name:      "no error new labelschema is empty",
			old:       v1.LabelsSchema{privateProbeLabelKey: "test"},
			new:       v1.LabelsSchema{},
			expectErr: false,
		},
		{
			name:      "no error new labelschema changes unprotected labels",
			old:       v1.LabelsSchema{privateProbeLabelKey: "test"},
			new:       v1.LabelsSchema{"unprotectedLabel": "true"},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateProtectedLabels(tt.new, tt.old)

			if (err != nil) != tt.expectErr {
				t.Errorf("unexpected test result: expectedErr=%t, got err=%v", tt.expectErr, err)
			}
		})
	}
}
