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
	"testing"

	util "github.com/keikoproj/kubedog/internal/utilities"
	"github.com/onsi/gomega"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakeDiscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/dynamic"
	fakeDynamic "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
	kTesting "k8s.io/client-go/testing"
)

func TestPositiveResourceOperation(t *testing.T) {
	var (
		err               error
		g                 = gomega.NewWithT(t)
		dynScheme         = runtime.NewScheme()
		fakeDynamicClient = fakeDynamic.NewSimpleDynamicClient(dynScheme)
		testResource      *unstructured.Unstructured
		fakeDiscovery     = fakeDiscovery.FakeDiscovery{}
		fakeClient        *fake.Clientset
	)

	const fileName = "test-resourcefile.yaml"

	testResource, err = resourceFromYaml(fileName)
	if err != nil {
		t.Errorf("Failed getting the test resource from the file %v: %v", fileName, err)
	}

	fakeDiscovery.Fake = &fakeDynamicClient.Fake
	fakeDiscovery.Resources = append(fakeDiscovery.Resources, newTestAPIResourceList(testResource.GetAPIVersion(), testResource.GetName(), testResource.GetKind()))

	kc := ClientSet{
		KubeInterface:      fakeClient,
		DynamicInterface:   fakeDynamicClient,
		DiscoveryInterface: &fakeDiscovery,
		FilesPath:          "../../test/templates",
	}

	err = kc.ResourceOperation(operationCreate, fileName)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	err = kc.ResourceOperation(operationDelete, fileName)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
}

func TestPositiveResourceShouldBe(t *testing.T) {
	var (
		err                 error
		g                   = gomega.NewWithT(t)
		dynScheme           = runtime.NewScheme()
		fakeDynamicClient   = fakeDynamic.NewSimpleDynamicClient(dynScheme)
		fakeDiscovery       = fakeDiscovery.FakeDiscovery{}
		fakeClient          *fake.Clientset
		testResource        *unstructured.Unstructured
		createdReactionFunc = func(action kTesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, testResource, nil
		}
		deletedReactionFunc = func(action kTesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, nil, kerrors.NewNotFound(schema.GroupResource{}, testResource.GetName())
		}
	)

	const fileName = "test-resourcefile.yaml"

	testResource, err = resourceFromYaml(fileName)
	if err != nil {
		t.Errorf("Failed getting the test resource from the file %v: %v", fileName, err)
	}

	fakeDiscovery.Fake = &fakeDynamicClient.Fake
	fakeDiscovery.Resources = append(fakeDiscovery.Resources, newTestAPIResourceList(testResource.GetAPIVersion(), testResource.GetName(), testResource.GetKind()))

	fakeDynamicClient.PrependReactor("get", "someResource", createdReactionFunc)

	kc := ClientSet{
		DynamicInterface:   fakeDynamicClient,
		DiscoveryInterface: &fakeDiscovery,
		KubeInterface:      fakeClient,
		FilesPath:          "../../test/templates",
	}

	err = kc.ResourceShouldBe(fileName, stateCreated)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())

	fakeDiscovery.ReactionChain[0] = &kTesting.SimpleReactor{
		Verb:     "get",
		Resource: "someResource",
		Reaction: deletedReactionFunc,
	}

	err = kc.ResourceShouldBe(fileName, stateDeleted)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
}

func TestPositiveResourceShouldConvergeToSelector(t *testing.T) {

	var (
		err               error
		g                 = gomega.NewWithT(t)
		fakeDynamicClient = fakeDynamic.NewSimpleDynamicClient(runtime.NewScheme())
		fakeDiscovery     = fakeDiscovery.FakeDiscovery{}
		fakeClient        *fake.Clientset
		testResource      *unstructured.Unstructured
		getReactionFunc   = func(action kTesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, testResource, nil
		}
	)

	const (
		fileName = "test-resourcefile.yaml"
		selector = ".metadata.labels.someTestKey=someTestValue"
	)

	testResource, err = resourceFromYaml(fileName)
	if err != nil {
		t.Errorf("Failed getting the test resource from the file %v: %v", fileName, err)
	}

	fakeDiscovery.Fake = &fakeDynamicClient.Fake
	fakeDiscovery.Resources = append(fakeDiscovery.Resources, newTestAPIResourceList(testResource.GetAPIVersion(), testResource.GetName(), testResource.GetKind()))

	fakeDynamicClient.PrependReactor("get", "someResource", getReactionFunc)

	kc := ClientSet{
		DynamicInterface:   fakeDynamicClient,
		DiscoveryInterface: &fakeDiscovery,
		KubeInterface:      fakeClient,
		FilesPath:          "../../test/templates",
	}

	err = kc.ResourceShouldConvergeToSelector(fileName, selector)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
}

func TestPositiveResourceConditionShouldBe(t *testing.T) {

	var (
		err               error
		g                 = gomega.NewWithT(t)
		fakeDynamicClient = fakeDynamic.NewSimpleDynamicClient(runtime.NewScheme())
		fakeDiscovery     = fakeDiscovery.FakeDiscovery{}
		fakeClient        *fake.Clientset
		testResource      *unstructured.Unstructured
		getReactionFunc   = func(action kTesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, testResource, nil
		}
	)

	const (
		fileName            = "test-resourcefile.yaml"
		testConditionType   = "someConditionType"
		testConditionStatus = "true"
	)

	testResource, err = resourceFromYaml(fileName)
	if err != nil {
		t.Errorf("Failed getting the test resource from the file %v: %v", fileName, err)
	}

	fakeDiscovery.Fake = &fakeDynamicClient.Fake
	fakeDiscovery.Resources = append(fakeDiscovery.Resources, newTestAPIResourceList(testResource.GetAPIVersion(), testResource.GetName(), testResource.GetKind()))

	fakeDynamicClient.PrependReactor("get", "someResource", getReactionFunc)

	kc := ClientSet{
		DynamicInterface:   fakeDynamicClient,
		DiscoveryInterface: &fakeDiscovery,
		KubeInterface:      fakeClient,
		FilesPath:          "../../test/templates",
	}

	err = kc.ResourceConditionShouldBe(fileName, testConditionType, testConditionStatus)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
}

func TestPositiveUpdateResourceWithField(t *testing.T) {

	const (
		fileName           = "test-resourcefile.yaml"
		testUpdateKeyChain = ".metadata.labels.testUpdateKey"
		testUpdateKey      = "testUpdateKey"
		testUpdateValue    = "testUpdateValue"
	)

	var (
		err               error
		g                 = gomega.NewWithT(t)
		fakeDynamicClient = fakeDynamic.NewSimpleDynamicClient(runtime.NewScheme())
		fakeDiscovery     = fakeDiscovery.FakeDiscovery{}
		fakeClient        *fake.Clientset
		testResource      *unstructured.Unstructured
		getReactionFunc   = func(action kTesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, testResource, nil
		}
		updateReactionFunc = func(action kTesting.Action) (handled bool, ret runtime.Object, err error) {
			addLabel(testResource, testUpdateKey, testUpdateValue)
			return true, testResource, nil
		}
	)

	testResource, err = resourceFromYaml(fileName)
	if err != nil {
		t.Errorf("Failed getting the test resource from the file %v: %v", fileName, err)
	}

	fakeDiscovery.Fake = &fakeDynamicClient.Fake
	fakeDiscovery.Resources = append(fakeDiscovery.Resources, newTestAPIResourceList(testResource.GetAPIVersion(), testResource.GetName(), testResource.GetKind()))

	fakeDynamicClient.PrependReactor("get", "someResource", getReactionFunc)
	fakeDynamicClient.PrependReactor("update", "someResource", updateReactionFunc)

	kc := ClientSet{
		DynamicInterface:   fakeDynamicClient,
		DiscoveryInterface: &fakeDiscovery,
		KubeInterface:      fakeClient,
		FilesPath:          "../../test/templates",
	}

	err = kc.UpdateResourceWithField(fileName, testUpdateKeyChain, testUpdateValue)
	g.Expect(err).ShouldNot(gomega.HaveOccurred())

	expectedLabelValue, found, err := unstructured.NestedString(testResource.UnstructuredContent(), "metadata", "labels", "testUpdateKey")
	g.Expect(err).ShouldNot(gomega.HaveOccurred())
	g.Expect(found).To(gomega.BeTrue())
	g.Expect(expectedLabelValue).To(gomega.Equal(testUpdateValue))
}

func Test_unstructuredResourceOperation(t *testing.T) {
	type clientFields struct {
		DynamicInterface dynamic.Interface
	}
	type funcArgs struct {
		operation            string
		ns                   string
		unstructuredResource util.K8sUnstructuredResource
	}

	resourceNoNs, err := resourceFromYaml("../../test/templates/resource-without-namespace.yaml")
	if err != nil {
		t.Errorf(err.Error())
	}

	resourceNs, err := resourceFromYaml("../../test/templates/resource-with-namespace.yaml")
	if err != nil {
		t.Errorf(err.Error())
	}

	resourceNoNsUpdate, err := resourceFromYaml("../../test/templates/resource-without-namespace-update.yaml")
	if err != nil {
		t.Errorf(err.Error())
	}

	resourceNsUpdate, err := resourceFromYaml("../../test/templates/resource-with-namespace-update.yaml")
	if err != nil {
		t.Errorf(err.Error())
	}

	dynScheme := runtime.NewScheme()
	fakeDynamicClient := fakeDynamic.NewSimpleDynamicClient(dynScheme)

	tests := []struct {
		name         string
		clientFields clientFields
		funcArgs     funcArgs
		wantErr      bool
	}{
		{
			name: "Resource create succeeds when namespace is configurable",
			clientFields: clientFields{
				DynamicInterface: fakeDynamicClient,
			},
			funcArgs: funcArgs{
				operation: "create",
				ns:        "test-namespace",
				unstructuredResource: util.K8sUnstructuredResource{
					GVR:      &meta.RESTMapping{},
					Resource: resourceNoNs,
				},
			},
			wantErr: false,
		},
		{
			name: "Resource update succeeds when namespace is configurable",
			clientFields: clientFields{
				DynamicInterface: fakeDynamicClient,
			},
			funcArgs: funcArgs{
				operation: "update",
				ns:        "test-namespace",
				unstructuredResource: util.K8sUnstructuredResource{
					GVR:      &meta.RESTMapping{},
					Resource: resourceNoNsUpdate,
				},
			},
			wantErr: false,
		},
		{
			name: "Resource delete succeeds when namespace is configurable",
			clientFields: clientFields{
				DynamicInterface: fakeDynamicClient,
			},
			funcArgs: funcArgs{
				operation: "delete",
				ns:        "test-namespace",
				unstructuredResource: util.K8sUnstructuredResource{
					GVR:      &meta.RESTMapping{},
					Resource: resourceNoNs,
				},
			},
			wantErr: false,
		},
		{
			name: "Resource create succeeds when namespace in YAML",
			clientFields: clientFields{
				DynamicInterface: fakeDynamicClient,
			},
			funcArgs: funcArgs{
				operation: "create",
				unstructuredResource: util.K8sUnstructuredResource{
					GVR:      &meta.RESTMapping{},
					Resource: resourceNs,
				},
			},
			wantErr: false,
		},
		{
			name: "Resource update succeeds when namespace is in YAML",
			clientFields: clientFields{
				DynamicInterface: fakeDynamicClient,
			},
			funcArgs: funcArgs{
				operation: "update",
				unstructuredResource: util.K8sUnstructuredResource{
					GVR:      &meta.RESTMapping{},
					Resource: resourceNsUpdate,
				},
			},
			wantErr: false,
		},
		{
			name: "Resource create fails when namespace configured and in YAML",
			clientFields: clientFields{
				DynamicInterface: fakeDynamicClient,
			},
			funcArgs: funcArgs{
				operation: "create",
				ns:        "override-ns",
				unstructuredResource: util.K8sUnstructuredResource{
					GVR:      &meta.RESTMapping{},
					Resource: resourceNs,
				},
			},
			wantErr: true,
		},
		{
			name: "Unsupported operation produces error",
			clientFields: clientFields{
				DynamicInterface: fakeDynamicClient,
			},
			funcArgs: funcArgs{
				operation: "invalid",
				unstructuredResource: util.K8sUnstructuredResource{
					GVR:      &meta.RESTMapping{},
					Resource: resourceNs,
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kc := &ClientSet{
				DynamicInterface: tt.clientFields.DynamicInterface,
			}
			if err := kc.unstructuredResourceOperation(tt.funcArgs.operation, tt.funcArgs.ns, tt.funcArgs.unstructuredResource); (err != nil) != tt.wantErr {
				t.Errorf("ClientSet.unstructuredResourceOperation() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
