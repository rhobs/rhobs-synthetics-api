package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/google/uuid"
)

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
	dummyProbe := ProbeObject{
		Id:                  clusterId,
		ApiserverUrl:        "https://api.example.com/cluster-1",
		ManagementClusterId: managementClusterId,
		Private:             false,
	}

	responseStruct := ProbesArrayResponse{
		Probes: []ProbeObject{
			dummyProbe,
		},
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(responseStruct); err != nil {
		// Handle JSON encoding errors
		http.Error(w, "Internal Server Error: Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// (GET /metrics/probe/{cluster_id})
func (Server) GetProbeById(w http.ResponseWriter, r *http.Request, clusterId ClusterIdPathParam) {
	// Fake respone while we wire things together
	managementClusterId, err := uuid.Parse("957c5277-f74c-4b24-938a-f70bab28aab5")
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	dummyProbe := ProbeObject{
		Id:                  clusterId,
		ApiserverUrl:        "https://api.example.com/cluster-1",
		ManagementClusterId: managementClusterId,
		Private:             false,
	}

	responseStruct := ProbesArrayResponse{
		Probes: []ProbeObject{
			dummyProbe,
		},
	}

	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(responseStruct); err != nil {
		// Handle JSON encoding errors
		http.Error(w, "Internal Server Error: Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// (POST /metrics/probes)
func (Server) CreateProbe(w http.ResponseWriter, r *http.Request) {
	var createReq CreateProbeRequest

	// Fake respone while we wire things together
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&createReq); err != nil {
		log.Printf("ERROR: Failed to decode CreateProbeRequest: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	createdProbe := ProbeObject{
		Id:                  createReq.ClusterId,
		ApiserverUrl:        createReq.ApiserverUrl,
		ManagementClusterId: createReq.ManagementClusterId,
		Private:             createReq.Private,
	}

	responseBody := ProbesArrayResponse{
		Probes: []ProbeObject{createdProbe},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(responseBody); err != nil {
		log.Printf("ERROR: Failed to encode CreateProbe response: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	log.Printf("Successfully created probe for cluster ID: %s", createdProbe.Id)

}

// (DELETE /metrics/probe/{cluster_id})
func (Server) DeleteProbe(w http.ResponseWriter, r *http.Request, clusterId ClusterIdPathParam) {
	// Fake respone while we wire things together
	w.WriteHeader(http.StatusOK)
}
