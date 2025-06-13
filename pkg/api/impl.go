package api

import (
	"encoding/json"
	"github.com/google/uuid"
	"net/http"
)

// optional code omitted

type Server struct{}

func NewServer() Server {
	return Server{}
}

// (GET /metrics/probes)
func (Server) ListProbes(w http.ResponseWriter, r *http.Request, params ListProbesParams) {
	// Fake respone while we wire things together
	clusterId, err := uuid.Parse("957c5277-f74c-4b24-938a-f70bab28aab5")
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	managementClusterId, err := uuid.Parse("957c5277-f74c-4b24-938a-f70bab28aab5")
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	resp := ProbeObject{
		Id:                  clusterId,
		ManagementClusterId: managementClusterId,
		ApiserverUrl:        "https://example.com",
		Private:             false,
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// (POST /metrics/probes)
func (Server) CreateProbe(w http.ResponseWriter, r *http.Request) {
	// Fake respone while we wire things together
	resp := ProbesArrayResponse{
		Probes: []ProbeObject{},
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// (DELETE /metrics/probe/{cluster_id})
func (Server) DeleteProbe(w http.ResponseWriter, r *http.Request, clusterId ClusterIdPathParam) {
	// Fake respone while we wire things together
	w.WriteHeader(http.StatusOK)
}

// (GET /metrics/probe/{cluster_id})
func (Server) GetProbeById(w http.ResponseWriter, r *http.Request, clusterId ClusterIdPathParam) {
	// Fake respone while we wire things together

	managementClusterID, err := uuid.Parse("957c5277-f74c-4b24-938a-f70bab28")
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	resp := ProbeObject{
		Id:                  clusterId,
		ManagementClusterId: managementClusterID,
		ApiserverUrl:        "https://example.com",
		Private:             false,
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
