package k8sutil

import (
	"context"
	"log"
	"time"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	coreType "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

// KubeInterface abstracts the k8s api
type KubeInterface interface {
	Secrets(namespace string) coreType.SecretInterface
	Namespaces() coreType.NamespaceInterface
	ServiceAccounts(namespace string) coreType.ServiceAccountInterface
	RESTClient() rest.Interface
}

type K8sutilInterface struct {
	Kclient    KubeInterface
	MasterHost string
}

// New creates a new instance of k8sutil
func New(kubeCfgFile, masterHost string) (*K8sutilInterface, error) {

	client, err := newKubeClient(kubeCfgFile)

	if err != nil {
		logrus.Fatalf("Could not init Kubernetes client! [%s]", err)
	}

	k := &K8sutilInterface{
		Kclient:    client,
		MasterHost: masterHost,
	}

	return k, nil
}

func newKubeClient(kubeCfgFile string) (KubeInterface, error) {

	var client *kubernetes.Clientset

	// Should we use in cluster or out of cluster config
	if len(kubeCfgFile) == 0 {
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
		logrus.Infof("Using OutOfCluster k8s config with kubeConfigFile: %s", kubeCfgFile)
		cfg, err := clientcmd.BuildConfigFromFlags("", kubeCfgFile)

		if err != nil {
			logrus.Error("Got error trying to create client: ", err)
			return nil, err
		}

		client, err = kubernetes.NewForConfig(cfg)

		if err != nil {
			return nil, err
		}
	}

	return client.CoreV1(), nil
}

// GetNamespaces returns all namespaces
func (k *K8sutilInterface) GetNamespaces() (*v1.NamespaceList, error) {
	namespaces, err := k.Kclient.Namespaces().List(context.Background(), meta.ListOptions{})
	if err != nil {
		logrus.Error("Error getting namespaces: ", err)
		return nil, err
	}

	return namespaces, nil
}

// GetSecret get a secret
func (k *K8sutilInterface) GetSecret(namespace, secretname string) (*v1.Secret, error) {
	secret, err := k.Kclient.Secrets(namespace).Get(context.Background(), secretname, meta.GetOptions{})
	if err != nil {
		logrus.Error("Error getting secret: ", err)
		return nil, err
	}

	return secret, nil
}

// CreateSecret creates a secret
func (k *K8sutilInterface) CreateSecret(namespace string, secret *v1.Secret) error {
	_, err := k.Kclient.Secrets(namespace).Create(context.Background(), secret, meta.CreateOptions{})

	if err != nil {
		logrus.Error("Error creating secret: ", err)
		return err
	}

	return nil
}

// UpdateSecret updates a secret
func (k *K8sutilInterface) UpdateSecret(namespace string, secret *v1.Secret) error {
	_, err := k.Kclient.Secrets(namespace).Update(context.Background(), secret, meta.UpdateOptions{})

	if err != nil {
		logrus.Error("Error updating secret: ", err)
		return err
	}

	return nil
}

// GetServiceAccount updates a secret
func (k *K8sutilInterface) GetServiceAccount(namespace, name string) (*v1.ServiceAccount, error) {
	sa, err := k.Kclient.ServiceAccounts(namespace).Get(context.Background(), name, meta.GetOptions{})

	if err != nil {
		logrus.Error("Error getting service account: ", err)
		return nil, err
	}

	return sa, nil
}

// UpdateServiceAccount updates a secret
func (k *K8sutilInterface) UpdateServiceAccount(namespace string, sa *v1.ServiceAccount) error {
	_, err := k.Kclient.ServiceAccounts(namespace).Update(context.Background(), sa, meta.UpdateOptions{})

	if err != nil {
		logrus.Error("Error updating service account: ", err)
		return err
	}

	return nil
}

func (k *K8sutilInterface) WatchNamespaces(resyncPeriod time.Duration, handler func(*v1.Namespace) error) {
	stopC := make(chan struct{})
	_, c := cache.NewInformer(
		cache.NewListWatchFromClient(k.Kclient.RESTClient(), "namespaces", v1.NamespaceAll, fields.Everything()),
		&v1.Namespace{},
		resyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				if err := handler(obj.(*v1.Namespace)); err != nil {
					log.Println(err)
				}
			},
			UpdateFunc: func(_ interface{}, obj interface{}) {
				if err := handler(obj.(*v1.Namespace)); err != nil {
					log.Println(err)
				}
			},
		},
	)
	c.Run(stopC)
}
