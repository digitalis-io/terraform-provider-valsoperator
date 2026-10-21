# Changelog

## Unreleased

### Breaking

- `valsoperator_valssecret` data source: the `ttl` attribute and the `data`
  nested attributes are now read-only (computed), and the `template` block
  field `key` has been renamed to `name` to match the resource. Configurations
  that set `ttl` or read `template[*].key` on this data source must be updated.

### Added

- Resource import for `valsoperator_valssecret` and `valsoperator_dbsecret`
  using a `namespace/name` identifier.

### Fixed

- Kubernetes clients are now cached correctly instead of being rebuilt on every
  call.
- `ignore_labels` is applied to labels instead of annotations.
- Resources are removed from state when the underlying object is deleted out of
  band, and `Read` now refreshes nested spec blocks so drift is detected.
- The `valsoperator_secret` data source now returns the secret `data` and
  `binary_data`, both marked sensitive.
- `Create` fails when the object already exists and points to `terraform
  import`; `Update` no longer clobbers externally managed metadata.
- Custom resource contents are no longer written to logs outside debug.
