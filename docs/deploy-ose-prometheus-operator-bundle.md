## Deploy a version of OSE Prometheus Operator Bundle

## Prerequisites

- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [podman](https://podman.io/) or [docker](https://www.docker.com/)

## Deploy the Bundle

(cd ose-prometheus-operator/ && \
podman build . -f bundle.Dockerfile -t "quay.io/tomerfi/ose-prometheus-operator-bundle:v4.10.0" && \
podman push quay.io/tomerfi/ose-prometheus-operator-bundle:v4.10.0)
