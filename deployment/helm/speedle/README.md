# Speedle

Helm chart for the Speedle Policy Management Service (PMS) and Authorization Decision Service (ADS). Both Deployments use the `speedle/speedle` image. The container command is `speedle-pms` or `speedle-ads`.

PMS listens on port 6733. ADS listens on port 6734.

## Install

From a checkout of this repository:

```bash
helm install speedle deployment/helm/speedle
```

A release that contains this chart also pushes it to `oci://ghcr.io/teramoby/charts/speedle`. The chart version is the image version (`vX.Y.Z` is chart version `X.Y.Z`):

```bash
helm install speedle oci://ghcr.io/teramoby/charts/speedle --version X.Y.Z
```

Helm 3.8 or newer is required for an OCI install.

## Policy store

`store.type` selects the store. The default is `file`: PMS and ADS mount one PersistentVolumeClaim and ADS pods are scheduled onto the PMS node. The file store keeps a single PMS replica. Raise `ads.replicaCount` to scale ADS on that node.

`etcd` passes `--store-type etcd`, `--etcdstore-endpoint`, `--etcdstore-keyprefix`, and the `--etcdstore-tls-*` flags when `store.etcd.tls` is set. `mongodb` passes `--store-type mongodb`, `--mongostore_uri` and `--mongostore_database`. MongoDB must already be a replica set. This chart does not deploy etcd or MongoDB.

With etcd or MongoDB, ADS replicas can be scheduled on any node.

## Requests

The containers run as UID 65532 with a dropped capability set and a read-only root filesystem. The default Service type is ClusterIP and the listeners use plain HTTP. Set `tls.enabled` and a certificate Secret before exposing the Services beyond the cluster.

See `values.yaml` for replicas, resources, probes and store settings.
