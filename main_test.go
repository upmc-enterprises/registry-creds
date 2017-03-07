package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/stretchr/testify/assert"
	"github.com/upmc-enterprises/registry-creds/k8sutil"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	coreType "k8s.io/client-go/kubernetes/typed/core/v1"
	v1fake "k8s.io/client-go/kubernetes/typed/core/v1/fake"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/watch"
)

func init() {
	log.SetOutput(ioutil.Discard)
	logrus.SetOutput(ioutil.Discard)
}

type fakeKubeClient struct {
	secrets         map[string]*fakeSecrets
	namespaces      *fakeNamespaces
	serviceaccounts map[string]*fakeServiceAccounts
}

type fakeSecrets struct {
	store map[string]*v1.Secret
}

type fakeServiceAccounts struct {
	store map[string]*v1.ServiceAccount
}

type fakeNamespaces struct {
	store map[string]v1.Namespace
}

func (f *fakeKubeClient) Core() coreType.CoreV1Interface {
	return &v1fake.FakeCoreV1{}
}

func (f *fakeKubeClient) Secrets(namespace string) coreType.SecretInterface {
	return f.secrets[namespace]
}

func (f *fakeKubeClient) Namespaces() coreType.NamespaceInterface {
	return f.namespaces
}

func (f *fakeKubeClient) ServiceAccounts(namespace string) coreType.ServiceAccountInterface {
	return f.serviceaccounts[namespace]
}

func (f *fakeSecrets) Create(secret *v1.Secret) (*v1.Secret, error) {
	_, ok := f.store[secret.Name]

	if ok {
		return nil, fmt.Errorf("Secret %v already exists", secret.Name)
	}

	f.store[secret.Name] = secret
	return secret, nil
}

func (f *fakeSecrets) Update(secret *v1.Secret) (*v1.Secret, error) {
	_, ok := f.store[secret.Name]

	if !ok {
		return nil, fmt.Errorf("Secret: %v not found", secret.Name)
	}

	f.store[secret.Name] = secret
	return secret, nil
}

func (f *fakeSecrets) Get(name string) (*v1.Secret, error) {
	secret, ok := f.store[name]

	if !ok {
		return nil, fmt.Errorf("Secret with name: %v not found", name)
	}

	return secret, nil
}

func (f *fakeSecrets) Delete(name string, options *v1.DeleteOptions) error { return nil }
func (f *fakeSecrets) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return nil
}
func (f *fakeSecrets) List(opts v1.ListOptions) (*v1.SecretList, error)   { return nil, nil }
func (f *fakeSecrets) Watch(opts v1.ListOptions) (watch.Interface, error) { return nil, nil }
func (f *fakeSecrets) Patch(name string, pt api.PatchType, data []byte, subresources ...string) (result *v1.Secret, err error) {
	return nil, nil
}

func (f *fakeServiceAccounts) Get(name string) (*v1.ServiceAccount, error) {
	serviceAccount, ok := f.store[name]

	if !ok {
		return nil, fmt.Errorf("Failed to find service account: %v", name)
	}

	return serviceAccount, nil
}

func (f *fakeServiceAccounts) Update(serviceAccount *v1.ServiceAccount) (*v1.ServiceAccount, error) {
	serviceAccount, ok := f.store[serviceAccount.Name]

	if !ok {
		return nil, fmt.Errorf("Service account: %v not found", serviceAccount.Name)
	}

	f.store[serviceAccount.Name] = serviceAccount
	return serviceAccount, nil
}

func (f *fakeServiceAccounts) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return nil
}

func (f *fakeServiceAccounts) Patch(name string, pt api.PatchType, data []byte, subresources ...string) (result *v1.ServiceAccount, err error) {
	return nil, nil
}

func (f *fakeServiceAccounts) Delete(name string, options *v1.DeleteOptions) error {
	_, ok := f.store[name]

	if !ok {
		return fmt.Errorf("Service account: %v not found", name)
	}

	delete(f.store, name)
	return nil
}

func (f *fakeServiceAccounts) Create(serviceAccount *v1.ServiceAccount) (*v1.ServiceAccount, error) {
	return nil, nil
}
func (f *fakeServiceAccounts) List(opts v1.ListOptions) (*v1.ServiceAccountList, error) {
	return nil, nil
}
func (f *fakeServiceAccounts) Watch(opts v1.ListOptions) (watch.Interface, error) { return nil, nil }

func (f *fakeNamespaces) List(opts v1.ListOptions) (*v1.NamespaceList, error) {
	namespaces := []v1.Namespace{}

	for _, v := range f.store {
		namespaces = append(namespaces, v)
	}

	return &v1.NamespaceList{Items: namespaces}, nil
}

func (f *fakeNamespaces) Create(item *v1.Namespace) (*v1.Namespace, error)    { return nil, nil }
func (f *fakeNamespaces) Get(name string) (result *v1.Namespace, err error)   { return nil, nil }
func (f *fakeNamespaces) UpdateStatus(*v1.Namespace) (*v1.Namespace, error)   { return nil, nil }
func (f *fakeNamespaces) Delete(name string, options *v1.DeleteOptions) error { return nil }
func (f *fakeNamespaces) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	return nil
}
func (f *fakeNamespaces) Update(item *v1.Namespace) (*v1.Namespace, error)   { return nil, nil }
func (f *fakeNamespaces) Watch(opts v1.ListOptions) (watch.Interface, error) { return nil, nil }
func (f *fakeNamespaces) Finalize(item *v1.Namespace) (*v1.Namespace, error) { return nil, nil }
func (f *fakeNamespaces) Patch(name string, pt api.PatchType, data []byte, subresources ...string) (result *v1.Namespace, err error) {
	return nil, nil
}
func (f *fakeNamespaces) Status(item *v1.Namespace) (*v1.Namespace, error) { return nil, nil }

type fakeEcrClient struct{}

func (f *fakeEcrClient) GetAuthorizationToken(input *ecr.GetAuthorizationTokenInput) (*ecr.GetAuthorizationTokenOutput, error) {
	return &ecr.GetAuthorizationTokenOutput{
		AuthorizationData: []*ecr.AuthorizationData{
			&ecr.AuthorizationData{
				AuthorizationToken: aws.String("fakeToken"),
				ProxyEndpoint:      aws.String("fakeEndpoint"),
			},
		},
	}, nil
}

type fakeFailingEcrClient struct{}

func (f *fakeFailingEcrClient) GetAuthorizationToken(input *ecr.GetAuthorizationTokenInput) (*ecr.GetAuthorizationTokenOutput, error) {
	return nil, errors.New("fake error")
}

type fakeGcrClient struct{}

type fakeTokenSource struct{}

func (f fakeTokenSource) Token() (*oauth2.Token, error) {
	return &oauth2.Token{
		AccessToken: "fakeToken",
	}, nil
}

func newFakeTokenSource() fakeTokenSource {
	return fakeTokenSource{}
}

func (f *fakeGcrClient) DefaultTokenSource(ctx context.Context, scope ...string) (oauth2.TokenSource, error) {
	return newFakeTokenSource(), nil
}

type fakeFailingGcrClient struct{}

func (f *fakeFailingGcrClient) DefaultTokenSource(ctx context.Context, scope ...string) (oauth2.TokenSource, error) {
	return nil, errors.New("fake error")
}

func newKubeUtil() *k8sutil.K8sutilInterface {
	return &k8sutil.K8sutilInterface{
		Kclient:    newFakeKubeClient(),
		MasterHost: "foo",
	}
}

func newFakeKubeClient() k8sutil.KubeInterface {
	return &fakeKubeClient{
		secrets: map[string]*fakeSecrets{
			"namespace1": &fakeSecrets{
				store: map[string]*v1.Secret{},
			},
			"namespace2": &fakeSecrets{
				store: map[string]*v1.Secret{},
			},
			"kube-system": &fakeSecrets{
				store: map[string]*v1.Secret{},
			},
		},
		namespaces: &fakeNamespaces{store: map[string]v1.Namespace{
			"namespace1": v1.Namespace{
				ObjectMeta: v1.ObjectMeta{
					Name: "namespace1",
				},
			},
			"namespace2": v1.Namespace{
				ObjectMeta: v1.ObjectMeta{
					Name: "namespace2",
				},
			},
			"kube-system": v1.Namespace{
				ObjectMeta: v1.ObjectMeta{
					Name: "kube-system",
				},
			},
		}},
		serviceaccounts: map[string]*fakeServiceAccounts{
			"namespace1": &fakeServiceAccounts{
				store: map[string]*v1.ServiceAccount{
					"default": &v1.ServiceAccount{
						ObjectMeta: v1.ObjectMeta{
							Name: "default",
						},
					},
				},
			},
			"namespace2": &fakeServiceAccounts{
				store: map[string]*v1.ServiceAccount{
					"default": &v1.ServiceAccount{
						ObjectMeta: v1.ObjectMeta{
							Name: "default",
						},
					},
				},
			},
			"kube-system": &fakeServiceAccounts{
				store: map[string]*v1.ServiceAccount{
					"default": &v1.ServiceAccount{
						ObjectMeta: v1.ObjectMeta{
							Name: "default",
						},
					},
				},
			},
		},
	}
}

func newFakeEcrClient() *fakeEcrClient {
	return &fakeEcrClient{}
}

func newFakeGcrClient() *fakeGcrClient {
	return &fakeGcrClient{}
}

func newFakeFailingGcrClient() *fakeFailingGcrClient {
	return &fakeFailingGcrClient{}
}

func newFakeFailingEcrClient() *fakeFailingEcrClient {
	return &fakeFailingEcrClient{}
}

func process(t *testing.T, c *controller) {
	namespaces, _ := c.k8sutil.Kclient.Namespaces().List(v1.ListOptions{})
	for _, ns := range namespaces.Items {
		err := handler(c, &ns)
		assert.Nil(t, err)
	}
}

func TestGetECRAuthorizationKey(t *testing.T) {
	util := newKubeUtil()
	ecrClient := newFakeEcrClient()
	gcrClient := newFakeGcrClient()
	c := &controller{util, ecrClient, gcrClient}

	token, err := c.getECRAuthorizationKey()

	assert.Equal(t, "fakeToken", token.AccessToken)
	assert.Equal(t, "fakeEndpoint", token.Endpoint)
	assert.Nil(t, err)
}

func TestProcessOnce(t *testing.T) {
	util := newKubeUtil()

	ecrClient := newFakeEcrClient()
	*argGCRURL = "fakeEndpoint"
	gcrClient := newFakeGcrClient()
	c := &controller{util, ecrClient, gcrClient}

	process(t, c)

	// Test GCR
	secret, err := c.k8sutil.GetSecret("namespace1", *argGCRSecretName)
	assert.Nil(t, err)
	assert.Equal(t, *argGCRSecretName, secret.Name)
	assert.Equal(t, map[string][]byte{
		".dockercfg": []byte(fmt.Sprintf(dockerCfgTemplate, "fakeEndpoint", "fakeToken")),
	}, secret.Data)
	assert.Equal(t, v1.SecretType("kubernetes.io/dockercfg"), secret.Type)

	secret, err = c.k8sutil.GetSecret("namespace1", *argGCRSecretName)
	assert.Nil(t, err)
	assert.Equal(t, *argGCRSecretName, secret.Name)
	assert.Equal(t, map[string][]byte{
		".dockercfg": []byte(fmt.Sprintf(dockerCfgTemplate, "fakeEndpoint", "fakeToken")),
	}, secret.Data)
	assert.Equal(t, v1.SecretType("kubernetes.io/dockercfg"), secret.Type)

	_, err = c.k8sutil.GetSecret("kube-system", *argGCRSecretName)
	assert.NotNil(t, err)

	serviceAccount, err := c.k8sutil.GetServiceAccount("namespace1", "default")
	assert.Nil(t, err)
	assert.Equal(t, *argGCRSecretName, serviceAccount.ImagePullSecrets[0].Name)

	serviceAccount, err = c.k8sutil.GetServiceAccount("namespace1", "default")
	assert.Nil(t, err)
	assert.Equal(t, *argGCRSecretName, serviceAccount.ImagePullSecrets[0].Name)

	// Test AWS
	secret, err = c.k8sutil.GetSecret("namespace2", *argAWSSecretName)
	assert.Nil(t, err)
	assert.Equal(t, *argAWSSecretName, secret.Name)
	assert.Equal(t, map[string][]byte{
		".dockerconfigjson": []byte(fmt.Sprintf(dockerJSONTemplate, "fakeEndpoint", "fakeToken")),
	}, secret.Data)
	assert.Equal(t, v1.SecretType("kubernetes.io/dockerconfigjson"), secret.Type)

	secret, err = c.k8sutil.GetSecret("namespace2", *argAWSSecretName)
	assert.Nil(t, err)
	assert.Equal(t, *argAWSSecretName, secret.Name)
	assert.Equal(t, map[string][]byte{
		".dockerconfigjson": []byte(fmt.Sprintf(dockerJSONTemplate, "fakeEndpoint", "fakeToken")),
	}, secret.Data)
	assert.Equal(t, v1.SecretType("kubernetes.io/dockerconfigjson"), secret.Type)

	_, err = c.k8sutil.GetSecret("kube-system", *argAWSSecretName)
	assert.NotNil(t, err)

	serviceAccount, err = c.k8sutil.GetServiceAccount("namespace2", "default")
	assert.Nil(t, err)
	assert.Equal(t, 2, len(serviceAccount.ImagePullSecrets))
	assert.Equal(t, *argAWSSecretName, serviceAccount.ImagePullSecrets[1].Name)

	serviceAccount, err = c.k8sutil.GetServiceAccount("namespace2", "default")
	assert.Nil(t, err)
	assert.Equal(t, 2, len(serviceAccount.ImagePullSecrets))
	assert.Equal(t, *argAWSSecretName, serviceAccount.ImagePullSecrets[1].Name)
}

func TestProcessTwice(t *testing.T) {
	util := newKubeUtil()
	ecrClient := newFakeEcrClient()
	*argGCRURL = "fakeEndpoint"
	gcrClient := newFakeGcrClient()
	c := &controller{util, ecrClient, gcrClient}

	process(t, c)
	// test processing twice for idempotency
	process(t, c)

	// Test GCR
	secret, err := c.k8sutil.GetSecret("namespace1", *argGCRSecretName)
	assert.Nil(t, err)
	assert.Equal(t, *argGCRSecretName, secret.Name)
	assert.Equal(t, map[string][]byte{
		".dockercfg": []byte(fmt.Sprintf(dockerCfgTemplate, "fakeEndpoint", "fakeToken")),
	}, secret.Data)
	assert.Equal(t, v1.SecretType("kubernetes.io/dockercfg"), secret.Type)

	secret, err = c.k8sutil.GetSecret("namespace1", *argGCRSecretName)
	assert.Nil(t, err)
	assert.Equal(t, *argGCRSecretName, secret.Name)
	assert.Equal(t, map[string][]byte{
		".dockercfg": []byte(fmt.Sprintf(dockerCfgTemplate, "fakeEndpoint", "fakeToken")),
	}, secret.Data)
	assert.Equal(t, v1.SecretType("kubernetes.io/dockercfg"), secret.Type)

	_, err = c.k8sutil.GetSecret("kube-system", *argGCRSecretName)
	assert.NotNil(t, err)

	serviceAccount, err := c.k8sutil.GetServiceAccount("namespace1", "default")
	assert.Nil(t, err)
	assert.Equal(t, *argGCRSecretName, serviceAccount.ImagePullSecrets[0].Name)

	serviceAccount, err = c.k8sutil.GetServiceAccount("namespace1", "default")
	assert.Nil(t, err)
	assert.Equal(t, *argGCRSecretName, serviceAccount.ImagePullSecrets[0].Name)

	// Test AWS
	secret, err = c.k8sutil.GetSecret("namespace2", *argAWSSecretName)
	assert.Nil(t, err)
	assert.Equal(t, *argAWSSecretName, secret.Name)
	assert.Equal(t, map[string][]byte{
		".dockerconfigjson": []byte(fmt.Sprintf(dockerJSONTemplate, "fakeEndpoint", "fakeToken")),
	}, secret.Data)
	assert.Equal(t, v1.SecretType("kubernetes.io/dockerconfigjson"), secret.Type)

	secret, err = c.k8sutil.GetSecret("namespace2", *argAWSSecretName)
	assert.Nil(t, err)
	assert.Equal(t, *argAWSSecretName, secret.Name)
	assert.Equal(t, map[string][]byte{
		".dockerconfigjson": []byte(fmt.Sprintf(dockerJSONTemplate, "fakeEndpoint", "fakeToken")),
	}, secret.Data)
	assert.Equal(t, v1.SecretType("kubernetes.io/dockerconfigjson"), secret.Type)

	_, err = c.k8sutil.GetSecret("kube-system", *argAWSSecretName)
	assert.NotNil(t, err)

	serviceAccount, err = c.k8sutil.GetServiceAccount("namespace2", "default")
	assert.Nil(t, err)
	assert.Equal(t, 2, len(serviceAccount.ImagePullSecrets))
	assert.Equal(t, *argAWSSecretName, serviceAccount.ImagePullSecrets[1].Name)

	serviceAccount, err = c.k8sutil.GetServiceAccount("namespace2", "default")
	assert.Nil(t, err)
	assert.Equal(t, 2, len(serviceAccount.ImagePullSecrets))
	assert.Equal(t, *argAWSSecretName, serviceAccount.ImagePullSecrets[1].Name)
}

func TestProcessWithExistingSecrets(t *testing.T) {
	util := newKubeUtil()
	ecrClient := newFakeEcrClient()
	*argGCRURL = "fakeEndpoint"
	gcrClient := newFakeGcrClient()
	c := &controller{util, ecrClient, gcrClient}

	secretGCR := &v1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name: *argGCRSecretName,
		},
		Data: map[string][]byte{
			".dockercfg": []byte("some other config"),
		},
		Type: "some other type",
	}

	err := c.k8sutil.CreateSecret("namespace1", secretGCR)
	assert.Nil(t, err)
	err = c.k8sutil.CreateSecret("namespace2", secretGCR)
	assert.Nil(t, err)

	secretAWS := &v1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name: *argAWSSecretName,
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte("some other config"),
		},
		Type: "some other type",
	}

	err = c.k8sutil.CreateSecret("namespace1", secretAWS)
	assert.Nil(t, err)
	err = c.k8sutil.CreateSecret("namespace2", secretAWS)
	assert.Nil(t, err)

	process(t, c)

	// Test GCR
	secretGCR, err = c.k8sutil.GetSecret("namespace1", *argGCRSecretName)
	assert.Nil(t, err)
	assert.Equal(t, *argGCRSecretName, secretGCR.Name)
	assert.Equal(t, map[string][]byte{
		".dockercfg": []byte(fmt.Sprintf(dockerCfgTemplate, "fakeEndpoint", "fakeToken")),
	}, secretGCR.Data)
	assert.Equal(t, secretGCR.Type, v1.SecretType("kubernetes.io/dockercfg"))

	secretGCR, err = c.k8sutil.GetSecret("namespace2", *argGCRSecretName)
	assert.Nil(t, err)
	assert.Equal(t, *argGCRSecretName, secretGCR.Name)
	assert.Equal(t, map[string][]byte{
		".dockercfg": []byte(fmt.Sprintf(dockerCfgTemplate, "fakeEndpoint", "fakeToken")),
	}, secretGCR.Data)
	assert.Equal(t, v1.SecretType("kubernetes.io/dockercfg"), secretGCR.Type)

	secretGCR, err = c.k8sutil.GetSecret("namespace1", *argGCRSecretName)
	assert.Nil(t, err)
	assert.Equal(t, *argGCRSecretName, secretGCR.Name)
	assert.Equal(t, map[string][]byte{
		".dockercfg": []byte(fmt.Sprintf(dockerCfgTemplate, "fakeEndpoint", "fakeToken")),
	}, secretGCR.Data)
	assert.Equal(t, secretGCR.Type, v1.SecretType("kubernetes.io/dockercfg"))

	secretGCR, err = c.k8sutil.GetSecret("namespace2", *argGCRSecretName)
	assert.Nil(t, err)
	assert.Equal(t, *argGCRSecretName, secretGCR.Name)
	assert.Equal(t, map[string][]byte{
		".dockercfg": []byte(fmt.Sprintf(dockerCfgTemplate, "fakeEndpoint", "fakeToken")),
	}, secretGCR.Data)
	assert.Equal(t, v1.SecretType("kubernetes.io/dockercfg"), secretGCR.Type)

	// Test AWS
	secretAWS, err = c.k8sutil.GetSecret("namespace1", *argAWSSecretName)
	assert.Nil(t, err)
	assert.Equal(t, *argAWSSecretName, secretAWS.Name)
	assert.Equal(t, map[string][]byte{
		".dockerconfigjson": []byte(fmt.Sprintf(dockerJSONTemplate, "fakeEndpoint", "fakeToken")),
	}, secretAWS.Data)
	assert.Equal(t, secretAWS.Type, v1.SecretType("kubernetes.io/dockerconfigjson"))

	secretAWS, err = c.k8sutil.GetSecret("namespace2", *argAWSSecretName)
	assert.Nil(t, err)
	assert.Equal(t, *argAWSSecretName, secretAWS.Name)
	assert.Equal(t, map[string][]byte{
		".dockerconfigjson": []byte(fmt.Sprintf(dockerJSONTemplate, "fakeEndpoint", "fakeToken")),
	}, secretAWS.Data)
	assert.Equal(t, v1.SecretType("kubernetes.io/dockerconfigjson"), secretAWS.Type)

	secretAWS, err = c.k8sutil.GetSecret("namespace1", *argAWSSecretName)
	assert.Nil(t, err)
	assert.Equal(t, *argAWSSecretName, secretAWS.Name)
	assert.Equal(t, map[string][]byte{
		".dockerconfigjson": []byte(fmt.Sprintf(dockerJSONTemplate, "fakeEndpoint", "fakeToken")),
	}, secretAWS.Data)
	assert.Equal(t, secretAWS.Type, v1.SecretType("kubernetes.io/dockerconfigjson"))

	secretAWS, err = c.k8sutil.GetSecret("namespace2", *argAWSSecretName)
	assert.Nil(t, err)
	assert.Equal(t, *argAWSSecretName, secretAWS.Name)
	assert.Equal(t, map[string][]byte{
		".dockerconfigjson": []byte(fmt.Sprintf(dockerJSONTemplate, "fakeEndpoint", "fakeToken")),
	}, secretAWS.Data)
	assert.Equal(t, v1.SecretType("kubernetes.io/dockerconfigjson"), secretAWS.Type)
}

// func TestProcessNoDefaultServiceAccount(t *testing.T) {
// 	util := newKubeUtil()
// 	ecrClient := newFakeEcrClient()
// 	gcrClient := newFakeGcrClient()
// 	testConfig := providerConfig{true, true}
// 	c := &controller{util, ecrClient, gcrClient, testConfig}

// 	err := c.k8sutil.DeleteServiceAccounts("namespace1").Delete("default")
// 	assert.Nil(t, err)
// 	err = c.k8sutil.ServiceAccounts("namespace2").Delete("default")
// 	assert.Nil(t, err)

// 	err = c.process()
// 	assert.NotNil(t, err)
// }

func TestProcessWithExistingImagePullSecrets(t *testing.T) {
	util := newKubeUtil()
	ecrClient := newFakeEcrClient()
	gcrClient := newFakeGcrClient()
	c := &controller{util, ecrClient, gcrClient}

	serviceAccount, err := c.k8sutil.GetServiceAccount("namespace1", "default")
	assert.Nil(t, err)
	serviceAccount.ImagePullSecrets = append(serviceAccount.ImagePullSecrets, v1.LocalObjectReference{Name: "someOtherSecret"})
	err = c.k8sutil.UpdateServiceAccount("namespace1", serviceAccount)

	serviceAccount, err = c.k8sutil.GetServiceAccount("namespace2", "default")
	assert.Nil(t, err)
	serviceAccount.ImagePullSecrets = append(serviceAccount.ImagePullSecrets, v1.LocalObjectReference{Name: "someOtherSecret"})
	err = c.k8sutil.UpdateServiceAccount("namespace2", serviceAccount)

	process(t, c)

	serviceAccount, err = c.k8sutil.GetServiceAccount("namespace1", "default")
	assert.Nil(t, err)
	assert.Equal(t, 3, len(serviceAccount.ImagePullSecrets))
	assert.Equal(t, "someOtherSecret", serviceAccount.ImagePullSecrets[0].Name)
	assert.Equal(t, *argGCRSecretName, serviceAccount.ImagePullSecrets[1].Name)
	assert.Equal(t, *argAWSSecretName, serviceAccount.ImagePullSecrets[2].Name)

	serviceAccount, err = c.k8sutil.GetServiceAccount("namespace2", "default")
	assert.Nil(t, err)
	assert.Equal(t, 3, len(serviceAccount.ImagePullSecrets))
	assert.Equal(t, "someOtherSecret", serviceAccount.ImagePullSecrets[0].Name)
	assert.Equal(t, *argGCRSecretName, serviceAccount.ImagePullSecrets[1].Name)
	assert.Equal(t, *argAWSSecretName, serviceAccount.ImagePullSecrets[2].Name)
}

func TestDefaultAwsRegionFromArgs(t *testing.T) {
	assert.Equal(t, "us-east-1", *argAWSRegion)
}

func TestAwsRegionFromEnv(t *testing.T) {
	expectedRegion := "us-steve-1"

	os.Setenv("awsaccount", "12345678")
	os.Setenv("awsregion", expectedRegion)
	validateParams()

	assert.Equal(t, expectedRegion, *argAWSRegion)
}

func TestFailingGcrPassingEcrStillSucceeds(t *testing.T) {
	util := newKubeUtil()
	ecrClient := newFakeEcrClient()
	gcrClient := newFakeFailingGcrClient()
	c := &controller{util, ecrClient, gcrClient}

	process(t, c)
}

func TestPassingGcrPassingEcrStillSucceeds(t *testing.T) {
	util := newKubeUtil()
	ecrClient := newFakeFailingEcrClient()
	gcrClient := newFakeGcrClient()
	c := controller{util, ecrClient, gcrClient}

	process(t, &c)
}
