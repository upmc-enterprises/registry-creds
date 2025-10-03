/*
Copyright (c) 2017, UPMC Enterprises
All rights reserved.
Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:
    * Redistributions of source code must retain the above copyright
      notice, this list of conditions and the following disclaimer.
    * Redistributions in binary form must reproduce the above copyright
      notice, this list of conditions and the following disclaimer in the
      documentation and/or other materials provided with the distribution.
    * Neither the name UPMC Enterprises nor the
      names of its contributors may be used to endorse or promote products
      derived from this software without specific prior written permission.
THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL UPMC ENTERPRISES BE LIABLE FOR ANY
DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
(INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
*/

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	"github.com/cenkalti/backoff"
	"github.com/manifest-cyber/registry-creds/k8sutil"
	"github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// Retry Types
	retryTypeSimple      = "simple"
	retryTypeExponential = "exponential"

	dockerCfgTemplate                = `{"%s":{"username":"oauth2accesstoken","password":"%s","email":"none"}}`
	dockerPrivateRegistryPasswordKey = "DOCKER_PRIVATE_REGISTRY_PASSWORD"
	dockerPrivateRegistryServerKey   = "DOCKER_PRIVATE_REGISTRY_SERVER"
	dockerPrivateRegistryUserKey     = "DOCKER_PRIVATE_REGISTRY_USER"
	acrURLKey                        = "ACR_URL"
	acrClientIDKey                   = "ACR_CLIENT_ID"
	acrPasswordKey                   = "ACR_PASSWORD"
	tokenGenRetryTypeKey             = "TOKEN_RETRY_TYPE"
	tokenGenRetriesKey               = "TOKEN_RETRIES"
	tokenGenRetryDelayKey            = "TOKEN_RETRY_DELAY"
	defaultTokenGenRetries           = 3
	defaultTokenGenRetryDelay        = 5 // in seconds
	defaultTokenGenRetryType         = retryTypeSimple
)

var (
	flags                    = flag.NewFlagSet("", flag.ContinueOnError)
	argKubecfgFile           = flags.String("kubecfg-file", "", `Location of kubecfg file for access to kubernetes master service; --kube_master_url overrides the URL part of this; if neither this nor --kube_master_url are provided, defaults to service account tokens`)
	argKubeMasterURL         = flags.String("kube-master-url", "", `URL to reach kubernetes master. Env variables in this flag will be expanded.`)
	argAWSSecretName         = flags.String("aws-secret-name", "awsecr-cred", `Default AWS secret name`)
	argDPRSecretName         = flags.String("dpr-secret-name", "dpr-secret", `Default Docker Private Registry secret name`)
	argGCRSecretName         = flags.String("gcr-secret-name", "gcr-secret", `Default GCR secret name`)
	argACRSecretName         = flags.String("acr-secret-name", "acr-secret", "Default Azure Container Registry secret name")
	argGCRURL                = flags.String("gcr-url", "https://gcr.io", `Default GCR URL`)
	argAWSRegion             = flags.String("aws-region", "us-east-1", `Default AWS region`)
	argDPRPassword           = flags.String("dpr-password", "", "Docker Private Registry password")
	argDPRServer             = flags.String("dpr-server", "", "Docker Private Registry server")
	argDPRUser               = flags.String("dpr-user", "", "Docker Private Registry user")
	argACRURL                = flags.String("acr-url", "", "Azure Container Registry URL")
	argACRClientID           = flags.String("acr-client-id", "", "Azure Container Registry client ID (user name)")
	argACRPassword           = flags.String("acr-password", "", "Azure Container Registry password (client secret)")
	argRefreshMinutes        = flags.Int("refresh-mins", 60, `Default time to wait before refreshing (60 minutes)`)
	argSkipKubeSystem        = flags.Bool("skip-kube-system", true, `If true, will not attempt to set ImagePullSecrets on the kube-system namespace`)
	argSkipNamespaces        = flags.String("skip-namespaces", "", `Comma-separated list of namespaces to skip (in addition to kube-system if skip-kube-system is true)`)
	argAWSAssumeRole         = flags.String("aws_assume_role", "", `If specified AWS will assume this role and use it to retrieve tokens`)
	argTokenGenFxnRetryType  = flags.String("token-retry-type", defaultTokenGenRetryType, `The type of retry timer to use when generating a secret token; either simple or exponential (simple)`)
	argTokenGenFxnRetries    = flags.Int("token-retries", defaultTokenGenRetries, `Default number of times to retry generating a secret token (3)`)
	argTokenGenFxnRetryDelay = flags.Int("token-retry-delay", defaultTokenGenRetryDelay, `Default number of seconds to wait before retrying secret token generation (5 seconds)`)
	argServiceAccount        = flags.String("service-account", "default", "ServiceAccount to use, defaults to 'default'")
)

var (
	awsAccountIDs []string

	// RetryCfg represents the currently-configured number of retries + retry delay
	RetryCfg RetryConfig

	// The retry backoff timers
	simpleBackoff      *backoff.ConstantBackOff
	exponentialBackoff *backoff.ExponentialBackOff
)

type dockerJSON struct {
	Auths map[string]registryAuth `json:"auths,omitempty"`
}

type registryAuth struct {
	Auth  string `json:"auth"`
	Email string `json:"email"`
}

type controller struct {
	k8sutil *k8sutil.K8sutilInterface
}

// RetryConfig represents the number of retries + the retry delay for retrying an operation if it should fail
type RetryConfig struct {
	Type                string
	NumberOfRetries     int
	RetryDelayInSeconds int
}

// Docker Private Registry interface
type dprInterface interface {
	getAuthToken(server, user, password string) (AuthToken, error)
}

type ecrInterface interface {
	GetAuthorizationToken(input *ecr.GetAuthorizationTokenInput) (*ecr.GetAuthorizationTokenOutput, error)
}

type gcrInterface interface {
	DefaultTokenSource(ctx context.Context, scope ...string) (oauth2.TokenSource, error)
}

type acrInterface interface {
	getAuthToken(registryURL, clientID, password string) (AuthToken, error)
}

type dprClient struct{}

func (dpr dprClient) getAuthToken(server, user, password string) (AuthToken, error) {
	if server == "" {
		return AuthToken{}, fmt.Errorf("Failed to get auth token for docker private registry: empty value for %s", dockerPrivateRegistryServerKey)
	}

	if user == "" {
		return AuthToken{}, fmt.Errorf("Failed to get auth token for docker private registry: empty value for %s", dockerPrivateRegistryUserKey)
	}

	if password == "" {
		return AuthToken{}, fmt.Errorf("Failed to get auth token for docker private registry: empty value for %s", dockerPrivateRegistryPasswordKey)
	}

	token := base64.StdEncoding.EncodeToString([]byte(strings.Join([]string{user, password}, ":")))

	return AuthToken{AccessToken: token, Endpoint: server}, nil
}

func newDprClient() dprInterface {
	return dprClient{}
}

type gcrClient struct{}

func (gcr gcrClient) DefaultTokenSource(ctx context.Context, scope ...string) (oauth2.TokenSource, error) {
	return google.DefaultTokenSource(ctx, scope...)
}

func getDPRToken(client dprInterface) ([]AuthToken, error) {
	token, err := client.getAuthToken(*argDPRServer, *argDPRUser, *argDPRPassword)
	return []AuthToken{token}, err
}

func getGCRAuthorizationKey(client gcrInterface) ([]AuthToken, error) {
	ts, err := client.DefaultTokenSource(context.TODO(), "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return []AuthToken{}, err
	}

	token, err := ts.Token()
	if err != nil {
		return []AuthToken{}, err
	}

	if !token.Valid() {
		return []AuthToken{}, fmt.Errorf("token was invalid")
	}

	if token.Type() != "Bearer" {
		return []AuthToken{}, fmt.Errorf("expected token type \"Bearer\" but got \"%s\"", token.Type())
	}

	tokens := make([]AuthToken, 0)
	tokens = append(tokens, AuthToken{token.AccessToken, *argGCRURL})

	return tokens, nil
}

func getECRAuthorizationKey(client ecrInterface) ([]AuthToken, error) {
	var tokens []AuthToken
	var regIds []*string
	regIds = make([]*string, len(awsAccountIDs))

	for i, awsAccountID := range awsAccountIDs {
		regIds[i] = aws.String(awsAccountID)
	}

	params := &ecr.GetAuthorizationTokenInput{
		RegistryIds: regIds,
	}

	resp, err := client.GetAuthorizationToken(params)

	if err != nil {
		// Print the error, cast err to awserr.Error to get the Code and
		// Message from an error.
		logrus.Println(err.Error())
		return []AuthToken{}, err
	}

	for _, auth := range resp.AuthorizationData {
		tokens = append(tokens, AuthToken{
			AccessToken: *auth.AuthorizationToken,
			Endpoint:    *auth.ProxyEndpoint,
		})
	}
	return tokens, nil
}

func generateSecretObj(tokens []AuthToken, isJSONCfg bool, secretName string) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: secretName,
		},
	}
	if isJSONCfg {
		auths := map[string]registryAuth{}
		for _, token := range tokens {
			auths[token.Endpoint] = registryAuth{
				Auth:  token.AccessToken,
				Email: "none",
			}
		}
		configJSON, err := json.Marshal(dockerJSON{Auths: auths})
		if err != nil {
			return secret, nil
		}
		secret.Data = map[string][]byte{".dockerconfigjson": configJSON}
		secret.Type = "kubernetes.io/dockerconfigjson"
	} else {
		if len(tokens) == 1 {
			secret.Data = map[string][]byte{
				".dockercfg": []byte(fmt.Sprintf(dockerCfgTemplate, tokens[0].Endpoint, tokens[0].AccessToken))}
			secret.Type = "kubernetes.io/dockercfg"
		}
	}
	return secret, nil
}

type acrClient struct{}

func (c acrClient) getAuthToken(registryURL, clientID, password string) (AuthToken, error) {
	if registryURL == "" {
		return AuthToken{}, fmt.Errorf("Azure Container Registry URL is missing; ensure %s parameter is set", acrURLKey)
	}

	if clientID == "" {
		return AuthToken{}, fmt.Errorf("Client ID needed to access Azure Container Registry is missing; ensure %s parameter is set", acrClientIDKey)
	}

	if password == "" {
		return AuthToken{}, fmt.Errorf("Password needed to access Azure Container Registry is missing; ensure %s paremeter is set", acrClientIDKey)
	}

	token := base64.StdEncoding.EncodeToString([]byte(strings.Join([]string{clientID, password}, ":")))

	return AuthToken{AccessToken: token, Endpoint: registryURL}, nil
}

func getACRToken(client acrInterface) ([]AuthToken, error) {
	token, err := client.getAuthToken(*argACRURL, *argACRClientID, *argACRPassword)
	return []AuthToken{token}, err
}

func newACRClient() acrInterface {
	return acrClient{}
}

// Credential validation functions
// CredentialValidator interface for validating and creating clients
type CredentialValidator interface {
	HasValidAWSCredentials() bool
	HasValidGCRCredentials() bool
	ValidateDPRCredentials() bool
	ValidateACRCredentials() bool
}

// ProductionCredentialValidator implements real credential validation
type ProductionCredentialValidator struct{}

func (p *ProductionCredentialValidator) HasValidAWSCredentials() bool {
	// Create session with explicit credential chain
	sess, err := session.NewSessionWithOptions(session.Options{
		Config: aws.Config{
			Region: aws.String(*argAWSRegion),
		},
		// This ensures the credential chain is properly initialized
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		logrus.Debugf("Failed to create AWS session, skipping ECR: %v", err)
		return false
	}

	// Test if we can get credentials
	_, err = sess.Config.Credentials.Get()
	if err != nil {
		logrus.Debugf("No AWS credentials available, skipping ECR: %v", err)
		return false
	}

	logrus.Info("AWS credentials found, enabling ECR support")
	return true
}

func (p *ProductionCredentialValidator) HasValidGCRCredentials() bool {
	// Test if we can get GCP credentials
	_, err := google.DefaultTokenSource(context.TODO(), "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		logrus.Debugf("No GCP credentials available, skipping GCR: %v", err)
		return false
	}

	logrus.Info("GCP credentials found, enabling GCR support")
	return true
}

func (p *ProductionCredentialValidator) ValidateDPRCredentials() bool {
	hasCredentials := *argDPRServer != "" && *argDPRUser != "" && *argDPRPassword != ""
	if hasCredentials {
		logrus.Info("Docker Private Registry credentials found, enabling DPR support")
	} else {
		logrus.Debug("Docker Private Registry credentials not found, skipping DPR")
	}
	return hasCredentials
}

func (p *ProductionCredentialValidator) ValidateACRCredentials() bool {
	hasCredentials := *argACRURL != "" && *argACRClientID != "" && *argACRPassword != ""
	if hasCredentials {
		logrus.Info("Azure Container Registry credentials found, enabling ACR support")
	} else {
		logrus.Debug("Azure Container Registry credentials not found, skipping ACR")
	}
	return hasCredentials
}

// Global credential validator - can be overridden for testing
var credentialValidator CredentialValidator = &ProductionCredentialValidator{}

// createECRClient creates an ECR client with proper AWS configuration
func createECRClient() ecrInterface {
	sess, err := session.NewSessionWithOptions(session.Options{
		Config: aws.Config{
			Region: aws.String(*argAWSRegion),
		},
		SharedConfigState: session.SharedConfigEnable,
	})
	if err != nil {
		logrus.Errorf("Failed to create AWS session: %v", err)
		return nil
	}

	awsConfig := aws.NewConfig().WithRegion(*argAWSRegion)

	if *argAWSAssumeRole != "" {
		creds := stscreds.NewCredentials(sess, *argAWSAssumeRole)
		awsConfig.Credentials = creds
	}

	return ecr.New(sess, awsConfig)
}

// AuthToken represents an Access Token and an Endpoint for a registry service
type AuthToken struct {
	AccessToken string
	Endpoint    string
}

// SecretGenerator represents a token generation function for a registry service
type SecretGenerator struct {
	TokenGenFxn func() ([]AuthToken, error)
	IsJSONCfg   bool
	SecretName  string
}

func getSecretGenerators() []SecretGenerator {
	secretGenerators := make([]SecretGenerator, 0)

	// Only add GCR if we have valid credentials
	if credentialValidator.HasValidGCRCredentials() {
		gcrClient := gcrClient{}
		secretGenerators = append(secretGenerators, SecretGenerator{
			TokenGenFxn: func() ([]AuthToken, error) {
				return getGCRAuthorizationKey(gcrClient)
			},
			IsJSONCfg:  false,
			SecretName: *argGCRSecretName,
		})
	}

	// Only add ECR if we have valid AWS credentials
	if credentialValidator.HasValidAWSCredentials() {
		ecrClient := createECRClient()
		secretGenerators = append(secretGenerators, SecretGenerator{
			TokenGenFxn: func() ([]AuthToken, error) {
				return getECRAuthorizationKey(ecrClient)
			},
			IsJSONCfg:  true,
			SecretName: *argAWSSecretName,
		})
	}

	// Only add DPR if we have all required credentials
	if credentialValidator.ValidateDPRCredentials() {
		dprClient := newDprClient()
		secretGenerators = append(secretGenerators, SecretGenerator{
			TokenGenFxn: func() ([]AuthToken, error) {
				return getDPRToken(dprClient)
			},
			IsJSONCfg:  true,
			SecretName: *argDPRSecretName,
		})
	}

	// Only add ACR if we have all required credentials
	if credentialValidator.ValidateACRCredentials() {
		acrClient := newACRClient()
		secretGenerators = append(secretGenerators, SecretGenerator{
			TokenGenFxn: func() ([]AuthToken, error) {
				return getACRToken(acrClient)
			},
			IsJSONCfg:  true,
			SecretName: *argACRSecretName,
		})
	}

	logrus.Infof("Initialized %d registry credential providers", len(secretGenerators))
	return secretGenerators
}

func (c *controller) processNamespace(namespace *corev1.Namespace, secret *corev1.Secret) error {
	log := logrus.WithField("function", "processNamespace")
	// Check if the secret exists for the namespace
	log.Debugf("checking for secret %s in namespace %s", secret.Name, namespace.GetName())
	_, err := c.k8sutil.GetSecret(namespace.GetName(), secret.Name)

	if err != nil {
		log.Debugf("Could not find secret %s in namespace %s; will try to create it", secret.Name, namespace.GetName())
		// Secret not found, create
		err := c.k8sutil.CreateSecret(namespace.GetName(), secret)
		if err != nil {
			return fmt.Errorf("could not create Secret: %v", err)
		}
		log.Infof("Created new secret %s in namespace %s", secret.Name, namespace.GetName())
	} else {
		// Existing secret needs updated
		log.Debugf("Found secret %s in namespace %s; will try to update it", secret.Name, namespace.GetName())
		err := c.k8sutil.UpdateSecret(namespace.GetName(), secret)
		if err != nil {
			return fmt.Errorf("could not update Secret: %v", err)
		}
		log.Infof("Updated secret %s in namespace %s", secret.Name, namespace.GetName())
	}

	// Check if ServiceAccount exists
	serviceAccount, err := c.k8sutil.GetServiceAccount(namespace.GetName(), *argServiceAccount)
	if err != nil {
		log.Errorf("error getting service account '%s' in namespace %s: %s", *argServiceAccount, namespace.GetName(), err)
		return fmt.Errorf("could not get ServiceAccounts: %v", err)
	}

	// Update existing one if image pull secrets already exists for aws ecr token
	imagePullSecretFound := false
	for i, imagePullSecret := range serviceAccount.ImagePullSecrets {
		if imagePullSecret.Name == secret.Name {
			serviceAccount.ImagePullSecrets[i] = corev1.LocalObjectReference{Name: secret.Name}
			imagePullSecretFound = true
			break
		}
	}

	// Append to list of existing service accounts if there isn't one already
	if !imagePullSecretFound {
		serviceAccount.ImagePullSecrets = append(serviceAccount.ImagePullSecrets, corev1.LocalObjectReference{Name: secret.Name})
	}

	log.Infof("Updating ServiceAccount %s in namespace %s", serviceAccount.Name, namespace.GetName())
	err = c.k8sutil.UpdateServiceAccount(namespace.GetName(), serviceAccount)
	if err != nil {
		log.Errorf("error updating ServiceAccount %s in namespace %s: %s", serviceAccount.Name, namespace.GetName(), err)
		return fmt.Errorf("could not update ServiceAccount: %v", err)
	}

	return nil
}

func (c *controller) generateSecrets() []*corev1.Secret {
	var secrets []*corev1.Secret
	secretGenerators := getSecretGenerators()

	if len(secretGenerators) == 0 {
		logrus.Warn("No registry credentials configured - no secrets will be generated")
		return secrets
	}

	maxTries := RetryCfg.NumberOfRetries + 1
	for _, secretGenerator := range secretGenerators {
		resetRetryTimer()
		logrus.Infof("------------------ [%s] ------------------\n", secretGenerator.SecretName)

		var newTokens []AuthToken
		tries := 0
		for {
			tries++
			logrus.Infof("Getting secret; try #%d of %d", tries, maxTries)
			tokens, err := secretGenerator.TokenGenFxn()
			if err != nil {
				if tries < maxTries {
					delayDuration := nextRetryDuration()
					if delayDuration == backoff.Stop {
						logrus.Errorf("Error getting secret for provider %s. Retry timer exceeded max tries/duration; will not try again until the next refresh cycle. [Err: %s]", secretGenerator.SecretName, err)
						break
					}
					logrus.Errorf("Error getting secret for provider %s. Will try again after %f seconds. [Err: %s]", secretGenerator.SecretName, delayDuration.Seconds(), err)
					<-time.After(delayDuration)
					continue
				}
				logrus.Errorf("Error getting secret for provider %s. Tried %d time(s); will not try again until the next refresh cycle. [Err: %s]", secretGenerator.SecretName, tries, err)
				break
			} else {
				logrus.Infof("Successfully got secret for provider %s after trying %d time(s)", secretGenerator.SecretName, tries)
				newTokens = tokens
				break
			}
		}

		if len(newTokens) > 0 {
			newSecret, err := generateSecretObj(newTokens, secretGenerator.IsJSONCfg, secretGenerator.SecretName)
			if err != nil {
				logrus.Errorf("Error generating secret for provider %s. Skipping secret provider until the next refresh cycle! [Err: %s]", secretGenerator.SecretName, err)
			} else {
				secrets = append(secrets, newSecret)
			}
		}
	}
	return secrets
}

// SetupRetryTimer initializes and configures the Retry Timer
func SetupRetryTimer() {
	delayDuration := time.Duration(RetryCfg.RetryDelayInSeconds) * time.Second
	switch RetryCfg.Type {
	case retryTypeSimple:
		simpleBackoff = backoff.NewConstantBackOff(delayDuration)
		break
	case retryTypeExponential:
		exponentialBackoff = backoff.NewExponentialBackOff()
		break
	}
}

func resetRetryTimer() {
	switch RetryCfg.Type {
	case retryTypeSimple:
		simpleBackoff.Reset()
		break
	case retryTypeExponential:
		exponentialBackoff.Reset()
		break
	}
}

func nextRetryDuration() time.Duration {
	switch RetryCfg.Type {
	case retryTypeSimple:
		return simpleBackoff.NextBackOff()
	case retryTypeExponential:
		return exponentialBackoff.NextBackOff()
	default:
		return time.Duration(defaultTokenGenRetryDelay) * time.Second
	}
}

func validateParams() {
	// Allow environment variables to overwrite args
	awsAccountIDEnv := os.Getenv("awsaccount")
	awsRegionEnv := os.Getenv("awsregion")
	argAWSAssumeRoleEnv := os.Getenv("aws_assume_role")
	dprPassword := os.Getenv(dockerPrivateRegistryPasswordKey)
	dprServer := os.Getenv(dockerPrivateRegistryServerKey)
	dprUser := os.Getenv(dockerPrivateRegistryUserKey)
	acrURL := os.Getenv(acrURLKey)
	acrClientID := os.Getenv(acrClientIDKey)
	acrPassword := os.Getenv(acrPasswordKey)
	gcrURLEnv := os.Getenv("gcrurl")

	// initialize the retry configuration using command line values
	RetryCfg = RetryConfig{
		Type:                *argTokenGenFxnRetryType,
		NumberOfRetries:     *argTokenGenFxnRetries,
		RetryDelayInSeconds: *argTokenGenFxnRetryDelay,
	}
	// ensure command line values are valid
	if RetryCfg.Type != retryTypeSimple && RetryCfg.Type != retryTypeExponential {
		logrus.Errorf("Unknown Retry Timer type '%s'! Defaulting to %s", RetryCfg.Type, defaultTokenGenRetryType)
		RetryCfg.Type = defaultTokenGenRetryType
	}
	if RetryCfg.NumberOfRetries < 0 {
		logrus.Errorf("Cannot use a negative value for the number of retries! Defaulting to %d", defaultTokenGenRetries)
		RetryCfg.NumberOfRetries = defaultTokenGenRetries
	}
	if RetryCfg.RetryDelayInSeconds < 0 {
		logrus.Errorf("Cannot use a negative value for the retry delay in seconds! Defaulting to %d", defaultTokenGenRetryDelay)
		RetryCfg.RetryDelayInSeconds = defaultTokenGenRetryDelay
	}
	// look for overrides in environment variables and use them if they exist and are valid
	tokenType, ok := os.LookupEnv(tokenGenRetryTypeKey)
	if ok && len(tokenType) > 0 {
		if tokenType != retryTypeSimple && tokenType != retryTypeExponential {
			logrus.Errorf("Unknown Retry Timer type '%s'! Defaulting to %s", tokenType, defaultTokenGenRetryType)
			RetryCfg.Type = defaultTokenGenRetryType
		} else {
			RetryCfg.Type = tokenType
		}
	}
	tokenRetries, ok := os.LookupEnv(tokenGenRetriesKey)
	if ok && len(tokenRetries) > 0 {
		tokenRetriesInt, err := strconv.Atoi(tokenRetries)
		if err != nil {
			logrus.Errorf("Unable to parse value of environment variable %s! [Err: %s]", tokenGenRetriesKey, err)
		} else {
			if tokenRetriesInt < 0 {
				logrus.Errorf("Cannot use a negative value for environment variable %s! Defaulting to %d", tokenGenRetriesKey, defaultTokenGenRetries)
				RetryCfg.NumberOfRetries = defaultTokenGenRetries
			} else {
				RetryCfg.NumberOfRetries = tokenRetriesInt
			}
		}
	}
	tokenRetryDelay, ok := os.LookupEnv(tokenGenRetryDelayKey)
	if ok && len(tokenRetryDelay) > 0 {
		tokenRetryDelayInt, err := strconv.Atoi(tokenRetryDelay)
		if err != nil {
			logrus.Errorf("Unable to parse value of environment variable %s! [Err: %s]", tokenGenRetryDelayKey, err)
		} else {
			if tokenRetryDelayInt < 0 {
				logrus.Errorf("Cannot use a negative value for environment variable %s! Defaulting to %d", tokenGenRetryDelayKey, defaultTokenGenRetryDelay)
				RetryCfg.RetryDelayInSeconds = defaultTokenGenRetryDelay
			} else {
				RetryCfg.RetryDelayInSeconds = tokenRetryDelayInt
			}
		}
	}
	// Set up the Retry Timer
	SetupRetryTimer()

	if len(awsRegionEnv) > 0 {
		argAWSRegion = &awsRegionEnv
	}

	if len(awsAccountIDEnv) > 0 {
		awsAccountIDs = strings.Split(awsAccountIDEnv, ",")
	} else {
		awsAccountIDs = []string{""}
	}

	if len(dprPassword) > 0 {
		argDPRPassword = &dprPassword
	}

	if len(dprServer) > 0 {
		argDPRServer = &dprServer
	}

	if len(dprUser) > 0 {
		argDPRUser = &dprUser
	}

	if len(gcrURLEnv) > 0 {
		argGCRURL = &gcrURLEnv
	}

	if len(argAWSAssumeRoleEnv) > 0 {
		argAWSAssumeRole = &argAWSAssumeRoleEnv
	}

	if len(acrURL) > 0 {
		argACRURL = &acrURL
	}

	if len(acrClientID) > 0 {
		argACRClientID = &acrClientID
	}

	if len(acrPassword) > 0 {
		argACRPassword = &acrPassword
	}
}

// shouldSkipNamespace checks if a namespace should be skipped based on configuration
func shouldSkipNamespace(namespaceName string) bool {
	// Check if kube-system should be skipped
	if *argSkipKubeSystem && namespaceName == "kube-system" {
		return true
	}

	// Check if namespace is in the skip list
	if *argSkipNamespaces != "" {
		skipList := strings.Split(*argSkipNamespaces, ",")
		for _, skipNs := range skipList {
			skipNs = strings.TrimSpace(skipNs)
			if skipNs == namespaceName {
				return true
			}
		}
	}

	return false
}

func handler(c *controller, ns *corev1.Namespace) error {
	logrus.Infof("Refreshing credentials for namespace %s", ns.GetName())
	secrets := c.generateSecrets()
	logrus.Infof("Got %d refreshed credentials for namespace %s", len(secrets), ns.GetName())
	for _, secret := range secrets {
		if shouldSkipNamespace(ns.GetName()) {
			logrus.Debugf("Skipping namespace %s", ns.GetName())
			continue
		}

		logrus.Infof("Processing secret for namespace %s, secret %s", ns.Name, secret.Name)

		if err := c.processNamespace(ns, secret); err != nil {
			logrus.Errorf("error processing secret for namespace %s, secret %s: %s", ns.Name, secret.Name, err)
			return err
		}

		logrus.Infof("Finished processing secret for namespace %s, secret %s", ns.Name, secret.Name)
	}
	logrus.Infof("Finished refreshing credentials for namespace %s", ns.GetName())
	return nil
}

func main() {
	logrus.Info("Starting up...")
	err := flags.Parse(os.Args)
	if err != nil {
		logrus.Fatalf("Could not parse command line arguments! [Err: %s]", err)
	}

	validateParams()

	logrus.Info("Using AWS Account: ", strings.Join(awsAccountIDs, ","))
	logrus.Info("Using AWS Region: ", *argAWSRegion)
	logrus.Info("Using AWS Assume Role: ", *argAWSAssumeRole)
	logrus.Info("Refresh Interval (minutes): ", *argRefreshMinutes)
	logrus.Infof("Retry Timer: %s", RetryCfg.Type)
	logrus.Info("Token Generation Retries: ", RetryCfg.NumberOfRetries)
	logrus.Info("Token Generation Retry Delay (seconds): ", RetryCfg.RetryDelayInSeconds)

	util, err := k8sutil.New(*argKubecfgFile, *argKubeMasterURL)
	if err != nil {
		logrus.Fatal("Could not create k8s client!!", err)
	}

	c := &controller{k8sutil: util}

	util.WatchNamespaces(time.Duration(*argRefreshMinutes)*time.Minute, func(ns *corev1.Namespace) error {
		return handler(c, ns)
	})
}
