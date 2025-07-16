package probestore

import (
	"context"
	"errors"

	"github.com/google/uuid"
	v1 "github.com/rhobs/rhobs-synthetics-api/pkg/apis/v1"
)

// LocalProbeStore is a placeholder for a future implementation using a local database like SQLite.
type LocalProbeStore struct{}

// NewLocalProbeStore creates a new LocalProbeStore.
func NewLocalProbeStore() (*LocalProbeStore, error) {
	return &LocalProbeStore{}, nil
}

func (l *LocalProbeStore) ListProbes(ctx context.Context, selector string) ([]v1.ProbeObject, error) {
	return nil, errors.New("not implemented")
}

func (l *LocalProbeStore) GetProbe(ctx context.Context, probeID uuid.UUID) (*v1.ProbeObject, error) {
	return nil, errors.New("not implemented")
}

func (l *LocalProbeStore) CreateProbe(ctx context.Context, probe v1.ProbeObject, urlHashString string) (*v1.ProbeObject, error) {
	return nil, errors.New("not implemented")
}

func (l *LocalProbeStore) UpdateProbe(ctx context.Context, probe v1.ProbeObject) (*v1.ProbeObject, error) {
	return nil, errors.New("not implemented")
}

func (l *LocalProbeStore) DeleteProbe(ctx context.Context, probeID uuid.UUID) error {
	return errors.New("not implemented")
}

func (l *LocalProbeStore) ProbeWithURLHashExists(ctx context.Context, urlHashString string) (bool, error) {
	return false, errors.New("not implemented")
}
