package api

import (
	"context"
	"log"

	"github.com/google/uuid"
	"github.com/rhobs/rhobs-synthetics-api/pkg/apis/v1"
)

type Server struct{}

func NewServer() Server {
	return Server{}
}

// (GET /metrics/probes)
func (Server) ListProbes(ctx context.Context, request v1.ListProbesRequestObject) (v1.ListProbesResponseObject, error) {
	// Fake respone while we wire things together
	clusterId, err := uuid.Parse("957c5277-f74c-4b24-938a-f70bab28aab5")
	if err != nil {
		return v1.ListProbes400JSONResponse{
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
		return v1.ListProbes400JSONResponse{
			Error: struct {
				Code    int32  `json:"code"`
				Message string `json:"message"`
			}{
				Code:    400,
				Message: "Invalid Management Cluster ID",
			},
		}, nil
	}
	dummyProbe := v1.ProbeObject{
		Id:                  clusterId,
		ApiserverUrl:        "https://v1.example.com/cluster-1",
		ManagementClusterId: managementClusterId,
		Private:             false,
	}

	responseStruct := v1.ProbesArrayResponse{
		Probes: []v1.ProbeObject{
			dummyProbe,
		},
	}

	return v1.ListProbes200JSONResponse(responseStruct), nil
}

// (GET /metrics/probe/{cluster_id})
func (Server) GetProbeById(ctx context.Context, request v1.GetProbeByIdRequestObject) (v1.GetProbeByIdResponseObject, error) {
	// Fake respone while we wire things together
	managementClusterId, err := uuid.Parse("957c5277-f74c-4b24-938a-f70bab28aab5")
	if err != nil {
		return v1.GetProbeById404JSONResponse{}, nil
	}
	dummyProbe := v1.ProbeObject{
		Id:                  request.ClusterId,
		ApiserverUrl:        "https://v1.example.com/cluster-1",
		ManagementClusterId: managementClusterId,
		Private:             false,
	}

	responseStruct := v1.ProbesArrayResponse{
		Probes: []v1.ProbeObject{
			dummyProbe,
		},
	}

	return v1.GetProbeById200JSONResponse(responseStruct), nil
}

// (POST /metrics/probes)
func (Server) CreateProbe(ctx context.Context, request v1.CreateProbeRequestObject) (v1.CreateProbeResponseObject, error) {

	createdProbe := v1.ProbeObject{
		Id:                  request.Body.ClusterId,
		ApiserverUrl:        request.Body.ApiserverUrl,
		ManagementClusterId: request.Body.ManagementClusterId,
		Private:             request.Body.Private,
	}

	responseBody := v1.ProbesArrayResponse{
		Probes: []v1.ProbeObject{createdProbe},
	}

	log.Printf("Successfully created probe for cluster ID: %s", createdProbe.Id)
	return v1.CreateProbe201JSONResponse(responseBody), nil
}

// (DELETE /metrics/probe/{cluster_id})
func (Server) DeleteProbe(ctx context.Context, request v1.DeleteProbeRequestObject) (v1.DeleteProbeResponseObject, error) {
	// Fake respone while we wire things together
	clusterId := request.ClusterId
	log.Printf("Successfully deleted probe for cluster ID: %s", clusterId)
	return v1.DeleteProbe204Response{}, nil
}
