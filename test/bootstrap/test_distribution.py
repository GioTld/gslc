import gzip
import json
from pathlib import Path
import sys
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts"))
import install
import package


class DistributionTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.base = Path(self.temporary.name)
        self.root = self.base / "source"
        names = ["scripts/gslc.py", "scripts/gsl.py", "scripts/seed.py", "scripts/install.py",
                 "test/bootstrap/seed-policy.json", "test/kernel/boot.S", "test/kernel/linker.ld",
                 "lib/std/io.gsl", "README.md"]
        for name in names:
            target = self.root / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(name)
        binary = b"compiler fixture"
        compressed = gzip.compress(binary, mtime=0)
        (self.root / "test/bootstrap/compiler.gz").write_bytes(compressed)
        metadata = {"compiler_sha256": package.digest(binary), "compressed_sha256": package.digest(compressed),
                    "source_files": {"lib/std/io.gsl": package.digest((self.root / "lib/std/io.gsl").read_bytes())}}
        (self.root / "test/bootstrap/compiler.json").write_text(json.dumps(metadata))

    def unpack(self):
        bundle = self.base / "bundle"
        for name, (data, mode) in package.payload(self.root).items():
            path = bundle / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(data)
            path.chmod(mode)
        return bundle

    def test_archive_is_reproducible_and_excludes_development_files(self):
        (self.root / "private.md").write_text("private")
        (self.root / "lib/ignored.ll").write_text("temporary")
        first = package.build(self.root, self.base / "first.tar.gz")
        second = package.build(self.root, self.base / "second.tar.gz")
        self.assertEqual(first.read_bytes(), second.read_bytes())
        with tarfile.open(first) as archive:
            names = archive.getnames()
            self.assertIn("gslc-linux-x86_64/bin/gslc", names)
            self.assertFalse(any("private" in n or "ignored.ll" in n for n in names))
        with self.assertRaises(FileExistsError):
            package.build(self.root, first)

    def test_changed_sources_or_compiler_reject_packaging(self):
        source = self.root / "lib/std/io.gsl"
        original = source.read_bytes()
        source.write_text("changed")
        with self.assertRaisesRegex(ValueError, "source differs"):
            package.payload(self.root)
        source.write_bytes(original)
        (self.root / "test/bootstrap/compiler.gz").write_bytes(gzip.compress(b"changed"))
        with self.assertRaisesRegex(ValueError, "checksum"):
            package.payload(self.root)

    def test_install_prefix_with_spaces_and_existing_installation(self):
        bundle = self.unpack()
        prefix = self.base / "user prefix"
        command = install.install(bundle, prefix)
        self.assertTrue(command.stat().st_mode & 0o111)
        self.assertEqual((prefix / "lib/gslc/compiler").read_bytes(), b"compiler fixture")
        self.assertEqual((prefix / "lib/gslc/lib/std/io.gsl").read_text(), "lib/std/io.gsl")
        with self.assertRaisesRegex(ValueError, "already installed"):
            install.install(bundle, prefix)
        self.assertTrue(command.is_file())

    def test_corrupt_bundle_leaves_prefix_untouched(self):
        bundle = self.unpack()
        (bundle / "lib/gslc/compiler").write_bytes(b"corrupted")
        prefix = self.base / "destination"
        with self.assertRaisesRegex(ValueError, "checksum"):
            install.install(bundle, prefix)
        self.assertFalse(prefix.exists())

    def test_unrelated_existing_command_is_preserved(self):
        bundle = self.unpack()
        prefix = self.base / "destination"
        command = prefix / "bin/gslc"
        command.parent.mkdir(parents=True)
        command.write_text("existing command")
        with self.assertRaisesRegex(ValueError, "already installed"):
            install.install(bundle, prefix)
        self.assertEqual(command.read_text(), "existing command")
        self.assertFalse((prefix / "lib/gslc").exists())
