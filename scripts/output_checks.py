"""Exercise real filesystem failures in the GSL compiler's output transaction."""
import json
import os
import resource
import signal
import sys


def limit_output():
    signal.signal(signal.SIGXFSZ, signal.SIG_IGN)
    resource.setrlimit(resource.RLIMIT_FSIZE, (1024, 1024))


def check_output(runner):
    work = runner.work / "output-errors"
    work.mkdir()
    source = work / "source.gsl"
    source.write_text("func main() u32 { return 0; }\n")
    output = work / "result.ll"
    expected = {"code": "E3001", "span": {"start": 0, "end": 0}, "message": "output error"}
    for index, compiler in enumerate([runner.compiler, runner.compare]):
        def compile_to(path, *, failure=False, limited=False):
            result = runner.run([compiler, "--diagnostics=json", source, path],
                                status=3 if failure else 0,
                                preexec_fn=limit_output if limited else None)
            if failure:
                if json.loads(result.stdout) != expected or result.stderr:
                    raise AssertionError("wrong output failure diagnostic")
            elif result.stdout or result.stderr:
                raise AssertionError("unexpected success diagnostic")
            if list(work.glob("*.gsl-tmp-*")):
                raise AssertionError("temporary output was not removed")

        for present in [False, True]:
            output.unlink(missing_ok=True)
            if present:
                output.write_bytes(b"preserved")
            compile_to(output, failure=True, limited=True)
            if present:
                if output.read_bytes() != b"preserved":
                    raise AssertionError("failed write replaced previous output")
            elif output.exists():
                raise AssertionError("failed write published an incomplete output")
            runner.passed(f"output-write-failure-{index}-{present}")

        directory = work / f"directory-{index}"
        directory.mkdir()
        compile_to(directory, failure=True)
        fifo = work / f"fifo-{index}"
        os.mkfifo(fifo)
        compile_to(fifo, failure=True)
        if not fifo.is_fifo():
            raise AssertionError("compiler replaced a special file")
        compile_to(work / "missing" / "result.ll", failure=True)
        target = work / f"target-{index}"
        target.write_bytes(b"preserved")
        link = work / f"link-{index}"
        link.symlink_to(target)
        compile_to(link, failure=True)
        if not link.is_symlink() or target.read_bytes() != b"preserved":
            raise AssertionError("compiler changed a symlink or its target")
        runner.passed(f"output-invalid-destinations-{index}")

        output.write_bytes(b"old output")
        output.chmod(0o640)
        compile_to(output)
        if output.stat().st_mode & 0o777 != 0o640:
            raise AssertionError("output permissions changed")
        if b"define i32 @main" not in output.read_bytes():
            raise AssertionError("successful compile did not replace output")
        runner.passed(f"output-replacement-{index}")

        masked = work / f"masked-{index}.ll"
        result = runner.run([compiler, "--diagnostics=json", source, masked], preexec_fn=lambda: os.umask(0o077))
        if result.stdout or result.stderr or masked.stat().st_mode & 0o777 != 0o600:
            raise AssertionError("new output ignored the process umask")
        runner.passed(f"output-umask-{index}")

        expected_ir = output.read_bytes()
        for fault in ["short", "interrupted", "zero", "close", "sync", "rename"]:
            output.write_bytes(b"preserved")
            success = fault in ["short", "interrupted"]
            result = runner.run([sys.executable, runner.root / "scripts/io_faults.py", fault,
                                 compiler, "--diagnostics=json", source, output], status=0 if success else 3)
            if success:
                if result.stdout or result.stderr or output.read_bytes() != expected_ir:
                    raise AssertionError("retry changed emitted LLVM IR")
            elif json.loads(result.stdout) != expected or result.stderr or output.read_bytes() != b"preserved":
                raise AssertionError("output fault lost the original file or diagnostic")
            if list(work.glob("*.gsl-tmp-*")):
                raise AssertionError("output fault left a temporary file")
            runner.passed(f"output-{fault}-{index}")
