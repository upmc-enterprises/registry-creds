
# registry-creds

A helm chart used to deploy releases of `registry-creds` project.


## Deployment

To deploy `registry-creds` with `helm` run the following

```bash
helm upgrade --install registry-creds registry-creds/registry-creds
```

## Removal
To remove `registry-creds` you need to do first uninstalls the software
```bash
helm uninstall registry-creds
```
After that to completelly cleanup after registry-creds you need to run the following script

```bash
temp_dir="$(mktemp -d)"
secret_names="acr-secret awsecr-cred dpr-secret gcr-secret"

for namespace in $(kubectl get namespaces --template "{{ range .items }}{{ .metadata.name }} {{ end }}"); do
    kubectl -n ${namespace} get sa default -o yaml > ${temp_dir}/${namespace}.yaml
    kubectl -n ${namespace} delete secret ${secret_names} || true
    for secret in ${secret_names}; do
        sed -i "/^- name: ${secret}$/d" ${temp_dir}/${namespace}.yaml || true
    done
    kubectl -n ${namespace} apply -f ${temp_dir}/${namespace}.yaml
done
```

## Variables

Supported values for this chart:

|                Variable                |                                                                                                                                                                       Description                                                                                                                                                                      |           Default Value          |
|:---------------------------------------|:-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|:---------------------------------|
| replicaCount                           | Number of replicas to be deployed                                                                                                                                                                                                                                                                                                                      | 1                                |
| image.repository                       | Image repository. Will be configured in `image` section for the pod                                                                                                                                                                                                                                                                                    | `upmcenterprises/registry-creds` |
| image.pullPolicy                       | K8s pull policy                                                                                                                                                                                                                                                                                                                                        | IfNotPresent                     |
| image.tag                              | Image tag. If not set will default to chart appVersion                                                                                                                                                                                                                                                                                                 | 1.10                             |
| imagePullSecrets                       | List of standard k8s image pull secrets                                                                                                                                                                                                                                                                                                                | []                               |
| nameOverride                           | Used to override names of created resources                                                                                                                                                                                                                                                                                                            | ""                               |
| fullnameOverride                       | Used to override names of created resources                                                                                                                                                                                                                                                                                                            | ""                               |
| serviceAccount.annotations             | Annotations to be added to the service account that is attached to `registry-creds` pod. E.x. ``` serviceAccount:   annotations:     annotated: yes ```                                                                                                                                                                                                | {}                               |
| serviceAccount.name                    | Service account name. If not set, default values will be used                                                                                                                                                                                                                                                                                          | ""                               |
| podAnnotations                         | Annotations to be added to the pod running `registry-creds` E.x. ``` podAnnotations:   co.elastic.logs/enabled: true   co.elastic.logs/multiline.type: pattern   co.elastic.logs/multiline.pattern: '^time'   co.elastic.logs/multiline.negate: true   co.elastic.logs/multiline.match: after ```                                                      | {}                               |
| podEnvironmentVars                     | Environment vars to be injected to pods. Will be stored in a config map that is after that included as `envFrom`  E.x. ``` podEnvironmentVars:   AWS_REGION: us-east-1 ```                                                                                                                                                                             | {}                               |
| aws                                    | Map of environment vars to pass for aws authentication. It is stored as a secret that is after that included as  `envFrom`. Check the main doc for more information E.x. ``` aws:   AWS_ACCESS_KEY_ID: changeme   AWS_SECRET_ACCESS_KEY: changeme   AWS_SESSION_TOKEN: ""   awsaccount: changeme   awsregion: changeme   aws_assume_role: changeme ``` | {}                               |
| docker                                 | Map of environment vars to pass for private container registry authentication. It is stored as a secret that is after that included as  `envFrom` . Check the main doc for more information E.x. ```docker:   DOCKER_PRIVATE_REGISTRY_SERVER: changeme   DOCKER_PRIVATE_REGISTRY_USER: changeme   DOCKER_PRIVATE_REGISTRY_PASSWORD: changeme ```       | {}                               |
| acr                                    | Map of environment vars to pass for acr authentication. It is stored as a secret that is after that included as  `envFrom` . Check the main doc for more information E.x. ``` acr:    ACR_URL: changeme   ACR_CLIENT_ID: changeme   ACR_PASSWORD: changeme ```                                                                                         | {}                               |
| gcr.credentials                        | `.json` file as described in https://cloud.google.com/docs/authentication/getting-started                                                                                                                                                                                                                                                              | changeme                         |
| gcr.gcrurl                             | gcr url                                                                                                                                                                                                                                                                                                                                                | https://gcr.io                   |
| gcr.userHomeDir                        | Home dir for a user. If you are changing the default user you need to adjust this to its home directory                                                                                                                                                                                                                                                | /                                |
| podSecurityContext.fsGroup             | Refer to https://kubernetes.io/docs/tasks/configure-pod-container/security-context/                                                                                                                                                                                                                                                                    | 65534                            |
| podSecurityContext.runAsUser           | Refer to https://kubernetes.io/docs/tasks/configure-pod-container/security-context/                                                                                                                                                                                                                                                                    | 65534                            |
| podSecurityContext.runAsGroup          | Refer to https://kubernetes.io/docs/tasks/configure-pod-container/security-context/                                                                                                                                                                                                                                                                    | 65534                            |
| securityContext.capabilities           | Refer to  https://kubernetes.io/docs/tasks/configure-pod-container/security-context/                                                                                                                                                                                                                                                                   | `drop: ["ALL"]`                  |
| securityContext.readOnlyRootFilesystem | Refer to  https://kubernetes.io/docs/tasks/configure-pod-container/security-context/                                                                                                                                                                                                                                                                   | true                             |
| securityContext.runAsNonRoot           | Refer to  https://kubernetes.io/docs/tasks/configure-pod-container/security-context/                                                                                                                                                                                                                                                                   | true                             |
| securityContext.runAsUser              | Refer to  https://kubernetes.io/docs/tasks/configure-pod-container/security-context/                                                                                                                                                                                                                                                                   | 65534                            |
| resources                              | Map, describing resources to be provided/dedicated to a pod. Refer to https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/                                                                                                                                                                                                   | {}                               |
| nodeSelector                           | Map for node selector rules. Refer to https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/                                                                                                                                                                                                                                         | {}                               |
| tolerations                            | List of tolerations. Refer to https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/                                                                                                                                                                                                                                            | []                               |
| affinity                               | Map of k8s affinity rules. Refer to https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/                                                                                                                                                                                                                                           | {}                               |

