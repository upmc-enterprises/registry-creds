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
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/aws/aws-sdk-go/aws"
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
	dockerCfgTemplate  = `{"%s":{"username":"oauth2accesstoken","password":"%s","email":"none"}}`
	dockerJSONTemplate = `{"auths":{"%s":{"auth":"%s","email":"none"}}}`
)

var (
	flags               = flag.NewFlagSet("", flag.ContinueOnError)
	argKubecfgFile      = flags.String("kubecfg-file", "", `Location of kubecfg file for access to kubernetes master service; --kube_master_url overrides the URL part of this; if neither this nor --kube_master_url are provided, defaults to service account tokens`)
	argKubeMasterURL    = flags.String("kube-master-url", "", `URL to reach kubernetes master. Env variables in this flag will be expanded.`)
	argAWSSecretName    = flags.String("aws-secret-name", "awsecr-cred", `Default aws secret name`)
	argGCRSecretName    = flags.String("gcr-secret-name", "gcr-secret", `Default gcr secret name`)
	argDefaultNamespace = flags.String("default-namespace", "default", `Default namespace`)
	argGCRURL           = flags.String("gcr-url", "https://gcr.io", `Default GCR URL`)
	argAWSRegion        = flags.String("aws-region", "us-east-1", `Default AWS region`)
	argRefreshMinutes   = flags.Int("refresh-mins", 60, `Default time to wait before refreshing (60 minutes)`)
	argSkipKubeSystem   = flags.Bool("skip-kube-system", true, `If true, will not attempt to set ImagePullSecrets on the kube-system namespace`)
)

var (
	awsAccountID string
)

type controller struct {
	k8sutil   *k8sutil.K8sutilInterface
	ecrClient ecrInterface
	gcrClient gcrInterface
}

type ecrInterface interface {
	GetAuthorizationToken(input *ecr.GetAuthorizationTokenInput) (*ecr.GetAuthorizationTokenOutput, error)
}

type gcrInterface interface {
	DefaultTokenSource(ctx context.Context, scope ...string) (oauth2.TokenSource, error)
}

func newEcrClient() ecrInterface {
	return ecr.New(session.New(), aws.NewConfig().WithRegion(*argAWSRegion))
}

type gcrClient struct{}

func (gcr gcrClient) DefaultTokenSource(ctx context.Context, scope ...string) (oauth2.TokenSource, error) {
	return google.DefaultTokenSource(ctx, scope...)
}

func newGcrClient() gcrInterface {
	return gcrClient{}
}

func (c *controller) getGCRAuthorizationKey() (AuthToken, error) {
	ts, err := c.gcrClient.DefaultTokenSource(context.TODO(), "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return AuthToken{}, err
	}

	token, err := ts.Token()
	if err != nil {
		return AuthToken{}, err
	}

	if !token.Valid() {
		return AuthToken{}, fmt.Errorf("token was invalid")
	}

	if token.Type() != "Bearer" {
		return AuthToken{}, fmt.Errorf(fmt.Sprintf("expected token type \"Bearer\" but got \"%s\"", token.Type()))
	}

	return AuthToken{
		AccessToken: token.AccessToken,
		Endpoint:    *argGCRURL}, nil
}

func (c *controller) getECRAuthorizationKey() (AuthToken, error) {
	params := &ecr.GetAuthorizationTokenInput{
		RegistryIds: []*string{
			aws.String(awsAccountID),
		},
	}

	resp, err := c.ecrClient.GetAuthorizationToken(params)

	if err != nil {
		// Print the error, cast err to awserr.Error to get the Code and
		// Message from an error.
		logrus.Println(err.Error())
		return AuthToken{}, err
	}

	token := resp.AuthorizationData[0]

	return AuthToken{
		AccessToken: *token.AuthorizationToken,
		Endpoint:    *token.ProxyEndpoint}, err
}

func generateSecretObj(token string, endpoint string, isJSONCfg bool, secretName string) *v1.Secret {
	secret := &v1.Secret{
		ObjectMeta: v1.ObjectMeta{
			Name: secretName,
		},
	}
	if isJSONCfg {
		secret.Data = map[string][]byte{
			".dockerconfigjson": []byte(fmt.Sprintf(dockerJSONTemplate, endpoint, token))}
		secret.Type = "kubernetes.io/dockerconfigjson"
	} else {
		secret.Data = map[string][]byte{
			".dockercfg": []byte(fmt.Sprintf(dockerCfgTemplate, endpoint, token))}
		secret.Type = "kubernetes.io/dockercfg"
	}
	return secret
}

type AuthToken struct {
	AccessToken string
	Endpoint    string
}

type SecretGenerator struct {
	TokenGenFxn func() (AuthToken, error)
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
		fmt.Errorf("Could not get ServiceAccounts! %v", err)
		return err
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

		newToken, err := secretGenerator.TokenGenFxn()
		if err != nil {
			logrus.Printf("Error getting secret for provider %s. Skipping secret provider! [Err: %s]", secretGenerator.SecretName, err)
			continue
		}
		newSecret := generateSecretObj(newToken.AccessToken, newToken.Endpoint, secretGenerator.IsJSONCfg, secretGenerator.SecretName)
		secrets = append(secrets, newSecret)
	}
	return secrets
}

func validateParams() {
	// Allow environment variables to overwrite args
	awsAccountIDEnv := os.Getenv("awsaccount")
	awsRegionEnv := os.Getenv("awsregion")

	if len(awsRegionEnv) > 0 {
		argAWSRegion = &awsRegionEnv
	}

	if len(awsAccountIDEnv) > 0 {
		awsAccountID = awsAccountIDEnv
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

	validateParams()

	log.Print("Using AWS Account: ", awsAccountID)
	log.Printf("Using AWS Region: %s", *argAWSRegion)
	log.Print("Refresh Interval (minutes): ", *argRefreshMinutes)

	util, err := k8sutil.New(*argKubecfgFile, *argKubeMasterURL)

	if err != nil {
		logrus.Error("Could not create k8s client!!", err)
	}

	ecrClient := newEcrClient()
	gcrClient := newGcrClient()
	c := &controller{util, ecrClient, gcrClient}

	util.WatchNamespaces(time.Duration(*argRefreshMinutes)*time.Minute, func(ns *v1.Namespace) error {
		return handler(c, ns)
	})
}
