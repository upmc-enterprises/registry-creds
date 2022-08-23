> NOTE: this is a fork of [upmc-enterprises/registry-creds](https://github.com/upmc-enterprises/registry-creds/network/members)

Doddle has done the following modifications (or would like to):


- [x] reduced the support down to AWS ECR only (its all we need and its easier to maintain this way
- [x] exit on errors instead of just logging an error and hanging in a useless state (still not ideal)
- [x] github action to publish a public docker image
- [x] new `--excluded-namespaces` (comma seperated) flag for namespaces that can be ignored
- [x] updated the k8s client to automatically select if its inCluster or else default to $KUBECONFIG
- [ ] removed `vendor/`
- [ ] logging updated to zap with structured json
- [x] example `.envrc` for local development






## running this locally..

You will need 4 env vars to make this run correctly.

- The first two should be self evident `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`
- Additionally you'll need to set `awsaccount` to the account ID and `awsregion`

> We suggest using [direnv](https://direnv.net/) for this.. simply `cp .envrc.example .envrc` and edit the file to your needs




---

# Registry Credentials

Allow for Registry credentials to be refreshed inside your Kubernetes cluster via `ImagePullSecrets`.

## How it works

1. The tool runs as a pod in the `kube-system` namespace.
- It gets credentials from AWS ECR, Google Container Registry, Docker private registry, or Azure Container Registry.
- Next it creates a secret with credentials for your registry
- Then it sets up this secret to be used in the `ImagePullSecrets` for the default service account
- Whenever a pod is created, this secret is attached to the pod
- The container will refresh the credentials by default every 60 minutes
- Enabled for use with Minikube as an [addon](https://github.com/kubernetes/minikube#add-ons)

> **NOTE:** This will setup credentials across ALL namespaces!

## Parameters

The following parameters are driven via Environment variables.

- Environment Variables:
  - AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY: Credentials to access AWS.
  - awsaccount: Comma separated list of AWS Account Ids.
  - awsregion: (optional) Can override the default AWS region by setting this variable.
  - aws-assume-role (optional) can provide a role ARN that will be assumed for getting ECR authorization tokens
    > **Note:** The region can also be specified as an arg to the binary.
  - TOKEN_RETRY_TYPE: The type of Timer to use when getting a registry token fails and must be retried; "simple" or "exponential" (default: simple)
  - TOKEN_RETRIES: The number of times to retry getting a registry token if an error occurred (default: 3)
  - TOKEN_RETRY_DELAY: The number of seconds to delay between successive retries at getting a registry token; applies to "simple" retry timer only (default: 5)
  - GCRURL: URL to Google Container Registry
  - DOCKER_PRIVATE_REGISTRY_SERVER, DOCKER_PRIVATE_REGISTRY_USER, DOCKER_PRIVATE_REGISTRY_PASSWORD: the URL, user name, and password for a Docker private registry
  - ACR_URL, ACR_CLIENT_ID, ACR_PASSWORD: the registry URL, client ID, and password to access to access an Azure Container Registry.

## How to setup running in AWS

1. Clone the repo and navigate to directory

2. Configure

   1. If running on AWS EC2, make sure your EC2 instances have the following IAM permissions:

      ```json
      {
       "Effect": "Allow",
        "Action": [
         "ecr:GetAuthorizationToken",
         "ecr:BatchCheckLayerAvailability",
         "ecr:GetDownloadUrlForLayer",
         "ecr:GetRepositoryPolicy",
         "ecr:DescribeRepositories",
         "ecr:ListImages",
         "ecr:BatchGetImage"
       ],
       "Resource": "*"
      }
      ```

   2. If you are not running in AWS Cloud, then you can still use this tool! Edit & create the sample [secret](k8s/secret.yaml) and update values for `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `aws-account`, and `aws-region` (base64 encoded).

      ```bash
      echo -n "secret-key" | base64

      kubectl create -f k8s/secret.yaml
      ```

3. Create the replication controller.

   ```bash
   kubectl create -f k8s/replicationController.yaml
   ```

   > **NOTE:** If running on premise, no need to provide `AWS_ACCESS_KEY_ID` or `AWS_SECRET_ACCESS_KEY` since that will come from the EC2 instance.

4. Use `awsecr-cred` for name of `imagePullSecrets` on your `deployment.yaml` file.

## How to setup running in GCR

1. Clone the repo and navigate to directory

2. Input your `application_default_credentials.json` information into the `secret.yaml` template located [here](k8s/secret.yaml#L17):
The value for `application_default_credentials.json` can be obtained with the following command:

   ```bash
   base64 -w 0 $HOME/.config/gcloud/application_default_credentials.json
   ```

3. Create the secret in kubernetes

   ```bash
   kubectl create -f k8s/secret.yml
   ```

4. Create the replication controller:

   ```bash
   kubectl create -f k8s/replicationController.yaml
   ```

## How to setup running in Docker Private Registry

1. Clone the repo and navigate to directory

2. Edit the sample [secret](k8s/secret.yaml) and update values for `DOCKER_PRIVATE_REGISTRY_SERVER`, `DOCKER_PRIVATE_REGISTRY_USER`, and `DOCKER_PRIVATE_REGISTRY_PASSWORD` (base64 encoded).

   ```bash
   echo -n "secret-key" | base64
   ```

3. Create the secret in kubernetes

   ```bash
   kubectl create -f k8s/secret.yml
   ```

4. Create the replication controller:

   ```bash
   kubectl create -f k8s/replicationController.yaml
   ```

## How to set up Azure Container Registry

1. [Create a service principal](https://docs.microsoft.com/en-us/azure/container-registry/container-registry-auth-service-principal) that your Kubernetes cluster will use to access the registry.

2. Clone the repo and navigate to the repo root

3. Edit the sample [secret](k8s/secret.yaml) and update values for `ACR_URL`, `ACR_CLIENT_ID`, and `ACR_PASSWORD` (base64 encoded). Use service principal application ID as the client ID, and service principal password (client secret) as the password.

   ```bash
   echo -n "secret-key" | base64
   ```

3. Create the secret in kubernetes

   ```bash
   kubectl create -f k8s/secret.yml
   ```

4. Create the replication controller:

   ```bash
   kubectl create -f k8s/replicationController.yaml
   ```

## DockerHub Image

- [upmcenterprises/registry-creds](https://hub.docker.com/r/upmcenterprises/registry-creds/)

## Developing Locally

If you want to hack on this project:

1. Clone the repo
2. Build: `make build`
3. Test: `make test`
4. Run on your machine: `go run ./main.go --kubecfg-file=<pathToKubecfgFile>`

## About

Built by UPMC Enterprises in Pittsburgh, PA. http://enterprises.upmc.com/
