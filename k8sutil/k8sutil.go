package k8sutil

import (
	"log"
	"os"
	"time"

	"fmt"
	"github.com/Sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	coreType "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/fields"
	"k8s.io/client-go/pkg/util/homedir"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"path/filepath"
)

// KubeInterface abstracts the k8s api
type KubeInterface interface {
	Secrets(namespace string) coreType.SecretInterface
	Namespaces() coreType.NamespaceInterface
	ServiceAccounts(namespace string) coreType.ServiceAccountInterface
	Core() coreType.CoreV1Interface
}

type K8sutilInterface struct {
	Kclient            KubeInterface
	ExcludedNamespaces []string
}

// New creates a new instance of k8sutil
func New(excludedNamespaces []string) (*K8sutilInterface, error) {

	client, err := newKubeClient()

	if err != nil {
		logrus.Fatalf("Could not init Kubernetes client! [%s]", err)
	}

	k := &K8sutilInterface{
		Kclient:            client,
		ExcludedNamespaces: excludedNamespaces,
	}

	return k, nil
}

func envVarExists(key string) bool {
	_, exists := os.LookupEnv(key)
	return exists
}

// does a best guess to the location of your kubeconfig
func findKubeConfig() string {
	home := homedir.HomeDir()
	if envVarExists("KUBECONFIG") {
		kubeconfig := os.Getenv("KUBECONFIG")
		return kubeconfig
	}
	kubeconfig := fmt.Sprint(filepath.Join(home, ".kube", "config"))
	return kubeconfig
}

func newKubeClient() (KubeInterface, error) {

	var client *kubernetes.Clientset

	// we will automatically decide if this is running inside the cluster or on someones laptop
	// if the ENV vars KUBERNETES_SERVICE_HOST and KUBERNETES_SERVICE_PORT exist
	// then we can assume this app is running inside a k8s cluster
	if envVarExists("KUBERNETES_SERVICE_HOST") && envVarExists("KUBERNETES_SERVICE_PORT") {
		logrus.Info("Using InCluster k8s config")
		cfg, err := rest.InClusterConfig()

		if err != nil {
			return nil, err
		}

		client, err = kubernetes.NewForConfig(cfg)

		if err != nil {
			return nil, err
		}
	} else {
		logrus.Infof("using KUBECONFIG to determine your kubernetes connection")
		cfg, err := clientcmd.BuildConfigFromFlags("", findKubeConfig())

		if err != nil {
			logrus.Error("Got error trying to create client: ", err)
			return nil, err
		}

		client, err = kubernetes.NewForConfig(cfg)

		if err != nil {
			return nil, err
		}
	}

	return client, nil
}

// GetNamespaces returns all namespaces
func (k *K8sutilInterface) GetNamespaces() (*v1.NamespaceList, error) {
	namespaces, err := k.Kclient.Namespaces().List(v1.ListOptions{})
	if err != nil {
		logrus.Error("Error getting namespaces: ", err)
		return nil, err
	}

	return namespaces, nil
}

// GetSecret get a secret
func (k *K8sutilInterface) GetSecret(namespace, secretname string) (*v1.Secret, error) {
	secret, err := k.Kclient.Secrets(namespace).Get(secretname)
	if err != nil {
		logrus.Error("Error getting secret: ", err)
		return nil, err
	}

	return secret, nil
}

// CreateSecret creates a secret
func (k *K8sutilInterface) CreateSecret(namespace string, secret *v1.Secret) error {
	_, err := k.Kclient.Secrets(namespace).Create(secret)

	if err != nil {
		logrus.Error("Error creating secret: ", err)
		return err
	}

	return nil
}

// UpdateSecret updates a secret
func (k *K8sutilInterface) UpdateSecret(namespace string, secret *v1.Secret) error {
	_, err := k.Kclient.Secrets(namespace).Update(secret)

	if err != nil {
		logrus.Error("Error updating secret: ", err)
		return err
	}

	return nil
}

// GetServiceAccount updates a secret
func (k *K8sutilInterface) GetServiceAccount(namespace, name string) (*v1.ServiceAccount, error) {
	sa, err := k.Kclient.ServiceAccounts(namespace).Get(name)

	if err != nil {
		logrus.Error("Error getting service account: ", err)
		return nil, err
	}

	return sa, nil
}

// UpdateServiceAccount updates a secret
func (k *K8sutilInterface) UpdateServiceAccount(namespace string, sa *v1.ServiceAccount) error {
	_, err := k.Kclient.ServiceAccounts(namespace).Update(sa)

	if err != nil {
		logrus.Error("Error updating service account: ", err)
		return err
	}

	return nil
}

func (k *K8sutilInterface) WatchNamespaces(resyncPeriod time.Duration, handler func(*v1.Namespace) error) {
	stopC := make(chan struct{})
	_, c := cache.NewInformer(
		cache.NewListWatchFromClient(k.Kclient.Core().RESTClient(), "namespaces", v1.NamespaceAll, fields.Everything()),
		&v1.Namespace{},
		resyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				if err := handler(obj.(*v1.Namespace)); err != nil {
					log.Println(err)
					os.Exit(1)
				}
			},
			UpdateFunc: func(_ interface{}, obj interface{}) {
				if err := handler(obj.(*v1.Namespace)); err != nil {
					log.Println(err)
					os.Exit(1)
				}
			},
		},
	)
	c.Run(stopC)
}
