![build](https://github.com/opster/opensearch-k8s-operator/actions/workflows/docker-build.yaml/badge.svg) ![test](https://github.com/opster/opensearch-k8s-operator/actions/workflows/testing.yaml/badge.svg) ![release](https://img.shields.io/github/v/release/opster/opensearch-k8s-operator)

# OpenSearch-k8s-operator

The Kubernetes OpenSearch Operator is used for automating the deployment, provisioning, management, and orchestration of OpenSearch clusters and OpenSearch dashboards.

## Getting started

The operator can be installed easily using helm on any CNCF-certified kubernetes cluster. Please refer to the [User Guide](./docs/userguide/main.md) for installation instructions.

## Roadmap

The full roadmap is available in the [Development plan](./docs/designs/dev-plan.md)

Currently planned features:

- [x] Deploy a new OS cluster.
- [x] Ability to deploy multiple clusters.
- [x] Spin up OS dashboards.
- [x] Configuration of all node roles (master, data, coordinating..).
- [x] Scale the cluster resources (manually), per nodes' role group.
- [x] Drain strategy for scale down.
- [x] Version updates.
- [x] Change nodes' memory allocation and limits.
- [x] Secured installation features.
- [x] Certificate management.
- [x] Rolling restarts - through API.
- [ ] Scaling nodes' disks - increase/replace disks.
- [ ] Cluster configurations and nodes' settings updates.
- [ ] Auto scaler based on usage, load, and resources.
- [ ] Operator Monitoring, with Prometheus and Grafana.
- [ ] Control shard balancing and allocation: AZ/Rack awareness, Hot/Warm.

## Development

### Running the Operator locally

- Clone the repo and go to the `opensearch-operator` folder.
- Run `make build manifests` to build the controller binary and the manifests
- Start a kubernetes cluster (e.g. with k3d or minikube) and make sure your `~/.kube/config` points to it
- Run `make install` to create the CRD in the kubernetes cluster
- Start the operator by running `make run`

Now you can deploy an opensearch cluster.

Go to `opensearch-operator` and use `opensearch-cluster.yaml` as a starting point to define your cluster. Then run:

```bash
kubectl apply -f opensearch-cluster.yaml
```

In order to delete the cluster, you just delete your OpenSearch cluster resource. This will delete the cluster and all its resources.

```bash
kubectl delete -f opensearch-cluster.yaml
```

## Contributions

We welcome contributions! See how you can get involved by reading [CONTRIBUTING.md](./CONTRIBUTING.md).
