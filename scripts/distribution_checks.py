"""Exercise the installed compiler outside the source checkout."""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile

import package


def check(output, clang, linker):
    output = output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    archive = package.build(package.ROOT, output / "gslc-linux-x86_64.tar.gz")
    with tarfile.open(archive) as tar:
        for member in tar.getmembers():
            target = output / member.name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_bytes(tar.extractfile(member).read())
            target.chmod(member.mode)
    unpacked = output / "gslc-linux-x86_64"
    prefix = output / "user prefix"
    project = output / "external project"
    project.mkdir()
    environment = os.environ.copy()
    environment.pop("PYTHONPATH", None)
    environment.pop("GSL_SEED", None)
    commands = []

    def run(args, stdout=None):
        result = subprocess.run(list(map(str, args)), cwd=project, env=environment,
                                capture_output=True, text=True, timeout=60)
        commands.append({"args": list(map(str, args)), "status": result.returncode,
                         "stdout": result.stdout, "stderr": result.stderr})
        (output / "report.json").write_text(json.dumps(commands, indent=2) + "\n")
        if result.returncode or (stdout is not None and result.stdout != stdout):
            raise RuntimeError(f"distribution check failed: {args}: {result.stdout}{result.stderr}")
        return result

    run(["python3", unpacked / "install.py", "--prefix", prefix])
    shutil.rmtree(unpacked)
    relocated = output / "relocated prefix"
    prefix.rename(relocated)
    command = relocated / "bin/gslc"
    environment["PATH"] = str(command.parent) + os.pathsep + environment["PATH"]
    run(["gslc", "--version"])
    run(["gslc", "--help"])
    (project / "local.gsl").write_text("func answer() u32 { return 42 }\n")
    (project / "main.gsl").write_text('import "std/io"\nimport "local"\nfunc main() u32 { println("installed GSL"); if answer()!=42 { return 1 }; return 0 }\n')
    flags = ["--clang", clang, "--linker", linker]
    run(["gslc", "build", "main.gsl", "--output", "hosted", *flags])
    run([project / "hosted/program"], stdout="installed GSL\n")
    (project / "kernel.gsl").write_text('func kernel_main() { unsafe { hlt() } }\n')
    run(["gslc", "build", "kernel.gsl", "--target", "kernel", "--emit", "object", "--output", "kernel", *flags])
    if not (project / "kernel/program.o").is_file():
        raise RuntimeError("missing freestanding object")
    (project / "bad.gsl").write_text('func main() u32 { return missing }\n')
    result = subprocess.run([str(command), "build", "bad.gsl", "--output", "bad", *flags],
                            cwd=project, env=environment, capture_output=True, timeout=60)
    if result.returncode == 0 or (project / "bad/program").exists():
        raise RuntimeError("installed compiler accepted invalid source")
    print(f"PASS installed compiler: {output}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--clang", default="clang-18")
    parser.add_argument("--linker", default="ld.lld-18")
    args = parser.parse_args()
    check(args.output, args.clang, args.linker)


if __name__ == "__main__":
    main()
