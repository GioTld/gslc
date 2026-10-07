import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
MODULE = importlib.util.spec_from_file_location("seed", ROOT / "scripts/seed.py")
seed = importlib.util.module_from_spec(MODULE)
MODULE.loader.exec_module(seed)


class SeedTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.bundle = Path(self.temp.name) / "seed"
        self.bundle.mkdir()
        self.policy = seed.read_json(ROOT / "test/bootstrap/seed-policy.json")
        elf = bytearray(20)
        elf[:6], elf[18:20] = b"\x7fELF\x02\x01", b"\x3e\x00"
        (self.bundle / "compiler").write_bytes(elf)
        (self.bundle / "compiler").chmod(0o755)
        for name in ["candidate-b", "candidate-c"]:
            (self.bundle / name).write_bytes(elf)
        for name in ["candidate-b.ll", "candidate-c.ll"]:
            (self.bundle / name).write_text("IR")
        (self.bundle / "inputs.json").write_text("{}")
        self.archive("lib/compiler/cli.gsl", b"func main() {}")
        self.metadata = {"format": 2, "host": self.policy["host"], "targets": self.policy["targets"],
                         "source_profile": self.policy["source_profile"],
                         "source_files": {"lib/compiler/cli.gsl": seed.sha(b"func main() {}")}, "native_inputs": {}}
        self.refresh()

    def archive(self, name, data):
        with tarfile.open(self.bundle / "sources.tar", "w") as archive:
            member = tarfile.TarInfo(name)
            member.size = len(data)
            archive.addfile(member, io.BytesIO(data))

    def refresh(self):
        artifacts = {name: seed.sha((self.bundle / name).read_bytes()) for name in ["compiler", "sources.tar", "inputs.json", "candidate-b", "candidate-c", "candidate-b.ll", "candidate-c.ll"]}
        self.metadata.update(artifacts=artifacts, compiler_sha256=artifacts["compiler"],
                             source_snapshot_sha256=artifacts["sources.tar"],
                             source_revision="sha256:" + artifacts["sources.tar"])
        self.write_metadata()

    def write_metadata(self):
        (self.bundle / "seed.json").write_text(json.dumps(self.metadata))

    def test_verify_and_extract_sources(self):
        self.assertEqual(seed.verify(self.bundle, self.policy)["compiler_sha256"], self.metadata["compiler_sha256"])
        destination = Path(self.temp.name) / "extracted"
        seed.source_members(self.bundle, self.metadata["source_files"], destination)
        self.assertEqual((destination / "lib/compiler/cli.gsl").read_bytes(), b"func main() {}")

    def test_corrupted_compiler(self):
        (self.bundle / "compiler").write_bytes(b"corrupt")
        with self.assertRaisesRegex(seed.SeedError, "checksum mismatch"):
            seed.verify(self.bundle, self.policy)

    def test_expected_identity(self):
        with self.assertRaisesRegex(seed.SeedError, "expected identity"):
            seed.verify(self.bundle, self.policy, "0" * 64)

    def test_wrong_host_targets_profile_and_format(self):
        for key, value in [("host", "windows-x86_64"), ("targets", []), ("source_profile", "future"), ("format", 9)]:
            with self.subTest(key=key):
                original = self.metadata[key]
                self.metadata[key] = value
                self.write_metadata()
                with self.assertRaises(seed.SeedError):
                    seed.verify(self.bundle, self.policy)
                self.metadata[key] = original

    def test_source_tampering_despite_updated_archive_digest(self):
        self.archive("lib/compiler/cli.gsl", b"changed")
        self.refresh()
        with self.assertRaisesRegex(seed.SeedError, "source checksum"):
            seed.verify(self.bundle, self.policy)

    def test_archive_traversal_and_unrecorded_members(self):
        for name in ["../escaped", "/escaped", "unrecorded.gsl"]:
            self.archive(name, b"payload")
            self.refresh()
            with self.assertRaises(seed.SeedError):
                seed.verify(self.bundle, self.policy)
        self.assertFalse((Path(self.temp.name) / "escaped").exists())

    def test_missing_artifact_identity(self):
        del self.metadata["artifacts"]["compiler"]
        self.write_metadata()
        with self.assertRaisesRegex(seed.SeedError, "inconsistent identity"):
            seed.verify(self.bundle, self.policy)

    def test_explicit_selection_over_environment(self):
        with patch.dict(os.environ, {"GSL_SEED": "/missing"}), contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(seed.main(["verify", "--seed", str(self.bundle)]), 0)
        with patch.dict(os.environ, {"GSL_SEED": str(self.bundle)}), contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(seed.main(["verify"]), 0)

    def test_missing_seed_never_invokes_go(self):
        sentinel = Path(self.temp.name) / "go-called"
        go = Path(self.temp.name) / "go"
        go.write_text(f"#!/bin/sh\ntouch '{sentinel}'\nexit 99\n")
        go.chmod(0o755)
        environment = dict(os.environ, PATH=self.temp.name)
        environment.pop("GSL_SEED", None)
        result = subprocess.run([sys.executable, str(ROOT / "scripts/seed.py"), "verify"], env=environment, capture_output=True, text=True)
        self.assertEqual(result.returncode, 1)
        self.assertIn("no Go fallback", result.stderr)
        self.assertFalse(sentinel.exists())

    def test_divergent_fixed_point(self):
        (self.bundle / "candidate-c.ll").write_text("different IR")
        self.refresh()
        with self.assertRaisesRegex(seed.SeedError, "IR fixed point"):
            seed.verify(self.bundle, self.policy)

    def test_inconsistent_native_inputs(self):
        self.metadata["native_inputs"] = {"flags": ["-O0"]}
        self.write_metadata()
        with self.assertRaisesRegex(seed.SeedError, "records disagree"):
            seed.verify(self.bundle, self.policy)

    def test_tool_symlink_preserves_driver_name(self):
        driver = Path(self.temp.name) / "lld"
        driver.write_text("driver")
        driver.chmod(0o755)
        alias = Path(self.temp.name) / "ld.lld-18"
        alias.symlink_to(driver)
        self.assertEqual(Path(seed.executable(str(alias))).name, "ld.lld-18")

    def test_malformed_manifest(self):
        (self.bundle / "seed.json").write_text("[]")
        with self.assertRaisesRegex(seed.SeedError, "JSON object required"):
            seed.verify(self.bundle, self.policy)

    def test_existing_output_is_preserved(self):
        with patch.dict(os.environ, {}, clear=True), contextlib.redirect_stderr(io.StringIO()):
            result = seed.main(["create", "--seed", str(self.bundle), "--output", str(self.bundle)])
        self.assertEqual(result, 1)
        self.assertTrue((self.bundle / "compiler").exists())


if __name__ == "__main__":
    unittest.main()
