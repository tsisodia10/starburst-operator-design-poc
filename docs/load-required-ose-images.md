## Load OSE required images

For OSE to work, we need the following images to be accessible in our cluster.<br/>
For _Kind_, we can simply load them.

## Prerequisites

- [podman](https://podman.io/) or [docker](https://www.docker.com/)

## Load Images

```bash
podman pull registry.redhat.io/openshift4/ose-prometheus-operator:v4.10.0-202204090935.p0.g73ddd44.assembly.stream
```

```bash
podman save registry.redhat.io/openshift4/ose-prometheus-operator:v4.10.0-202204090935.p0.g73ddd44.assembly.stream -o operator
```

```bash
kind load image-archive operator --name starburst
```

```bash
podman pull registry.redhat.io/openshift4/ose-prometheus-alertmanager@sha256:5065c09b9da8cbb4cf0e582855f4f04a042d49c2b7947afa11a510bbae1e234e
```

```bash
podman save registry.redhat.io/openshift4/ose-prometheus-alertmanager@sha256:5065c09b9da8cbb4cf0e582855f4f04a042d49c2b7947afa11a510bbae1e234e -o alertmanager
```

```bash
kind load image-archive alertmanager --name starburst
```

```bash
podman pull registry.redhat.io/openshift4/ose-prometheus@sha256:348fd2cb790c30f642fd8e4bc9e6e6ed8ca5ec2b57489bfe4142e12c016268b8
```

```bash
podman save registry.redhat.io/openshift4/ose-prometheus@sha256:348fd2cb790c30f642fd8e4bc9e6e6ed8ca5ec2b57489bfe4142e12c016268b8 -o prometheus
```

```bash
kind load image-archive prometheus --name starburst
```

```bash
podman pull registry.redhat.io/openshift4/ose-prometheus-config-reloader@sha256:a501c4c9f5054175fc2a9ec97326b8f4409277ba463cb592b511847a8264688f

```

```bash
podman save registry.redhat.io/openshift4/ose-prometheus-config-reloader@sha256:a501c4c9f5054175fc2a9ec97326b8f4409277ba463cb592b511847a8264688f -o reloader
```

```bash
kind load image-archive reloader --name starburst
```

```bash
podman pull registry.redhat.io/openshift4/ose-prometheus-operator@sha256:370f2fa849f8045964e30c3a4d34be022f419d243b0cf37c2c81ea19faaab4a8
```

```bash
podman save registry.redhat.io/openshift4/ose-prometheus-operator@sha256:370f2fa849f8045964e30c3a4d34be022f419d243b0cf37c2c81ea19faaab4a8 -o ose
```

```bash
kind load image-archive ose --name starburst
```
