# registry-creds

A kustomization for deploying registry-creds.

An example set of secrets exists in `secrets.example.yaml` but is not included in the kustomization.
You will need to add secrets via your overlay.

The deployment makes the following secrets available to the container:

    * registry-creds-dpr
    * registry-creds-ecr
    * registry-creds-gcr
    * registry-creds-acr
