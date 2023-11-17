/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package pod

import (
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	fakeDiscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/dynamic"
	fakeDynamic "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func Test_PodsInNamespaceWithSelectorShouldHaveLabels(t *testing.T) {
	ns := v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "foo"}}
	podWithLabels1 := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-foo-xhhxj",
			Namespace: "foo",
			Labels: map[string]string{
				"app":   "foo",
				"label": "true",
			},
		},
	}
	podWithLabels2 := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-foo-xhhzd",
			Namespace: "foo",
			Labels: map[string]string{
				"app":   "foo",
				"label": "true",
			},
		},
	}
	podMissingLabel := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-foo-xhhzr",
			Namespace: "foo",
			Labels: map[string]string{
				"app": "foo",
			},
		},
	}
	clientNoErr := fake.NewSimpleClientset(&ns, &podWithLabels1, &podWithLabels2)
	clientErr := fake.NewSimpleClientset(&ns, &podWithLabels1, &podWithLabels2, &podMissingLabel)
	dynScheme := runtime.NewScheme()
	fakeDynamicClient := fakeDynamic.NewSimpleDynamicClient(dynScheme)
	fakeDiscoveryClient := fakeDiscovery.FakeDiscovery{}

	type fields struct {
		KubeInterface      kubernetes.Interface
		DynamicInterface   dynamic.Interface
		DiscoveryInterface *fakeDiscovery.FakeDiscovery
	}
	type args struct {
		namespace string
		selector  string
		labels    string
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		{
			name: "No pods found",
			fields: fields{
				KubeInterface:      clientErr,
				DiscoveryInterface: &fakeDiscoveryClient,
				DynamicInterface:   fakeDynamicClient,
			},
			args: args{
				selector:  "app=doesnotexist",
				namespace: "foo",
				labels:    "app=foo,label=true",
			},
			wantErr: true,
		},
		{
			name: "Pods should have labels",
			fields: fields{
				KubeInterface:      clientNoErr,
				DiscoveryInterface: &fakeDiscoveryClient,
				DynamicInterface:   fakeDynamicClient,
			},
			args: args{
				selector:  "app=foo",
				namespace: "foo",
				labels:    "app=foo,label=true",
			},
			wantErr: false,
		},
		{
			name: "Error from pod missing label",
			fields: fields{
				KubeInterface:      clientErr,
				DiscoveryInterface: &fakeDiscoveryClient,
				DynamicInterface:   fakeDynamicClient,
			},
			args: args{
				selector:  "app=foo",
				namespace: "foo",
				labels:    "app=foo,label=true",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := PodsInNamespaceWithSelectorShouldHaveLabels(tt.fields.KubeInterface, tt.args.namespace, tt.args.selector, tt.args.labels); (err != nil) != tt.wantErr {
				t.Errorf("ThePodsInNamespaceWithSelectorShouldHaveLabels() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// func TestListPods(t *testing.T) {
// 	type args struct {
// 		kubeClientset kubernetes.Interface
// 		namespace     string
// 	}
// 	tests := []struct {
// 		name    string
// 		args    args
// 		wantErr bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			if err := ListPods(tt.args.kubeClientset, tt.args.namespace); (err != nil) != tt.wantErr {
// 				t.Errorf("ListPods() error = %v, wantErr %v", err, tt.wantErr)
// 			}
// 		})
// 	}
// }

func TestListPodsWithSelector(t *testing.T) {
	type args struct {
		kubeClientset kubernetes.Interface
		namespace     string
		selector      string
	}
	namespace := "namespace1"
	selector := "label-key1=label-value1"
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "Positive Test",
			args: args{
				kubeClientset: fake.NewSimpleClientset(getPodWithSelector(t, "pod1", namespace, selector)),
				namespace:     namespace,
				selector:      selector,
			},
		},
		{
			name: "Positive Test: no selector",
			args: args{
				kubeClientset: fake.NewSimpleClientset(getPod(t, "pod1", namespace)),
				namespace:     namespace,
			},
		},
		{
			name: "Negative Test: pod not found",
			args: args{
				kubeClientset: fake.NewSimpleClientset(getPod(t, "pod1", namespace)),
				namespace:     namespace,
				selector:      selector,
			},
			wantErr: true,
		},
		{
			name: "Negative Test: pod not found",
			args: args{
				kubeClientset: fake.NewSimpleClientset(getPod(t, "pod1", namespace)),
				// namespace:     namespace,
				// selector:      selector,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ListPodsWithSelector(tt.args.kubeClientset, tt.args.namespace, tt.args.selector); (err != nil) != tt.wantErr {
				t.Errorf("ListPodsWithSelector() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPodsWithSelectorHaveRestartCountLessThan(t *testing.T) {
	type args struct {
		kubeClientset kubernetes.Interface
		namespace     string
		selector      string
		restartCount  int
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := PodsWithSelectorHaveRestartCountLessThan(tt.args.kubeClientset, tt.args.namespace, tt.args.selector, tt.args.restartCount); (err != nil) != tt.wantErr {
				t.Errorf("PodsWithSelectorHaveRestartCountLessThan() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSomeOrAllPodsInNamespaceWithSelectorHaveStringInLogsSinceTime(t *testing.T) {
	type args struct {
		kubeClientset kubernetes.Interface
		expBackoff    wait.Backoff
		SomeOrAll     string
		namespace     string
		selector      string
		searchKeyword string
		since         time.Time
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := SomeOrAllPodsInNamespaceWithSelectorHaveStringInLogsSinceTime(tt.args.kubeClientset, tt.args.expBackoff, tt.args.SomeOrAll, tt.args.namespace, tt.args.selector, tt.args.searchKeyword, tt.args.since); (err != nil) != tt.wantErr {
				t.Errorf("SomeOrAllPodsInNamespaceWithSelectorHaveStringInLogsSinceTime() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSomePodsInNamespaceWithSelectorDontHaveStringInLogsSinceTime(t *testing.T) {
	type args struct {
		kubeClientset kubernetes.Interface
		namespace     string
		selector      string
		searchkeyword string
		since         time.Time
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := SomePodsInNamespaceWithSelectorDontHaveStringInLogsSinceTime(tt.args.kubeClientset, tt.args.namespace, tt.args.selector, tt.args.searchkeyword, tt.args.since); (err != nil) != tt.wantErr {
				t.Errorf("SomePodsInNamespaceWithSelectorDontHaveStringInLogsSinceTime() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPodsInNamespaceWithSelectorHaveNoErrorsInLogsSinceTime(t *testing.T) {
	type args struct {
		kubeClientset kubernetes.Interface
		namespace     string
		selector      string
		since         time.Time
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := PodsInNamespaceWithSelectorHaveNoErrorsInLogsSinceTime(tt.args.kubeClientset, tt.args.namespace, tt.args.selector, tt.args.since); (err != nil) != tt.wantErr {
				t.Errorf("PodsInNamespaceWithSelectorHaveNoErrorsInLogsSinceTime() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPodsInNamespaceWithSelectorHaveSomeErrorsInLogsSinceTime(t *testing.T) {
	type args struct {
		kubeClientset kubernetes.Interface
		namespace     string
		selector      string
		since         time.Time
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := PodsInNamespaceWithSelectorHaveSomeErrorsInLogsSinceTime(tt.args.kubeClientset, tt.args.namespace, tt.args.selector, tt.args.since); (err != nil) != tt.wantErr {
				t.Errorf("PodsInNamespaceWithSelectorHaveSomeErrorsInLogsSinceTime() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestPodInNamespaceShouldHaveLabels(t *testing.T) {
	type args struct {
		kubeClientset kubernetes.Interface
		name          string
		namespace     string
		labels        string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := PodInNamespaceShouldHaveLabels(tt.args.kubeClientset, tt.args.name, tt.args.namespace, tt.args.labels); (err != nil) != tt.wantErr {
				t.Errorf("PodInNamespaceShouldHaveLabels() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// func TestPodsInNamespaceWithSelectorShouldHaveLabels(t *testing.T) {
// 	type args struct {
// 		kubeClientset kubernetes.Interface
// 		namespace     string
// 		selector      string
// 		labels        string
// 	}
// 	tests := []struct {
// 		name    string
// 		args    args
// 		wantErr bool
// 	}{
// 		// TODO: Add test cases.
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			if err := PodsInNamespaceWithSelectorShouldHaveLabels(tt.args.kubeClientset, tt.args.namespace, tt.args.selector, tt.args.labels); (err != nil) != tt.wantErr {
// 				t.Errorf("PodsInNamespaceWithSelectorShouldHaveLabels() error = %v, wantErr %v", err, tt.wantErr)
// 			}
// 		})
// 	}
// }

func getPod(t *testing.T, name, namespace string) *corev1.Pod {
	return getPodWithSelector(t, name, namespace, "")

}
func getPodWithSelector(t *testing.T, name, namespace, selector string) *corev1.Pod {
	labels := map[string]string{}
	if selector != "" {
		key, value := getLabelParts(t, selector)
		labels[key] = value
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
	}
}

// TODO: duplicated in structured. Common test utility pkg?
func getLabelParts(t *testing.T, label string) (string, string) {
	labelSplit := strings.Split(label, "=")
	if len(labelSplit) != 2 {
		t.Errorf("expected label format '<key>=<value>', got '%s'", label)
	}
	return labelSplit[0], labelSplit[1]
}
