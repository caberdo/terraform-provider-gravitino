# Built-in IDP group with members
resource "gravitino_idp_group" "engineers" {
  name  = "engineers"
  users = ["alice", "bob"]
}
