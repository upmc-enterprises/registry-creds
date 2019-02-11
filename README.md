# Registry Credentials

Allow for Registry credentials to be refreshed inside your Kubernetes cluster via `ImagePullSecrets`.

## How it works

1. The tool runs as a pod in the `kube-system` namespace.
- It gets credentials from AWS ECR or Google Container Registry
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

## Setup Minikube

```bash
# Ensure the addon is enabled.
minikube addons enable registry-creds

# Generate the secret manifest and create the resources.
cd contrib && ./generate-secrets.sh
```

To use the credentials, either patch the service account:
```bash
kubectl patch serviceaccount default -p '{"imagePullSecrets": [{"name": "awsecr-cred"}]}'
```

Or add in the `imagePullSecrets` section in your `deployment.yaml` file:
```bash
spec:
  imagePullSecrets:
  - name: awsecr-cred
```

See the last section of the [Configure Service Account](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/) documentation page for full reference.

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
