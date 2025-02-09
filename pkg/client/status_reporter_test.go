// Copyright 2019 The Cluster Monitoring Operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package client

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	v1 "github.com/openshift/api/config/v1"
	configv1 "github.com/openshift/client-go/config/applyconfigurations/config/v1"
	clientv1 "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
)

func TestStatusReporterSetRolloutDone(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		given givenStatusReporter
		when  []whenFunc
		check []checkFunc
	}{
		{
			name: "not found",

			given: givenStatusReporter{
				operatorName:          "foo",
				namespace:             "bar",
				userWorkloadNamespace: "fred",
				version:               "1.0",
			},

			when: []whenFunc{
				getReturnsError(&apierrors.StatusError{
					ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound},
				}),
				createReturnsError(nil),
				updateStatusReturnsError(nil),
			},

			check: []checkFunc{
				hasCreated(true),
				hasUpdatedStatus(true),
				hasUpdatedStatusVersions("1.0"),
				hasUpdatedStatusConditions(
					"Available", "True",
					"Degraded", "False",
					"Progressing", "False",
					"Upgradeable", "Unknown",
				),
			},
		},
		{
			name: "found",

			given: givenStatusReporter{
				operatorName:          "foo",
				namespace:             "bar",
				userWorkloadNamespace: "fred",
				version:               "1.0",
			},

			when: []whenFunc{
				getReturnsClusterOperator(&v1.ClusterOperator{}),
				updateStatusReturnsError(nil),
			},

			check: []checkFunc{
				hasCreated(false),
				hasUpdatedStatus(true),
				hasUpdatedStatusVersions("1.0"),
				hasUpdatedStatusConditions(
					"Available", "True",
					"Degraded", "False",
					"Progressing", "False",
					"Upgradeable", "Unknown",
				),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := &clusterOperatorMock{}

			sr := NewStatusReporter(
				mock,
				tc.given.operatorName,
				tc.given.namespace,
				tc.given.userWorkloadNamespace,
				tc.given.version,
			)

			for _, w := range tc.when {
				w(mock)
			}

			got := sr.SetRollOutDone(ctx, "", "")

			for _, check := range tc.check {
				if err := check(mock, got); err != nil {
					t.Error(err)
				}
			}
		})
	}
}

func TestStatusReporterSetInProgress(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		given givenStatusReporter
		when  []whenFunc
		check []checkFunc
	}{
		{
			name: "not found",

			given: givenStatusReporter{
				operatorName:          "foo",
				namespace:             "bar",
				userWorkloadNamespace: "fred",
				version:               "1.0",
			},

			when: []whenFunc{
				getReturnsError(&apierrors.StatusError{
					ErrStatus: metav1.Status{Reason: metav1.StatusReasonNotFound},
				}),
				createReturnsError(nil),
				updateStatusReturnsError(nil),
			},

			check: []checkFunc{
				hasCreated(true),
				hasUpdatedStatus(true),
				hasUpdatedStatusVersions(),
				hasUpdatedStatusConditions(
					"Available", "Unknown",
					"Degraded", "Unknown",
					"Progressing", "True",
					"Upgradeable", "Unknown",
				),
			},
		},
		{
			name: "found",

			given: givenStatusReporter{
				operatorName:          "foo",
				namespace:             "bar",
				userWorkloadNamespace: "fred",
				version:               "1.0",
			},

			when: []whenFunc{
				getReturnsClusterOperator(&v1.ClusterOperator{}),
				updateStatusReturnsError(nil),
			},

			check: []checkFunc{
				hasCreated(false),
				hasUpdatedStatus(true),
				hasUpdatedStatusVersions(),
				hasUpdatedStatusConditions(
					"Available", "Unknown",
					"Degraded", "Unknown",
					"Progressing", "True",
					"Upgradeable", "Unknown",
				),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := &clusterOperatorMock{}

			sr := NewStatusReporter(
				mock,
				tc.given.operatorName,
				tc.given.namespace,
				tc.given.userWorkloadNamespace,
				tc.given.version,
			)

			for _, w := range tc.when {
				w(mock)
			}

			got := sr.SetRollOutInProgress(ctx)

			for _, check := range tc.check {
				if err := check(mock, got); err != nil {
					t.Errorf("test case name '%s' failed with error: %v", tc.name, err)
				}
			}
		})
	}
}

type asExpected Status

func (e asExpected) Status() Status {
	return Status(e)
}
func (asExpected) Reason() string {
	return "AsExpected"
}

func (e asExpected) Message() string {
	return "as expected" + string(e)
}

type unexpected struct {
	err    error
	status Status
}

func (si unexpected) Status() Status {
	return si.status
}
func (unexpected) Reason() string {
	return "Unexpected"
}

func (si unexpected) Message() string {
	return si.err.Error()
}

type fakeStateReport struct {
	degraded     StateInfo
	availability StateInfo
}

func (sr fakeStateReport) Degraded() StateInfo {
	return sr.degraded
}

func (sr fakeStateReport) Available() StateInfo {
	return sr.availability
}

func TestStatusReporterReportState(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name string

		sr           givenStatusReporter
		degraded     StateInfo
		availability StateInfo

		when  []whenFunc
		check []checkFunc
	}{{
		name: "normal",

		sr: givenStatusReporter{
			operatorName:          "foo",
			namespace:             "bar",
			userWorkloadNamespace: "fred",
			version:               "1.0",
		},

		degraded:     asExpected(FalseStatus),
		availability: asExpected(TrueStatus),

		when: []whenFunc{
			getReturnsClusterOperator(operatorWithConditions(
				"Available", "Unkwown", "Degraded", "Unknown",
				"Progressing", "False", "Upgradeable", "False",
			)),
			updateStatusReturnsError(nil),
		},

		check: []checkFunc{
			hasUpdatedStatus(true),
			hasUpdatedStatusVersions(),
			hasUpdatedStatusConditions(
				"Available", "True", "Degraded", "False",
				"Progressing", "False", "Upgradeable", "False",
			),
		},
	}, {
		name: "degraded and availabilty is nil",

		sr: givenStatusReporter{
			operatorName:          "foo",
			namespace:             "bar",
			userWorkloadNamespace: "fred",
			version:               "1.0",
		},

		degraded:     nil,
		availability: nil,

		when: []whenFunc{
			getReturnsClusterOperator(operatorWithConditions(
				"Available", "True", "Degraded", "False",
				"Progressing", "False", "Upgradeable", "False",
			)),
			updateStatusReturnsError(nil),
		},

		check: []checkFunc{
			hasUpdatedStatus(true),
			hasUpdatedStatusVersions(),
			hasUpdatedStatusConditions(
				"Available", "True", "Degraded", "False",
				"Progressing", "False", "Upgradeable", "False",
			),
		},
	}, {
		name: "degraded but availabile",

		sr: givenStatusReporter{
			operatorName:          "foo",
			namespace:             "bar",
			userWorkloadNamespace: "fred",
			version:               "1.0",
		},

		degraded:     unexpected{err: fmt.Errorf("foobar"), status: TrueStatus},
		availability: nil,

		when: []whenFunc{
			getReturnsClusterOperator(operatorWithConditions(
				"Available", "True", "Degraded", "False",
				"Progressing", "False", "Upgradeable", "False",
			)),
			updateStatusReturnsError(nil),
		},

		check: []checkFunc{
			hasUpdatedStatus(true),
			hasUpdatedStatusVersions(),
			hasUpdatedStatusConditions(
				"Available", "True", "Degraded", "True",
				"Progressing", "False", "Upgradeable", "False",
			),
		},
	}, {
		name: "degraded and unavailabile",

		sr: givenStatusReporter{
			operatorName:          "foo",
			namespace:             "bar",
			userWorkloadNamespace: "fred",
			version:               "1.0",
		},

		degraded:     unexpected{err: fmt.Errorf("foobar"), status: TrueStatus},
		availability: unexpected{err: fmt.Errorf("foobar"), status: FalseStatus},

		when: []whenFunc{
			getReturnsClusterOperator(operatorWithConditions(
				"Available", "True", "Degraded", "False",
				"Progressing", "False", "Upgradeable", "False",
			)),
			updateStatusReturnsError(nil),
		},

		check: []checkFunc{
			hasUpdatedStatus(true),
			hasUpdatedStatusVersions(),
			hasUpdatedStatusConditions(
				"Available", "False", "Degraded", "True",
				"Progressing", "False", "Upgradeable", "False",
			),
		},
	},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := &clusterOperatorMock{}

			sr := NewStatusReporter(
				mock,
				tc.sr.operatorName,
				tc.sr.namespace,
				tc.sr.userWorkloadNamespace,
				tc.sr.version,
			)

			for _, w := range tc.when {
				w(mock)
			}

			got := sr.ReportState(ctx, fakeStateReport{
				degraded:     tc.degraded,
				availability: tc.availability,
			})

			for _, check := range tc.check {
				if err := check(mock, got); err != nil {
					t.Errorf("test case name '%s' failed with error: %v", tc.name, err)
				}
			}
		})
	}
}

type givenStatusReporter struct {
	operatorName, namespace, userWorkloadNamespace, version string
	err                                                     error
	degraded, availabilty                                   error
}

func operatorWithConditions(conditions ...string) *v1.ClusterOperator {
	c := []v1.ClusterOperatorStatusCondition{}

	for i := 0; i < len(conditions); i += 2 {
		ctype, status := conditions[i], conditions[i+1]
		c = append(c, v1.ClusterOperatorStatusCondition{
			Type:   v1.ClusterStatusConditionType(ctype),
			Status: v1.ConditionStatus(status),
		})
	}

	return &v1.ClusterOperator{
		Status: v1.ClusterOperatorStatus{
			Conditions: c,
		},
	}
}

type checkFunc func(*clusterOperatorMock, error) error

func hasCreated(want bool) checkFunc {
	return func(mock *clusterOperatorMock, _ error) error {
		if got := mock.created != nil; got != want {
			return fmt.Errorf("want created %t, got %t", want, got)
		}
		return nil
	}
}

func hasUpdatedStatus(want bool) checkFunc {
	return func(mock *clusterOperatorMock, _ error) error {
		if got := mock.statusUpdated != nil; got != want {
			return fmt.Errorf("want status updated %t, got %t", want, got)
		}
		return nil
	}
}

func hasUpdatedStatusVersions(want ...string) checkFunc {
	return func(mock *clusterOperatorMock, _ error) error {
		var got []string
		for _, s := range mock.statusUpdated.Status.Versions {
			got = append(got, s.Version)
		}
		if !reflect.DeepEqual(got, want) {
			return fmt.Errorf("want versions to be equal, but they aren't: want %q got %q", want, got)
		}
		return nil
	}
}

func hasUpdatedStatusConditions(want ...string) checkFunc {
	return func(mock *clusterOperatorMock, _ error) error {
		sort.Sort(byType(mock.statusUpdated.Status.Conditions))
		var got []string
		for _, c := range mock.statusUpdated.Status.Conditions {
			got = append(got, string(c.Type))
			got = append(got, string(c.Status))
		}
		if !reflect.DeepEqual(got, want) {
			return fmt.Errorf("want conditions to be equal, but they aren't: want %q got %q", want, got)
		}
		return nil
	}
}

func hasUnavailableMessage() checkFunc {
	return func(mock *clusterOperatorMock, _ error) error {
		sort.Sort(byType(mock.statusUpdated.Status.Conditions))
		for _, c := range mock.statusUpdated.Status.Conditions {
			if c.Type == v1.OperatorAvailable && c.Status == v1.ConditionFalse && c.Message == "" {
				return fmt.Errorf("want a message if available status is false, got %q", c.Message)
			}
		}
		return nil
	}
}

type whenFunc func(*clusterOperatorMock)

func getReturnsClusterOperator(co *v1.ClusterOperator) whenFunc {
	return func(mock *clusterOperatorMock) {
		mock.getFunc = func(string, metav1.GetOptions) (*v1.ClusterOperator, error) {
			return co, nil
		}
	}
}

func getReturnsError(e error) whenFunc {
	return func(mock *clusterOperatorMock) {
		mock.getFunc = func(string, metav1.GetOptions) (*v1.ClusterOperator, error) {
			return nil, e
		}
	}
}

func createReturnsError(e error) whenFunc {
	return func(mock *clusterOperatorMock) {
		mock.createFunc = func(co *v1.ClusterOperator) (*v1.ClusterOperator, error) {
			return co, e
		}
	}
}

func updateStatusReturnsError(e error) whenFunc {
	return func(mock *clusterOperatorMock) {
		mock.updateStatusFunc = func(co *v1.ClusterOperator) (*v1.ClusterOperator, error) {
			return co, e
		}
	}
}

type clusterOperatorMock struct {
	createFunc, updateFunc, updateStatusFunc func(*v1.ClusterOperator) (*v1.ClusterOperator, error)
	getFunc                                  func(string, metav1.GetOptions) (*v1.ClusterOperator, error)

	created, updated, statusUpdated *v1.ClusterOperator
}

// ensure the mock satisfies the ClusterOperatorInterface interface.
var _ clientv1.ClusterOperatorInterface = (*clusterOperatorMock)(nil)

func (com *clusterOperatorMock) Create(ctx context.Context, co *v1.ClusterOperator, opts metav1.CreateOptions) (*v1.ClusterOperator, error) {
	com.created = co
	return com.createFunc(co)
}

func (com *clusterOperatorMock) Update(ctx context.Context, co *v1.ClusterOperator, opts metav1.UpdateOptions) (*v1.ClusterOperator, error) {
	com.updated = co
	return com.updateFunc(co)
}

func (com *clusterOperatorMock) UpdateStatus(ctx context.Context, co *v1.ClusterOperator, opts metav1.UpdateOptions) (*v1.ClusterOperator, error) {
	com.statusUpdated = co
	return com.updateStatusFunc(co)
}

func (com *clusterOperatorMock) Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error {
	return nil
}

func (com *clusterOperatorMock) DeleteCollection(ctx context.Context, options metav1.DeleteOptions, listOptions metav1.ListOptions) error {
	return nil
}

func (com *clusterOperatorMock) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v1.ClusterOperator, error) {
	return com.getFunc(name, opts)
}

func (com *clusterOperatorMock) List(ctx context.Context, opts metav1.ListOptions) (*v1.ClusterOperatorList, error) {
	return nil, nil
}

func (com *clusterOperatorMock) Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (com *clusterOperatorMock) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *v1.ClusterOperator, err error) {
	return nil, nil
}

func (com *clusterOperatorMock) Apply(ctx context.Context, clusterOperator *configv1.ClusterOperatorApplyConfiguration, opts metav1.ApplyOptions) (result *v1.ClusterOperator, err error) {
	panic("not supported")
}

func (com *clusterOperatorMock) ApplyStatus(ctx context.Context, clusterOperator *configv1.ClusterOperatorApplyConfiguration, opts metav1.ApplyOptions) (result *v1.ClusterOperator, err error) {
	panic("not supported")
}
