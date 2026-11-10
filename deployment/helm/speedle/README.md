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

`etcd` passes `--store-type etcd`, `--etcdstore-endpoint`, `--etcdstore-keyprefix`, and the `--etcdstore-tls-*` flags when `store.etcd.tls` is set. `mongodb` passes `--store-type mongodb` and `--mongostore_database`. A URI with no credentials is passed as `--mongostore_uri`. A URI that contains credentials is rejected: put it in a Secret and set `store.mongodb.existingSecret` (and `store.mongodb.secretKey` when the key is not `uri`). The chart then sets `SPDL_MONGOSTORE_URI` from that Secret and does not put the URI in the Pod spec. Do not set `uri` and `existingSecret` together. MongoDB must already be a replica set. This chart does not deploy etcd or MongoDB.

With etcd or MongoDB, ADS replicas can be scheduled on any node.

## Requests

The containers run as UID 65532 with a dropped capability set and a read-only root filesystem. The default Service type is ClusterIP. Listeners use plain HTTP until `tls.enabled` and a certificate Secret are set. When `tls.forceClientCert` is true, startup, readiness and liveness probes use TCP, because the kubelet cannot present a client certificate.

PMS is where policies are written. The chart installs a NetworkPolicy that denies ingress to PMS from other pods. kubelet probes and `kubectl port-forward` are not pod-network ingress, so an administrator can still reach PMS with port-forward. To let a workload call PMS, add a peer to `networkPolicy.pms.ingress`. Set `networkPolicy.pms.enabled` to `false` only when every workload that can route to the PMS Service is trusted to administer policy. ADS stays reachable so decision clients can call it. The file store accepts `ReadWriteOnce` or `ReadWriteMany`.

See `values.yaml` for replicas, resources, probes and store settings.
