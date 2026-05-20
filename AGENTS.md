# AGENTS.md - ovn-operator

## Project overview

ovn-operator is a Kubernetes operator that manages
[OVN](https://www.ovn.org/) (Open Virtual Network: distributed virtual
networking for OpenStack including northd, OVN DB clusters, and OVN
controllers) on OpenShift/Kubernetes. It is part of the
[openstack-k8s-operators](https://github.com/openstack-k8s-operators) project.

Key OVN domain concepts: **OVN Northd** (central daemon translating logical
to physical flows), **OVN DB clusters** (northbound and southbound OVSDB
clusters), **OVN Controller** (per-node agent managing local OVS flows),
**OVS external IDs** (Open vSwitch configuration), **bonds** (NIC bonding).

## Tech stack

| Layer | Technology |
|-------|------------|
| Language | Go (modules, multi-module workspace via `go.work` - local only, not committed) |
| Scaffolding | [Kubebuilder v4](https://book.kubebuilder.io/) + [Operator SDK](https://sdk.operatorframework.io/) |
| CRD generation | controller-gen (DeepCopy, CRDs, RBAC, webhooks) |
| Config management | Kustomize |
| Packaging | OLM bundle |
| Testing | Ginkgo/Gomega + envtest (functional), KUTTL (integration) |
| Linting | golangci-lint (`.golangci.yaml`) |
| CI | Zuul (`zuul.d/`), Prow (`.ci-operator.yaml`), GitHub Actions |

## Custom Resources

| Kind | Purpose |
|------|---------|
| `OVNNorthd` | Manages the OVN Northd deployment (logical-to-physical flow translation). |
| `OVNDBCluster` | Manages OVN database clusters (northbound and southbound OVSDB). |
| `OVNController` | Manages the OVN Controller, OVS, and metrics daemonsets. |

The CRs have defaulting and validating admission webhooks.

## Directory structure

**Maintenance rule:** when directories are added, removed, or renamed, or when
their purpose changes, update this table to match.

| Directory | Contents |
|-----------|----------|
| `api/v1beta1/` | CRD types (`ovnnorthd_types.go`, `ovndbcluster_types.go`, `ovncontroller_types.go`), conditions, webhook markers |
| `api/test/` | Test helpers (`TestHelper`) for use by other operators' tests |
| `cmd/` | `main.go` entry point |
| `internal/controller/` | Reconcilers: `ovnnorthd_controller.go`, `ovndbcluster_controller.go`, `ovncontroller_controller.go` |
| `internal/ovnnorthd/` | OVNNorthd resource builders |
| `internal/ovndbcluster/` | OVNDBCluster resource builders |
| `internal/ovncontroller/` | OVNController resource builders |
| `internal/common/` | Shared helper functions |
| `internal/webhook/` | Webhook implementation |
| `templates/` | Config files and scripts mounted into pods via `OPERATOR_TEMPLATES` env var. Subdirs: `ovncontroller/`, `ovndbcluster/`, `ovnnorthd/` |
| `config/crd,rbac,manager,webhook/` | Generated Kubernetes manifests (CRDs, RBAC, deployment, webhooks) |
| `config/samples/` | Example CRs (Kustomize overlays). Includes NAD (NetworkAttachmentDefinition) samples. |
| `test/functional/` | envtest-based Ginkgo/Gomega tests |
| `test/kuttl/` | KUTTL integration tests |
| `hack/` | Helper scripts (CRD schema checker, local webhook runner) |

## Build commands

After modifying Go code, always run: `make generate manifests fmt vet`.

Always run `pre-commit run --all-files` before committing. Hooks are
configured in `.pre-commit-config.yaml` and run `make tidy`,
`make manifests`, `make generate`, `make bundle`, `make operator-lint`,
`make crd-schema-check`, golangci-lint, bashate, and standard file checks
(trailing whitespace, YAML/JSON validation, etc.).

## Code style guidelines

- Follow standard openstack-k8s-operators conventions and lib-common patterns.
- Use `lib-common` modules for conditions, endpoints, TLS, storage, and other
  cross-cutting concerns rather than re-implementing them.
- CRD types go in `api/v1beta1/`. Controller logic goes in
  `internal/controller/`. Resource-building helpers go in `internal/ovn*`
  packages matching the CR they support.
- Config templates are plain files in `templates/` -- they are mounted at
  runtime via the `OPERATOR_TEMPLATES` environment variable.
- Webhook logic is split between the kubebuilder markers in `api/v1beta1/` and
  the implementation in `internal/webhook/`.
- Test helpers in `api/test/` are consumed by other operators for their
  functional tests.

## Testing

- Functional tests use the envtest framework with Ginkgo/Gomega and live in
  `test/functional/`.
- KUTTL integration tests live in `test/kuttl/`.
- Run all functional tests: `make test`.
- When adding a new field or feature, add corresponding test cases in
  `test/functional/` and update fixture data accordingly.

## Key dependencies

- [lib-common](https://github.com/openstack-k8s-operators/lib-common): shared modules for conditions, endpoints, TLS, secrets, etc.
- [infra-operator](https://github.com/openstack-k8s-operators/infra-operator): topology APIs.
- [dev-docs/developer.md](https://github.com/openstack-k8s-operators/dev-docs/blob/main/developer.md): developer guide and coding conventions.
