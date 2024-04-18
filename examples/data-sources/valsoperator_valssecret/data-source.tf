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

data "valsoperator_valssecret" "example" {
  name      = "example"
  namespace = "default"
}

output "example" {
  value = data.valsoperator_valssecret.example
}
