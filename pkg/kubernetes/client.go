package kube

import (
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	operationCreate = "create"
	operationSubmit = "submit"
	operationUpdate = "update"
	operationDelete = "delete"

	stateCreated  = "created"
	stateDeleted  = "deleted"
	stateUpgraded = "upgraded"
	stateReady    = "ready"
	stateFound    = "found"
)

type Client struct {
	KubeInterface      kubernetes.Interface
	DynamicInterface   dynamic.Interface
	DiscoveryInterface discovery.DiscoveryInterface
	FilesPath          string
	TemplateArguments  interface{}
	WaiterInterval     time.Duration
	WaiterTries        int
	Timestamps         map[string]time.Time
}

func (kc *Client) Validate() error {
	commonMessage := "'AKubernetesCluster' sets this interface, try calling it before using this method"
	if kc.DynamicInterface == nil {
		return errors.Errorf("'Client.DynamicInterface' is nil. %s", commonMessage)
	}
	if kc.DiscoveryInterface == nil {
		return errors.Errorf("'Client.DiscoveryInterface' is nil. %s", commonMessage)
	}
	if kc.KubeInterface == nil {
		return errors.Errorf("'Client.KubeInterface' is nil. %s", commonMessage)
	}
	return nil
}

// TODO: rename this method
func (kc *Client) KubernetesCluster() error {
	var (
		home, _        = os.UserHomeDir()
		kubeconfigPath = filepath.Join(home, ".kube", "config")
	)

	if exported := os.Getenv("KUBECONFIG"); exported != "" {
		kubeconfigPath = exported
	}

	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		return errors.Errorf("[KUBEDOG] expected kubeconfig to exist for create operation, '%v'", kubeconfigPath)
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return err
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Fatal("Unable to construct dynamic client", err)
	}

	_, err = client.Discovery().ServerVersion()
	if err != nil {
		return err
	}

	kc.KubeInterface = client
	kc.DynamicInterface = dynClient
	kc.DiscoveryInterface = discoveryClient

	return nil
}

func (kc *Client) SetTimestamp(timestampName string) error {
	if kc.Timestamps == nil {
		kc.Timestamps = map[string]time.Time{}
	}
	now := time.Now()
	kc.Timestamps[timestampName] = now
	log.Infof("Memorizing '%s' time is %v", timestampName, now)
	return nil
}

func (kc *Client) DeleteAllTestResources() error {
	resourcesPath := kc.getTemplatesPath()

	return kc.DeleteResourcesAtPath(resourcesPath)
}

func (kc *Client) getResourcePath(resourceFileName string) string {
	templatesPath := kc.getTemplatesPath()
	return filepath.Join(templatesPath, resourceFileName)
}

func (kc *Client) getTemplatesPath() string {
	defaultFilePath := "templates"
	if kc.FilesPath != "" {
		return kc.FilesPath
	}
	return defaultFilePath
}

func (kc *Client) getWaiterInterval() time.Duration {
	defaultWaiterInterval := time.Second * 30
	if kc.WaiterInterval > 0 {
		return kc.WaiterInterval
	}
	return defaultWaiterInterval
}

func (kc *Client) getWaiterTries() int {
	defaultWaiterTries := 40
	if kc.WaiterTries > 0 {
		return kc.WaiterTries
	}
	return defaultWaiterTries
}
