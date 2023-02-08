# Running the POC

Cloning is **not** required, all operators, bundles, and catalogs, are publicly accessible on *quay.io*.<br/>
Follow the instructions in this document and you will:

- Create a Kind cluster
- Install OLM
- Apply a *CatalogSource* CR referencing our *starburst-addon* and *starburst-enterprise* packages
- Create a *Secret* containing the desired manifest for the *StarburstEnterprise* CR
- Subscribe to the *starburst-addon* package (depends on *starburst-enterprise*)
  - Verify both operators are up
- Apply a *StarburstAddon* CR
  - Verify creation of the *StarburstEnterprise* CR and owner
  - Attempt (and fail) manually deleting the *StarburstEnterprise* CR
  - Attempt (and fail) manually patching the *StarburstEnterprise* CR
  - Attempt (and fail) manually creating a new *StarburstEnterprise* CR
- Delete the *StarburstAddon* CR
  - Verify deletion of the *StarburstEnterprise* CR
- Delete the previously created Kind cluster

## Prerequisites

- [kind](https://kind.sigs.k8s.io/)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [operator-sdk](https://sdk.operatorframework.io/docs/installation/)

## Prepare POC environment

```bash
kind create cluster --name starburst
```

```bash
operator-sdk olm install
```

## Load OSE Images

Follow instructions [here](load-required-ose-images.md) to load the required images

## Create catalog source

```bash
kubectl apply -f -<< EOF
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: starburst-combined-catalog
  namespace: olm
spec:
  sourceType: grpc
  grpcPodConfig:
    securityContextConfig: restricted
  image: quay.io/tomerfi/starburst-combined-catalog:dev
  displayName: Starburst Combined Catalog
  publisher: Me
  updateStrategy:
    registryPoll:
      interval: 60m
EOF
```

```bash
# the package manifest records can take up to a minute to appear
$ watch 'kubectl get packagemanifests.packages.operators.coreos.com | grep "starburst\|ose"'

ose-prometheus-operator                    Starburst Combined Catalog   45s
starburst-addon                            Starburst Combined Catalog   45s
starburst-enterprise                       Starburst Combined Catalog   45s
```

## Subscribe to addon package

### Create target namespace

```bash
kubectl create ns starburst-playground
```

### Create the Vault Secret

```bash
# a secret named "addon" structured from the vault keys will be manifested by osd
kubectl create secret generic addon -n starburst-playground \
--from-file enterprise.yaml=starburst-enterprise/config/samples/example.com_v1alpha1_starburstenterprise.yaml
```

### Apply OperatorGroup for target namespace

```bash
kubectl apply -f -<< EOF
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: my-starburst-op-group
  namespace: starburst-playground
spec:
  targetNamespaces:
  - starburst-playground
EOF
```

### Apply a Subscription to the addon package

```bash
kubectl apply -f -<< EOF
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: starburst-addon-subscription
  namespace: starburst-playground
spec:
  channel: alpha1
  name: starburst-addon
  source: starburst-combined-catalog
  sourceNamespace: olm
  installPlanApproval: Automatic
EOF
```

### Verify both operators are up

```bash
# might take a couple of minutes
$ watch kubectl get pods -n starburst-playground

NAME                                                          READY   STATUS    RESTARTS   AGE
starburst-addon-controller-manager-7c7f978ff-svqv4            2/2     Running   0          51s
starburst-addon-validate-enterprise-webhook-685f4f7db-8qwjr   2/2     Running   0          51s
starburst-enterprise-controller-manager-8b4869d99-6wncl       2/2     Running   0          54s
```

### Verify both packages are deployed successfully

```bash
$ kubectl get csv -n starburst-playground

NAME                          DISPLAY                VERSION   REPLACES   PHASE
starburst-addon.v0.0.1        Starburst Addon        0.0.1                Succeeded
starburst-enterprise.v0.0.1   Starburst Enterprise   0.0.1                Succeeded
```

## Run Starburst Addon

### Apply a StarburstAddon CR

```bash
kubectl apply -f -<< EOF
apiVersion: example.com.example.com/v1alpha1
kind: StarburstAddon
metadata:
  labels:
    app: starburst-addon
  name: starburstaddon-sample
  namespace: starburst-playground
EOF
```

### Verify a StarburstEnterprise CR was created

```bash
# this might take a couple of seconds to appear
$ watch kubectl get starburstenterprises -n starburst-playground

NAME                               AGE
starburstaddon-sample-enterprise   10s
```

### Verify the owner for the StarburstEnterprise CR

```bash
$ kubectl get starburstenterprises -n starburst-playground starburstaddon-sample-enterprise -o jsonpath='{.metadata.ownerReferences[0]}' | jq

{
  "apiVersion": "example.com.example.com/v1alpha1",
  "blockOwnerDeletion": true,
  "controller": true,
  "kind": "StarburstAddon",
  "name": "starburstaddon-sample",
  "uid": "..."
}
```

### Attempt (and fail) manually deleting the StarburstEnterprise CR

```bash
$ kubectl delete starburstenterprises -n starburst-playground starburstaddon-sample-enterprise

Error from server (Unauthorized Access): admission webhook "starburst-addon-validate-enterprise-webhook.example.com.example.com" denied the request: DELETE not allowed
```

### Attempt (and fail) manually patching the StarburstEnterprise CR

```bash
$ kubectl label starburstenterprises -n starburst-playground starburstaddon-sample-enterprise  someKey=someValue

Error from server (Unauthorized Access): admission webhook "starburst-addon-validate-enterprise-webhook.example.com.example.com" denied the request: UPDATE not allowed
```

### Attempt (and fail) manually creating a new StarburstEnterprise CR

```bash
$ kubectl apply -f -<< EOF
apiVersion: example.com.example.com/v1alpha1
kind: StarburstEnterprise
metadata:
  name: new-starburst-enterprise-cr
  namespace: starburst-playground
EOF

Error from server (Unauthorized Access): error when creating "STDIN": admission webhook "starburst-addon-validate-enterprise-webhook.example.com.example.com" denied the request: CREATE not allowed
```

### Delete the StarburstAddon CR

```bash
kubectl delete starburstaddons -n starburst-playground starburstaddon-sample
```

### Verify the StarburstEnterprise CR was deleted

```bash
$ kubectl get starburstenterprises -n starburst-playground

No resources found in starburst-playground namespace.
```

## Teardown POC environment

```bash
kind delete cluster --name starburst
```
