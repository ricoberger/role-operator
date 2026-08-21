# Role Operator

The Role Operator can be used to create Roles, RoleBindings, ClusterRoles and
ClusterRoleBindings for a subject. This is useful to manage permissions for a
group or user, where the permissions are defined in a single Role resource and
applied to multiple namespaces.

## Installation

The Role Operator can be installed via Helm:

```sh
helm upgrade --install role-operator oci://ghcr.io/ricoberger/charts/role-operator --version <VERSION>
```

## API Reference

### Role

```yaml
apiVersion: ricoberger.de/v1alpha1
kind: Role
metadata:
  name:
  namespace:
spec:
  # A list of subjects (users, groups, or service accounts) that will be granted
  # the permissions defined in the `roleRules` and `clusterRoleRules`.
  subjects:
  # A list of namespaces where Roles and RoleBindings will be created, with the
  # permissions defined in the `roleRules`.
  namespaces:
  # A list of rules to be applied to Roles created in the namespaces defined in
  # the `namespaces`.
  roleRules:
  # A list of rules to be applied to the ClusterRole created.
  clusterRoleRules:
```

<details>
<summary>Example</summary>

```yaml
apiVersion: ricoberger.de/v1alpha1
kind: Role
metadata:
  name: mygroup
  namespace: default
spec:
  subjects:
    - kind: Group
      name: mygroup
      apiGroup: rbac.authorization.k8s.io
  namespaces:
    - default
    - kube-system
  roleRules:
    - apiGroups:
        - "*"
      resources:
        - "*"
      verbs:
        - get
        - list
        - watch
        - create
        - update
        - patch
        - delete
  clusterRoleRules:
    - apiGroups:
        - "*"
      resources:
        - "*"
      verbs:
        - get
        - list
        - watch
```

</details>

## Development

After modifying the `*_types.go` file always run the following command to update
the generated code for that resource type:

```sh
make generate
```

The above Makefile target will invoke the
[controller-gen](https://sigs.k8s.io/controller-tools) utility to update the
`api/v1alpha1/zz_generated.deepcopy.go` file to ensure our API's Go type
definitons implement the `runtime.Object` interface that all Kind types must
implement.

Once the API is defined with spec/status fields and CRD validation markers, the
CRD manifests can be generated and updated with the following command:

```sh
make manifests
```

This Makefile target will invoke controller-gen to generate the CRD manifests at
`charts/role-operator/crds/ricoberger.de_roles.yaml`.

Deploy the CRD and run the operator locally with the default Kubernetes config
file present at `$HOME/.kube/config`:

```sh
make run
```
