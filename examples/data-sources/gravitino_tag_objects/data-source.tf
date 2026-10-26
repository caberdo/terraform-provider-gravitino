# List the metadata objects a tag is directly attached to.
data "gravitino_tag_objects" "pii" {
  metalake = "example_metalake"
  tag      = "PII"
}
