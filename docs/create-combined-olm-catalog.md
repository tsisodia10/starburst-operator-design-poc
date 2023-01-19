## Create the Combined OLM Catalog for the POC

Prior to creating teh catalog, the [Addon Bundle](create-new-design-addon-operator.md) and [Enterprise Bundle](create-placeholder-enterprise-operator.md) must be created and public.

## Prerequisites

- [opm](https://docs.openshift.com/container-platform/4.12/cli_reference/opm/cli-opm-install.html)
- [podman](https://podman.io/) or [docker](https://www.docker.com/)

## Generating the Catalog

> If the code is already created, i.e. you cloned this and you're making modifications,<br/>
> skip to the next step, [Updating the Catalog](#updating-the-catalog).

```bash
mkdir -p olm-catalog/catalog
```

```bash
(cd olm-catalog && \
opm generate dockerfile catalog)
```

### Updating the Catalog

```bash
rm -f olm-catalog/catalog/operator.yaml
```

```bash
touch olm-catalog/catalog/operator.yaml
```

#### Including the Addon Package

```bash
opm init starburst-addon --default-channel=alpha1 --output yaml >> olm-catalog/catalog/operator.yaml
```

```bash
opm render quay.io/tomerfi/starburst-addon-operator-bundle:v0.0.1 --output yaml >> olm-catalog/catalog/operator.yaml
```

```bash
cat << EOF >> olm-catalog/catalog/operator.yaml
---
schema: olm.channel
package: starburst-addon
name: alpha1
entries:
  - name: starburst-addon.v0.0.1
EOF
```

#### Including the Enterprise Package

```bash
opm init starburst-enterprise --default-channel=alpha1 --output yaml >> olm-catalog/catalog/operator.yaml
```

```bash
opm render quay.io/tomerfi/starburst-enterprise-operator-bundle:v0.0.1 --output yaml >> olm-catalog/catalog/operator.yaml
```

```bash
cat << EOF >> olm-catalog/catalog/operator.yaml
---
schema: olm.channel
package: starburst-enterprise
name: alpha1
entries:
  - name: starburst-enterprise.v0.0.1
EOF
```

### Validating and Deploying the Catalog

```bash
opm validate olm-catalog/catalog
```

```bash
(cd olm-catalog/ && \
podman build . -f catalog.Dockerfile -t "quay.io/tomerfi/starburst-combined-catalog:latest" && \
podman push quay.io/tomerfi/starburst-combined-catalog:latest)
```

> Don't forget to make `starburst-combined-catalog` PUBLIC.
