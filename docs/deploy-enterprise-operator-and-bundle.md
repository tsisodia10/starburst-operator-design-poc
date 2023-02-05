## Create a placeholder for the Starburst Enterprise Operator

## Prerequisites

- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [podman](https://podman.io/) or [docker](https://www.docker.com/)

## Deploy the Operator

```bash
(cd starburst-enterprise && \
make docker-build docker-push IMG="quay.io/tomerfi/starburst-enterprise-operator:v0.0.3")
```

> Don't forget to make `starburst-enterprise-operator` PUBLIC.

## Generate the Bundle

```bash
(cd starburst-enterprise && \
make bundle IMG="quay.io/tomerfi/starburst-enterprise-operator:v0.0.3" CHANNELS="alpha" DEFAULT_CHANNEL="alpha" VERSION="0.0.3")
```

## Deploy the Bundle

```bash
(cd starburst-enterprise && \
make bundle-build bundle-push BUNDLE_IMG="quay.io/tomerfi/starburst-enterprise-operator-bundle:v0.0.3")
```

> Don't forget to make `starburst-enterprise-operator-bundle` PUBLIC.
