#!/usr/bin/env python3
"""Self-check for scripts/release_metadata.py.

The rc rule this pins: `vX.Y.Z-rc.N` is an internal test build, so it adds a
dated `## vX.Y.Z-rc.N` heading and leaves server.json alone. An rc that moved
server.json would make the registry publish a version nobody asked for, and
`TestServerJSONMatchesChangelog` would disagree with this script about the same
changelog (it compares the newest FINAL heading - see the comment there).

Run: python3 -m unittest discover -s scripts -p '*_test.py'
"""

import contextlib
import io
import json
import os
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parent))
import release_metadata as R  # noqa: E402

CHANGELOG = """# Changelog

## Unreleased

### 新增

- **something worth shipping**

## v0.2.7 — 2026-10-09

- the release before this one
"""

MANIFEST = {
    "$schema": "https://static.modelcontextprotocol.io/schemas/2025-10-17/server.schema.json",
    "name": "io.github.open-mcp-ai/termcp",
    "version": "0.2.7",
    "packages": [{"registryType": "oci", "identifier": "ghcr.io/open-mcp-ai/termcp:0.2.7"}],
}


class ReleaseMetadataTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        (self.root / "CHANGELOG.md").write_text(CHANGELOG, encoding="utf-8")
        self.write_manifest(MANIFEST)
        self.addCleanup(self.tmp.cleanup)

    def write_manifest(self, manifest):
        (self.root / "server.json").write_text(
            json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
        )

    def manifest(self):
        return json.loads((self.root / "server.json").read_text(encoding="utf-8"))

    def changelog(self):
        return (self.root / "CHANGELOG.md").read_text(encoding="utf-8")

    def run_cli(self, *argv):
        """Run the script's CLI. Returns stdout; raises ValueError on refusal.

        main() reports failures by raising, and only the __main__ block turns
        that into SystemExit — so a test that expects a refusal asserts on
        ValueError (and can check the message a user would read).
        """
        with mock.patch.object(sys, "argv", ["release_metadata.py", "--root", str(self.root), *argv]):
            out = io.StringIO()
            with contextlib.redirect_stdout(out):
                R.main()
        return out.getvalue()

    def test_prepare_final_moves_the_manifest(self):
        self.run_cli("prepare", "v0.2.8", "--date", "2026-10-15")
        self.assertIn("## v0.2.8 — 2026-10-15", self.changelog())
        self.assertEqual(self.manifest()["version"], "0.2.8")
        self.assertTrue(self.manifest()["packages"][0]["identifier"].endswith(":0.2.8"))

    def test_prepare_prerelease_leaves_the_manifest_alone(self):
        self.run_cli("prepare", "v0.2.8-rc.1", "--date", "2026-10-15")
        self.assertIn("## v0.2.8-rc.1 — 2026-10-15", self.changelog())
        self.assertEqual(self.manifest()["version"], "0.2.7")
        self.assertTrue(self.manifest()["packages"][0]["identifier"].endswith(":0.2.7"))
        # The rc's notes moved out of Unreleased, like any release's do.
        self.assertNotIn("something worth shipping", R.changelog_parts(self.changelog())[2])

    def test_prepare_writes_lf_even_on_windows(self):
        """The repo stores LF; a CRLF rewrite hides the release change.

        read_text() normalises CRLF to LF and write_text() translates back to
        os.linesep, so on Windows `prepare` used to rewrite every line of
        CHANGELOG.md and server.json. The diff then contained the whole file and
        the three real lines were invisible in review. Asserting on bytes, not
        on text: text mode is what hid the problem.
        """
        self.run_cli("prepare", "v0.2.8", "--date", "2026-10-15")
        for name in ("CHANGELOG.md", "server.json"):
            with self.subTest(file=name):
                raw = (self.root / name).read_bytes()
                self.assertNotIn(b"\r\n", raw, f"{name} was written with CRLF")

    def test_check_accepts_prepared_prerelease(self):
        self.run_cli("prepare", "v0.2.8-rc.1", "--date", "2026-10-15")
        self.assertIn("v0.2.8-rc.1", self.run_cli("check", "--expected", "v0.2.8-rc.1"))

    def test_check_rejects_prerelease_that_moved_the_manifest(self):
        self.run_cli("prepare", "v0.2.8-rc.1", "--date", "2026-10-15")
        moved = self.manifest()
        moved["version"] = "0.2.8-rc.1"
        moved["packages"][0]["identifier"] = "ghcr.io/open-mcp-ai/termcp:0.2.8-rc.1"
        self.write_manifest(moved)
        with self.assertRaises(ValueError) as ctx:
            self.run_cli("check", "--expected", "v0.2.8-rc.1")
        self.assertIn("must not move server.json", str(ctx.exception))

    def test_check_rejects_an_rc_that_lost_its_dated_heading(self):
        self.run_cli("prepare", "v0.2.8-rc.1", "--date", "2026-10-15")
        text = self.changelog().replace("## v0.2.8-rc.1 — 2026-10-15", "## v0.2.8-rc.1")
        (self.root / "CHANGELOG.md").write_text(text, encoding="utf-8")
        with self.assertRaises(ValueError):
            self.run_cli("check", "--expected", "v0.2.8-rc.1")

    def test_versions_and_order(self):
        self.assertEqual(R.normalized_version("v0.2.8-rc.1"), "0.2.8-rc.1")
        self.assertTrue(R.is_prerelease("0.2.8-rc.1"))
        self.assertFalse(R.is_prerelease("0.2.8"))
        # A final release outranks any rc of the same base, so v0.2.8 may follow
        # v0.2.8-rc.2 without being rejected as "not newer".
        self.assertLess(R.version_order("0.2.8-rc.2"), R.version_order("0.2.8"))
        self.assertLess(R.version_order("0.2.8-rc.1"), R.version_order("0.2.8-rc.2"))
        self.assertLess(R.version_order("0.2.7"), R.version_order("0.2.8-rc.1"))
        for bad in ("v0.2.8-rc", "v0.2.8-rc.1.2", "v0.2.8-1", "v0.2.8-beta.1", "0.2"):
            with self.assertRaises(ValueError):
                R.normalized_version(bad)

    def test_final_release_may_follow_its_rc(self):
        # The rc takes the Unreleased notes with it, so the final release of the
        # same version needs notes of its own (the post-rc fixes). That is the
        # documented split: the rc section keeps the full list of what was in
        # the rc, the final section says what changed since.
        self.run_cli("prepare", "v0.2.8-rc.1", "--date", "2026-10-15")
        text = self.changelog().replace(
            "## Unreleased\n", "## Unreleased\n\n- **a fix found by testing the rc**\n", 1
        )
        (self.root / "CHANGELOG.md").write_text(text, encoding="utf-8")
        self.run_cli("prepare", "v0.2.8", "--date", "2026-10-16")
        self.assertEqual(self.manifest()["version"], "0.2.8")
        self.assertIn("## v0.2.8 — 2026-10-16", self.changelog())
        self.assertIn("## v0.2.8-rc.1 — 2026-10-15", self.changelog())
        self.assertIn("a fix found by testing the rc", self.changelog())

    def test_release_pr_signal_names_the_exact_build(self):
        # The branch/PR name IS the version being cut, so an rc has its own
        # branch: release/v0.2.8-rc.1 for v0.2.8-rc.1, release/v0.2.8 for the
        # final. The signal is matched exactly, like the tag is.
        for branch, title, want in (
            ("release/v0.2.8-rc.1", "", "0.2.8-rc.1"),
            ("", "release: v0.2.8-rc.1", "0.2.8-rc.1"),
            ("release/v0.2.8", "", "0.2.8"),
            ("", "release: v0.2.8", "0.2.8"),
        ):
            with mock.patch.dict(
                os.environ, {"RELEASE_PR_HEAD_REF": branch, "RELEASE_PR_TITLE": title}, clear=False
            ):
                self.assertEqual(R.expected_from_pr(), want)
        # A release signal that is neither shape is refused, not ignored.
        for branch in ("release/0.2.8", "release/v0.2.8-beta.1"):
            with mock.patch.dict(os.environ, {"RELEASE_PR_HEAD_REF": branch}, clear=False), self.assertRaises(
                ValueError
            ):
                R.expected_from_pr()

    def test_release_branch_must_match_the_changelog_exactly(self):
        # An rc PR is release/v0.2.8-rc.1: the branch naming the final release is
        # refused while the changelog describes the rc, so the PR name always
        # says which build is being cut. (This is the state my earlier version
        # of this script wrongly accepted.)
        self.run_cli("prepare", "v0.2.8-rc.1", "--date", "2026-10-15")
        for branch, accepted in (
            ("release/v0.2.8-rc.1", True),
            ("release/v0.2.8", False),
            ("release/v0.2.8-rc.2", False),
            ("release/v0.2.9", False),
        ):
            with self.subTest(branch=branch):
                with mock.patch.dict(os.environ, {"RELEASE_PR_HEAD_REF": branch}, clear=False):
                    if accepted:
                        self.assertIn("v0.2.8-rc.1", self.run_cli("check"))
                    else:
                        with self.assertRaises(ValueError):
                            self.run_cli("check")

    def test_final_release_rejects_an_rc_of_a_newer_base(self):
        # v0.2.7 cannot follow v0.2.8-rc.1: order is by base version first.
        with self.assertRaises(ValueError) as ctx:
            self.run_cli("prepare", "v0.2.7", "--date", "2026-10-15")
        self.assertIn("must be newer than", str(ctx.exception))


if __name__ == "__main__":
    unittest.main()
