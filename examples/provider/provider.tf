terraform {
  required_providers {
    valsoperator = {
      source = "digitalis-io/valsoperator"
    }
  }
}

provider "valsoperator" {
  config_paths = [
    "/Users/sergio.rua/.kube/saas-dev.yml"
  ]
}

data "valsoperator_secret" "example_secret" {
  name = "ceph-bucket"
  namespace = "default"
}

data "valsoperator_valssecret" "example_vals_secret" {
  name = "cassandra-tls"
  namespace = "cst-axonopsdev"
}

output "example_secret" {
  value = data.valsoperator_secret.example_secret
}

output "example_vals_secret" {
  value = data.valsoperator_valssecret.example_vals_secret
}