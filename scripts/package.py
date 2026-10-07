"""Build a relocatable Linux x86_64 distribution from the accepted compiler."""
import argparse
import gzip
import hashlib
import io
import json
from pathlib import Path
import tarfile

ROOT = Path(__file__).resolve().parents[1]


def digest(data):
    return hashlib.sha256(data).hexdigest()


def payload(root):
    metadata = json.loads((root / "test/bootstrap/compiler.json").read_text())
    archive = (root / "test/bootstrap/compiler.gz").read_bytes()
    compiler = gzip.decompress(archive)
    if digest(archive) != metadata["compressed_sha256"] or digest(compiler) != metadata["compiler_sha256"]:
        raise ValueError("compiler checksum mismatch")
    for name, expected in metadata["source_files"].items():
        if digest((root / name).read_bytes()) != expected:
            raise ValueError(f"compiler source differs from accepted build: {name}")
    files = {
        "bin/gslc": ((root / "scripts/gslc.py").read_bytes(), 0o755),
        "lib/gslc/compiler": (compiler, 0o755),
        "lib/gslc/compiler.json": ((root / "test/bootstrap/compiler.json").read_bytes(), 0o644),
        "install.py": ((root / "scripts/install.py").read_bytes(), 0o644),
        "README.md": ((root / "README.md").read_bytes(), 0o644),
    }
    names = ["scripts/gsl.py", "scripts/seed.py", "test/bootstrap/seed-policy.json",
             "test/kernel/boot.S", "test/kernel/linker.ld"]
    names += [str(p.relative_to(root)) for p in sorted((root / "lib").rglob("*.gsl"))]
    for name in names:
        files["lib/gslc/" + name] = ((root / name).read_bytes(), 0o644)
    manifest = {name: digest(data) for name, (data, _) in sorted(files.items())}
    files["checksums.json"] = ((json.dumps(manifest, indent=2) + "\n").encode(), 0o644)
    return files


def build(root, output):
    files = payload(root)
    output.parent.mkdir(parents=True, exist_ok=True)
    with output.open("xb") as destination:
        with gzip.GzipFile(filename="", fileobj=destination, mode="wb", mtime=0) as compressed:
            with tarfile.open(fileobj=compressed, mode="w") as archive:
                for name, (data, mode) in sorted(files.items()):
                    member = tarfile.TarInfo("gslc-linux-x86_64/" + name)
                    member.size, member.mode, member.mtime = len(data), mode, 0
                    archive.addfile(member, io.BytesIO(data))
    return output


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    try:
        print(build(ROOT, args.output))
    except (OSError, ValueError, KeyError, EOFError) as error:
        parser.exit(1, f"gslc package: {error}\n")


if __name__ == "__main__":
    main()
