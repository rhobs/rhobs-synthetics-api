package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/rhobs/rhobs-synthetics-api/internal/probestore"
	v1 "github.com/rhobs/rhobs-synthetics-api/pkg/apis/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	baseAppLabelKey   = "app"
	baseAppLabelValue = "rhobs-synthetics-probe"
)

// Server is the main API server object.
type Server struct {
	Store probestore.ProbeStorage
}

// NewServer creates a new API server.
func NewServer(store probestore.ProbeStorage) Server {
	return Server{
		Store: store,
	}
}

// (GET /metrics/probes)
func (s Server) ListProbes(ctx context.Context, request v1.ListProbesRequestObject) (v1.ListProbesResponseObject, error) {
	baseSelector := fmt.Sprintf("%s=%s", baseAppLabelKey, baseAppLabelValue)
	finalSelector := baseSelector

	// If the user provided a selector, validate and append it
	if request.Params.LabelSelector != nil && *request.Params.LabelSelector != "" {
		userSelector := *request.Params.LabelSelector
		// Validate the user-provided selector syntax
		_, err := labels.Parse(userSelector)
		if err != nil {
			return v1.ListProbes400JSONResponse{
				Error: v1.ErrorObject{
					Message: fmt.Sprintf("invalid label_selector: %v", err),
				},
			}, nil
		}
		finalSelector = fmt.Sprintf("%s,%s", baseSelector, userSelector)
	}

	probes, err := s.Store.ListProbes(ctx, finalSelector)
	if err != nil {
		return nil, fmt.Errorf("failed to list probes from storage: %w", err)
	}

	return v1.ListProbes200JSONResponse(v1.ProbesArrayResponse{Probes: probes}), nil
}

// (GET /metrics/probes/{probe_id})
func (s Server) GetProbeById(ctx context.Context, request v1.GetProbeByIdRequestObject) (v1.GetProbeByIdResponseObject, error) {
	probe, err := s.Store.GetProbe(ctx, request.ProbeId)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return v1.GetProbeById404JSONResponse{
				Warning: v1.WarningObject{
					Message: fmt.Sprintf("probe with ID %s not found", request.ProbeId),
				},
			}, nil
		}
		return nil, fmt.Errorf("failed to get probe from storage: %w", err)
	}

	return v1.GetProbeById200JSONResponse(*probe), nil
}

// (POST /metrics/probes)
func (s Server) CreateProbe(ctx context.Context, request v1.CreateProbeRequestObject) (v1.CreateProbeResponseObject, error) {
	urlHash := sha256.Sum256([]byte(request.Body.StaticUrl))
	urlHashString := hex.EncodeToString(urlHash[:])[:63]

	exists, err := s.Store.ProbeWithURLHashExists(ctx, urlHashString)
	if err != nil {
		return nil, fmt.Errorf("failed to check for existing probes: %w", err)
	}

	if exists {
		return v1.CreateProbe409JSONResponse{
			Error: v1.ErrorObject{
				Message: fmt.Sprintf("a probe for static_url %q already exists", request.Body.StaticUrl),
			},
		}, nil
	}

	probeToStore := v1.ProbeObject{
		Id:        uuid.New(),
		StaticUrl: request.Body.StaticUrl,
		Labels:    request.Body.Labels,
		Status:    v1.Pending, // Default status to pending
	}

	createdProbe, err := s.Store.CreateProbe(ctx, probeToStore, urlHashString)
	if err != nil {
		return v1.CreateProbe500JSONResponse{
			Error: v1.ErrorObject{
				Message: fmt.Sprintf("failed to create probe: %v", err),
			},
		}, nil
	}

	log.Printf("Successfully created probe and config map for probe ID: %s", createdProbe.Id)
	return v1.CreateProbe201JSONResponse(*createdProbe), nil
}

// (PATCH /metrics/probes/{probe_id})
func (s Server) UpdateProbe(ctx context.Context, request v1.UpdateProbeRequestObject) (v1.UpdateProbeResponseObject, error) {
	// First, get the existing probe.
	existingProbe, err := s.Store.GetProbe(ctx, request.ProbeId)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return v1.UpdateProbe404JSONResponse{
				Warning: v1.WarningObject{
					Message: fmt.Sprintf("probe with ID %s not found", request.ProbeId),
				},
			}, nil
		}
		return nil, fmt.Errorf("failed to get probe from storage for update: %w", err)
	}

	// Now, update the fields from the request.
	if request.Body.Status != nil {
		existingProbe.Status = *request.Body.Status
	}

	// Persist the updated probe.
	updatedProbe, err := s.Store.UpdateProbe(ctx, *existingProbe)
	if err != nil {
		return nil, fmt.Errorf("failed to update probe in storage: %w", err)
	}

	return v1.UpdateProbe200JSONResponse(*updatedProbe), nil
}

// (DELETE /metrics/probes/{probe_id})
func (s Server) DeleteProbe(ctx context.Context, request v1.DeleteProbeRequestObject) (v1.DeleteProbeResponseObject, error) {
	err := s.Store.DeleteProbe(ctx, request.ProbeId)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return v1.DeleteProbe404JSONResponse{
				Warning: v1.WarningObject{
					Message: fmt.Sprintf("probe with ID %s not found", request.ProbeId),
				},
			}, nil
		}
		return nil, fmt.Errorf("failed to delete probe from storage: %w", err)
	}

	log.Printf("Successfully deleted probe for probe ID: %s", request.ProbeId)
	return v1.DeleteProbe204Response{}, nil
}
