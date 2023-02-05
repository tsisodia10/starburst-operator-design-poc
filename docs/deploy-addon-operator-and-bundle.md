## Create the Addon Operator for the POC

## Prerequisites

- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [podman](https://podman.io/) or [docker](https://www.docker.com/)

## Deploy the Operator

```bash
(cd starburst-addon && \
make docker-build docker-push IMG="quay.io/tomerfi/starburst-addon-operator:v0.0.3")
```

> Don't forget to make `starburst-addon-operator` PUBLIC.

## Generate the Bundle

```bash
(cd starburst-addon && \
make bundle IMG="quay.io/tomerfi/starburst-addon-operator:v0.0.3" CHANNELS="alpha" DEFAULT_CHANNEL="alpha" VERSION="0.0.3")
```

## Deploy the Bundle

```bash
(cd starburst-addon && \
make bundle-build bundle-push BUNDLE_IMG="quay.io/tomerfi/starburst-addon-operator-bundle:v0.0.3")
```

> Don't forget to make `starburst-addon-operator-bundle` PUBLIC.
