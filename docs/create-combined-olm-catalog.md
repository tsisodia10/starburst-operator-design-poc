## Create the Combined OLM Catalog for the POC

Prior to creating the catalog, the [Addon Bundle](deploy-addon-operator-and-bundle.md) and [Enterprise Bundle](deploy-enterprise-operator-and-bundle.md) must deployed publicly.

## Prerequisites

- [opm](https://docs.openshift.com/container-platform/4.12/cli_reference/opm/cli-opm-install.html)
- [podman](https://podman.io/) or [docker](https://www.docker.com/)

## Initialize with the Addon Package

```bash
opm init starburst-addon --default-channel=alpha1 --output yaml > olm-catalog/catalog/operator.yaml
```

```bash
opm render quay.io/tomerfi/starburst-addon-operator-bundle:v0.0.2 --output yaml >> olm-catalog/catalog/operator.yaml
```

```bash
cat << EOF >> olm-catalog/catalog/operator.yaml
---
schema: olm.channel
package: starburst-addon
name: alpha1
entries:
  - name: starburst-addon.v0.0.2
EOF
```

## Adding the Enterprise Package

```bash
opm init starburst-enterprise --default-channel=alpha1 --output yaml >> olm-catalog/catalog/operator.yaml
```

```bash
opm render quay.io/tomerfi/starburst-enterprise-operator-bundle:v0.0.2 --output yaml >> olm-catalog/catalog/operator.yaml
```

```bash
cat << EOF >> olm-catalog/catalog/operator.yaml
---
schema: olm.channel
package: starburst-enterprise
name: alpha1
entries:
  - name: starburst-enterprise.v0.0.2
EOF
```

## Validating and Deploying the Catalog

```bash
opm validate olm-catalog/catalog
```

```bash
(cd olm-catalog/ && \
podman build . -f catalog.Dockerfile -t "quay.io/tomerfi/starburst-combined-catalog:dev" && \
podman push quay.io/tomerfi/starburst-combined-catalog:dev)
```

> Don't forget to make `starburst-combined-catalog` PUBLIC.
