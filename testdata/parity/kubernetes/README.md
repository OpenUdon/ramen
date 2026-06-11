# Kubernetes Provider Parity Fixtures

This tree is reserved for the Kxx Kubernetes provider/runtime parity lane.
It is separate from `testdata/equivalence/kubernetes`, which records
Ramen-only live/replay executor evidence from M21 and M22.

K-lane parity compares Kubernetes API-visible observations from:

- OpenTofu with `hashicorp/kubernetes` pinned to `3.1.0`;
- Terraform with `hashicorp/kubernetes` pinned to `3.1.0`;
- Ramen with Kubernetes OpenAPI 2.0 source metadata and the `udon` executor.

Default tests only validate committed normalized observation metadata. Live
parity recording is opt-in through `RAMEN_K8S_PARITY=1`, must use a `kind-*`
kubectl context, and updates committed observations only when
`RAMEN_K8S_PARITY_RECORD_UPDATE=1` is also set.

The planning baseline uses the local source artifact:

```text
../apitools/catalog-openapi-cache/openapi/kubernetes-v1-19-2-swagger.json
```

Committed focused OpenAPI subsets should be added under this tree before a K
scenario moves from `planned` to `recorded`, so default tests never depend on
the sibling `../apitools` checkout.

## Parked Updates

RoleBinding and ClusterRole update are intentionally unsupported in the public
mapping registry. K07 and K08 recorded create/read/no-op/destroy behavior only;
they did not record update-specific API-visible parity. A future K-lane must
add focused update fixtures, committed replay or sanitized live observations,
and mapping evidence before `kubernetes_role_binding_v1` or
`kubernetes_cluster_role_v1` update can be advertised.
