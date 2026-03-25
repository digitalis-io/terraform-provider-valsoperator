resource "valsoperator_dbsecret" "example" {
  name      = "cassandra"
  namespace = "default"

  vault {
    role  = "application_role"
    mount = "database"
  }

  template {
    name  = "CASSANDRA_USERNAME"
    value = "{{ .username }}"
  }

  template {
    name  = "CASSANDRA_PASSWORD"
    value = "{{ .password }}"
  }

  rollout {
    kind = "Deployment"
    name = "my-cass-client"
  }

  renew = true
}
