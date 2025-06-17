package api

import (
	"context"
	"log"

	"github.com/google/uuid"
)

type Server struct{}

func NewServer() Server {
	return Server{}
}

// (GET /metrics/probes)
func (Server) ListProbes(ctx context.Context, request ListProbesRequestObject) (ListProbesResponseObject, error) {
	// Fake respone while we wire things together
	clusterId, err := uuid.Parse("957c5277-f74c-4b24-938a-f70bab28aab5")
	if err != nil {
		return ListProbes400JSONResponse{
			Error: struct {
				Code    int32  `json:"code"`
				Message string `json:"message"`
			}{
				Code:    400,
				Message: "Invalid Cluster ID",
			},
		}, nil
	}

	managementClusterId, err := uuid.Parse("957c5277-f74c-4b24-938a-f70bab28aab5")
	if err != nil {
		return ListProbes400JSONResponse{
			Error: struct {
				Code    int32  `json:"code"`
				Message string `json:"message"`
			}{
				Code:    400,
				Message: "Invalid Management Cluster ID",
			},
		}, nil
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

	return ListProbes200JSONResponse(responseStruct), nil
}

// (GET /metrics/probe/{cluster_id})
func (Server) GetProbeById(ctx context.Context, request GetProbeByIdRequestObject) (GetProbeByIdResponseObject, error) {
	// Fake respone while we wire things together
	managementClusterId, err := uuid.Parse("957c5277-f74c-4b24-938a-f70bab28aab5")
	if err != nil {
		return GetProbeById404JSONResponse{}, nil
	}
	dummyProbe := ProbeObject{
		Id:                  request.ClusterId,
		ApiserverUrl:        "https://api.example.com/cluster-1",
		ManagementClusterId: managementClusterId,
		Private:             false,
	}

	responseStruct := ProbesArrayResponse{
		Probes: []ProbeObject{
			dummyProbe,
		},
	}

	return GetProbeById200JSONResponse(responseStruct), nil
}

// (POST /metrics/probes)
func (Server) CreateProbe(ctx context.Context, request CreateProbeRequestObject) (CreateProbeResponseObject, error) {

	createdProbe := ProbeObject{
		Id:                  request.Body.ClusterId,
		ApiserverUrl:        request.Body.ApiserverUrl,
		ManagementClusterId: request.Body.ManagementClusterId,
		Private:             request.Body.Private,
	}

	responseBody := ProbesArrayResponse{
		Probes: []ProbeObject{createdProbe},
	}

	log.Printf("Successfully created probe for cluster ID: %s", createdProbe.Id)
	return CreateProbe201JSONResponse(responseBody), nil
}

// (DELETE /metrics/probe/{cluster_id})
func (Server) DeleteProbe(ctx context.Context, request DeleteProbeRequestObject) (DeleteProbeResponseObject, error) {
	// Fake respone while we wire things together
	clusterId := request.ClusterId
	log.Printf("Successfully deleted probe for cluster ID: %s", clusterId)
	return DeleteProbe204Response{}, nil
}
