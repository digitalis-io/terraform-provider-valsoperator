resource "valsoperator_dbsecret" "example" {
  name      = "example"
  namespace = "default"

  vault_role  = "role"
  vault_mount = "cass000"

  rollout {
    name = "my-app"
    kind = "Deployment"
  }

  template {
    name  = "CASSANDRA_USERNAME"
    value = "{{ .username }}"
  }
  template {
    name  = "CASSANDRA_PASSWORD"
    value = "{{ .password }}"
  }
}