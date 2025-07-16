package probestore

import (
	"context"

	"github.com/google/uuid"
	v1 "github.com/rhobs/rhobs-synthetics-api/pkg/apis/v1"
)

// ProbeStorage defines the interface for storing and retrieving probes.
type ProbeStorage interface {
	ListProbes(ctx context.Context, selector string) ([]v1.ProbeObject, error)
	GetProbe(ctx context.Context, probeID uuid.UUID) (*v1.ProbeObject, error)
	CreateProbe(ctx context.Context, probe v1.ProbeObject, urlHashString string) (*v1.ProbeObject, error)
	UpdateProbe(ctx context.Context, probe v1.ProbeObject) (*v1.ProbeObject, error)
	DeleteProbe(ctx context.Context, probeID uuid.UUID) error
	ProbeWithURLHashExists(ctx context.Context, urlHashString string) (bool, error)
}
