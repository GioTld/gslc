"""Portable std contracts, Linux I/O and explicit backend selection."""
import signal

import seed


def check_std(runner):
    work = runner.work / "stdlib"
    work.mkdir()
    inventory = seed.read_json(runner.root / "test/bootstrap/acceptance.json")["stdlib"]
    fixtures = {name: runner.root / path for name, path in inventory.items()}
    backend = fixtures["backend"]
    original = runner.platform, runner.platform_root
    try:
        runner.platform, runner.platform_root = "custom", backend
        for optimization in ["-O0", "-O2"]:
            binary = runner.build(fixtures["contracts"], "std-contracts" + optimization, "hosted", optimization)
            result = runner.run([binary])
            if result.stdout or result.stderr:
                raise seed.SeedError("test backend leaked output")
            runner.passed("std-contracts" + optimization)
        runner.build(fixtures["contracts"], "std-custom-kernel", "kernel", object_only=True)
        ir = (runner.work / "std-custom-kernel.ll").read_text()
        if 'asm sideeffect "syscall"' in ir or "@linux_" in ir:
            raise seed.SeedError("Linux leaked into custom freestanding std")
        runner.passed("std-custom-freestanding")
        runner.platform, runner.platform_root = "linux-x86_64", None
        data = work / "input.bin"
        data.write_bytes(bytes([0, 255, 2, 3, 4]))
        for optimization in ["-O0", "-O2"]:
            binary = runner.build(fixtures["linux"], "std-linux" + optimization, "hosted", optimization)
            result = runner.run([binary, data])
            if result.stdout != b"stdout\n" or result.stderr != b"stderr\n":
                raise seed.SeedError("std streams differ")
            copy = runner.build(fixtures["copy"], "std-copy" + optimization, "hosted", optimization)
            for length in [0, 5, 8193]:
                source, target = work / "copy-in", work / "copy-out"
                source.write_bytes(bytes(i % 256 for i in range(length)))
                target.unlink(missing_ok=True)
                runner.run([copy, source, target])
                if target.read_bytes() != source.read_bytes():
                    raise seed.SeedError("portable copy differs")
                runner.run([copy, source, target], 5)
                runner.run([copy, source, source], 5)
                if target.read_bytes() != source.read_bytes():
                    raise seed.SeedError("exclusive creation changed existing file")
            runner.run([copy, work / "missing", work / "unused"], 3)
            pipe = runner.build(fixtures["pipe"], "std-pipe" + optimization, "hosted", optimization)
            # exec preserves ignored dispositions; std must not install a handler.
            runner.run([pipe], preexec_fn=lambda: signal.signal(signal.SIGPIPE, signal.SIG_IGN))
            runner.run([pipe], -signal.SIGPIPE, preexec_fn=lambda: signal.signal(signal.SIGPIPE, signal.SIG_DFL))
            runner.passed("std-linux-files-streams-signals" + optimization)
        runner.platform, runner.platform_root = None, None
        source = work / "import.gsl"
        source.write_text('import "std/io"\nfunc main() { io_print("hello") }\n')
        output = work / "rejected.ll"
        empty = work / "empty-backend"
        empty.mkdir()
        incomplete = work / "incomplete-backend"
        incomplete.mkdir()
        (incomplete / "io.gsl").write_text('func unrelated() {}\n')
        cases = [(["--platform", "none"], 10),
                 (["--platform", "custom"], 64),
                 (["--platform-root", backend], 64),
                 (["--target=x86_64-unknown-none-elf", "--platform", "linux-x86_64"], 64),
                 (["--platform", "custom", "--platform-root", empty], 10),
                 (["--platform", "custom", "--platform-root", incomplete], 2),
                 (["--target=x86_64-unknown-none-elf"], 10)]
        for index, (options, status) in enumerate(cases):
            for present in [False, True]:
                output.unlink(missing_ok=True)
                if present:
                    output.write_bytes(b"preserved")
                runner.run([runner.compiler, *options, source, output], status)
                preserved = output.read_bytes() == b"preserved" if present else not output.exists()
                if not preserved:
                    raise seed.SeedError("platform rejection changed output")
            runner.passed(f"std-platform-rejection-{index}")
        # A local file cannot shadow the reserved platform imports.
        source.write_text('import "platform/io"\nfunc main() {}\n')
        (work / "platform").mkdir()
        (work / "platform/io.gsl").write_text('func wrong() {}\n')
        runner.run([runner.compiler, "--platform", "custom", "--platform-root", backend, source, output])
        if "@wrong" in output.read_text() or "@linux_" in output.read_text():
            raise seed.SeedError("reserved import escaped selected backend")
        core = work / "core.gsl"
        core.write_text('import "core/string"\nimport "core/mem"\nfunc kernel_main() {}\n')
        runner.build(core, "std-core-kernel", "kernel", object_only=True)
        runner.passed("std-core-freestanding")
    finally:
        runner.platform, runner.platform_root = original
