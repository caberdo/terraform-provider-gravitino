The repository now runs a **weekly maintenance pipeline**: it reports which upstream release the
supported-versions matrix does not cover, runs the acceptance suite against that release as a canary so a
version bump can be judged as a matrix edit or a code change, and reports when the repository is behind
the shared CI kit or pins two different kit versions. Findings appear as one issue per category and
close themselves when resolved; nothing is bumped automatically.
