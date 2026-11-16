#!/usr/bin/env python3
"""Prepare and verify the versioned files included in a release commit."""

import argparse
import datetime
import json
import os
import re
import sys
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
IMAGE = "ghcr.io/open-mcp-ai/termcp"
# Releases are vX.Y.Z, or vX.Y.Z-rc.N for a pre-release ("rc" means an internal
# test build: binaries and an image, no registry entry).
VERSION = re.compile(
    r"v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-rc\.(0|[1-9][0-9]*))?\Z"
)
RC = re.compile(r"-rc\.(0|[1-9][0-9]*)\Z")
UNRELEASED = re.compile(r"^## Unreleased\s*$", re.MULTILINE)
RELEASE = re.compile(r"^## v(\d+\.\d+\.\d+(?:-rc\.\d+)?) — (\d{4}-\d{2}-\d{2})\s*$", re.MULTILINE)
# A pre-release heading does not name what server.json must hold (see validate),
# so the newest FINAL heading is a separate query.
FINAL_RELEASE = re.compile(r"^## v(\d+\.\d+\.\d+) — \d{4}-\d{2}-\d{2}\s*$", re.MULTILINE)
RELEASE_BRANCH = re.compile(r"^release/v(\d+\.\d+\.\d+(?:-rc\.\d+)?)$")
RELEASE_TITLE = re.compile(r"^release: v(\d+\.\d+\.\d+(?:-rc\.\d+)?)$", re.IGNORECASE)


def is_prerelease(version):
    return RC.search(version) is not None


def normalized_version(value):
    match = VERSION.fullmatch(value)
    if not match:
        raise ValueError(
            f"invalid release version {value!r}; use vMAJOR.MINOR.PATCH or vMAJOR.MINOR.PATCH-rc.N"
        )
    base = ".".join(match.group(1, 2, 3))
    return f"{base}-rc.{match.group(4)}" if match.group(4) else base


def base_version(version):
    """The release a version belongs to: 0.2.8-rc.1 and 0.2.8 are both 0.2.8."""
    return version.partition("-rc.")[0]


def version_order(version):
    """Sort key: a final release outranks any rc of the same base version."""
    _, _, rc = version.partition("-rc.")
    return (*map(int, base_version(version).split(".")), (0, int(rc)) if rc else (1, 0))


def newest_final(text):
    match = FINAL_RELEASE.search(text)
    return match.group(1) if match else None


def changelog_parts(text):
    unreleased = UNRELEASED.search(text)
    release = RELEASE.search(text)
    if not release or (unreleased and unreleased.end() >= release.start()):
        raise ValueError("CHANGELOG.md needs a dated release heading after any Unreleased section")
    next_heading = re.search(r"^## ", text[release.end():], re.MULTILINE)
    release_end = release.end() + next_heading.start() if next_heading else len(text)
    unreleased_body = text[unreleased.end():release.start()] if unreleased else ""
    return unreleased, release, unreleased_body, text[release.end():release_end]


def validate(text, manifest, expected=None):
    _, release, unreleased_body, release_body = changelog_parts(text)
    version = release.group(1)
    try:
        datetime.date.fromisoformat(release.group(2))
    except ValueError as exc:
        raise ValueError(f"CHANGELOG.md v{version} has an invalid date") from exc
    # An rc is an internal test build: it is never published to the registry, so
    # server.json keeps naming the newest FINAL release and an rc leaves it
    # alone. Requiring that here (rather than tolerating a bumped manifest) is
    # what keeps this script and TestServerJSONMatchesChangelog from disagreeing
    # about what an rc heading means.
    published = newest_final(text) if is_prerelease(version) else version
    if not published:
        raise ValueError("CHANGELOG.md has no final vX.Y.Z release for server.json to name")
    if manifest.get("version") != published:
        raised = f" (v{version} is a pre-release and must not move server.json)" if published != version else ""
        raise ValueError(f"server.json version {manifest.get('version')!r} != {published}{raised}")
    packages = manifest.get("packages")
    if not isinstance(packages, list) or not packages:
        raise ValueError("server.json needs at least one package")
    identifier = packages[0].get("identifier") if isinstance(packages[0], dict) else None
    if identifier != f"{IMAGE}:{published}":
        raise ValueError(f"server.json package identifier {identifier!r} != {IMAGE}:{published}")
    if not re.search(r"^\s*-\s+\S", release_body, re.MULTILINE):
        raise ValueError(f"CHANGELOG.md v{version} needs at least one release note")
    if expected is not None:
        expected = normalized_version(expected)
        # The branch/PR signal and the tag are the same rule: the name must be
        # the version the changelog describes, exactly. An rc names itself
        # (`release/v0.2.8-rc.1`) and the final release names itself
        # (`release/v0.2.8`) - so the name on the PR always says which build is
        # being cut, and neither can be waved through by the other.
        if version != expected:
            raise ValueError(f"release v{expected} requested, but files describe v{version}")
        if unreleased_body.strip():
            raise ValueError("move Unreleased notes into the release section before tagging")
    return version


def expected_from_pr():
    branch = os.environ.get("RELEASE_PR_HEAD_REF", "")
    title = os.environ.get("RELEASE_PR_TITLE", "")
    branch_match = RELEASE_BRANCH.fullmatch(branch)
    title_match = RELEASE_TITLE.fullmatch(title)
    if branch.startswith("release/") and not branch_match:
        raise ValueError(f"release branch {branch!r} must be release/vMAJOR.MINOR.PATCH")
    if title.lower().startswith("release:") and not title_match:
        raise ValueError(f"release PR title {title!r} must be 'release: vMAJOR.MINOR.PATCH'")
    branch_version = branch_match.group(1) if branch_match else None
    title_version = title_match.group(1) if title_match else None
    if branch_version and title_version and branch_version != title_version:
        raise ValueError(f"release PR branch v{branch_version} and title v{title_version} disagree")
    return branch_version or title_version


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, default=ROOT, help=argparse.SUPPRESS)
    commands = parser.add_subparsers(dest="command", required=True)
    check = commands.add_parser("check", help="verify server.json and CHANGELOG.md")
    check.add_argument("--expected", help="release version, e.g. v0.2.5 or v0.2.8-rc.1")
    prepare = commands.add_parser("prepare", help="move Unreleased notes into a dated release")
    prepare.add_argument("version", help="release tag, e.g. v0.2.5")
    prepare.add_argument("--date", default=datetime.datetime.now(datetime.timezone.utc).date().isoformat())
    args = parser.parse_args()

    changelog_path = args.root / "CHANGELOG.md"
    manifest_path = args.root / "server.json"
    text = changelog_path.read_text(encoding="utf-8")
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))

    if args.command == "prepare":
        version = normalized_version(args.version)
        try:
            datetime.date.fromisoformat(args.date)
        except ValueError as exc:
            raise ValueError("--date must be YYYY-MM-DD") from exc
        unreleased, release, unreleased_body, _ = changelog_parts(text)
        if unreleased is None:
            raise ValueError("add an Unreleased section with notes before preparing a release")
        if not re.search(r"^\s*-\s+\S", unreleased_body, re.MULTILINE):
            raise ValueError("Unreleased has no notes to move into the new release")
        if version_order(version) <= version_order(release.group(1)):
            raise ValueError(f"v{version} must be newer than v{release.group(1)}")
        text = (
            text[:unreleased.end()]
            + f"\n\n## v{version} — {args.date}\n\n"
            + unreleased_body.strip()
            + "\n\n"
            + text[release.start():]
        )
        if not is_prerelease(version):
            manifest["version"] = version
            manifest["packages"][0]["identifier"] = f"{IMAGE}:{version}"
        validate(text, manifest, version)
        # newline="\n" is load-bearing on Windows: read_text() normalised the
        # file to LF, and write_text() would translate it back to os.linesep,
        # turning a three-line release commit into a whole-file rewrite that
        # hides the real change from review. The repo stores LF everywhere.
        changelog_path.write_text(text, encoding="utf-8", newline="\n")
        manifest_path.write_text(
            json.dumps(manifest, ensure_ascii=False, indent=2) + "\n",
            encoding="utf-8",
            newline="\n",
        )
        print(f"prepared v{version}; review the release notes before merging")
        return

    expected = args.expected or expected_from_pr()
    version = validate(text, manifest, expected)
    print(f"release metadata consistent: v{version}" + (" (release ready)" if expected else ""))


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, KeyError, json.JSONDecodeError) as exc:
        print(f"release metadata: {exc}", file=sys.stderr)
        sys.exit(1)
