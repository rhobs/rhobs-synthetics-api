package probestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	v1 "github.com/rhobs/rhobs-synthetics-api/pkg/apis/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const (
	testNamespace = "test-namespace"
)

func mustMarshal(t *testing.T, v interface{}) string {
	bytes, err := json.Marshal(v)
	require.NoError(t, err)
	return string(bytes)
}

func TestKubernetesProbeStore_ListProbes(t *testing.T) {
	ctx := context.Background()
	probe1ID := uuid.New()
	probe1 := v1.ProbeObject{Id: probe1ID, StaticUrl: "http://example.com/1", Status: v1.Active, Labels: &v1.LabelsSchema{"env": "prod"}}
	cm1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(probeConfigMapNameFormat, probe1ID),
			Namespace: testNamespace,
			Labels:    map[string]string{baseAppLabelKey: baseAppLabelValue, "env": "prod"},
		},
		Data: map[string]string{"probe-config.json": mustMarshal(t, probe1)},
	}

	probe2ID := uuid.New()
	probe2 := v1.ProbeObject{Id: probe2ID, StaticUrl: "http://example.com/2", Status: v1.Active, Labels: &v1.LabelsSchema{"env": "dev"}}
	cm2 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(probeConfigMapNameFormat, probe2ID),
			Namespace: testNamespace,
			Labels:    map[string]string{baseAppLabelKey: baseAppLabelValue, "env": "dev"},
		},
		Data: map[string]string{"probe-config.json": mustMarshal(t, probe2)},
	}

	malformedCmID := uuid.New()
	malformedCm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(probeConfigMapNameFormat, malformedCmID),
			Namespace: testNamespace,
			Labels:    map[string]string{baseAppLabelKey: baseAppLabelValue},
		},
		Data: map[string]string{"probe-config.json": "{not-a-valid-json"},
	}

	errorClientset := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}})
	errorClientset.PrependReactor("list", "configmaps", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, errors.New("simulated API error")
	})

	testCases := []struct {
		name                string
		selector            string
		clientset           *fake.Clientset
		expectErr           bool
		expectedProbesCount int
	}{
		{
			name:                "list multiple probes",
			selector:            fmt.Sprintf("%s=%s", baseAppLabelKey, baseAppLabelValue),
			clientset:           fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}, cm1, cm2),
			expectErr:           false,
			expectedProbesCount: 2,
		},
		{
			name:                "list no probes",
			selector:            fmt.Sprintf("%s=%s", baseAppLabelKey, baseAppLabelValue),
			clientset:           fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}),
			expectErr:           false,
			expectedProbesCount: 0,
		},
		{
			name:                "filter with label selector",
			selector:            fmt.Sprintf("%s=%s,env=prod", baseAppLabelKey, baseAppLabelValue),
			clientset:           fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}, cm1, cm2),
			expectErr:           false,
			expectedProbesCount: 1,
		},
		{
			name:                "skip malformed probe",
			selector:            fmt.Sprintf("%s=%s", baseAppLabelKey, baseAppLabelValue),
			clientset:           fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}, cm1, malformedCm),
			expectErr:           false,
			expectedProbesCount: 1,
		},
		{
			name:                "kubernetes api error",
			selector:            fmt.Sprintf("%s=%s", baseAppLabelKey, baseAppLabelValue),
			clientset:           errorClientset,
			expectErr:           true,
			expectedProbesCount: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store, err := NewKubernetesProbeStore(ctx, tc.clientset, testNamespace)
			require.NoError(t, err)

			probes, err := store.ListProbes(ctx, tc.selector)

			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, probes, tc.expectedProbesCount)
			}
		})
	}
}

func TestKubernetesProbeStore_GetProbe(t *testing.T) {
	ctx := context.Background()

	probeID := uuid.New()
	probe := v1.ProbeObject{Id: probeID, StaticUrl: "http://example.com/1", Status: v1.Active, Labels: &v1.LabelsSchema{"env": "prod"}}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(probeConfigMapNameFormat, probeID),
			Namespace: testNamespace,
			Labels:    map[string]string{baseAppLabelKey: baseAppLabelValue, "env": "prod"},
		},
		Data: map[string]string{"probe-config.json": mustMarshal(t, probe)},
	}

	malformedCmID := uuid.New()
	malformedCm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(probeConfigMapNameFormat, malformedCmID),
			Namespace: testNamespace,
			Labels:    map[string]string{baseAppLabelKey: baseAppLabelValue},
		},
		Data: map[string]string{"probe-config.json": "{not-a-valid-json"},
	}

	testCases := []struct {
		name          string
		probeID       uuid.UUID
		clientset     *fake.Clientset
		expectErr     bool
		expectedProbe *v1.ProbeObject
		checkErr      func(t *testing.T, err error)
	}{
		{
			name:          "get existing probe",
			clientset:     fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}, cm),
			probeID:       probeID,
			expectErr:     false,
			expectedProbe: &probe,
		},
		{
			name:      "get non-existent probe",
			clientset: fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}),
			probeID:   uuid.New(),
			expectErr: true,
			checkErr: func(t *testing.T, err error) {
				assert.True(t, k8serrors.IsNotFound(err), "expected a 'not found' error")
			},
		},
		{
			name:      "error getting malformed probe",
			clientset: fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}, malformedCm),
			probeID:   malformedCmID,
			expectErr: true,
			checkErr: func(t *testing.T, err error) {
				assert.False(t, k8serrors.IsNotFound(err), "expected a non-'not found' error")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store, err := NewKubernetesProbeStore(ctx, tc.clientset, testNamespace)
			require.NoError(t, err)

			returnedProbe, err := store.GetProbe(ctx, tc.probeID)

			if tc.expectErr {
				require.Error(t, err)
				if tc.checkErr != nil {
					tc.checkErr(t, err)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedProbe, returnedProbe)
			}
		})
	}
}

func TestKubernetesProbeStore_CreateProbe(t *testing.T) {
	ctx := context.Background()
	probeToCreate := v1.ProbeObject{
		Id:        uuid.New(),
		StaticUrl: "http://example.com/create",
		Status:    v1.Pending,
		Labels:    &v1.LabelsSchema{"team": "sre"},
	}
	urlHash := "testhash"

	successClientset := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}})

	alreadyExistsClientset := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}})
	alreadyExistsClientset.PrependReactor("create", "configmaps", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, k8serrors.NewAlreadyExists(corev1.Resource("configmaps"), "probe-already-exists")
	})

	testCases := []struct {
		name      string
		clientset *fake.Clientset
		expectErr bool
		postCheck func(t *testing.T, cs *fake.Clientset)
		checkErr  func(t *testing.T, err error)
	}{
		{
			name:      "successfully creates a probe",
			clientset: successClientset,
			expectErr: false,
			postCheck: func(t *testing.T, cs *fake.Clientset) {
				cmName := fmt.Sprintf(probeConfigMapNameFormat, probeToCreate.Id)
				cm, err := cs.CoreV1().ConfigMaps(testNamespace).Get(ctx, cmName, metav1.GetOptions{})
				require.NoError(t, err)

				assert.Equal(t, baseAppLabelValue, cm.Labels[baseAppLabelKey])
				assert.Equal(t, string(v1.Pending), cm.Labels[probeStatusLabelKey])
				assert.Equal(t, urlHash, cm.Labels[probeURLHashLabelKey])
				assert.Equal(t, "sre", cm.Labels["team"])

				var probeFromData v1.ProbeObject
				err = json.Unmarshal([]byte(cm.Data["probe-config.json"]), &probeFromData)
				require.NoError(t, err)
				assert.Equal(t, probeToCreate, probeFromData)
			},
		},
		{
			name:      "error when probe already exists",
			clientset: alreadyExistsClientset,
			expectErr: true,
			checkErr: func(t *testing.T, err error) {
				assert.True(t, k8serrors.IsAlreadyExists(err), "expected an 'already exists' error")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store, err := NewKubernetesProbeStore(ctx, tc.clientset, testNamespace)
			require.NoError(t, err)

			createdProbe, err := store.CreateProbe(ctx, probeToCreate, urlHash)

			if tc.expectErr {
				require.Error(t, err)
				if tc.checkErr != nil {
					tc.checkErr(t, err)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, &probeToCreate, createdProbe)
			}

			if tc.postCheck != nil {
				tc.postCheck(t, tc.clientset)
			}
		})
	}
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

func TestKubernetesProbeStore_DeleteProbe(t *testing.T) {
	ctx := context.Background()
	probeID := uuid.New()
	probe := v1.ProbeObject{Id: probeID, StaticUrl: "http://example.com/1", Status: v1.Active, Labels: &v1.LabelsSchema{"env": "prod"}}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(probeConfigMapNameFormat, probeID),
			Namespace: testNamespace,
			Labels:    map[string]string{baseAppLabelKey: baseAppLabelValue, "env": "prod"},
		},
		Data: map[string]string{"probe-config.json": mustMarshal(t, probe)},
	}

	testCases := []struct {
		name      string
		clientset *fake.Clientset
		expectErr bool
		postCheck func(t *testing.T, cs *fake.Clientset)
		checkErr  func(t *testing.T, err error)
	}{
		{
			name:      "successfully deletes a probe",
			clientset: fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}, cm),
			expectErr: false,
			postCheck: func(t *testing.T, cs *fake.Clientset) {
				_, err := cs.CoreV1().ConfigMaps(testNamespace).Get(ctx, fmt.Sprintf(probeConfigMapNameFormat, probeID), metav1.GetOptions{})
				require.Error(t, err)
				assert.True(t, k8serrors.IsNotFound(err), "expected a 'not found' error")
			},
		},
		{
			name:      "error deleting non-existent probe",
			clientset: fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}),
			expectErr: true,
			checkErr: func(t *testing.T, err error) {
				assert.True(t, k8serrors.IsNotFound(err), "expected a 'not found' error")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store, err := NewKubernetesProbeStore(ctx, tc.clientset, testNamespace)
			require.NoError(t, err)

			err = store.DeleteProbe(ctx, probeID)

			if tc.expectErr {
				require.Error(t, err)
				if tc.checkErr != nil {
					tc.checkErr(t, err)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestKubernetesProbeStore_ProbeWithURLHashExists(t *testing.T) {
	ctx := context.Background()
	urlHash := "test-url-hash"
	
	probeID := uuid.New()
	probe := v1.ProbeObject{
		Id:        probeID,
		StaticUrl: "http://example.com",
		Status:    v1.Active,
		Labels:    &v1.LabelsSchema{"env": "test"},
	}
	
	// ConfigMap with the URL hash we're looking for
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(probeConfigMapNameFormat, probeID),
			Namespace: testNamespace,
			Labels: map[string]string{
				baseAppLabelKey:        baseAppLabelValue,
				probeURLHashLabelKey:   urlHash,
				probeStatusLabelKey:    string(v1.Active),
			},
		},
		Data: map[string]string{"probe-config.json": mustMarshal(t, probe)},
	}

	testCases := []struct {
		name        string
		urlHash     string
		clientset   *fake.Clientset
		expectExists bool
		expectErr   bool
	}{
		{
			name:         "probe with URL hash exists",
			urlHash:      urlHash,
			clientset:    fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}, cm),
			expectExists: true,
			expectErr:    false,
		},
		{
			name:         "probe with URL hash does not exist",
			urlHash:      "different-hash",
			clientset:    fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}, cm),
			expectExists: false,
			expectErr:    false,
		},
		{
			name:         "no probes exist",
			urlHash:      urlHash,
			clientset:    fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}}),
			expectExists: false,
			expectErr:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store, err := NewKubernetesProbeStore(ctx, tc.clientset, testNamespace)
			require.NoError(t, err)

			exists, err := store.ProbeWithURLHashExists(ctx, tc.urlHash)

			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectExists, exists)
			}
		})
	}
}
