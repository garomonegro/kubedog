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

package kube

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"testing"

	util "github.com/keikoproj/kubedog/internal/utilities"
	"github.com/onsi/gomega"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	hpa "k8s.io/api/autoscaling/v2beta2"
	v1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	serializer "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	fakeDiscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/dynamic"
	fakeDynamic "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

// TODO: redistribute this test functions
func TestPositiveNodesWithSelectorShouldBe(t *testing.T) {

	var (
		g                 = gomega.NewWithT(t)
		testReadySelector = "testing-ShouldBeReady=some-value"
		testFoundSelector = "testing-ShouldBeFound=some-value"
		testReadyLabel    = map[string]string{"testing-ShouldBeReady": "some-value"}
		testFoundLabel    = map[string]string{"testing-ShouldBeFound": "some-value"}
		fakeClient        *fake.Clientset
		dynScheme         = runtime.NewScheme()
		fakeDynamicClient = fakeDynamic.NewSimpleDynamicClient(dynScheme)
		fakeDiscovery     = &fakeDiscovery.FakeDiscovery{}
	)

	fakeClient = fake.NewSimpleClientset(&v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "SomeReady-Node",
			Labels: testReadyLabel,
		},
		Status: v1.NodeStatus{
			Conditions: []v1.NodeCondition{
				{
					Type:   v1.NodeReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}, &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "SomeFound-name",
			Labels: testFoundLabel,
		},
	})

	kc := ClientSet{
		KubeInterface:      fakeClient,
		DiscoveryInterface: fakeDiscovery,
		DynamicInterface:   fakeDynamicClient,
		FilesPath:          "../../test/templates",
	}

	err := kc.NodesWithSelectorShouldBe(1, testReadySelector, stateReady)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	err = kc.NodesWithSelectorShouldBe(1, testFoundSelector, stateFound)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
}

// TODO: this doesnt seem to be testing MultipleResourcesOperation but GetMultipleResourcesFromYaml
func TestMultipleResourcesOperation(t *testing.T) {

	var (
		dynScheme           = runtime.NewScheme()
		fakeDynamicClient   = fakeDynamic.NewSimpleDynamicClient(dynScheme)
		fakeDiscovery       = fakeDiscovery.FakeDiscovery{}
		g                   = gomega.NewWithT(t)
		testTemplatePath, _ = filepath.Abs("../../test/templates")
	)

	expectedResources := []*metav1.APIResourceList{
		newTestAPIResourceList("someGroup.apiVersion/SomeVersion", "someResource", "SomeKind"),
		newTestAPIResourceList("otherGroup.apiVersion/OtherVersion", "otherResource", "OtherKind"),
		newTestAPIResourceList("argoproj.io/v1alpha1", "AnalysisTemplate", "AnalysisTemplate"),
	}

	fakeDiscovery.Fake = &fakeDynamicClient.Fake
	fakeDiscovery.Resources = append(fakeDiscovery.Resources, expectedResources...)

	resourceToApiResourceList := func(resource *unstructured.Unstructured) *metav1.APIResourceList {
		return newTestAPIResourceList(
			resource.GetAPIVersion(),
			resource.GetName(),
			resource.GetKind(),
		)
	}

	tests := []struct {
		testResourcePath  string
		numResources      int
		expectError       bool
		expectedResources []*metav1.APIResourceList
	}{
		{ // PositiveTest
			testResourcePath: testTemplatePath + "/test-multi-resourcefile.yaml",
			numResources:     2,
			expectError:      false,
			expectedResources: []*metav1.APIResourceList{
				newTestAPIResourceList("someGroup.apiVersion/SomeVersion", "someResource", "SomeKind"),
				newTestAPIResourceList("otherGroup.apiVersion/OtherVersion", "otherResource", "OtherKind"),
			},
		},
		{ // NegativeTest: file doesn't exist
			testResourcePath:  testTemplatePath + "/wrongName_manifest.yaml",
			numResources:      0,
			expectError:       true,
			expectedResources: []*metav1.APIResourceList{},
		},
		{ // Avoid text/template no function found error when working with AnalysisTemplate/no template args
			testResourcePath: testTemplatePath + "/analysis-template.yaml",
			numResources:     1,
			expectError:      false,
			expectedResources: []*metav1.APIResourceList{
				newTestAPIResourceList("argoproj.io/v1alpha1", "args-test", "AnalysisTemplate"),
			},
		},
	}

	for _, test := range tests {
		resourceList, err := util.GetMultipleResourcesFromYaml(test.testResourcePath, &fakeDiscovery, nil)

		g.Expect(len(resourceList)).To(gomega.Equal(test.numResources))
		if test.expectError {
			g.Expect(err).Should(gomega.HaveOccurred())
		} else {
			g.Expect(err).ShouldNot(gomega.HaveOccurred())
			for i, resource := range resourceList {
				g.Expect(resourceToApiResourceList(resource.Resource)).To(gomega.Equal(test.expectedResources[i]))
			}
		}
	}
}

func TestResourceInNamespace(t *testing.T) {
	var (
		err               error
		g                 = gomega.NewWithT(t)
		fakeKubeClient    = fake.NewSimpleClientset()
		namespace         = "test_ns"
		fakeDynamicClient = fakeDynamic.NewSimpleDynamicClient(runtime.NewScheme())
		fakeDiscovery     = &fakeDiscovery.FakeDiscovery{}
	)

	tests := []struct {
		resource string
		name     string
	}{
		{
			resource: "deployment",
			name:     "test_deploy",
		},
		{
			resource: "service",
			name:     "test_service",
		},
		{
			resource: "hpa",
			name:     "test_hpa",
		},
		{
			resource: "horizontalpodautoscaler",
			name:     "test_hpa",
		},
		{
			resource: "pdb",
			name:     "test_pdb",
		},
		{
			resource: "poddisruptionbudget",
			name:     "test_pdb",
		},
		{
			resource: "serviceaccount",
			name:     "mock_service_account",
		},
	}

	kc := ClientSet{
		KubeInterface:      fakeKubeClient,
		DynamicInterface:   fakeDynamicClient,
		DiscoveryInterface: fakeDiscovery,
	}

	_, _ = kc.KubeInterface.CoreV1().Namespaces().Create(context.Background(), &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
		Status: v1.NamespaceStatus{Phase: v1.NamespaceActive},
	}, metav1.CreateOptions{})

	for _, tt := range tests {
		t.Run(tt.resource, func(t *testing.T) {
			meta := metav1.ObjectMeta{
				Name: tt.name,
			}

			switch tt.resource {
			case "deployment":
				_, _ = kc.KubeInterface.AppsV1().Deployments(namespace).Create(context.Background(), &appsv1.Deployment{
					ObjectMeta: meta,
				}, metav1.CreateOptions{})
			case "service":
				_, _ = kc.KubeInterface.CoreV1().Services(namespace).Create(context.Background(), &v1.Service{
					ObjectMeta: meta,
				}, metav1.CreateOptions{})
			case "hpa":
				_, _ = kc.KubeInterface.AutoscalingV2beta2().HorizontalPodAutoscalers(namespace).Create(context.Background(), &hpa.HorizontalPodAutoscaler{
					ObjectMeta: meta,
				}, metav1.CreateOptions{})
			case "pdb":
				_, _ = kc.KubeInterface.PolicyV1beta1().PodDisruptionBudgets(namespace).Create(context.Background(), &policy.PodDisruptionBudget{
					ObjectMeta: meta,
				}, metav1.CreateOptions{})
			case "serviceaccount":
				_, _ = kc.KubeInterface.CoreV1().ServiceAccounts(namespace).Create(context.Background(), &v1.ServiceAccount{
					ObjectMeta: meta,
				}, metav1.CreateOptions{})
			}
			err = kc.ResourceInNamespace(tt.resource, tt.name, namespace)
			g.Expect(err).ShouldNot(gomega.HaveOccurred())
		})
	}
}

func TestScaleDeployment(t *testing.T) {
	var (
		err            error
		g              = gomega.NewWithT(t)
		fakeKubeClient = fake.NewSimpleClientset()
		namespace      = "test_ns"
		deployName     = "test_deploy"
		replicaCount   = int32(1)
	)

	kc := ClientSet{
		KubeInterface: fakeKubeClient,
	}

	_, _ = kc.KubeInterface.CoreV1().Namespaces().Create(context.Background(), &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
		Status: v1.NamespaceStatus{Phase: v1.NamespaceActive},
	}, metav1.CreateOptions{})

	_, _ = kc.KubeInterface.AppsV1().Deployments(namespace).Create(context.Background(), &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name: deployName,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicaCount,
		},
	}, metav1.CreateOptions{})
	err = kc.ScaleDeployment(deployName, namespace, 2)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	s, _ := kc.KubeInterface.AppsV1().Deployments(namespace).GetScale(context.Background(), deployName, metav1.GetOptions{})
	g.Expect(s.Spec.Replicas).To(gomega.Equal(int32(2)))
}

func resourceFromYaml(resourceFileName string) (*unstructured.Unstructured, error) {

	resourcePath := filepath.Join("../../test/templates", resourceFileName)
	d, err := ioutil.ReadFile(resourcePath)
	if err != nil {
		return nil, err
	}
	return resourceFromBytes(d)
}

func resourceFromBytes(bytes []byte) (*unstructured.Unstructured, error) {
	resource := &unstructured.Unstructured{}
	dec := serializer.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err := dec.Decode(bytes, nil, resource)
	if err != nil {
		return nil, err
	}

	return resource, nil
}

func newTestAPIResourceList(apiVersion, name, kind string) *metav1.APIResourceList {
	return &metav1.APIResourceList{
		GroupVersion: apiVersion,
		APIResources: []metav1.APIResource{
			{
				Name:       name,
				Kind:       kind,
				Namespaced: true,
			},
		},
	}
}

func addLabel(in *unstructured.Unstructured, key, value string) {
	labels, _, _ := unstructured.NestedMap(in.Object, "metadata", "labels")

	labels[key] = value

	err := unstructured.SetNestedMap(in.Object, labels, "metadata", "labels")
	if err != nil {
		log.Errorf("Failed adding label %v=%v to the resource %v: %v", key, value, in.GetName(), err)
	}
}

func TestClusterRoleAndBindingIsFound(t *testing.T) {
	var (
		err            error
		g              = gomega.NewWithT(t)
		fakeKubeClient = fake.NewSimpleClientset()
	)

	kc := ClientSet{
		KubeInterface: fakeKubeClient,
	}

	tests := []struct {
		resource string
		name     string
	}{
		{
			resource: "clusterrole",
			name:     "mock_cluster_role",
		},
		{
			resource: "clusterrolebinding",
			name:     "mock_cluster_role_binding",
		},
	}

	for _, tt := range tests {
		t.Run(tt.resource, func(t *testing.T) {
			meta := metav1.ObjectMeta{
				Name: tt.name,
			}

			switch tt.resource {
			case "clusterrole":
				_, _ = kc.KubeInterface.RbacV1().ClusterRoles().Create(context.Background(), &rbacv1.ClusterRole{

					ObjectMeta: meta,
				}, metav1.CreateOptions{})
			case "clusterrolebinding":
				_, _ = kc.KubeInterface.RbacV1().ClusterRoleBindings().Create(context.Background(), &rbacv1.ClusterRoleBinding{

					ObjectMeta: meta,
				}, metav1.CreateOptions{})
			}
			err = kc.ClusterRbacIsFound(tt.resource, tt.name)
			g.Expect(err).ShouldNot(gomega.HaveOccurred())
		})
	}
}

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
			kc := &ClientSet{
				KubeInterface:      tt.fields.KubeInterface,
				DynamicInterface:   tt.fields.DynamicInterface,
				DiscoveryInterface: tt.fields.DiscoveryInterface,
			}
			if err := kc.PodsInNamespaceWithSelectorShouldHaveLabels(tt.args.namespace, tt.args.selector, tt.args.labels); (err != nil) != tt.wantErr {
				t.Errorf("ThePodsInNamespaceWithSelectorShouldHaveLabels() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
