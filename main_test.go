package main

import (
	context2 "context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	v14 "k8s.io/api/authentication/v1"
	meta1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/typed/core/v1/fake"
	"log"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	v1 "k8s.io/api/core/v1"
	coreType "k8s.io/client-go/kubernetes/typed/core/v1"
)

func init() {
	log.SetOutput(ioutil.Discard)
	logrus.SetOutput(ioutil.Discard)
}

func enableShortRetries() {
	RetryCfg = RetryConfig{
		Type:                "simple",
		NumberOfRetries:     2,
		RetryDelayInSeconds: 1,
	}
	SetupRetryTimer()
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

func (f *fakeKubeClient) CoreV1() coreType.CoreV1Interface {
	return &fake.FakeCoreV1{}
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

func (f *fakeSecrets) Create(_ context.Context, secret *v1.Secret, _ meta1.CreateOptions) (*v1.Secret, error) {
	_, ok := f.store[secret.Name]

	if ok {
		return nil, fmt.Errorf("secret %v already exists", secret.Name)
	}

	f.store[secret.Name] = secret
	return secret, nil
}

func (f *fakeSecrets) Update(_ context.Context, secret *v1.Secret, _ meta1.UpdateOptions) (*v1.Secret, error) {
	_, ok := f.store[secret.Name]

	if !ok {
		return nil, fmt.Errorf("secret %v not found", secret.Name)
	}

	f.store[secret.Name] = secret
	return secret, nil
}

func (f *fakeSecrets) Get(_ context.Context, name string, _ meta1.GetOptions) (*v1.Secret, error) {
	secret, ok := f.store[name]

	if !ok {
		return nil, fmt.Errorf("secret with name '%v' not found", name)
	}

	return secret, nil
}

func (f *fakeSecrets) Delete(context.Context, string, meta1.DeleteOptions) error {
	return nil
}
func (f *fakeSecrets) DeleteCollection(context.Context, meta1.DeleteOptions, meta1.ListOptions) error {
	return nil
}
func (f *fakeSecrets) List(context.Context, meta1.ListOptions) (*v1.SecretList, error) {
	return nil, nil
}
func (f *fakeSecrets) Watch(context.Context, meta1.ListOptions) (watch.Interface, error) {
	return nil, nil
}
func (f *fakeSecrets) Patch(context.Context, string, types.PatchType, []byte, meta1.PatchOptions, ...string) (result *v1.Secret, err error) {
	return nil, nil
}

func (f *fakeServiceAccounts) Create(context2.Context, *v1.ServiceAccount, meta1.CreateOptions) (*v1.ServiceAccount, error) {
	return nil, nil
}

func (f *fakeServiceAccounts) Update(_ context2.Context, serviceAccount *v1.ServiceAccount, _ meta1.UpdateOptions) (*v1.ServiceAccount, error) {
	serviceAccount, ok := f.store[serviceAccount.Name]

	if !ok {
		return nil, fmt.Errorf("service account '%v' not found", serviceAccount.Name)
	}

	f.store[serviceAccount.Name] = serviceAccount
	return serviceAccount, nil
}

func (f *fakeServiceAccounts) Delete(_ context2.Context, name string, _ meta1.DeleteOptions) error {
	_, ok := f.store[name]

	if !ok {
		return fmt.Errorf("service account '%v' not found", name)
	}

	delete(f.store, name)
	return nil
}

func (f *fakeServiceAccounts) DeleteCollection(context2.Context, meta1.DeleteOptions, meta1.ListOptions) error {
	return nil
}

func (f *fakeServiceAccounts) Get(_ context2.Context, name string, _ meta1.GetOptions) (*v1.ServiceAccount, error) {
	serviceAccount, ok := f.store[name]

	if !ok {
		return nil, fmt.Errorf("failed to find service account '%v'", name)
	}

	return serviceAccount, nil
}

func (f *fakeServiceAccounts) List(context2.Context, meta1.ListOptions) (*v1.ServiceAccountList, error) {
	return nil, nil
}

func (f *fakeServiceAccounts) Watch(context2.Context, meta1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (f *fakeServiceAccounts) Patch(context2.Context, string, types.PatchType, []byte, meta1.PatchOptions, ...string) (result *v1.ServiceAccount, err error) {
	return nil, nil
}

func (f *fakeServiceAccounts) CreateToken(context2.Context, string, *v14.TokenRequest, meta1.CreateOptions) (*v14.TokenRequest, error) {
	return nil, nil
}

func (f *fakeNamespaces) List(context.Context, meta1.ListOptions) (*v1.NamespaceList, error) {
	namespaces := make([]v1.Namespace, 0)

	for _, v := range f.store {
		namespaces = append(namespaces, v)
	}

	return &v1.NamespaceList{Items: namespaces}, nil
}

func (f *fakeNamespaces) Create(context.Context, *v1.Namespace, meta1.CreateOptions) (*v1.Namespace, error) {
	return nil, nil
}
func (f *fakeNamespaces) Get(context.Context, string, meta1.GetOptions) (result *v1.Namespace, err error) {
	return nil, nil
}
func (f *fakeNamespaces) UpdateStatus(context.Context, *v1.Namespace, meta1.UpdateOptions) (*v1.Namespace, error) {
	return nil, nil
}
func (f *fakeNamespaces) Delete(context.Context, string, meta1.DeleteOptions) error {
	return nil
}
func (f *fakeNamespaces) DeleteCollection() error {
	return nil
}
func (f *fakeNamespaces) Update(context.Context, *v1.Namespace, meta1.UpdateOptions) (*v1.Namespace, error) {
	return nil, nil
}
func (f *fakeNamespaces) Watch(context.Context, meta1.ListOptions) (watch.Interface, error) {
	return nil, nil
}
func (f *fakeNamespaces) Finalize(context.Context, *v1.Namespace, meta1.UpdateOptions) (*v1.Namespace, error) {
	return nil, nil
}
func (f *fakeNamespaces) Patch(context.Context, string, types.PatchType, []byte, meta1.PatchOptions, ...string) (result *v1.Namespace, err error) {
	return nil, nil
}
func (f *fakeNamespaces) Status() (*v1.Namespace, error) { return nil, nil }

type fakeEcrClient struct{}

func (f *fakeEcrClient) GetAuthorizationToken(input *ecr.GetAuthorizationTokenInput) (*ecr.GetAuthorizationTokenOutput, error) {
	if len(input.RegistryIds) == 2 {
		return &ecr.GetAuthorizationTokenOutput{
			AuthorizationData: []*ecr.AuthorizationData{
				{
					AuthorizationToken: aws.String("fakeToken1"),
					ProxyEndpoint:      aws.String("fakeEndpoint1"),
				},
				{
					AuthorizationToken: aws.String("fakeToken2"),
					ProxyEndpoint:      aws.String("fakeEndpoint2"),
				},
			},
		}, nil
	}
	return &ecr.GetAuthorizationTokenOutput{
		AuthorizationData: []*ecr.AuthorizationData{
			{
				AuthorizationToken: aws.String("fakeToken"),
				ProxyEndpoint:      aws.String("fakeEndpoint"),
			},
		},
	}, nil
}

type fakeFailingEcrClient struct{}

func (f *fakeFailingEcrClient) GetAuthorizationToken(*ecr.GetAuthorizationTokenInput) (*ecr.GetAuthorizationTokenOutput, error) {
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

func (f *fakeGcrClient) DefaultTokenSource(context.Context, ...string) (oauth2.TokenSource, error) {
	return newFakeTokenSource(), nil
}

type fakeFailingGcrClient struct{}

func (f *fakeFailingGcrClient) DefaultTokenSource(context.Context, ...string) (oauth2.TokenSource, error) {
	return nil, errors.New("fake error")
}

type fakeDprClient struct{}

func (f *fakeDprClient) getAuthToken(string, string, string) (AuthToken, error) {
	return AuthToken{AccessToken: "fakeToken", Endpoint: "fakeEndpoint"}, nil
}

type fakeFailingDprClient struct{}

func (f *fakeFailingDprClient) getAuthToken(string, string, string) (AuthToken, error) {
	return AuthToken{}, errors.New("fake error")
}

type fakeACRClient struct{}

func (f *fakeACRClient) getAuthToken(string, string, string) (AuthToken, error) {
	return AuthToken{AccessToken: "fakeACRToken", Endpoint: "fakeACREndpoint"}, nil
}

type fakeFailingACRClient struct{}

func (f *fakeFailingACRClient) getAuthToken(string, string, string) (AuthToken, error) {
	return AuthToken{}, errors.New("fake error")
}

func newKubeUtil() *K8sutilInterface {
	return &K8sutilInterface{
		KubernetesCoreV1: newFakeKubeClient(),
		MasterHost:       "foo",
	}
}

func newFakeKubeClient() KubeInterface {
	return &fakeKubeClient{
		secrets: map[string]*fakeSecrets{
			"namespace1": {
				store: map[string]*v1.Secret{},
			},
			"namespace2": {
				store: map[string]*v1.Secret{},
			},
			"kube-system": {
				store: map[string]*v1.Secret{},
			},
		},
		namespaces: &fakeNamespaces{store: map[string]v1.Namespace{
			"namespace1": {
				ObjectMeta: meta1.ObjectMeta{
					Name: "namespace1",
				},
			},
			"namespace2": {
				ObjectMeta: meta1.ObjectMeta{
					Name: "namespace2",
				},
			},
			"kube-system": {
				ObjectMeta: meta1.ObjectMeta{
					Name: "kube-system",
				},
			},
		}},
		serviceaccounts: map[string]*fakeServiceAccounts{
			"namespace1": {
				store: map[string]*v1.ServiceAccount{
					"default": {
						ObjectMeta: meta1.ObjectMeta{
							Name: "default",
						},
					},
				},
			},
			"namespace2": {
				store: map[string]*v1.ServiceAccount{
					"default": {
						ObjectMeta: meta1.ObjectMeta{
							Name: "default",
						},
					},
				},
			},
			"kube-system": {
				store: map[string]*v1.ServiceAccount{
					"default": {
						ObjectMeta: meta1.ObjectMeta{
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

func newFakeDprClient() *fakeDprClient {
	return &fakeDprClient{}
}

func newFakeACRClient() *fakeACRClient {
	return &fakeACRClient{}
}

func newFakeFailingGcrClient() *fakeFailingGcrClient {
	return &fakeFailingGcrClient{}
}

func newFakeFailingEcrClient() *fakeFailingEcrClient {
	return &fakeFailingEcrClient{}
}

func newFakeFailingDprClient() *fakeFailingDprClient {
	return &fakeFailingDprClient{}
}

func newFakeFailingACRClient() *fakeFailingACRClient {
	return &fakeFailingACRClient{}
}

func process(t *testing.T, c *controller) {
	namespaces, _ := c.k8sutil.KubernetesCoreV1.Namespaces().List(context.TODO(), meta1.ListOptions{})
	for _, ns := range namespaces.Items {
		err := handler(c, &ns)
		assert.Nil(t, err)
	}
}

func newFakeController() *controller {
	util := newKubeUtil()
	ecrClient := newFakeEcrClient()
	gcrClient := newFakeGcrClient()
	dprClient := newFakeDprClient()
	acrClient := newFakeACRClient()
	c := controller{util, ecrClient, gcrClient, dprClient, acrClient}
	return &c
}

func newFakeFailingController() *controller {
	util := newKubeUtil()
	ecrClient := newFakeFailingEcrClient()
	gcrClient := newFakeFailingGcrClient()
	dprClient := newFakeFailingDprClient()
	acrClient := newFakeFailingACRClient()
	c := controller{util, ecrClient, gcrClient, dprClient, acrClient}
	return &c
}

func TestGetECRAuthorizationKey(t *testing.T) {
	awsAccountIDs = []string{"12345678", "999999"}
	c := newFakeController()

	tokens, err := c.getECRAuthorizationKey()

	assert.Nil(t, err)
	assert.Equal(t, 2, len(tokens))
	assert.Equal(t, "fakeToken1", tokens[0].AccessToken)
	assert.Equal(t, "fakeEndpoint1", tokens[0].Endpoint)
	assert.Equal(t, "fakeToken2", tokens[1].AccessToken)
	assert.Equal(t, "fakeEndpoint2", tokens[1].Endpoint)
}

func assertDockerJSONContains(t *testing.T, endpoint, token string, secret *v1.Secret) {
	d := dockerJSON{}
	assert.Nil(t, json.Unmarshal(secret.Data[".dockerconfigjson"], &d))
	assert.Contains(t, d.Auths, endpoint)
	assert.Equal(t, d.Auths[endpoint].Auth, token)
	assert.Equal(t, d.Auths[endpoint].Email, "none")
}

func assertSecretPresent(t *testing.T, secrets []v1.LocalObjectReference, name string) {
	for _, s := range secrets {
		if s.Name == name {
			return
		}
	}
	assert.Failf(t, "ImagePullSecrets validation failed", "Expected secret %v not present", name)
}

func assertAllSecretsPresent(t *testing.T, secrets []v1.LocalObjectReference) {
	if needAWS {
		assertSecretPresent(t, secrets, *argAWSSecretName)
	}
	if needDPR {
		assertSecretPresent(t, secrets, *argDPRSecretName)
	}
	if needGCR {
		assertSecretPresent(t, secrets, *argGCRSecretName)
	}
	if needACR {
		assertSecretPresent(t, secrets, *argACRSecretName)
	}
}

func assertAllExpectedSecrets(t *testing.T, c *controller) {
	// Test GCR
	if needGCR {
		for _, ns := range []string{"namespace1", "namespace2"} {
			secret, err := c.k8sutil.GetSecret(ns, *argGCRSecretName)
			assert.Nil(t, err)
			assert.Equal(t, *argGCRSecretName, secret.Name)
			assert.Equal(t, map[string][]byte{
				".dockercfg": []byte(fmt.Sprintf(dockerCfgTemplate, "fakeEndpoint", "fakeToken")),
			}, secret.Data)
			assert.Equal(t, v1.SecretType("kubernetes.io/dockercfg"), secret.Type)
		}

		_, err := c.k8sutil.GetSecret("kube-system", *argGCRSecretName)
		assert.NotNil(t, err)
	}

	if needAWS {

		// Test AWS
		for _, ns := range []string{"namespace1", "namespace2"} {
			secret, err := c.k8sutil.GetSecret(ns, *argAWSSecretName)
			assert.Nil(t, err)
			assert.Equal(t, *argAWSSecretName, secret.Name)
			assertDockerJSONContains(t, "fakeEndpoint", "fakeToken", secret)
			assert.Equal(t, v1.SecretType("kubernetes.io/dockerconfigjson"), secret.Type)
		}

		_, err := c.k8sutil.GetSecret("kube-system", *argAWSSecretName)
		assert.NotNil(t, err)
	}

	if needACR {
		// Test Azure Container Registry support
		for _, ns := range []string{"namespace1", "namespace2"} {
			if *argACRClientID != "" {
				secret, err := c.k8sutil.GetSecret(ns, *argACRSecretName)
				assert.Nil(t, err)
				assert.Equal(t, *argACRSecretName, secret.Name)
				assertDockerJSONContains(t, "fakeACREndpoint", "fakeACRToken", secret)
				assert.Equal(t, v1.SecretType("kubernetes.io/dockerconfigjson"), secret.Type)
			}
		}

		_, err := c.k8sutil.GetSecret("kube-system", *argACRSecretName)
		assert.NotNil(t, err)
	}

	// Verify that all expected secrets have been created in all namespaces
	serviceAccount, err := c.k8sutil.GetServiceAccount("namespace1", "default")
	assert.Nil(t, err)
	assertAllSecretsPresent(t, serviceAccount.ImagePullSecrets)

	serviceAccount, err = c.k8sutil.GetServiceAccount("namespace2", "default")
	assert.Nil(t, err)
	assertAllSecretsPresent(t, serviceAccount.ImagePullSecrets)
}

func assertExpectedSecretNumber(t *testing.T, c *controller, n int) {
	for _, ns := range []string{"namespace1", "namespace2"} {
		serviceAccount, err := c.k8sutil.GetServiceAccount(ns, "default")
		assert.Nil(t, err)
		assert.Exactly(t, n, len(serviceAccount.ImagePullSecrets))
	}
}

func TestProcessOnce(t *testing.T) {
	*argGCRURL = "fakeEndpoint"
	awsAccountIDs = []string{""}
	c := newFakeController()

	process(t, c)

	assertAllExpectedSecrets(t, c)
}

func TestProcessTwice(t *testing.T) {
	*argGCRURL = "fakeEndpoint"
	c := newFakeController()

	process(t, c)
	// test processing twice for idempotency
	process(t, c)

	assertAllExpectedSecrets(t, c)

	// Verify that secrets have not been created twice
	assertExpectedSecretNumber(t, c, 0)
}

func TestProcessWithExistingSecrets(t *testing.T) {
	*argGCRURL = "fakeEndpoint"
	c := newFakeController()

	secretGCR := &v1.Secret{
		ObjectMeta: meta1.ObjectMeta{
			Name: *argGCRSecretName,
		},
		Data: map[string][]byte{
			".dockercfg": []byte("some other config"),
		},
		Type: "some other type",
	}

	secretAWS := &v1.Secret{
		ObjectMeta: meta1.ObjectMeta{
			Name: *argAWSSecretName,
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte("some other config"),
		},
		Type: "some other type",
	}

	secretDPR := &v1.Secret{
		ObjectMeta: meta1.ObjectMeta{
			Name: *argDPRSecretName,
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte("some other config"),
		},
		Type: "some other type",
	}

	secretACR := &v1.Secret{
		ObjectMeta: meta1.ObjectMeta{
			Name: *argACRSecretName,
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte("some other config"),
		},
		Type: "some other type",
	}

	for _, ns := range []string{"namespace1", "namespace2"} {
		for _, secret := range []*v1.Secret{secretGCR, secretAWS, secretDPR, secretACR} {
			err := c.k8sutil.CreateSecret(ns, secret)
			assert.Nil(t, err)
		}
	}

	process(t, c)

	assertAllExpectedSecrets(t, c)
	assertExpectedSecretNumber(t, c, 0)
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
	c := newFakeController()

	for _, ns := range []string{"namespace1", "namespace2"} {
		serviceAccount, err := c.k8sutil.GetServiceAccount(ns, "default")
		assert.Nil(t, err)
		serviceAccount.ImagePullSecrets = append(serviceAccount.ImagePullSecrets, v1.LocalObjectReference{Name: "someOtherSecret"})
		err = c.k8sutil.UpdateServiceAccount(ns, serviceAccount)
	}

	process(t, c)

	for _, ns := range []string{"namespace1", "namespace2"} {
		serviceAccount, err := c.k8sutil.GetServiceAccount(ns, "default")
		assert.Nil(t, err)
		assertAllSecretsPresent(t, serviceAccount.ImagePullSecrets)
		assertSecretPresent(t, serviceAccount.ImagePullSecrets, "someOtherSecret")
	}
}

func TestDefaultAwsRegionFromArgs(t *testing.T) {
	assert.Equal(t, "us-east-1", *argAWSRegion)
}

func TestAwsRegionFromEnv(t *testing.T) {
	expectedRegion := "us-steve-1"

	_ = os.Setenv("awsaccount", "12345678")
	_ = os.Setenv("awsregion", expectedRegion)
	validateParams()

	assert.Equal(t, expectedRegion, *argAWSRegion)
}

func TestGcrURLFromEnv(t *testing.T) {
	expectedURL := "http://test.me"

	_ = os.Setenv("gcrurl", "http://test.me")
	validateParams()

	assert.Equal(t, expectedURL, *argGCRURL)
}

func TestFailingGcrPassingEcrStillSucceeds(t *testing.T) {
	enableShortRetries()

	awsAccountIDs = []string{""}
	c := newFakeFailingController()
	c.ecrClient = newFakeEcrClient()

	process(t, c)
}

func TestPassingGcrPassingEcrStillSucceeds(t *testing.T) {
	enableShortRetries()

	awsAccountIDs = []string{""}
	c := newFakeFailingController()
	c.gcrClient = newFakeGcrClient()

	process(t, c)
}

func TestControllerGenerateSecretsSimpleRetryOnError(t *testing.T) {
	// enable log output for this test
	log.SetOutput(os.Stdout)
	logrus.SetOutput(os.Stdout)
	// disable log output when the test has completed
	defer func() {
		log.SetOutput(ioutil.Discard)
		logrus.SetOutput(ioutil.Discard)
	}()
	enableShortRetries()

	awsAccountIDs = []string{""}
	c := newFakeFailingController()

	process(t, c)
}

func TestControllerGenerateSecretsExponentialRetryOnError(t *testing.T) {
	// enable log output for this test
	log.SetOutput(os.Stdout)
	logrus.SetOutput(os.Stdout)
	// disable log output when the test has completed
	defer func() {
		log.SetOutput(ioutil.Discard)
		logrus.SetOutput(ioutil.Discard)
	}()
	RetryCfg = RetryConfig{
		Type:                "exponential",
		NumberOfRetries:     3,
		RetryDelayInSeconds: 1,
	}
	SetupRetryTimer()
	awsAccountIDs = []string{""}
	c := newFakeFailingController()

	process(t, c)
}
