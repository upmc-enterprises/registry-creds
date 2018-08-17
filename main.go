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
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	flag "github.com/spf13/pflag"
	"github.com/upmc-enterprises/registry-creds/k8sutil"
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"k8s.io/client-go/pkg/api/v1"
)

const (
	dockerCfgTemplate                = `{"%s":{"username":"oauth2accesstoken","password":"%s","email":"none"}}`
	dockerPrivateRegistryPasswordKey = "DOCKER_PRIVATE_REGISTRY_PASSWORD"
	dockerPrivateRegistryServerKey   = "DOCKER_PRIVATE_REGISTRY_SERVER"
	dockerPrivateRegistryUserKey     = "DOCKER_PRIVATE_REGISTRY_USER"
)

var (
	flags             = flag.NewFlagSet("", flag.ContinueOnError)
	argKubecfgFile    = flags.String("kubecfg-file", "", `Location of kubecfg file for access to kubernetes master service; --kube_master_url overrides the URL part of this; if neither this nor --kube_master_url are provided, defaults to service account tokens`)
	argKubeMasterURL  = flags.String("kube-master-url", "", `URL to reach kubernetes master. Env variables in this flag will be expanded.`)
	argAWSSecretName  = flags.String("aws-secret-name", "awsecr-cred", `Default AWS secret name`)
	argDPRSecretName  = flags.String("dpr-secret-name", "dpr-secret", `Default Docker Private Registry secret name`)
	argGCRSecretName  = flags.String("gcr-secret-name", "gcr-secret", `Default GCR secret name`)
	argGCRURL         = flags.String("gcr-url", "https://gcr.io", `Default GCR URL`)
	argAWSRegion      = flags.String("aws-region", "us-east-1", `Default AWS region`)
	argDPRPassword    = flags.String("dpr-password", "", "Docker Private Registry password")
	argDPRServer      = flags.String("dpr-server", "", "Docker Private Registry server")
	argDPRUser        = flags.String("dpr-user", "", "Docker Private Registry user")
	argRefreshMinutes = flags.Int("refresh-mins", 60, `Default time to wait before refreshing (60 minutes)`)
	argSkipKubeSystem = flags.Bool("skip-kube-system", true, `If true, will not attempt to set ImagePullSecrets on the kube-system namespace`)
	argAWSAssumeRole  = flags.String("aws_assume_role", "", `If specified AWS will assume this role and use it to retrieve tokens`)
)

var (
	awsAccountIDs []string
)

type dockerJSON struct {
	Auths map[string]registryAuth `json:"auths,omitempty"`
}

type registryAuth struct {
	Auth  string `json:"auth"`
	Email string `json:"email"`
}

type controller struct {
	k8sutil   *k8sutil.K8sutilInterface
	ecrClient ecrInterface
	gcrClient gcrInterface
	dprClient dprInterface
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

func newEcrClient() ecrInterface {
	sess := session.Must(session.NewSession())
	awsConfig := aws.NewConfig().WithRegion(*argAWSRegion)

	if *argAWSAssumeRole != "" {
		creds := stscreds.NewCredentials(sess, *argAWSAssumeRole)
		awsConfig.Credentials = creds
	}

	return ecr.New(sess, awsConfig)
}

type dprClient struct{}

func (dpr dprClient) getAuthToken(server, user, password string) (AuthToken, error) {
	if server == "" {
		return AuthToken{}, fmt.Errorf(fmt.Sprintf("Failed to get auth token for docker private registry: empty value for %s", dockerPrivateRegistryServerKey))
	}

	if user == "" {
		return AuthToken{}, fmt.Errorf(fmt.Sprintf("Failed to get auth token for docker private registry: empty value for %s", dockerPrivateRegistryUserKey))
	}

	if password == "" {
		return AuthToken{}, fmt.Errorf(fmt.Sprintf("Failed to get auth token for docker private registry: empty value for %s", dockerPrivateRegistryPasswordKey))
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

func newGcrClient() gcrInterface {
	return gcrClient{}
}

func (c *controller) getDPRToken() ([]AuthToken, error) {
	token, err := c.dprClient.getAuthToken(*argDPRServer, *argDPRUser, *argDPRPassword)
	return []AuthToken{token}, err
}

func (c *controller) getGCRAuthorizationKey() ([]AuthToken, error) {
	ts, err := c.gcrClient.DefaultTokenSource(context.TODO(), "https://www.googleapis.com/auth/cloud-platform")
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
		return []AuthToken{}, fmt.Errorf(fmt.Sprintf("expected token type \"Bearer\" but got \"%s\"", token.Type()))
	}

	return []AuthToken{
		AuthToken{
			AccessToken: token.AccessToken,
			Endpoint:    *argGCRURL},
	}, nil
}

func (c *controller) getECRAuthorizationKey() ([]AuthToken, error) {

	var tokens []AuthToken
	var regIds []*string
	regIds = make([]*string, len(awsAccountIDs))

	for i, awsAccountID := range awsAccountIDs {
		regIds[i] = aws.String(awsAccountID)
	}

	params := &ecr.GetAuthorizationTokenInput{
		RegistryIds: regIds,
	}

	resp, err := c.ecrClient.GetAuthorizationToken(params)

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

func generateSecretObj(tokens []AuthToken, isJSONCfg bool, secretName string) (*v1.Secret, error) {
	secret := &v1.Secret{
		ObjectMeta: v1.ObjectMeta{
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

type AuthToken struct {
	AccessToken string
	Endpoint    string
}

type SecretGenerator struct {
	TokenGenFxn func() ([]AuthToken, error)
	IsJSONCfg   bool
	SecretName  string
}

func getSecretGenerators(c *controller) []SecretGenerator {
	secretGenerators := []SecretGenerator{}

	secretGenerators = append(secretGenerators, SecretGenerator{
		TokenGenFxn: c.getGCRAuthorizationKey,
		IsJSONCfg:   false,
		SecretName:  *argGCRSecretName,
	})

	secretGenerators = append(secretGenerators, SecretGenerator{
		TokenGenFxn: c.getECRAuthorizationKey,
		IsJSONCfg:   true,
		SecretName:  *argAWSSecretName,
	})

	secretGenerators = append(secretGenerators, SecretGenerator{
		TokenGenFxn: c.getDPRToken,
		IsJSONCfg:   true,
		SecretName:  *argDPRSecretName,
	})

	return secretGenerators
}

func (c *controller) processNamespace(namespace *v1.Namespace, secret *v1.Secret) error {
	// Check if the secret exists for the namespace
	_, err := c.k8sutil.GetSecret(namespace.GetName(), secret.Name)

	if err != nil {
		// Secret not found, create
		err := c.k8sutil.CreateSecret(namespace.GetName(), secret)
		if err != nil {
			return fmt.Errorf("Could not create Secret! %v", err)
		}
	} else {
		// Existing secret needs updated
		err := c.k8sutil.UpdateSecret(namespace.GetName(), secret)
		if err != nil {
			return fmt.Errorf("Could not update Secret! %v", err)
		}
	}

	// Check if ServiceAccount exists
	serviceAccount, err := c.k8sutil.GetServiceAccount(namespace.GetName(), "default")
	if err != nil {
		return fmt.Errorf("Could not get ServiceAccounts! %v", err)
	}

	// Update existing one if image pull secrets already exists for aws ecr token
	imagePullSecretFound := false
	for i, imagePullSecret := range serviceAccount.ImagePullSecrets {
		if imagePullSecret.Name == secret.Name {
			serviceAccount.ImagePullSecrets[i] = v1.LocalObjectReference{Name: secret.Name}
			imagePullSecretFound = true
			break
		}
	}

	// Append to list of existing service accounts if there isn't one already
	if !imagePullSecretFound {
		serviceAccount.ImagePullSecrets = append(serviceAccount.ImagePullSecrets, v1.LocalObjectReference{Name: secret.Name})
	}

	err = c.k8sutil.UpdateServiceAccount(namespace.GetName(), serviceAccount)
	if err != nil {
		return fmt.Errorf("Could not update ServiceAccount! %v", err)
	}

	return nil
}

func (c *controller) generateSecrets() []*v1.Secret {
	var secrets []*v1.Secret
	secretGenerators := getSecretGenerators(c)

	for _, secretGenerator := range secretGenerators {
		logrus.Printf("------------------ [%s] ----------------------\n", secretGenerator.SecretName)

		newTokens, err := secretGenerator.TokenGenFxn()
		if err != nil {
			logrus.Printf("Error getting secret for provider %s. Skipping secret provider! [Err: %s]", secretGenerator.SecretName, err)
			continue
		}
		newSecret, err := generateSecretObj(newTokens, secretGenerator.IsJSONCfg, secretGenerator.SecretName)
		if err != nil {
			logrus.Printf("Error generating secret for provider %s. Skipping secret provider! [Err: %s]", secretGenerator.SecretName, err)
		} else {
			secrets = append(secrets, newSecret)
		}
	}
	return secrets
}

func userHomeDir() string {
	if runtime.GOOS == "windows" {
		home := os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}
		return home
	}
	return os.Getenv("HOME")
}

func getDefaultAWSCredentials() (string, string) {
	awsAccountID, awsRegion := "", ""

	homeDir := userHomeDir()
	credPath := filepath.Join(homeDir, ".aws/credentials")

	if _, err := os.Stat(credPath); !os.IsNotExist(err) {
		output, err := exec.Command("aws", "sts", "get-caller-identity", "--output", "text", "--query", "Account").Output()
		if err != nil {
			logrus.Error("Get AWS accountID from default credential file error!", err)
		} else {
			awsAccountID = strings.TrimRight(string(output), "\n")
		}

		output, err = exec.Command("aws", "configure", "get", "region").Output()
		if err != nil {
			logrus.Error("Get AWS region from default credential file error!", err)
		} else {
			awsRegion = strings.TrimRight(string(output), "\n")
		}
	}

	return awsAccountID, awsRegion
}

func validateParams() {
	// Allow environment variables to overwrite args
	awsAccountIDEnv := os.Getenv("awsaccount")
	awsRegionEnv := os.Getenv("awsregion")
	argAWSAssumeRoleEnv := os.Getenv("aws_assume_role")
	dprPassword := os.Getenv(dockerPrivateRegistryPasswordKey)
	dprServer := os.Getenv(dockerPrivateRegistryServerKey)
	dprUser := os.Getenv(dockerPrivateRegistryUserKey)
	gcrURLEnv := os.Getenv("gcrurl")

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
}

func handler(c *controller, ns *v1.Namespace) error {
	log.Print("Refreshing credentials...")
	secrets := c.generateSecrets()
	for _, secret := range secrets {
		if *argSkipKubeSystem && ns.GetName() == "kube-system" {
			continue
		}

		if err := c.processNamespace(ns, secret); err != nil {
			return err
		}

		log.Printf("Finished processing secret for namespace %s, secret %s", ns.Name, secret.Name)
	}
	return nil
}

func main() {
	log.Print("Starting up...")
	flags.Parse(os.Args)

	awsAccountID, *argAWSRegion = getDefaultAWSCredentials()

	validateParams()

	log.Print("Using AWS Account: ", strings.Join(awsAccountIDs, ","))
	log.Print("Using AWS Region: ", *argAWSRegion)
	log.Print("Using AWS Assume Role: ", *argAWSAssumeRole)
	log.Print("Refresh Interval (minutes): ", *argRefreshMinutes)

	util, err := k8sutil.New(*argKubecfgFile, *argKubeMasterURL)

	if err != nil {
		logrus.Error("Could not create k8s client!!", err)
	}

	ecrClient := newEcrClient()
	gcrClient := newGcrClient()
	dprClient := newDprClient()
	c := &controller{util, ecrClient, gcrClient, dprClient}

	util.WatchNamespaces(time.Duration(*argRefreshMinutes)*time.Minute, func(ns *v1.Namespace) error {
		return handler(c, ns)
	})
}
