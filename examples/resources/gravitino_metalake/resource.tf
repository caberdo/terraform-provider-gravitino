resource "gravitino_metalake" "minimal" {
  name = "minimal_metalake"
}

resource "gravitino_metalake" "example" {
  name          = "example_metalake"
  comment       = "Example metalake for development"
  in_use        = true
  force_destroy = false
  properties = {
    env = "dev"
  }
}
