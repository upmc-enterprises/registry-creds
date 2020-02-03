#!/bin/bash
set -euo pipefail

# Retrieve private docker registry data from the AWS cli.
AWS_ECR_LOGIN_CMD=$(aws ecr get-login)
AWS_ECR_USER=$(echo "${AWS_ECR_LOGIN_CMD}"| cut -d ' ' -f4 | tr -d '\n' | base64 )
AWS_ECR_PASSWORD=$(echo "${AWS_ECR_LOGIN_CMD}"| cut -d ' ' -f6 | tr -d '\n' | base64 )
AWS_ECR_EMAIL=$(echo "${AWS_ECR_LOGIN_CMD}"| cut -d ' ' -f8 | tr -d '\n' | base64 )
AWS_ECR_SERVER=$(echo "${AWS_ECR_LOGIN_CMD}"| cut -d ' ' -f9 | tr -d '\n' | base64 )

# Retrieve AWS access from the AWS cli.
AWS_ACCESS_KEY_BASE64=$(aws configure get default.aws_access_key_id | tr -d '\n'| base64 )
AWS_SECRET_KEY_BASE64=$(aws configure get default.aws_secret_access_key | tr -d '\n' | base64 )
AWS_REGION_BASE64=$(aws configure get region | tr -d '\n' | base64 )
AWS_ACCOUNT_ID_BASE64=$(aws iam get-user | grep arn:aws | cut -d':' -f6 | tr -d '\n' | base64 )

# Retrieve GCloud credentials from default configuration.
GCLOUD_DEFAULT_CREDS_FILE="${HOME}/.config/gcloud/application_default_credentials.json"
if [ -f  "${GCLOUD_DEFAULT_CREDS_FILE}" ]; then
  GCLOUD_DEFAULT_CREDS_BASE64=$(base64 "${GCLOUD_DEFAULT_CREDS_FILE}")
else
  GCLOUD_DEFAULT_CREDS_BASE64="Y2hhbmdlbWU="
fi

# Generate the secrets file.
cat > secret.yaml  <<EOF
---
apiVersion: v1
kind: Secret
metadata:
  name: registry-creds-dpr
  namespace: kube-system
  labels:
    app: registry-creds
    kubernetes.io/minikube-addons: registry-creds
    cloud: private
data:
  DOCKER_PRIVATE_REGISTRY_SERVER: "${AWS_ECR_SERVER}"
  DOCKER_PRIVATE_REGISTRY_USER: "${AWS_ECR_USER}"
  DOCKER_PRIVATE_REGISTRY_PASSWORD: "${AWS_ECR_PASSWORD}"
type: Opaque

---
apiVersion: v1
kind: Secret
metadata:
  name: registry-creds-ecr
  namespace: kube-system
  labels:
    app: registry-creds
    kubernetes.io/minikube-addons: registry-creds
    cloud: ecr
data:
  AWS_ACCESS_KEY_ID: "${AWS_ACCESS_KEY_BASE64}"
  AWS_SECRET_ACCESS_KEY: "${AWS_SECRET_KEY_BASE64}"
  aws-region: "${AWS_REGION_BASE64}"
  aws-account: "${AWS_ACCOUNT_ID_BASE64}"
  aws-assume-role: ""
  AWS_SESSION_TOKEN: ""
type: Opaque
---

apiVersion: v1
kind: Secret
metadata:
  name: registry-creds-gcr
  namespace: kube-system
  labels:
    app: registry-creds
    kubernetes.io/minikube-addons: registry-creds
    cloud: gcr
data:
  application_default_credentials.json: "${GCLOUD_DEFAULT_CREDS_BASE64}"
  gcrurl: aHR0cHM6Ly9nY3IuaW8=
type: Opaque
EOF

# Create the Kubernetes objects.
kubectl apply -f secret.yaml

# Clean up.
rm -f secret.yaml
