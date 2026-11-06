# ovn-operator
// TODO(user): Add simple overview of use/purpose

## Description
// TODO(user): An in-depth paragraph about your project and overview of use

## Getting Started
You’ll need a Kubernetes cluster to run against. You can use [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.
**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

### Running on the cluster
1. Install Instances of Custom Resources:

```sh
kubectl apply -f config/samples/
```

2. Build and push your image to the location specified by `IMG`:

```sh
make docker-build docker-push IMG=<some-registry>/ovn-operator:tag
```

3. Deploy the controller to the cluster with the image specified by `IMG`:

```sh
make deploy IMG=<some-registry>/ovn-operator:tag
```

### Uninstall CRDs
To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller
UnDeploy the controller to the cluster:

```sh
make undeploy
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

### How it works
This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/)
which provides a reconcile function responsible for synchronizing resources untile the desired state is reached on the cluster

### Test It Out
1. Install the CRDs into the cluster:

```sh
make install
```

2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```sh
make run
```

**NOTE:** You can also run this in one step by running: `make install run`

### Modifying the API definitions
If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## OVN controller probe configuration

The `ovn-controller` container checks liveness and readiness every 5 seconds,
with a 5-second timeout and a 30-second initial delay. Three consecutive
liveness failures restart the container; readiness failures mark the Pod
unready. Liveness checks that the process exists and `debug/status` returns
`running`. Readiness checks that `connection-status` returns `connected`.
These commands use the controller's local control socket. Heavy flow computation
can delay their responses even when the controller is making progress.

Configure the container through `OVNController.spec.override.probes`. For example:

```yaml
apiVersion: ovn.openstack.org/v1beta1
kind: OVNController
metadata:
  name: ovncontroller
spec:
  override:
    probes:
      livenessProbes:
        timeoutSeconds: 30
```

The value above illustrates the API; select timings using measured probe latency
and the acceptable time to detect a persistent hang. Existing defaults are
unchanged when overrides are omitted. Increasing tolerance prevents premature
restarts but does not make flow computation faster. Readiness can be tuned
independently using `readinessProbes`.

The shared lib-common API supports `initialDelaySeconds`, `timeoutSeconds`,
`periodSeconds`, and `failureThreshold`, plus `type` (`Exec` or `HTTP`),
`command`, `path`, `port`, and `scheme`. Partial overrides inherit the defaults;
zero timing values use lib-common's inheritance semantics. Explicit zero timeout,
period, and failure threshold values are rejected by the CRD schema. Setting an
initial delay to zero retains the default delay. HTTP checks require a port and
a reachable endpoint supplied by the chosen image; the default controller does
not expose an HTTP health endpoint.

Startup probing is disabled by default. Providing `startupProbes` enables it,
starting with the default liveness command and timings. For example,
`startupProbes: {failureThreshold: 60}` tolerates a longer initialization.
Startup probing gates liveness and readiness until it succeeds; it does not
protect a controller that stalls after startup. Removing overrides restores
the defaults. Changes update the DaemonSet Pod template and cause a rolling
update. OVS and metrics container probes are unaffected.

For deployments managed by `OpenStackControlPlane`, configure the parent CR:

```yaml
spec:
  ovn:
    template:
      ovnController:
        override:
          probes:
            livenessProbes:
              timeoutSeconds: 30
```

This requires an openstack-operator release that includes the updated OVN API,
parent CRD, and bundled OVN CRD. Updating only ovn-operator does not add fields
to the parent CRD. Configure the parent rather than patching its child CR or
DaemonSet, which reconciliation manages.


## License

Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
