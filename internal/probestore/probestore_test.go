package probestore

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	v1 "github.com/rhobs/rhobs-synthetics-api/pkg/apis/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const (
	testNamespace = "test-namespace"
)

func mustMarshal(t *testing.T, v interface{}) string {
	bytes, err := json.Marshal(v)
	require.NoError(t, err)
	return string(bytes)
}

func TestKubernetesProbeStore_UpdateProbe(t *testing.T) {
	ctx := context.Background()
	probeID := uuid.New()
	initialProbe := v1.ProbeObject{
		Id:        probeID,
		StaticUrl: "http://example.com/update",
		Status:    v1.Pending,
	}
	initialConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(probeConfigMapNameFormat, probeID),
			Namespace: testNamespace,
			Labels: map[string]string{
				baseAppLabelKey:     baseAppLabelValue,
				probeStatusLabelKey: string(v1.Pending),
			},
		},
		Data: map[string]string{
			"probe-config.json": mustMarshal(t, initialProbe),
		},
	}

	testCases := []struct {
		name          string
		probeToUpdate v1.ProbeObject
		clientset     *fake.Clientset
		expectErr     bool
		postCheck     func(t *testing.T, cs *fake.Clientset)
	}{
		{
			name: "successfully updates a probe",
			probeToUpdate: func() v1.ProbeObject {
				p := initialProbe
				p.Status = v1.Active
				p.Labels = &v1.LabelsSchema{"new": "label"}
				return p
			}(),
			clientset: fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}, initialConfigMap),
			expectErr: false,
			postCheck: func(t *testing.T, cs *fake.Clientset) {
				cm, err := cs.CoreV1().ConfigMaps(testNamespace).Get(ctx, fmt.Sprintf(probeConfigMapNameFormat, probeID), metav1.GetOptions{})
				require.NoError(t, err)
				assert.Equal(t, string(v1.Active), cm.Labels[probeStatusLabelKey])
				assert.Equal(t, "label", cm.Labels["new"])
			},
		},
		{
			name:          "error updating non-existent probe",
			probeToUpdate: v1.ProbeObject{Id: uuid.New()},
			clientset:     fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}, initialConfigMap),
			expectErr:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			store, err := NewKubernetesProbeStore(ctx, tc.clientset, testNamespace)
			require.NoError(t, err)

			// Act
			updatedProbe, err := store.UpdateProbe(ctx, tc.probeToUpdate)

			// Assert
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.probeToUpdate, *updatedProbe)
			}

			if tc.postCheck != nil {
				tc.postCheck(t, tc.clientset)
			}
		})
	}
}
