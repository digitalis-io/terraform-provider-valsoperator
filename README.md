# Terraform Provider ValsOperator

This provider can be use to deploy ValsSecrets to a Kubernetes cluster running the [vals-operator](https://github.com/digitalis-io/vals-operator)

## Example Usage

```terraform
# Example of a ValsSecret that reads a user and password from HashiCorp Vault
resource "valsoperator_valssecret" "example" {
  name      = "example"
  namespace = "default"

  secret_ref {
    name     = "username"
    ref      = "ref+vault://secret/myapp/username"
    encoding = "text"
  }

  secret_ref {
    name     = "password"
    ref      = "ref+vault://secret/myapp/password"
    encoding = "text"
  }

  template {
    name  = "config.yaml"
    value = <<END
# Config generated by Vals-Operator on {{ now | date "2006-01-02" }}
username: {{.username}}
password: {{.password}}
END
  }
}
```