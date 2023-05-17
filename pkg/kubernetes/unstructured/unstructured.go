package unstructured

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
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
)

// TODO: seems not used, check and delete
const (
	operationCreate = "create"
	operationSubmit = "submit"
	operationUpdate = "update"
	operationDelete = "delete"

	stateCreated = "created"
	stateDeleted = "deleted"
	//stateUpgraded = "upgraded"
	//stateReady = "ready"
	//stateFound = "found"
)

type WaiterConfig struct {
	tries    int
	interval time.Duration
}

func (w WaiterConfig) getInterval() time.Duration {
	defaultWaiterInterval := time.Second * 30
	if w.interval > 0 {
		return w.interval
	}
	return defaultWaiterInterval
}

func (w WaiterConfig) getTries() int {
	defaultWaiterTries := 40
	if w.tries > 0 {
		return w.tries
	}
	return defaultWaiterTries
}

//kc.TemplateArguments

// TODO: maybe make this its own pkg and have them take the client as input?
func ResourceOperation(dynamicClient dynamic.Interface, dc discovery.DiscoveryInterface, operation, resourceFilePath string) error {
	return ResourceOperationInNamespace(dynamicClient, dc, operation, resourceFilePath, "")
}

// TODO: use unstructuredResourceOperation directly, call parseSingleResource from kube.go
func ResourceOperationInNamespace(dynamicClient dynamic.Interface, dc discovery.DiscoveryInterface, unstructuredResource util.K8sUnstructuredResource, operation, ns, resourceFilePath string) error {
	unstructuredResource, err := getResource(dc, resourceFilePath)
	if err != nil {
		return err
	}
	return unstructuredResourceOperation(dynamicClient, operation, ns, unstructuredResource)
}

func getResource(dc discovery.DiscoveryInterface, TemplateArguments interface{}, resourceFilePath string) (util.K8sUnstructuredResource, error) {
	unstructuredResource, err := util.GetResourceFromYaml(resourceFilePath, dc, TemplateArguments)
	if err != nil {
		return util.K8sUnstructuredResource{}, err
	}
	return unstructuredResource, nil
}

func getResources(dc discovery.DiscoveryInterface, TemplateArguments interface{}, resourcesFilePath string) ([]util.K8sUnstructuredResource, error) {
	resourceList, err := util.GetMultipleResourcesFromYaml(resourcesFilePath, dc, TemplateArguments)
	if err != nil {
		return nil, err
	}
	return resourceList, nil
}

func validateDynamicClient(dynamicClient dynamic.Interface) error {
	if dynamicClient == nil {
		return errors.Errorf("'k8s.io/client-go/dynamic.Interface' is nil.")
	}
	return nil
}

func MultiResourceOperation(dynamicClient dynamic.Interface, dc discovery.DiscoveryInterface, operation, resourceFilePath string) error {
	resourceList, err := getResources(dc, resourceFilePath)
	if err != nil {
		return err
	}

	for _, unstructuredResource := range resourceList {
		err = unstructuredResourceOperation(dynamicClient, operation, "", unstructuredResource)
		if err != nil {
			return err
		}
	}

	return nil
}

func MultiResourceOperationInNamespace(dynamicClient dynamic.Interface, dc discovery.DiscoveryInterface, operation, resourceFilePath, ns string) error {
	resourceList, err := getResources(dc, resourceFilePath)
	if err != nil {
		return err
	}

	for _, unstructuredResource := range resourceList {
		err = unstructuredResourceOperation(dynamicClient, operation, ns, unstructuredResource)
		if err != nil {
			return err
		}
	}

	return nil
}

func unstructuredResourceOperation(dynamicClient dynamic.Interface, operation, ns string, unstructuredResource util.K8sUnstructuredResource) error {
	if err := validateDynamicClient(dynamicClient); err != nil {
		return err
	}

	gvr, resource := unstructuredResource.GVR, unstructuredResource.Resource

	if ns == "" {
		ns = resource.GetNamespace()
	}

	switch operation {
	case operationCreate, operationSubmit:
		_, err := dynamicClient.Resource(gvr.Resource).Namespace(ns).Create(context.Background(), resource, metav1.CreateOptions{})
		if err != nil {
			if kerrors.IsAlreadyExists(err) {
				log.Infof("%s %s already created", resource.GetKind(), resource.GetName())
				break
			}
			return err
		}
		log.Infof("%s %s has been created in namespace %s", resource.GetKind(), resource.GetName(), ns)
	case operationUpdate:
		currentResourceVersion, err := dynamicClient.Resource(gvr.Resource).Namespace(ns).Get(context.Background(), resource.GetName(), metav1.GetOptions{})
		if err != nil {
			return err
		}

		resource.SetResourceVersion(currentResourceVersion.DeepCopy().GetResourceVersion())

		_, err = dynamicClient.Resource(gvr.Resource).Namespace(ns).Update(context.Background(), resource, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		log.Infof("%s %s has been updated in namespace %s", resource.GetKind(), resource.GetName(), ns)
	case operationDelete:
		err := dynamicClient.Resource(gvr.Resource).Namespace(ns).Delete(context.Background(), resource.GetName(), metav1.DeleteOptions{})
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

func ResourceOperationWithResult(dynamicClient dynamic.Interface, dc discovery.DiscoveryInterface, operation, resourceFilePath, expectedResult string) error {
	return ResourceOperationWithResultInNamespace(dynamicClient, dc, operation, resourceFilePath, "", expectedResult)
}

func ResourceOperationWithResultInNamespace(dynamicClient dynamic.Interface, dc discovery.DiscoveryInterface, operation, resourceFilePath, namespace, expectedResult string) error {
	var expectError = strings.EqualFold(expectedResult, "fail")
	err := ResourceOperationInNamespace(dynamicClient, dc, operation, resourceFilePath, namespace)
	if !expectError && err != nil {
		return fmt.Errorf("unexpected error when '%s' '%s': '%s'", operation, resourceFilePath, err.Error())
	} else if expectError && err == nil {
		return fmt.Errorf("expected error when '%s' '%s', but received none", operation, resourceFilePath)
	}
	return nil
}

func ResourceShouldBe(dynamicClient dynamic.Interface, dc discovery.DiscoveryInterface, w WaiterConfig, resourceFilePath, state string) error {
	var (
		exists  bool
		counter int
	)

	if err := validateDynamicClient(dynamicClient); err != nil {
		return err
	}

	unstructuredResource, err := getResource(dc, resourceFilePath)
	if err != nil {
		return err
	}
	gvr, resource := unstructuredResource.GVR, unstructuredResource.Resource
	for {
		exists = true
		if counter >= w.getTries() {
			return errors.New("waiter timed out waiting for resource state")
		}
		log.Infof("[KUBEDOG] waiting for resource %v/%v to become %v", resource.GetNamespace(), resource.GetName(), state)

		_, err := dynamicClient.Resource(gvr.Resource).Namespace(resource.GetNamespace()).Get(context.Background(), resource.GetName(), metav1.GetOptions{})
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
		time.Sleep(w.getInterval())
	}
}

func ResourceShouldConvergeToSelector(dynamicClient dynamic.Interface, dc discovery.DiscoveryInterface, w WaiterConfig, resourceFilePath, selector string) error {
	var counter int

	if err := validateDynamicClient(dynamicClient); err != nil {
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

	unstructuredResource, err := getResource(dc, resourceFilePath)
	if err != nil {
		return err
	}
	gvr, resource := unstructuredResource.GVR, unstructuredResource.Resource

	for {
		if counter >= w.getTries() {
			return errors.New("waiter timed out waiting for resource")
		}
		//TODO: configure the logger to output "[KUBEDOG]" instead typing it in each log
		log.Infof("[KUBEDOG] waiting for resource %v/%v to converge to %v=%v", resource.GetNamespace(), resource.GetName(), key, value)
		cr, err := dynamicClient.Resource(gvr.Resource).Namespace(resource.GetNamespace()).Get(context.Background(), resource.GetName(), metav1.GetOptions{})
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
		time.Sleep(w.getInterval())
	}

	return nil
}

func ResourceConditionShouldBe(dynamicClient dynamic.Interface, dc discovery.DiscoveryInterface, w WaiterConfig, resourceFilePath, cType, status string) error {
	var (
		counter        int
		expectedStatus = cases.Title(language.English).String(status)
	)

	if err := validateDynamicClient(dynamicClient); err != nil {
		return err
	}

	unstructuredResource, err := getResource(dc, resourceFilePath)
	if err != nil {
		return err
	}
	gvr, resource := unstructuredResource.GVR, unstructuredResource.Resource

	for {
		if counter >= w.getTries() {
			return errors.New("waiter timed out waiting for resource state")
		}
		log.Infof("[KUBEDOG] waiting for resource %v/%v to meet condition %v=%v", resource.GetNamespace(), resource.GetName(), cType, expectedStatus)
		cr, err := dynamicClient.Resource(gvr.Resource).Namespace(resource.GetNamespace()).Get(context.Background(), resource.GetName(), metav1.GetOptions{})
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
		time.Sleep(w.getInterval())
	}
}

func UpdateResourceWithField(dynamicClient dynamic.Interface, dc discovery.DiscoveryInterface, resourceFilePath, key string, value string) error {
	var (
		keySlice     = util.DeleteEmpty(strings.Split(key, "."))
		overrideType bool
		intValue     int64
		//err          error
	)

	if err := validateDynamicClient(dynamicClient); err != nil {
		return err
	}

	unstructuredResource, err := getResource(dc, resourceFilePath)
	if err != nil {
		return err
	}
	gvr, resource := unstructuredResource.GVR, unstructuredResource.Resource

	n, err := strconv.ParseInt(value, 10, 64)
	if err == nil {
		overrideType = true
		intValue = n
	}

	updateTarget, err := dynamicClient.Resource(gvr.Resource).Namespace(resource.GetNamespace()).Get(context.Background(), resource.GetName(), metav1.GetOptions{})
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

	_, err = dynamicClient.Resource(gvr.Resource).Namespace(resource.GetNamespace()).Update(context.Background(), updateTarget, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}

func DeleteResourcesAtPath(dynamicClient dynamic.Interface, dc discovery.DiscoveryInterface, w WaiterConfig, resourcesPath string) error {
	if err := validateDynamicClient(dynamicClient); err != nil {
		return err
	}

	var deleteFn = func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if info.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}

		unstructuredResource, err := getResource(dc, path)
		if err != nil {
			return err
		}
		gvr, resource := unstructuredResource.GVR, unstructuredResource.Resource

		err = dynamicClient.Resource(gvr.Resource).Namespace(resource.GetNamespace()).Delete(context.Background(), resource.GetName(), metav1.DeleteOptions{})
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

		unstructuredResource, err := getResource(dc, path)
		if err != nil {
			return err
		}
		gvr, resource := unstructuredResource.GVR, unstructuredResource.Resource

		for {
			if counter >= w.getTries() {
				return errors.New("waiter timed out waiting for deletion")
			}
			log.Infof("[KUBEDOG] waiting for resource deletion of %v/%v", resource.GetNamespace(), resource.GetName())
			_, err := dynamicClient.Resource(gvr.Resource).Namespace(resource.GetNamespace()).Get(context.Background(), resource.GetName(), metav1.GetOptions{})
			if err != nil {
				if kerrors.IsNotFound(err) {
					log.Infof("[KUBEDOG] resource %v/%v is deleted", resource.GetNamespace(), resource.GetName())
					break
				}
			}
			counter++
			time.Sleep(w.getInterval())
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
