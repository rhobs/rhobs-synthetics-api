package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/rhobs/rhobs-synthetics-api/internal/metrics"
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

// (GET /probes)
func (s Server) ListProbes(ctx context.Context, request v1.ListProbesRequestObject) (v1.ListProbesResponseObject, error) {
	defer metrics.RecordProbestoreRequest("list_probes", time.Now())
	baseSelector := fmt.Sprintf("%s=%s", baseAppLabelKey, baseAppLabelValue)
	finalSelector := baseSelector

	// If the user provided a selector, validate and append it
	if request.Params.LabelSelector != nil && *request.Params.LabelSelector != "" {
		userSelector := *request.Params.LabelSelector
		// Validate the user-provided selector syntax
		_, err := labels.Parse(userSelector)
		if err != nil {
			metrics.RecordProbestoreError("list_probes")
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
		metrics.RecordProbestoreError("list_probes")
		log.Printf("Error listing probes from storage: %v", err)
		return nil, fmt.Errorf("failed to list probes from storage: %w", err)
	}

	return v1.ListProbes200JSONResponse(v1.ProbesArrayResponse{Probes: probes}), nil
}

// (GET /probes/{probe_id})
func (s Server) GetProbeById(ctx context.Context, request v1.GetProbeByIdRequestObject) (v1.GetProbeByIdResponseObject, error) {
	defer metrics.RecordProbestoreRequest("get_probe", time.Now())
	probe, err := s.Store.GetProbe(ctx, request.ProbeId)
	if err != nil {
		metrics.RecordProbestoreError("get_probe")
		if k8serrors.IsNotFound(err) {
			return v1.GetProbeById404JSONResponse{
				Warning: v1.WarningObject{
					Message: fmt.Sprintf("probe with ID %s not found", request.ProbeId),
				},
			}, nil
		}
		log.Printf("Error getting probe %s from storage: %v", request.ProbeId, err)
		return nil, fmt.Errorf("failed to get probe from storage: %w", err)
	}

	return v1.GetProbeById200JSONResponse(*probe), nil
}

// (POST /probes)
func (s Server) CreateProbe(ctx context.Context, request v1.CreateProbeRequestObject) (v1.CreateProbeResponseObject, error) {
	defer metrics.RecordProbestoreRequest("create_probe", time.Now())
	urlHash := sha256.Sum256([]byte(request.Body.StaticUrl))
	urlHashString := hex.EncodeToString(urlHash[:])[:63]

	exists, err := s.Store.ProbeWithURLHashExists(ctx, urlHashString)
	if err != nil {
		metrics.RecordProbestoreError("create_probe")
		log.Printf("Error checking for existing probes with URL hash %s: %v", urlHashString, err)
		return nil, fmt.Errorf("failed to check for existing probes: %w", err)
	}

	if exists {
		metrics.RecordProbestoreError("create_probe")
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
		metrics.RecordProbestoreError("create_probe")
		log.Printf("Error creating probe %s: %v", probeToStore.Id, err)
		return v1.CreateProbe500JSONResponse{
			Error: v1.ErrorObject{
				Message: fmt.Sprintf("failed to create probe: %v", err),
			},
		}, nil
	}

	return v1.CreateProbe201JSONResponse(*createdProbe), nil
}

// (PATCH /probes/{probe_id})
func (s Server) UpdateProbe(ctx context.Context, request v1.UpdateProbeRequestObject) (v1.UpdateProbeResponseObject, error) {
	defer metrics.RecordProbestoreRequest("update_probe", time.Now())
	// First, get the existing probe.
	existingProbe, err := s.Store.GetProbe(ctx, request.ProbeId)
	if err != nil {
		metrics.RecordProbestoreError("update_probe")
		if k8serrors.IsNotFound(err) {
			return v1.UpdateProbe404JSONResponse{
				Warning: v1.WarningObject{
					Message: fmt.Sprintf("probe with ID %s not found", request.ProbeId),
				},
			}, nil
		}
		log.Printf("Error getting probe %s from storage for update: %v", request.ProbeId, err)
		return nil, fmt.Errorf("failed to get probe from storage for update: %w", err)
	}

	// Now, update the fields from the request.
	if request.Body.Status != nil {
		existingProbe.Status = *request.Body.Status

		// If status is being set to "deleted", actually delete the probe
		if *request.Body.Status == v1.Deleted {
			err := s.Store.DeleteProbeStorage(ctx, request.ProbeId)
			if err != nil {
				log.Printf("Error deleting probe %s from storage: %v", request.ProbeId, err)
				return nil, fmt.Errorf("failed to delete probe from storage: %w", err)
			}

			// Return the probe as it was before deletion
			return v1.UpdateProbe200JSONResponse(*existingProbe), nil
		}
	}

	// Persist the updated probe (for non-deleted status changes).
	updatedProbe, err := s.Store.UpdateProbe(ctx, *existingProbe)
	if err != nil {
		metrics.RecordProbestoreError("update_probe")
		log.Printf("Error updating probe %s in storage: %v", request.ProbeId, err)
		return nil, fmt.Errorf("failed to update probe in storage: %w", err)
	}

	return v1.UpdateProbe200JSONResponse(*updatedProbe), nil
}

// (DELETE /probes/{probe_id})
func (s Server) DeleteProbe(ctx context.Context, request v1.DeleteProbeRequestObject) (v1.DeleteProbeResponseObject, error) {
	defer metrics.RecordProbestoreRequest("delete_probe", time.Now())
	err := s.Store.DeleteProbe(ctx, request.ProbeId)
	if err != nil {
		metrics.RecordProbestoreError("delete_probe")
		if k8serrors.IsNotFound(err) {
			return v1.DeleteProbe404JSONResponse{
				Warning: v1.WarningObject{
					Message: fmt.Sprintf("probe with ID %s not found", request.ProbeId),
				},
			}, nil
		}
		log.Printf("Error deleting probe %s from storage: %v", request.ProbeId, err)
		return nil, fmt.Errorf("failed to delete probe from storage: %w", err)
	}

	return v1.DeleteProbe204Response{}, nil
}

func (s Server) MonitorProbes(ctx context.Context) {
	log.Printf("Starting probe monitoring")
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	s.updateProbeMetrics(ctx)
	for {
		select {
		case <-ticker.C:
			s.updateProbeMetrics(ctx)
		case <-ctx.Done():
			log.Printf("Stopping probe monitoring")
			return
		}
	}
}

func (s Server) updateProbeMetrics(ctx context.Context) {
	probes, err := s.Store.ListProbes(ctx, "")
	if err != nil {
		log.Printf("error listing probes for metrics: %v", err)
		return
	}
	// Group probes by state and private label
	counts := make(map[string]map[string]int)
	for _, probe := range probes {
		state := string(probe.Status)
		if _, ok := counts[state]; !ok {
			counts[state] = make(map[string]int)
		}
		private := "false"
		if probe.Labels != nil {
			if val, ok := (*probe.Labels)["private"]; ok && val == "true" {
				private = "true"
			}
		}
		counts[state][private]++
	}
	for state, privateMap := range counts {
		for private, count := range privateMap {
			metrics.SetProbesTotal(state, private, count)
		}
	}
}
