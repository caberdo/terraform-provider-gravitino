# Built-in IDP user for local authentication
resource "gravitino_idp_user" "alice" {
  name     = "alice"
  password = "Passw0rd-Alice12"
}

resource "gravitino_idp_user" "bob" {
  name     = "bob"
  password = "Passw0rd-Bob1234"
}
