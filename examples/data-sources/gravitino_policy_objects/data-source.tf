# List the metadata objects a policy is directly attached to.
data "gravitino_policy_objects" "data_retention" {
  metalake = "example_metalake"
  policy   = "data_retention"
}
