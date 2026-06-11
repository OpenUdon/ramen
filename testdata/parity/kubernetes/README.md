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

## Recorded Update Lanes

K09, K10, and K13 record update-specific API-visible parity for
`kubernetes_role_binding_v1`, `kubernetes_cluster_role_v1`, and
`kubernetes_cluster_role_binding_v1`. Their committed artifacts prove
create/update/read/no-op/destroy behavior across OpenTofu, Terraform, and
Ramen+udon before the public mapping registry advertises the corresponding
replace operation.

## Recorded ClusterRoleBinding Lane

K12 records `kubernetes_cluster_role_binding_v1` create/read/no-op/destroy
parity across OpenTofu, Terraform, and Ramen+udon. The public mapping registry
advertises create/read/delete from K12 evidence, while K13 separately records
and gates the update mapping.
