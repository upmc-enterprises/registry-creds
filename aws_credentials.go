/*
Copyright (c) 2016, UPMC Enterprises
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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ecr"
	flag "github.com/spf13/pflag"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/unversioned"
	kubectl_util "k8s.io/kubernetes/pkg/kubectl/cmd/util"
)

var (
	flags                = flag.NewFlagSet("", flag.ContinueOnError)
	cluster              = flags.Bool("use-kubernetes-cluster-service", true, `If true, use the built in kubernetes cluster for creating the client`)
	argKubecfgFile       = flags.String("kubecfg-file", "", `Location of kubecfg file for access to kubernetes master service; --kube_master_url overrides the URL part of this; if neither this nor --kube_master_url are provided, defaults to service account tokens`)
	argKubeMasterURL     = flags.String("kube-master-url", "", `URL to reach kubernetes master. Env variables in this flag will be expanded.`)
	argDefaultSecretName = flags.String("default-secret-name", "awsecr-creds", `Default secret name`)
	argDefaultNamespace  = flags.String("default-namespace", "default", `Default namespace`)
	argAWSRegion         = flags.String("aws-region", "us-east-1", `Default AWS region`)
	argRefreshMinutes    = flags.Int("refresh-mins", 715, `Default time to wait before refreshing (11 hours 55 mins)`)
)

var (
	kubeClient   *unversioned.Client
	awsAccountID string
)

func getECRAuthorizationKey() (token *ecr.AuthorizationData, err error) {
	svc := ecr.New(session.New(), aws.NewConfig().WithRegion(*argAWSRegion))

	params := &ecr.GetAuthorizationTokenInput{
		RegistryIds: []*string{
			aws.String(awsAccountID),
		},
	}

	resp, err := svc.GetAuthorizationToken(params)

	if err != nil {
		// Print the error, cast err to awserr.Error to get the Code and
		// Message from an error.
		fmt.Println(err.Error())
		return
	}

	token = resp.AuthorizationData[0]

	return
}

// Gets the default secret from the api
func getSecret(namespace string) (*api.Secret, error) {
	secret, err := kubeClient.Secrets(namespace).Get(*argDefaultSecretName)
	return secret, err
}

func getSecretObj(dockerConfigJSONContent []byte) *api.Secret {
	secret := &api.Secret{
		ObjectMeta: api.ObjectMeta{
			Name: *argDefaultSecretName,
		},
		Data: map[string][]byte{
			".dockerconfigjson": dockerConfigJSONContent,
		},
		Type: "kubernetes.io/dockerconfigjson",
	}

	return secret
}

func process() {
	// Get new token to seed the secret
	dockerJSONTemplate := `{"auths":{"%v":{"auth":"%v","email":"none"}}}`

	newToken, _ := getECRAuthorizationKey()
	authToken := fmt.Sprintf(dockerJSONTemplate, *newToken.ProxyEndpoint, *newToken.AuthorizationToken)
	newSecret := getSecretObj([]byte(authToken))

	// Get all namespaces
	namespaces, _ := kubeClient.Namespaces().List(api.ListOptions{})

	for _, namespace := range namespaces.Items {

		if namespace.GetName() == "kube-system" {
			continue
		}

		// Check if the secret exists for the namespace
		_, err := getSecret(namespace.GetName())

		if err != nil {
			// Secret not found, create
			kubeClient.Secrets(namespace.GetName()).Create(newSecret)
		} else {
			// Existing secret needs updated
			kubeClient.Secrets(namespace.GetName()).Update(newSecret)
		}

		// Check if ServiceAccount exists
		serviceAccount, err := kubeClient.ServiceAccounts(namespace.GetName()).Get("default")

		if err != nil {
			log.Fatalf("Couldn't get default service account!")
		}

		// Update existing one if image pull secrets already exists for aws ecr token
		imagePullSecretFound := false
		for i, imagePullSecret := range serviceAccount.ImagePullSecrets {
			if imagePullSecret.Name == *argDefaultSecretName {
				serviceAccount.ImagePullSecrets[i] = api.LocalObjectReference{Name: *argDefaultSecretName}
				imagePullSecretFound = true
				break
			}
		}

		// Append to list of existing service accounts if there isn't one already
		if !imagePullSecretFound {
			serviceAccount.ImagePullSecrets = append(serviceAccount.ImagePullSecrets, api.LocalObjectReference{Name: *argDefaultSecretName})
		}

		_, err = kubeClient.ServiceAccounts(namespace.GetName()).Update(serviceAccount)
		if err != nil {
			fmt.Println("err: ", err)
		}
	}
}

func main() {
	log.Print("Starting up...")
	flags.Parse(os.Args)

	awsAccountID = os.Getenv("awsaccount")

	log.Print("Using AWS Account: ", awsAccountID)
	log.Print("Refresh Interval (minutes): ", *argRefreshMinutes)

	clientConfig := kubectl_util.DefaultClientConfig(flags)

	var err error

	if *cluster {
		if kubeClient, err = unversioned.NewInCluster(); err != nil {
			log.Fatalf("Failed to create client: %v", err)
		}
	} else {
		config, err := clientConfig.ClientConfig()
		if err != nil {
			log.Fatalf("error connecting to the client: %v", err)
		}
		kubeClient, err = unversioned.New(config)
	}

	tick := time.Tick(time.Duration(*argRefreshMinutes) * time.Minute)

	// Process once now, then wait for tick
	process()

	for {
		select {
		case <-tick:
			log.Print("Refreshing credentials...")
			process()
		}
	}

}
