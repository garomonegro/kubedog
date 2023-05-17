package kube

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	util "github.com/keikoproj/kubedog/internal/utilities"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TODO: maybe make this its own pkg and have them take the client as input?
func (kc *ClientSet) ResourceOperation(operation, resourceFileName string) error {
	return kc.ResourceOperationInNamespace(operation, resourceFileName, "")
}

func (kc *ClientSet) ResourceOperationInNamespace(operation, resourceFileName, ns string) error {
	unstructuredResource, err := kc.parseSingleResource(resourceFileName)
	if err != nil {
		return err
	}
	return kc.unstructuredResourceOperation(operation, ns, unstructuredResource)
}

func (kc *ClientSet) parseSingleResource(resourceFileName string) (util.K8sUnstructuredResource, error) {
	if err := kc.Validate(); err != nil {
		return util.K8sUnstructuredResource{}, err
	}

	resourcePath := kc.getResourcePath(resourceFileName)
	unstructuredResource, err := util.GetResourceFromYaml(resourcePath, kc.DiscoveryInterface, kc.TemplateArguments)
	if err != nil {
		return util.K8sUnstructuredResource{}, err
	}

	return unstructuredResource, nil
}

func (kc *ClientSet) MultiResourceOperation(operation, resourceFileName string) error {
	resourceList, err := kc.parseMultipleResources(resourceFileName)
	if err != nil {
		return err
	}

	for _, unstructuredResource := range resourceList {
		err = kc.unstructuredResourceOperation(operation, "", unstructuredResource)
		if err != nil {
			return err
		}
	}

	return nil
}

func (kc *ClientSet) MultiResourceOperationInNamespace(operation, resourceFileName, ns string) error {
	resourceList, err := kc.parseMultipleResources(resourceFileName)
	if err != nil {
		return err
	}

	for _, unstructuredResource := range resourceList {
		err = kc.unstructuredResourceOperation(operation, ns, unstructuredResource)
		if err != nil {
			return err
		}
	}

	return nil
}

func (kc *ClientSet) parseMultipleResources(resourceFileName string) ([]util.K8sUnstructuredResource, error) {
	if err := kc.Validate(); err != nil {
		return nil, err
	}

	resourcePath := kc.getResourcePath(resourceFileName)

	resourceList, err := util.GetMultipleResourcesFromYaml(resourcePath, kc.DiscoveryInterface, kc.TemplateArguments)
	if err != nil {
		return nil, err
	}

	return resourceList, nil
}

func (kc *ClientSet) unstructuredResourceOperation(operation, ns string, unstructuredResource util.K8sUnstructuredResource) error {
	gvr, resource := unstructuredResource.GVR, unstructuredResource.Resource

	if ns == "" {
		ns = resource.GetNamespace()
	}

	switch operation {
	case operationCreate, operationSubmit:
		_, err := kc.DynamicInterface.Resource(gvr.Resource).Namespace(ns).Create(context.Background(), resource, metav1.CreateOptions{})
		if err != nil {
			if kerrors.IsAlreadyExists(err) {
				log.Infof("%s %s already created", resource.GetKind(), resource.GetName())
				break
			}
			return err
		}
		log.Infof("%s %s has been created in namespace %s", resource.GetKind(), resource.GetName(), ns)
	case operationUpdate:
		currentResourceVersion, err := kc.DynamicInterface.Resource(gvr.Resource).Namespace(ns).Get(context.Background(), resource.GetName(), metav1.GetOptions{})
		if err != nil {
			return err
		}

		resource.SetResourceVersion(currentResourceVersion.DeepCopy().GetResourceVersion())

		_, err = kc.DynamicInterface.Resource(gvr.Resource).Namespace(ns).Update(context.Background(), resource, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		log.Infof("%s %s has been updated in namespace %s", resource.GetKind(), resource.GetName(), ns)
	case operationDelete:
		err := kc.DynamicInterface.Resource(gvr.Resource).Namespace(ns).Delete(context.Background(), resource.GetName(), metav1.DeleteOptions{})
		if err != nil {
			if kerrors.IsNotFound(err) {
				log.Infof("%s %s already deleted", resource.GetKind(), resource.GetName())
				break
			}
			return err
		}
		log.Infof("%s %s has been deleted from namespace %s", resource.GetKind(), resource.GetName(), ns)
	default:
		return fmt.Errorf("unsupported operation: %s", operation)
	}
	return nil
}

func (kc *ClientSet) ResourceOperationWithResult(operation, resourceFileName, expectedResult string) error {
	return kc.ResourceOperationWithResultInNamespace(operation, resourceFileName, "", expectedResult)
}

func (kc *ClientSet) ResourceOperationWithResultInNamespace(operation, resourceFileName, namespace, expectedResult string) error {
	var expectError = strings.EqualFold(expectedResult, "fail")
	err := kc.ResourceOperationInNamespace(operation, resourceFileName, namespace)
	if !expectError && err != nil {
		return fmt.Errorf("unexpected error when %s %s: %s", operation, resourceFileName, err.Error())
	} else if expectError && err == nil {
		return fmt.Errorf("expected error when %s %s, but received none", operation, resourceFileName)
	}
	return nil
}

func (kc *ClientSet) ResourceShouldBe(resourceFileName, state string) error {
	var (
		exists  bool
		counter int
	)

	if err := kc.Validate(); err != nil {
		return err
	}

	resourcePath := kc.getResourcePath(resourceFileName)

	unstructuredResource, err := util.GetResourceFromYaml(resourcePath, kc.DiscoveryInterface, kc.TemplateArguments)
	if err != nil {
		return err
	}
	gvr, resource := unstructuredResource.GVR, unstructuredResource.Resource
	for {
		exists = true
		if counter >= kc.getWaiterTries() {
			return errors.New("waiter timed out waiting for resource state")
		}
		log.Infof("[KUBEDOG] waiting for resource %v/%v to become %v", resource.GetNamespace(), resource.GetName(), state)

		_, err := kc.DynamicInterface.Resource(gvr.Resource).Namespace(resource.GetNamespace()).Get(context.Background(), resource.GetName(), metav1.GetOptions{})
		if err != nil {
			if !kerrors.IsNotFound(err) {
				return err
			}
			log.Infof("[KUBEDOG] %v/%v is not found: %v", resource.GetNamespace(), resource.GetName(), err)
			exists = false
		}

		switch state {
		case stateDeleted:
			if !exists {
				log.Infof("[KUBEDOG] %v/%v is deleted", resource.GetNamespace(), resource.GetName())
				return nil
			}
		case stateCreated:
			if exists {
				log.Infof("[KUBEDOG] %v/%v is created", resource.GetNamespace(), resource.GetName())
				return nil
			}
		}
		counter++
		time.Sleep(kc.getWaiterInterval())
	}
}

func (kc *ClientSet) ResourceShouldConvergeToSelector(resourceFileName, selector string) error {
	var counter int

	if err := kc.Validate(); err != nil {
		return err
	}

	split := util.DeleteEmpty(strings.Split(selector, "="))
	if len(split) != 2 {
		return errors.Errorf("Selector '%s' should meet format '<key>=<value>'", selector)
	}

	key := split[0]
	value := split[1]

	keySlice := util.DeleteEmpty(strings.Split(key, "."))
	if len(keySlice) < 1 {
		return errors.Errorf("Found empty 'key' in selector '%s' of form '<key>=<value>'", selector)
	}

	resourcePath := kc.getResourcePath(resourceFileName)

	unstructuredResource, err := util.GetResourceFromYaml(resourcePath, kc.DiscoveryInterface, kc.TemplateArguments)
	if err != nil {
		return err
	}
	gvr, resource := unstructuredResource.GVR, unstructuredResource.Resource

	for {
		if counter >= kc.getWaiterTries() {
			return errors.New("waiter timed out waiting for resource")
		}
		//TODO: configure the logger to output "[KUBEDOG]" instead typing it in each log
		log.Infof("[KUBEDOG] waiting for resource %v/%v to converge to %v=%v", resource.GetNamespace(), resource.GetName(), key, value)
		cr, err := kc.DynamicInterface.Resource(gvr.Resource).Namespace(resource.GetNamespace()).Get(context.Background(), resource.GetName(), metav1.GetOptions{})
		if err != nil {
			return err
		}

		if val, ok, err := unstructured.NestedString(cr.UnstructuredContent(), keySlice...); ok {
			if err != nil {
				return err
			}
			if strings.EqualFold(val, value) {
				break
			}
		}
		counter++
		time.Sleep(kc.getWaiterInterval())
	}

	return nil
}

func (kc *ClientSet) ResourceConditionShouldBe(resourceFileName, cType, status string) error {
	var (
		counter        int
		expectedStatus = cases.Title(language.English).String(status)
	)

	if err := kc.Validate(); err != nil {
		return err
	}

	resourcePath := kc.getResourcePath(resourceFileName)
	unstructuredResource, err := util.GetResourceFromYaml(resourcePath, kc.DiscoveryInterface, kc.TemplateArguments)
	if err != nil {
		return err
	}
	gvr, resource := unstructuredResource.GVR, unstructuredResource.Resource

	for {
		if counter >= kc.getWaiterTries() {
			return errors.New("waiter timed out waiting for resource state")
		}
		log.Infof("[KUBEDOG] waiting for resource %v/%v to meet condition %v=%v", resource.GetNamespace(), resource.GetName(), cType, expectedStatus)
		cr, err := kc.DynamicInterface.Resource(gvr.Resource).Namespace(resource.GetNamespace()).Get(context.Background(), resource.GetName(), metav1.GetOptions{})
		if err != nil {
			return err
		}

		if conditions, ok, err := unstructured.NestedSlice(cr.UnstructuredContent(), "status", "conditions"); ok {
			if err != nil {
				return err
			}

			for _, c := range conditions {
				condition, ok := c.(map[string]interface{})
				if !ok {
					continue
				}
				tp, found := condition["type"]
				if !found {
					continue
				}
				condType, ok := tp.(string)
				if !ok {
					continue
				}
				if condType == cType {
					status := condition["status"].(string)
					if corev1.ConditionStatus(status) == corev1.ConditionStatus(expectedStatus) {
						return nil
					}
				}
			}
		}
		counter++
		time.Sleep(kc.getWaiterInterval())
	}
}

func (kc *ClientSet) UpdateResourceWithField(resourceFileName, key string, value string) error {
	var (
		keySlice     = util.DeleteEmpty(strings.Split(key, "."))
		overrideType bool
		intValue     int64
		//err          error
	)

	if err := kc.Validate(); err != nil {
		return err
	}

	resourcePath := kc.getResourcePath(resourceFileName)
	unstructuredResource, err := util.GetResourceFromYaml(resourcePath, kc.DiscoveryInterface, kc.TemplateArguments)
	if err != nil {
		return err
	}
	gvr, resource := unstructuredResource.GVR, unstructuredResource.Resource

	n, err := strconv.ParseInt(value, 10, 64)
	if err == nil {
		overrideType = true
		intValue = n
	}

	updateTarget, err := kc.DynamicInterface.Resource(gvr.Resource).Namespace(resource.GetNamespace()).Get(context.Background(), resource.GetName(), metav1.GetOptions{})
	if err != nil {
		return err
	}

	switch overrideType {
	case true:
		if err := unstructured.SetNestedField(updateTarget.UnstructuredContent(), intValue, keySlice...); err != nil {
			return err
		}
	case false:
		if err := unstructured.SetNestedField(updateTarget.UnstructuredContent(), value, keySlice...); err != nil {
			return err
		}
	}

	_, err = kc.DynamicInterface.Resource(gvr.Resource).Namespace(resource.GetNamespace()).Update(context.Background(), updateTarget, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	time.Sleep(kc.getWaiterInterval())
	return nil
}

func (kc *ClientSet) DeleteResourcesAtPath(resourcesPath string) error {

	// Getting context
	err := kc.DiscoverClients()
	if err != nil {
		return errors.Errorf("Failed getting the kubernetes client: %v", err)
	}

	var deleteFn = func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if info.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}

		unstructuredResource, err := util.GetResourceFromYaml(path, kc.DiscoveryInterface, kc.TemplateArguments)
		if err != nil {
			return err
		}
		gvr, resource := unstructuredResource.GVR, unstructuredResource.Resource

		err = kc.DynamicInterface.Resource(gvr.Resource).Namespace(resource.GetNamespace()).Delete(context.Background(), resource.GetName(), metav1.DeleteOptions{})
		if err != nil {
			return err
		}
		log.Infof("[KUBEDOG] submitted deletion for %v/%v", resource.GetNamespace(), resource.GetName())
		return nil
	}

	var waitFn = func(path string, info os.FileInfo, walkErr error) error {
		var (
			counter int
		)

		if walkErr != nil {
			return walkErr
		}

		if info.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}

		unstructuredResource, err := util.GetResourceFromYaml(path, kc.DiscoveryInterface, kc.TemplateArguments)
		if err != nil {
			return err
		}
		gvr, resource := unstructuredResource.GVR, unstructuredResource.Resource

		for {
			if counter >= kc.getWaiterTries() {
				return errors.New("waiter timed out waiting for deletion")
			}
			log.Infof("[KUBEDOG] waiting for resource deletion of %v/%v", resource.GetNamespace(), resource.GetName())
			_, err := kc.DynamicInterface.Resource(gvr.Resource).Namespace(resource.GetNamespace()).Get(context.Background(), resource.GetName(), metav1.GetOptions{})
			if err != nil {
				if kerrors.IsNotFound(err) {
					log.Infof("[KUBEDOG] resource %v/%v is deleted", resource.GetNamespace(), resource.GetName())
					break
				}
			}
			counter++
			time.Sleep(kc.getWaiterInterval())
		}
		return nil
	}

	if err := filepath.Walk(resourcesPath, deleteFn); err != nil {
		return err
	}
	if err := filepath.Walk(resourcesPath, waitFn); err != nil {
		return err
	}

	return nil
}
