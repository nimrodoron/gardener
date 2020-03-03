# Extending project roles

The `Project` resource allows to specify a list of roles for every member (`.spec.members[*].roles`).
There are a few standard roles defined by Gardener itself:

* `owner` (describes the owner/main contact of the project (as of today only one owner can be specified))
* `admin` (describes administrators with full read/write access to all resources concerning the project)
* `viewer` (describes members with limited read access to some resources concerning the project)

However, extension controllers running in the garden cluster may also create `CustomResourceDefinition`s that project members might be able to CRUD.
For this purpose Gardener also allows to specify extension roles.

An extension role is prefixed with `extension:`, e.g.

```yaml
apiVersion: core.gardener.cloud/v1beta1
kind: Project
metadata:
  name: dev
spec:
  members:
  - apiGroup: rbac.authorization.k8s.io
    kind: User
    name: alice.doe@example.com
    roles:
    - "admin"
    - "owner"
    - "extension:foo"
```

The project controller will, for every extension role, create a `ClusterRole` with name `name: gardener.cloud:extension:project:<projectName>:<roleName>`, i.e., for above example: `name: gardener.cloud:extension:project:dev:foo`.
This `ClusterRole` aggregates other `ClusterRole`s that are labeled with `rbac.gardener.cloud/aggregate-to-extension-role=foo` which might be created by extension controllers.

Extension that might want to contribute to the core `admin` or `viewer` roles can use the labels `rbac.gardener.cloud/aggregate-to-project-member=true` or `rbac.gardener.cloud/aggregate-to-project-viewer=true`, respectively.

Please note that the names of the extension roles are restricted to 20 characters!

Moreover, the project controller will also create a corresponding `RoleBinding` with the same name in the project namespace.
It will automatically assign all members that are assigned to this extension role.
