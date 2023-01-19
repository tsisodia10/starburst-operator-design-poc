## Create the Addon Operator for the POC

## Prerequisites

- [kind](https://kind.sigs.k8s.io/)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [operator-sdk](https://sdk.operatorframework.io/docs/installation/)
- [podman](https://podman.io/) or [docker](https://www.docker.com/)

## Generating the Controller

> If the code is already created, i.e. you cloned this and you're making modifications,<br/>
> skip to the next step, [Deploying the Controller](#deploying-the-controller).

```bash
mkdir starburst-addon
```

```bash
(cd starburst-addon \
&& operator-sdk init --domain example.com --repo github.com/example/starburst-addon-operator)
```

```bash
(cd starburst-addon && \
operator-sdk create api --group example.com --version v1alpha1 --kind StarburstAddon --resource --controller)
```

### Deploying the Controller

```bash
(cd starburst-addon && \
make docker-build docker-push IMG="quay.io/tomerfi/starburst-addon-operator:v0.0.1")
```

> Don't forget to make `starburst-addon-operator` PUBLIC.

## Generating the Bundle

> If the code is already created, i.e. you cloned this and you're making modifications,<br/>
> skip to the next step, [Validating and Deploying the Bundle](#validating-and-deploying-the-bundle).

```bash
(cd starburst-addon && \
make bundle IMG="quay.io/tomerfi/starburst-addon-operator:v0.0.1")
```

### Validating and Deploying the Bundle

```bash
(cd starburst-addon && \
operator-sdk bundle validate ./bundle)
```

```bash
(cd starburst-addon && \
make bundle-build bundle-push BUNDLE_IMG="quay.io/tomerfi/starburst-addon-operator-bundle:v0.0.1")
```

> Don't forget to make `starburst-addon-operator-bundle` PUBLIC.
