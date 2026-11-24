The provider now supports Gravitino `1.1.1`, `1.2.0`, `1.2.1`, `1.3.0` and `1.3.1` from one
build. Resources and data sources that a later release introduced fail on an older server with an
explicit diagnostic naming the required version instead of the server's opaque `404`/`400`:
`gravitino_function` requires `1.2.0`; `gravitino_view`, `gravitino_idp_user`,
`gravitino_idp_group`, `gravitino_principal` and the health data sources require `1.3.0`; a
`gravitino_role` privilege update requires `1.2.0`; and group owners plus the `FUNCTION` role
object type and function privileges require `1.3.0`. The acceptance matrix runs against every
supported release. Apache published no final `1.2.1` image, so the `1.2.1` cell aliases the
published `1.2.1-rc2` image, which reports version `1.2.1`.
