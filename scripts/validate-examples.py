#!/usr/bin/env python3
"""Validate the Terraform examples in this repository against the provider that is
built from the working tree.

Why this exists: tfplugindocs embeds ``examples/resources/<type>/resource.tf`` and
``examples/data-sources/<type>/data-source.tf`` verbatim in the generated Terraform
Registry documentation, so an example referencing a removed or renamed attribute ships
a broken snippet to users.

Two kinds of example directory exist:

* real modules (``examples/complete``, ``examples/provider``) — validated in place;
* fragments (the per-resource/per-data-source snippets) — they reference a metalake,
  catalog, schema or table that another snippet declares. Terraform stops checking a
  resource's own arguments as soon as one of its expressions cannot be resolved, so
  validating such a snippet on its own would hide "missing required argument" bugs.
  All fragments are therefore concatenated into a single synthetic module (duplicate
  declarations are dropped, keeping the first) so every reference resolves and every
  fragment is fully validated. Diagnostics are mapped back to the originating file.

Usage:
    scripts/validate-examples.py            # after building/installing the provider
    scripts/validate-examples.sh            # builds the provider, sets dev_overrides, runs this
"""

from __future__ import annotations

import json
import os
import pathlib
import re
import subprocess
import sys
import tempfile

REPO = pathlib.Path(__file__).resolve().parent.parent
EXAMPLES = REPO / "examples"

# Directories that are complete Terraform modules and must be validated as they are.
STANDALONE_MODULES = ("examples/complete", "examples/provider")

# Minimal bodies for parents that a fragment references but no fragment declares (for
# example the Kafka catalog the topic snippet expects the reader to create). They only
# exist so that every reference resolves and every fragment is fully validated.
STUB_BODIES = {
    "gravitino_metalake": '  name = "{name}"',
    "gravitino_catalog": (
        '  metalake         = gravitino_metalake.example.name\n'
        '  name             = "{name}"\n'
        '  type             = "relational"\n'
        '  catalog_provider = "hive"'
    ),
    "gravitino_schema": (
        '  metalake = gravitino_metalake.example.name\n'
        '  catalog  = gravitino_catalog.example.name\n'
        '  name     = "{name}"'
    ),
    "gravitino_table": (
        '  metalake = gravitino_metalake.example.name\n'
        '  catalog  = gravitino_catalog.example.name\n'
        '  schema   = gravitino_schema.example.name\n'
        '  name     = "{name}"\n\n'
        '  column {\n    name = "id"\n    type = "long"\n  }'
    ),
    "gravitino_fileset": (
        '  metalake         = gravitino_metalake.example.name\n'
        '  catalog          = gravitino_catalog.example.name\n'
        '  schema           = gravitino_schema.example.name\n'
        '  name             = "{name}"\n'
        '  type             = "managed"\n'
        '  storage_location = "file:/tmp/{name}"'
    ),
}

REFERENCE_RE = re.compile(r"\b(gravitino_[a-z_]+)\.([a-z_][a-z0-9_]*)\.", re.IGNORECASE)
BLOCK_RE = re.compile(
    r"^(?P<indent>\s*)(?P<kind>resource|data|output|variable|module|locals)\s+"
    r'(?:"(?P<type>[^"]+)"\s+)?"(?P<name>[^"]+)"\s*\{',
    re.MULTILINE,
)


def terraform_validate(directory: pathlib.Path) -> dict:
    result = subprocess.run(
        ["terraform", "validate", "-json", "-no-color"],
        cwd=directory,
        capture_output=True,
        text=True,
        env=os.environ,
    )
    try:
        return json.loads(result.stdout or "{}")
    except json.JSONDecodeError:
        return {"diagnostics": [{"severity": "error", "summary": "terraform validate failed",
                                 "detail": (result.stderr or result.stdout or "").strip()[:2000]}]}


def split_blocks(text: str, rel: str):
    """Yield (key, source_rel, start_line, block_text) for every resource/data/output/
    variable/module/locals block in text."""
    for match in BLOCK_RE.finditer(text):
        depth = 0
        index = match.end() - 1  # position of the opening brace
        in_string = False
        in_comment = False
        while index < len(text):
            char = text[index]
            if in_comment:
                if char == "\n":
                    in_comment = False
            elif in_string:
                if char == "\\":
                    index += 1
                elif char == '"':
                    in_string = False
            elif char == "#":
                in_comment = True
            elif char == '"':
                in_string = True
            elif char == "{":
                depth += 1
            elif char == "}":
                depth -= 1
                if depth == 0:
                    break
            index += 1

        block = text[match.start():index + 1]
        start_line = text.count("\n", 0, match.start()) + 1
        key = (match.group("kind"), match.group("type") or "", match.group("name"))
        yield key, rel, start_line, block


def fragment_files():
    for path in sorted(EXAMPLES.rglob("*.tf")):
        rel = path.relative_to(REPO).as_posix()
        if any(rel == mod or rel.startswith(mod + "/") for mod in STANDALONE_MODULES):
            continue
        yield path, rel


def build_union(workdir: pathlib.Path):
    """Write every fragment into one module and return the line -> source map."""
    seen = set()
    out_lines: list[str] = []
    line_map: dict[int, tuple[str, int]] = {}
    skipped = 0
    references = set()
    stub_prefix = len(out_lines)

    for path, rel in fragment_files():
        text = path.read_text()
        for ref in REFERENCE_RE.finditer(text):
            references.add((ref.group(1), ref.group(2)))
        for key, source, start_line, block in split_blocks(text, rel):
            if key in seen:
                skipped += 1
                continue
            seen.add(key)

            out_lines.append(f"# >>> {source}:{start_line}")
            line_map[len(out_lines)] = (source, start_line - 1)
            for offset, block_line in enumerate(block.splitlines()):
                if offset:  # first line is covered by the marker above
                    line_map[len(out_lines) + 1] = (source, start_line + offset)
                out_lines.append(block_line)
            out_lines.append("")

    declared = {(key[1], key[2]) for key in seen if key[0] in ("resource", "data")}
    declared |= {(key[1], key[2]) for key in seen if key[0] == "data"}

    stubs = 0
    for type_name, label in sorted(references - declared):
        body = STUB_BODIES.get(type_name)
        if body is None:
            out_lines.append(f"# MISSING STUB for {type_name}.{label}")
            continue
        out_lines.append(f"# stub for the referenced {type_name}.{label}")
        out_lines.append(f'resource "{type_name}" "{label}" {{')
        out_lines.extend(body.format(name=label).splitlines())
        out_lines.append("}")
        out_lines.append("")
        stubs += 1

    (workdir / "zz_fragments.tf").write_text("\n".join(out_lines) + "\n")
    return line_map, len(seen), skipped, stubs


def format_diagnostic(diag: dict, line_map: dict[int, tuple[str, int]] | None) -> str:
    summary = diag.get("summary", "error")
    detail = (diag.get("detail") or "").strip().splitlines()
    message = detail[0] if detail else ""
    location = ""
    rng = diag.get("range") or {}
    if rng.get("filename"):
        line = (rng.get("start") or {}).get("line")
        filename = pathlib.Path(rng["filename"]).name
        if line_map is not None:
            source, source_line = line_map.get(line, (filename, line or 0))
            location = f"  at {source}:{source_line}"
        elif line:
            location = f"  at {filename}:{line}"
    return f"    {summary}: {message}{location}"


def main() -> int:
    failures = 0

    for rel in STANDALONE_MODULES:
        directory = REPO / rel
        if not directory.is_dir():
            continue
        report = terraform_validate(directory)
        errors = [d for d in report.get("diagnostics", []) if d.get("severity") == "error"]
        print(f"==> {rel} (module)")
        if not errors:
            print("    ok")
            continue
        failures += 1
        for diag in errors:
            print(format_diagnostic(diag, None))

    with tempfile.TemporaryDirectory(prefix="gravitino-examples-") as tmp:
        workdir = pathlib.Path(tmp)
        line_map, kept, skipped, stubs = build_union(workdir)
        report = terraform_validate(workdir)
        errors = [d for d in report.get("diagnostics", []) if d.get("severity") == "error"]

        print(f"==> examples/**/*.tf fragments ({kept} declarations, {skipped} duplicate(s) dropped, "
              f"{stubs} stub(s) for external references)")
        if errors:
            failures += 1
            for diag in errors:
                print(format_diagnostic(diag, line_map))
        else:
            print("    ok")

    if failures:
        print(f"example validation failed for {failures} module(s)", file=sys.stderr)
        return 1

    print("all examples validated")
    return 0


if __name__ == "__main__":
    sys.exit(main())
