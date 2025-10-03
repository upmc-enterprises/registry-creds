package main

import (
	"errors"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
)

// Simplified test suite focusing on the core logic without complex k8s mocking
type RegistryCredsTestSuite struct {
	suite.Suite
	originalValidator CredentialValidator
	originalArgs      map[string]interface{}
	mockCredentials   *MockCredentialValidator
}

func TestRegistryCredsTestSuite(t *testing.T) {
	suite.Run(t, new(RegistryCredsTestSuite))
}

func (s *RegistryCredsTestSuite) SetupTest() {
	// Save original state
	s.originalValidator = credentialValidator
	s.originalArgs = s.saveArgs()

	// Setup mock credentials
	s.mockCredentials = &MockCredentialValidator{}
	credentialValidator = s.mockCredentials

	// Disable retries for faster tests
	RetryCfg = RetryConfig{
		Type:                "simple",
		NumberOfRetries:     0,
		RetryDelayInSeconds: 1,
	}
	SetupRetryTimer()
}

func (s *RegistryCredsTestSuite) TearDownTest() {
	// Restore original state
	credentialValidator = s.originalValidator
	s.restoreArgs()
}

// Helper methods to reduce boilerplate
func (s *RegistryCredsTestSuite) saveArgs() map[string]interface{} {
	return map[string]interface{}{
		"dprServer":   *argDPRServer,
		"dprUser":     *argDPRUser,
		"dprPassword": *argDPRPassword,
		"acrURL":      *argACRURL,
		"acrClientID": *argACRClientID,
		"acrPassword": *argACRPassword,
		"gcrURL":      *argGCRURL,
		"awsRegion":   *argAWSRegion,
	}
}

func (s *RegistryCredsTestSuite) restoreArgs() {
	*argDPRServer = s.originalArgs["dprServer"].(string)
	*argDPRUser = s.originalArgs["dprUser"].(string)
	*argDPRPassword = s.originalArgs["dprPassword"].(string)
	*argACRURL = s.originalArgs["acrURL"].(string)
	*argACRClientID = s.originalArgs["acrClientID"].(string)
	*argACRPassword = s.originalArgs["acrPassword"].(string)
	*argGCRURL = s.originalArgs["gcrURL"].(string)
	*argAWSRegion = s.originalArgs["awsRegion"].(string)
}

// MockCredentialValidator with configurable responses
type MockCredentialValidator struct {
	hasAWS bool
	hasGCR bool
	hasDPR bool
	hasACR bool
}

func (m *MockCredentialValidator) HasValidAWSCredentials() bool { return m.hasAWS }
func (m *MockCredentialValidator) HasValidGCRCredentials() bool { return m.hasGCR }
func (m *MockCredentialValidator) ValidateDPRCredentials() bool { return m.hasDPR }
func (m *MockCredentialValidator) ValidateACRCredentials() bool { return m.hasACR }

func (m *MockCredentialValidator) EnableAll() {
	m.hasAWS, m.hasGCR, m.hasDPR, m.hasACR = true, true, true, true
}

func (m *MockCredentialValidator) DisableAll() {
	m.hasAWS, m.hasGCR, m.hasDPR, m.hasACR = false, false, false, false
}

// Simplified mock clients using interfaces
type MockECRClient struct {
	tokens []AuthToken
	err    error
}

func (m *MockECRClient) GetAuthorizationToken(input *ecr.GetAuthorizationTokenInput) (*ecr.GetAuthorizationTokenOutput, error) {
	if m.err != nil {
		return nil, m.err
	}

	data := make([]*ecr.AuthorizationData, len(m.tokens))
	for i, token := range m.tokens {
		data[i] = &ecr.AuthorizationData{
			AuthorizationToken: aws.String(token.AccessToken),
			ProxyEndpoint:      aws.String(token.Endpoint),
		}
	}

	return &ecr.GetAuthorizationTokenOutput{AuthorizationData: data}, nil
}

// Test cases using the suite - focusing on business logic
func (s *RegistryCredsTestSuite) TestSecretGeneration_NoCreds() {
	s.mockCredentials.DisableAll()

	generators := getSecretGenerators()
	s.Empty(generators, "Should have no generators when no credentials are available")
}

func (s *RegistryCredsTestSuite) TestSecretGeneration_DPROnly() {
	s.mockCredentials.DisableAll()
	s.mockCredentials.hasDPR = true

	// Set DPR credentials
	*argDPRServer = "test-server"
	*argDPRUser = "test-user"
	*argDPRPassword = "test-password"

	generators := getSecretGenerators()
	s.Len(generators, 1, "Should have exactly one generator for DPR")
	s.Equal(*argDPRSecretName, generators[0].SecretName)
	s.True(generators[0].IsJSONCfg)
}

func (s *RegistryCredsTestSuite) TestSecretGeneration_AllCreds() {
	s.mockCredentials.EnableAll()
	s.setAllCredentials()

	generators := getSecretGenerators()
	s.Len(generators, 4, "Should have generators for all four registry types")

	secretNames := make([]string, len(generators))
	for i, gen := range generators {
		secretNames[i] = gen.SecretName
	}

	s.Contains(secretNames, *argAWSSecretName)
	s.Contains(secretNames, *argGCRSecretName)
	s.Contains(secretNames, *argDPRSecretName)
	s.Contains(secretNames, *argACRSecretName)
}

func (s *RegistryCredsTestSuite) TestSkipNamespaces() {
	testCases := []struct {
		name           string
		skipKubeSystem bool
		skipNamespaces string
		namespace      string
		shouldSkip     bool
	}{
		{"kube-system with skip enabled", true, "", "kube-system", true},
		{"kube-system with skip disabled", false, "", "kube-system", false},
		{"custom skip list", false, "test1,test2", "test1", true},
		{"custom skip list with spaces", false, "test1, test2 ,test3", "test2", true},
		{"namespace not in skip list", false, "test1,test2", "default", false},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			*argSkipKubeSystem = tc.skipKubeSystem
			*argSkipNamespaces = tc.skipNamespaces

			result := shouldSkipNamespace(tc.namespace)
			s.Equal(tc.shouldSkip, result)
		})
	}
}

func (s *RegistryCredsTestSuite) TestECRTokenGeneration() {
	awsAccountIDs = []string{"123456789", "987654321"}

	mockClient := &MockECRClient{
		tokens: []AuthToken{
			{AccessToken: "token1", Endpoint: "endpoint1"},
			{AccessToken: "token2", Endpoint: "endpoint2"},
		},
	}

	tokens, err := getECRAuthorizationKey(mockClient)
	s.NoError(err)
	s.Len(tokens, 2)
	s.Equal("token1", tokens[0].AccessToken)
	s.Equal("endpoint1", tokens[0].Endpoint)
}

func (s *RegistryCredsTestSuite) TestECRTokenGeneration_Error() {
	mockClient := &MockECRClient{
		err: errors.New("AWS error"),
	}

	tokens, err := getECRAuthorizationKey(mockClient)
	s.Error(err)
	s.Empty(tokens)
	s.Contains(err.Error(), "AWS error")
}

func (s *RegistryCredsTestSuite) TestSecretObjectGeneration() {
	// Test JSON config secret generation
	tokens := []AuthToken{
		{AccessToken: "token1", Endpoint: "endpoint1"},
		{AccessToken: "token2", Endpoint: "endpoint2"},
	}

	secret, err := generateSecretObj(tokens, true, "test-secret")
	s.NoError(err)
	s.Equal("test-secret", secret.Name)
	s.Equal(corev1.SecretType("kubernetes.io/dockerconfigjson"), secret.Type)
	s.Contains(secret.Data, ".dockerconfigjson")

	// Test legacy config secret generation
	singleToken := []AuthToken{{AccessToken: "token", Endpoint: "endpoint"}}
	secret, err = generateSecretObj(singleToken, false, "legacy-secret")
	s.NoError(err)
	s.Equal("legacy-secret", secret.Name)
	s.Equal(corev1.SecretType("kubernetes.io/dockercfg"), secret.Type)
	s.Contains(secret.Data, ".dockercfg")
}

// Helper methods to reduce code duplication
func (s *RegistryCredsTestSuite) setAllCredentials() {
	*argDPRServer = "test-dpr-server"
	*argDPRUser = "test-dpr-user"
	*argDPRPassword = "test-dpr-password"
	*argACRURL = "test-acr-url"
	*argACRClientID = "test-acr-client"
	*argACRPassword = "test-acr-password"
	*argGCRURL = "test-gcr-url"
}

// Table-driven tests for various scenarios
func (s *RegistryCredsTestSuite) TestValidateParams() {
	testCases := []struct {
		name     string
		envVars  map[string]string
		expected map[string]string
	}{
		{
			name: "AWS region from environment",
			envVars: map[string]string{
				"awsregion": "us-west-2",
			},
			expected: map[string]string{
				"region": "us-west-2",
			},
		},
		{
			name: "GCR URL from environment",
			envVars: map[string]string{
				"gcrurl": "https://custom-gcr.io",
			},
			expected: map[string]string{
				"gcrurl": "https://custom-gcr.io",
			},
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Set environment variables
			for key, value := range tc.envVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}

			validateParams()

			// Verify expected values
			if region, ok := tc.expected["region"]; ok {
				s.Equal(region, *argAWSRegion)
			}
			if gcrURL, ok := tc.expected["gcrurl"]; ok {
				s.Equal(gcrURL, *argGCRURL)
			}
		})
	}
}

// Legacy test functions (simplified versions of original tests)
func TestGetECRAuthorizationKey(t *testing.T) {
	awsAccountIDs = []string{"12345678", "999999"}
	mockClient := &MockECRClient{
		tokens: []AuthToken{
			{AccessToken: "fakeToken1", Endpoint: "fakeEndpoint1"},
			{AccessToken: "fakeToken2", Endpoint: "fakeEndpoint2"},
		},
	}

	tokens, err := getECRAuthorizationKey(mockClient)

	assert.Nil(t, err)
	assert.Equal(t, 2, len(tokens))
	assert.Equal(t, "fakeToken1", tokens[0].AccessToken)
	assert.Equal(t, "fakeEndpoint1", tokens[0].Endpoint)
	assert.Equal(t, "fakeToken2", tokens[1].AccessToken)
	assert.Equal(t, "fakeEndpoint2", tokens[1].Endpoint)
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
