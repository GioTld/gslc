import contextlib
import io
import json
import os
import re
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts"))
import gsl


class RunnerTests(unittest.TestCase):
    def test_inventory_paths_names_and_generators(self):
        inventory = gsl.seed.read_json(ROOT / "test/bootstrap/acceptance.json")
        names = []
        for group, cases in inventory.items():
            if not isinstance(cases, list):
                continue
            for case in cases:
                names.append(case["name"])
                if "fixture" in case:
                    self.assertTrue((ROOT / case["fixture"]).is_file())
                if "generator" in case:
                    source = gsl.capacity_source(case["generator"], gsl.seed.read_json(ROOT / "test/bootstrap/manifest.json"))
                    self.assertTrue(source)
        self.assertEqual(len(names), len(set(names)))
        self.assertEqual(len(inventory["kernels"]), 9)

    def test_kernel_ir_requires_attributes_on_every_function(self):
        text = 'target triple = "x86_64-unknown-none-elf"\nattributes #0 = { noredzone noimplicitfloat "target-features"="-x87,-sse,-sse2,-mmx,-avx" }\ndefine void @f() #0 {\n}'
        gsl.kernel_ir(text)
        with self.assertRaises(gsl.seed.SeedError):
            gsl.kernel_ir(text.replace('() #0 {', '() {'))
        with self.assertRaises(gsl.seed.SeedError):
            gsl.kernel_ir(text + '\nasm sideeffect "syscall"')

    def test_kernel_assembly_rejects_x87_and_accepts_byte_continuations(self):
        gsl.kernel_assembly("ffffffff802001cd:\tff ff ff \n  100:\tfa \tcli")
        for instruction in ["fld1", "finit", "fnstcw", "fxsave", "fstp %st(0)"]:
            with self.subTest(instruction=instruction), self.assertRaises(gsl.seed.SeedError):
                gsl.kernel_assembly("  100:\td9 e8 \t" + instruction)

    def test_native_kernel_options_reject_before_creating_output(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "output"
            for options in [["--emit", "object"], ["--target", "kernel", "--boot", __file__],
                            ["--target", "kernel", "--linker-script", "missing.ld"],
                            ["--target", "kernel", "--emit", "object", "--entry", "start"]]:
                with contextlib.redirect_stderr(io.StringIO()), self.subTest(options=options):
                    status = gsl.main(["build", "input.gsl", "--output", str(output), *options])
                self.assertEqual(status, 1)
                self.assertFalse(output.exists())

    def test_build_cli_forwards_source_output_and_native_inputs(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source, boot, script, obj = [root / name for name in ("input.gsl", "boot.S", "link.ld", "extra.o")]
            for path in (source, boot, script, obj):
                path.touch()
            for target in ("hosted", "kernel"):
                output = root / target
                options = [] if target == "hosted" else [
                    "--target", "kernel", "--boot", str(boot), "--linker-script", str(script),
                    "--object", str(obj), "--entry", "kernel_entry"]
                with self.subTest(target=target), patch.object(gsl, "Runner") as runner, contextlib.redirect_stdout(io.StringIO()):
                    status = gsl.main(["build", str(source), "--output", str(output), *options])
                    self.assertEqual(status, 0)
                    args = runner.call_args.args[0]
                    self.assertEqual(args.output, output)
                    runner.return_value.build.assert_called_once_with(
                        source, "program", target, object_only=False,
                        boot_source=boot if options else None,
                        linker_script=script if options else None,
                        objects=[obj] if options else [], entry="kernel_entry" if options else "_start")
        with patch.object(gsl, "Runner") as runner, contextlib.redirect_stderr(io.StringIO()):
            with self.assertRaises(SystemExit) as error:
                gsl.main(["build", "input.gsl"])
            self.assertEqual(error.exception.code, 2)
            runner.assert_not_called()

    def test_missing_seed_does_not_require_tools_or_start_go(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "output"
            environment = dict(os.environ, PATH=directory)
            environment.pop("GSL_SEED", None)
            with patch.dict(os.environ, environment, clear=True), contextlib.redirect_stderr(io.StringIO()) as errors:
                status = gsl.main(["test", "--output", str(output)])
            self.assertEqual(status, 1)
            self.assertIn("no Go fallback", errors.getvalue())
            self.assertFalse(output.exists())

    def test_unknown_capacity_and_bad_elf_reject(self):
        with self.assertRaises(gsl.seed.SeedError):
            gsl.capacity_source("unknown", gsl.seed.read_json(ROOT / "test/bootstrap/manifest.json"))
        with self.assertRaises(gsl.seed.SeedError):
            gsl.kernel_elf(b"wrong")

    def test_initial_compiler_installs_and_rejects_overwrite(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / "initial"
            with contextlib.redirect_stdout(io.StringIO()):
                gsl.install_compiler(ROOT, output)
            metadata = gsl.seed.read_json(ROOT / "test/bootstrap/compiler.json")
            self.assertEqual(gsl.seed.sha((output / "compiler").read_bytes()), metadata["compiler_sha256"])
            self.assertTrue(os.access(output / "compiler", os.X_OK))
            with self.assertRaises(gsl.seed.SeedError):
                gsl.install_compiler(ROOT, output)

    def test_initial_compiler_corruption_fails_before_install(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            fixture = root / "test/bootstrap"
            fixture.mkdir(parents=True)
            (fixture / "compiler.json").write_bytes((ROOT / "test/bootstrap/compiler.json").read_bytes())
            (fixture / "compiler.gz").write_bytes(b"corrupt")
            output = root / "initial"
            with self.assertRaisesRegex(gsl.seed.SeedError, "checksum mismatch"):
                gsl.install_compiler(root, output)
            self.assertFalse(output.exists())

    def test_current_closure_and_unrecorded_import(self):
        manifest = gsl.seed.read_json(ROOT / "test/bootstrap/manifest.json")
        self.assertEqual(gsl.compiler_closure(ROOT, manifest), manifest["source_files"])
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "lib").mkdir()
            (root / "main.gsl").write_text('import "extra"\nfunc main() {}')
            (root / "lib/extra.gsl").write_text("func extra() {}")
            with self.assertRaisesRegex(gsl.seed.SeedError, "differs from manifest"):
                gsl.compiler_closure(root, dict(entry="main.gsl", source_files=["main.gsl"], excluded_modules=[]))

    def test_documentation_fixtures_have_acceptance_cases(self):
        manifest = gsl.seed.read_json(ROOT / "test/bootstrap/manifest.json")
        inventory = gsl.seed.read_json(ROOT / "test/bootstrap/acceptance.json")
        for example in manifest["doc_examples"]:
            matching = [(group, case) for group in ["hosted", "objects", "negative"]
                        for case in inventory[group] if case.get("fixture") == example["fixture"]]
            self.assertTrue(matching, example["fixture"])
            if example["gsl_status"]:
                self.assertTrue(any(group == "negative" and case["status"] == example["gsl_status"] for group, case in matching))
            elif "run_exit_code" in example:
                self.assertTrue(any(group == "hosted" and case["exit"] == example["run_exit_code"] for group, case in matching))
            else:
                self.assertTrue(any(group == "objects" for group, _ in matching))

    def test_storage_probe_arenas_match_compiler_source(self):
        manifest = gsl.seed.read_json(ROOT / "test/bootstrap/manifest.json")
        driver = (ROOT / "lib/compiler/driver.gsl").read_text()
        emitter = "\n".join((ROOT / name).read_text() for name in manifest["source_files"])
        probe = (ROOT / "test/bootstrap/acceptance/storage.gsl").read_text()
        work = str(manifest["storage"]["work_arena_bytes"])
        self.assertEqual(re.findall(r"arena work\((\d+)\)", driver), [work])
        self.assertEqual(re.findall(r"arena work\((\d+)\)", probe), [work])
        local = re.findall(r"arena local_storage\(CODEGEN_LOCAL_ARENA_BYTES\)", emitter)
        capacity = re.findall(r"const CODEGEN_LOCAL_ARENA_BYTES: u32 = (\d+)", emitter)
        self.assertTrue(local)
        self.assertEqual(set(capacity), set(re.findall(r'metric\("local_arena_bytes",(\d+) as u64', probe)))

    def test_storage_report_rejects_capacity_drift_and_overflow(self):
        manifest = gsl.seed.read_json(ROOT / "test/bootstrap/manifest.json")
        values = {name: 1 for name in manifest["storage"]["limits"]}
        values.update({"max_" + name: limit for name, limit in manifest["storage"]["limits"].items()})
        values.update(source_bytes=10, storage_bytes=10, local_bytes=10, local_arena_bytes=20,
                      max_locals=manifest["storage"]["limits"]["symbols"])
        gsl.storage_report(values, manifest)
        for name, value in [("max_expressions", 1), ("expressions", manifest["storage"]["limits"]["expressions"]+1),
                            ("storage_bytes", manifest["storage"]["work_arena_bytes"]+1), ("local_bytes", 21)]:
            with self.subTest(name=name), self.assertRaises(gsl.seed.SeedError):
                gsl.storage_report(dict(values, **{name: value}), manifest)

    def test_existing_output_preserves_files(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            marker = output / "original"
            marker.write_text("preserved")
            with contextlib.redirect_stderr(io.StringIO()):
                status = gsl.main(["build", "test/fixtures/arrays.gsl", "--compiler", str(ROOT / "scripts/gsl.py"), "--output", str(output)])
            self.assertEqual(status, 1)
            self.assertEqual(marker.read_text(), "preserved")


if __name__ == "__main__":
    unittest.main()
