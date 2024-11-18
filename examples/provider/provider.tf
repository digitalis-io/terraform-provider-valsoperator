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
