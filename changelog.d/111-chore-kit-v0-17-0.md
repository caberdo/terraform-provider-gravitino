The CI, release and coverage tooling moves to `caberdo/terraform-provider-kit` at `v0.17.0`.
Its weekly maintenance check no longer asks the supported-versions matrix for stable releases below
the declared support floor, so a young support window no longer produces a version-matrix issue that
cannot be fixed without testing releases the provider does not support.
