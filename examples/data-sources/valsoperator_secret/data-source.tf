terraform {
  required_providers {
    valsoperator = {
      source = "digitalis-io/valsoperator"
    }
  }
}

provider "valsoperator" {
  config_paths = [
    "~/.kube/config"
  ]
}

data "valsoperator_secret" "example_secret" {
  name      = "example"
  namespace = "default"
}

output "example_secret" {
  value = data.valsoperator_secret.example_secret
}
