"""Build and accept the GSL compiler from explicit GSL seeds, without Go."""
import argparse
import gzip
import platform
import json
import os
from pathlib import Path
import re
import signal
import struct
import subprocess
import sys
import seed

ROOT = Path(__file__).resolve().parents[1]
TARGETS = {"hosted": "x86_64-unknown-linux-gnu", "kernel": "x86_64-unknown-none-elf"}


def require(condition, message):
    if not condition:
        raise seed.SeedError(message)


class Runner:
    def __init__(self, args):
        self.root = args.root.resolve()
        self.policy = seed.read_json(self.root / "test/bootstrap/seed-policy.json")
        selected = args.seed or os.environ.get("GSL_SEED")
        if args.compiler:
            self.compiler = args.compiler.resolve()
            require(self.compiler.is_file(), "compiler missing")
            self.bundle = None
        else:
            require(selected, "missing seed: supply --seed or GSL_SEED; no Go fallback")
            self.bundle = Path(selected).expanduser().resolve()
            seed.verify(self.bundle, self.policy)
            self.compiler = self.bundle / "compiler"
        self.work = args.output.resolve()
        require(not self.work.exists(), f"output already exists: {self.work}")
        self.work.mkdir(parents=True)
        self.commands = []
        self.report = {"passed": [], "commands": self.commands}
        self.clang = seed.executable(args.clang)
        self.linker = seed.executable(args.linker)
        self.versions = {"clang": self.run([self.clang, "--version"]).stdout.decode(),
                         "linker": self.run([self.linker, "--version"]).stdout.decode()}
        if args.llvm_version:
            for name, version in self.versions.items():
                require(re.search(r'\b' + re.escape(args.llvm_version) + r'\b', version), f"unexpected {name} version: {version}")
        self.compare = None
        self.report.update(tools=self.versions, seed=str(self.bundle), compiler_sha256=seed.sha(self.compiler.read_bytes()))
        self.save()

    def save(self):
        (self.work / "report.json").write_text(json.dumps(self.report, indent=2) + "\n")

    def run(self, args, status=0, timeout=30, cwd=None, preexec_fn=None):
        args = list(map(str, args))
        try:
            result = subprocess.run(args, cwd=cwd or self.root, capture_output=True, timeout=timeout, preexec_fn=preexec_fn)
        except (OSError, subprocess.TimeoutExpired) as error:
            raise seed.SeedError(f"command failed: {args[0]}: {error}") from error
        self.commands.append({"args": args, "status": result.returncode, "stdout_hex": result.stdout.hex(), "stderr_hex": result.stderr.hex()})
        self.save()
        require(status is None or result.returncode == status, f"expected status {status}: {args}: got {result.returncode}\n{result.stdout!r}\n{result.stderr!r}")
        return result

    def passed(self, name):
        self.report["passed"].append(name)
        self.save()
        print(f"PASS {name}", flush=True)

    def emit(self, source, output, target="hosted"):
        self.run([self.compiler, "--target=" + TARGETS[target], source, output])
        if self.compare:
            alternate = output.with_suffix(".compare.ll")
            self.run([self.compare, "--target=" + TARGETS[target], source, alternate])
            require(output.read_bytes() == alternate.read_bytes(), f"fixture IR differs: {source}")
        if target == "kernel":
            kernel_ir(output.read_text())

    def build(self, source, name, target, optimization="-O2", *, object_only=False, boot_source=None, linker_script=None, objects=(), entry="_start"):
        ir = self.work / (name + ".ll")
        self.emit(source, ir, target)
        output = self.work / name
        if target == "hosted":
            self.run([self.clang, optimization, "--ld-path=" + self.linker, "-Wl,--build-id=none", ir, "-o", output])
        else:
            obj = self.work / (name + ".o")
            flags = ["--target=" + TARGETS[target], optimization, "-ffreestanding", "-mno-red-zone", "-mgeneral-regs-only", "-mcmodel=kernel", "-fno-pic", "-fno-pie"]
            self.run([self.clang, *flags, "-c", ir, "-o", obj])
            fixture = linker_script is None
            if object_only:
                output = obj
            else:
                inputs = [obj, *objects]
                script = linker_script or self.root / "test/kernel/linker.ld"
                boot_source = boot_source or (self.root / "test/kernel/boot.S" if fixture else None)
                if boot_source:
                    boot = self.work / (name + ".boot.o")
                    self.run([self.clang, *flags, "-c", boot_source, "-o", boot])
                    inputs.append(boot)
                self.report["link_inputs"] = {"entry": entry, "script": str(script), "script_sha256": seed.sha(script.read_bytes()),
                                               "objects": {str(path): seed.sha(path.read_bytes()) for path in inputs}}
                self.run([self.linker, "-static", "--build-id=none", "-e", entry, "-T", script, *inputs, "-o", output])
                kernel_elf(output.read_bytes(), entry=entry, fixture=fixture)
            self.report["kernel_flags"] = flags
            assembly = self.run([seed.executable("objdump"), "-d", output]).stdout.decode()
            kernel_assembly(assembly)
        return output

    def negative(self, case, source_text=None):
        name = case["name"]
        source, output = self.work / (name + ".gsl"), self.work / (name + ".ll")
        if source_text is None:
            source_text = case["source"] if "source" in case else (self.root / case["fixture"]).read_text()
        source.write_text(source_text)
        diagnostics = []
        for compiler in [self.compiler] + ([self.compare] if self.compare else []):
            output.write_bytes(b"preserved")
            result = self.run([compiler, "--diagnostics=json", "--target=" + TARGETS[case.get("target", "hosted")], source, output], case["status"])
            require(output.read_bytes() == b"preserved", f"rejected input overwrote output: {name}")
            record = json.loads(result.stdout)
            require(record.get("code", "").startswith("E"), f"missing diagnostic: {name}")
            if case.get("check_code"):
                require(record["code"] == case["code"], f"wrong diagnostic code: {name}")
            if "record" in case:
                require(record == case["record"], f"diagnostic span/message mismatch: {name}: {record}")
            if case.get("check_missing_output"):
                output.unlink()
                repeated = self.run([compiler, "--diagnostics=json", "--target=" + TARGETS[case.get("target", "hosted")], source, output], case["status"])
                require(not output.exists(), f"rejected input created output: {name}")
                require(json.loads(repeated.stdout) == record, "output existence changed diagnostic")
            diagnostics.append(record)
        require(all(record == diagnostics[0] for record in diagnostics), f"candidate diagnostics differ: {name}")
        self.passed(name)

    def object_case(self, case):
        ir = self.work / (case["name"] + ".ll")
        self.emit(self.root / case["fixture"], ir, "kernel")
        for optimization in ["-O0", "-O2"]:
            obj = self.work / (case["name"] + optimization + ".o")
            self.run([self.clang, "--target=" + TARGETS["kernel"], optimization, "-c", ir, "-o", obj])
            assembly = self.run([seed.executable("objdump"), "-d", obj]).stdout.decode()
            for pattern in case["patterns"]:
                require(re.search(pattern, assembly), f"missing object instruction: {pattern}")
            kernel_assembly(assembly)
            self.passed(case["name"] + optimization)

    def rebuild(self):
        manifest = seed.read_json(self.root / "test/bootstrap/manifest.json")
        ordered = compiler_closure(self.root, manifest)
        sources = self.work / "source"
        hashes = {}
        for name in ordered:
            data = (self.root / name).read_bytes()
            hashes[name] = seed.sha(data)
            output = sources / name
            output.parent.mkdir(parents=True, exist_ok=True)
            output.write_bytes(data)
        require(sum((sources / name).stat().st_size for name in ordered) <= manifest["input_max_bytes"], "compiler source exceeds input limit")
        flags = ["-O2", "--target=" + TARGETS["hosted"], "-Wl,--build-id=none", "--ld-path=" + self.linker]
        prior = self.compiler
        for name in ["candidate-a", "candidate-b", "candidate-c"]:
            ir, binary = self.work / (name + ".ll"), self.work / name
            native = self.work / "native.ll"
            self.run([prior, manifest["entry"], native], cwd=sources)
            ir.write_bytes(native.read_bytes())
            self.run([self.clang, *flags, native, "-o", binary])
            prior = binary
        require((self.work / "candidate-b.ll").read_bytes() == (self.work / "candidate-c.ll").read_bytes(), "compiler IR fixed point failed")
        require((self.work / "candidate-b").read_bytes() == (self.work / "candidate-c").read_bytes(), "compiler binary fixed point failed")
        require(str(self.work).encode() not in (self.work / "candidate-b").read_bytes(), "compiler embeds build path")
        self.compiler, self.compare = self.work / "candidate-b", self.work / "candidate-a"
        self.report.update(compiler_sha256=seed.sha(self.compiler.read_bytes()), source_files=ordered,
                           source_sha256=hashes, native_flags=flags)
        self.passed("compiler-fixed-point")

    def accept(self, args):
        from output_checks import check_output
        from mutation_checks import check_mutations
        from semantic_checks import check_semantics

        self.rebuild()
        inventory = seed.read_json(self.root / "test/bootstrap/acceptance.json")
        require(inventory.get("format") == 1, "unsupported acceptance inventory")
        for case in inventory["hosted"]:
            source = self.root / case["fixture"] if "fixture" in case else self.work / (case["name"] + ".gsl")
            if "source" in case:
                source.write_text(case["source"])
            for optimization in ["-O0", "-O2"]:
                name = case["name"] + optimization
                binary = self.build(source, name, "hosted", optimization)
                for instruction in case.get("ir_contains", []):
                    require(instruction in binary.with_suffix(".ll").read_text(), f"missing LLVM instruction: {instruction}")
                if "definitions" in case:
                    text = binary.with_suffix(".ll").read_text()
                    previous = -1
                    for function in case["definitions"]:
                        marker = "define i32 @" + function + "("
                        position = text.find(marker)
                        require(position > previous and text.count(marker) == 1, "import definition order/duplicates")
                        previous = position
                    repeat = self.work / (name + ".repeat.ll")
                    self.emit(source, repeat)
                    require(repeat.read_bytes() == binary.with_suffix(".ll").read_bytes(), "repeated import resolution differs")
                arguments = case.get("arguments", [[]])
                if arguments == "compiler_source_files":
                    arguments = [[name] for name in self.report["source_files"]]
                arguments = [[self.root / argument for argument in group] for group in arguments]
                generated = self.work / (name + ".generated.ll")
                if "generated" in case:
                    arguments = [[generated]]
                if "round_trip_hex" in case:
                    input_path, output_path = self.work / "input.bin", self.work / "output.bin"
                    input_path.write_bytes(bytes.fromhex(case["round_trip_hex"]))
                    output_path.unlink(missing_ok=True)
                    arguments = [[input_path, output_path]]
                for argv in arguments:
                    result = self.run([binary, *argv], case["exit"])
                    if case.get("storage_report"):
                        measured = json.loads(result.stdout)
                        storage_report(measured, seed.read_json(self.root / "test/bootstrap/manifest.json"))
                        self.report["storage"] = measured
                    else:
                        require(result.stdout == bytes.fromhex(case.get("stdout_hex", "")), f"unexpected hosted output: {name}")
                    require(not result.stderr, f"unexpected hosted stderr: {name}")
                    if "round_trip_hex" in case:
                        require(output_path.read_bytes() == input_path.read_bytes(), "file round trip differs")
                if "generated" in case:
                    require(generated.read_text().count("define i32 @") == case["generated"]["i32_functions"], "compiler API emitted wrong function count")
                    program = self.work / (name + ".generated")
                    self.run([self.clang, optimization, "--ld-path=" + self.linker, "-Wl,--build-id=none", generated, "-o", program])
                    result = self.run([program], case["generated"]["exit"])
                    require(not result.stdout and not result.stderr, "unexpected compiler API program output")
                self.passed(name)
        for case in inventory["objects"]:
            self.object_case(case)
        for case in inventory["negative"]:
            self.negative(case)
        limits = seed.read_json(self.root / "test/bootstrap/manifest.json")
        for case in inventory["capacities"]:
            self.negative(case, capacity_source(case["generator"], limits))
        for case in inventory["traps"]:
            source = self.work / (case["name"] + ".gsl")
            source.write_text(case["source"])
            for optimization in ["-O0", "-O2"]:
                binary = self.build(source, case["name"] + optimization, "hosted", optimization)
                require("call void @llvm.trap()" in binary.with_suffix(".ll").read_text(), "missing explicit trap")
                result = self.run([binary], -signal.SIGILL)
                require(not result.stdout and not result.stderr, "trap occurred after a side effect")
                self.passed(case["name"] + optimization)
        for case in inventory["kernels"]:
            objects = []
            for index, source in enumerate(case.get("object_sources", [])):
                obj = self.work / (case["name"] + f".external-{index}.o")
                self.run([self.clang, "--target=" + TARGETS["kernel"], "-c", self.root / source, "-o", obj])
                objects.append(obj)
            binary = self.build(self.root / case["fixture"], case["name"], "kernel",
                                boot_source=self.root / case["boot"] if "boot" in case else None,
                                linker_script=self.root / case["linker_script"] if "linker_script" in case else None,
                                objects=objects, entry=case.get("entry", "_start"))
            if "sections" in case:
                for section, expected in case["sections"].items():
                    dump = self.work / (case["name"] + section + ".bin")
                    self.run([seed.executable("objcopy"), "--dump-section", section + "=" + str(dump), binary, self.work / "section-probe.elf"])
                    require(dump.read_bytes() == bytes.fromhex(expected), "section contents differ")
            image = binary.with_suffix(".qemu.elf")
            self.run([seed.executable("objcopy"), "-O", "elf32-i386", binary, image])
            result = self.run([seed.executable("qemu-system-x86_64"), "-display", "none", "-serial", "stdio", "-monitor", "none", "-no-reboot", "-device", "isa-debug-exit,iobase=0xf4,iosize=0x04", "-kernel", image, *case["qemu_args"]], 1, 10)
            require(result.stdout == case["stdout"].encode() and not result.stderr, f"QEMU output mismatch: {case['name']}")
            self.passed(case["name"])
        check_output(self)
        check_mutations(self)
        check_semantics(self)
        self.report["acceptance"] = "passed"
        self.save()


def storage_report(measured, manifest):
    for name, maximum in manifest["storage"]["limits"].items():
        require(isinstance(measured.get(name), int) and 0 <= measured[name] <= maximum, f"compiler exceeds {name} storage limit")
        require(measured.get("max_" + name) == maximum, f"manifest/compiler {name} capacity differs")
    require(measured.get("source_bytes", manifest["input_max_bytes"]+1) <= manifest["input_max_bytes"], "compiler exceeds source limit")
    require(measured["storage_bytes"] <= manifest["storage"]["work_arena_bytes"], "compiler storage exceeds work arena")
    require(measured.get("max_locals") == manifest["storage"]["limits"]["symbols"], "codegen local capacity differs from manifest")
    require(measured["local_bytes"] <= measured["local_arena_bytes"], "compiler local storage exceeds arena")


def compiler_closure(root, manifest):
    visited, ordered = set(), []
    def visit(name):
        seed.relative(name)
        if name in visited:
            return
        visited.add(name)
        text = (root / name).read_text()
        for imported in re.findall(r'^\s*import\s+"([^"\n]+)"', text, re.MULTILINE):
            visit("lib/" + seed.relative(imported) + ".gsl")
        ordered.append(name)
    visit(manifest["entry"])
    require(ordered == manifest["source_files"], "compiler source closure differs from manifest")
    require(not visited.intersection(manifest["excluded_modules"]), "excluded module entered compiler closure")
    return ordered


def install_compiler(root, output):
    require(platform.system() == "Linux" and platform.machine() == "x86_64", "initial compiler requires Linux x86_64")
    require(not output.exists(), f"output already exists: {output}")
    record = seed.read_json(root / "test/bootstrap/compiler.json")
    compressed = (root / "test/bootstrap/compiler.gz").read_bytes()
    require(seed.sha(compressed) == record["compressed_sha256"], "initial compiler archive checksum mismatch")
    data = gzip.decompress(compressed)
    require(seed.sha(data) == record["compiler_sha256"], "initial compiler checksum mismatch")
    require(data[:6] == b"\x7fELF\x02\x01" and data[18:20] == b"\x3e\x00", "initial compiler is not ELF64 x86_64")
    output.mkdir(parents=True)
    compiler = output / "compiler"
    compiler.write_bytes(data)
    compiler.chmod(0o755)
    print(compiler)


def kernel_ir(text):
    for value in ['target triple = "x86_64-unknown-none-elf"', 'noredzone', '"target-features"="-x87,-sse,-sse2,-mmx,-avx"']:
        require(value in text, f"missing kernel invariant: {value}")
    require("noimplicitfloat" in text, "missing kernel invariant: noimplicitfloat")
    require(all(re.search(r' #0(?: section "[^"\n]+")? \{$', line) for line in text.splitlines() if line.startswith("define ")), "function lacks kernel attributes")
    require('asm sideeffect "syscall"' not in text, "Linux syscall in kernel IR")


def kernel_assembly(text):
    require(not re.search(r'\bsyscall\b|%(?:xmm|ymm|zmm|mm)[0-9]|%st(?:\(|\b)', text), "kernel contains syscall/SIMD/x87")
    for line in text.splitlines():
        fields = line.split("\t")
        if len(fields) >= 3 and re.match(r'^\s*[0-9a-f]+:', fields[0]):
            instruction = fields[-1].strip().split()
            require(not instruction or not instruction[0].startswith("f"), "kernel contains x87 instruction")


def kernel_elf(data, *, entry="_start", fixture=True):
    require(data[:6] == b"\x7fELF\x02\x01", "kernel is not ELF64 little endian")
    header = struct.unpack_from("<16sHHIQQQIHHHHHH", data)
    require(header[1:3] == (2, 62), "kernel is not static x86_64 EXEC")
    for index in range(header[10]):
        offset = header[5] + index * header[9]
        require(struct.unpack_from("<I", data, offset)[0] not in [2, 3], "kernel has dynamic runtime")
    names = {}
    sections = [struct.unpack_from("<IIQQQQIIQQ", data, header[6] + i * header[11]) for i in range(header[12])]
    section_names = sections[header[13]]
    strings = data[section_names[4]:section_names[4] + section_names[5]]
    for section in sections:
        name = strings[section[0]:].split(b"\0", 1)[0]
        if fixture and name in [b".text", b".rodata", b".data", b".bss"]:
            require(section[3] % 4096 == 0, "kernel section alignment")
        if section[1] == 2:
            table = sections[section[6]]
            strings_sym = data[table[4]:table[4] + table[5]]
            for offset in range(section[4], section[4] + section[5], section[9]):
                name_offset, _, _, index, value, _ = struct.unpack_from("<IBBHQQ", data, offset)
                symbol = strings_sym[name_offset:].split(b"\0", 1)[0].decode()
                require(not symbol or index != 0, f"unresolved symbol: {symbol}")
                names[symbol] = value
    require(names.get(entry) == header[4] and header[4] != 0, "invalid kernel entry")
    if not fixture:
        return
    start, end = names.get("__kernel_start", 0), names.get("__kernel_end", 0)
    require(start and end > start and start % 4096 == end % 4096 == 0, "kernel bounds")
    for name in ["kernel_gdt", "kernel_idt", "kernel_gdtr", "kernel_idtr", "interrupt_stub", "kernel_main"]:
        require(names.get(name), f"missing kernel symbol: {name}")
    require(any(struct.unpack_from("<III", data, i)[0] == 0x1BADB002 and sum(struct.unpack_from("<III", data, i)) % 2**32 == 0 for i in range(0, min(len(data)-12, 8192), 4)), "missing Multiboot1 header")
    multiboot2 = False
    for offset in range(0, min(len(data)-24, 32768), 8):
        magic, architecture, length, checksum = struct.unpack_from("<IIII", data, offset)
        if magic == 0xE85250D6 and architecture == 0 and length >= 24 and offset+length <= len(data) and (magic+architecture+length+checksum) % 2**32 == 0 and data[offset+16:offset+24] == b"\0\0\0\0\x08\0\0\0":
            multiboot2 = True
            break
    require(multiboot2, "missing Multiboot2 header")


def capacity_source(name, manifest):
    limits = manifest["storage"]["limits"]
    if name == "deep_parentheses":
        return "func main() u32 {return " + "(" * 8000 + "1" + ")" * 8000 + ";}"
    if name == "deep_prefix":
        return "func main() u32 {return " + "-" * 8000 + "1;}"
    if name == "deep_else_if":
        return "func main() {" + "if false {} else " * 3000 + "{} }"
    if name == "deep_blocks":
        return "func main() {" + "unsafe {" * 3000 + "}" * 3001
    if name == "deep_operator_tree":
        return "func main() u32 {return " + "+".join(["1"] * 4000) + ";}"
    if name == "deep_cast_tree":
        return "func main() u32 {return 1" + " as u32" * 4000 + ";}"
    if name == "section_function_capacity":
        return "".join(f'func f{i}() {{}}' for i in range(limits["functions"])) + 'section(".text") func extra() {}'
    if name == "input_bytes":
        return " " * (manifest["input_max_bytes"] + 1)
    if name == "expressions":
        return "func main() { var x: u32; " + "x=1+1+1+1+1+1+1+1;" * (limits[name] // 14 + 1) + "}"
    if name == "statements":
        return "func main() { while false {" + "break;" * (limits[name] + 1) + "} }"
    if name == "parameters":
        return "func main(" + ",".join(f"x{i}:u32" for i in range(limits[name]+1)) + ") {}"
    if name == "functions":
        return "".join(f"func f{i}() {{}}" for i in range(limits[name]+1))
    if name == "constants":
        return "".join(f"const c{i}:u32=0;" for i in range(limits[name]+1)) + "func main() {}"
    if name == "fields":
        return "struct Item {" + ",".join(f"x{i}:u32" for i in range(limits[name]+1)) + "} func main() {}"
    if name == "structs":
        return "".join(f"struct S{i} {{value:u32}}" for i in range(limits[name]+1)) + "func main() {}"
    if name == "symbols":
        return "func main() {" + "".join(f"var x{i}:u32;" for i in range(limits[name]+1)) + "}"
    if name == "array_types":
        return "func main() {" + "".join(f"var x{i}:[{i+1}]u8;" for i in range(limits["structs"]+1)) + "}"
    if name == "array_layout_overflow":
        return "struct S0 {value:u64}" + "".join(f"struct S{i} {{left:S{i-1},right:S{i-1}}}" for i in range(1,15)) + "func main() {var data:[65535]S14;}"
    raise seed.SeedError(f"unknown capacity generator: {name}")


def main(argv=None, installed=False):
    parser = argparse.ArgumentParser(description="Build GSL source with the installed compiler." if installed else __doc__)
    parser.add_argument("command", choices=["build"] if installed else ["bootstrap", "build", "test"])
    parser.add_argument("source", nargs="?", type=Path)
    if installed:
        parser.set_defaults(seed=None, compiler=ROOT / "compiler", root=ROOT)
    else:
        selection = parser.add_mutually_exclusive_group()
        selection.add_argument("--seed", type=Path)
        selection.add_argument("--compiler", type=Path)
        parser.add_argument("--root", type=Path, default=ROOT)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--target", choices=TARGETS, default="hosted")
    parser.add_argument("--clang", default="clang")
    parser.add_argument("--linker", default="ld.lld")
    parser.add_argument("--llvm-version")
    parser.add_argument("--emit", choices=["executable", "object"], default="executable")
    parser.add_argument("--boot", type=Path)
    parser.add_argument("--linker-script", type=Path)
    parser.add_argument("--object", dest="objects", action="append", type=Path, default=[])
    parser.add_argument("--entry", default="_start")
    args = parser.parse_args(argv)
    runner = None
    existed = args.output.exists()
    try:
        if args.command == "bootstrap":
            install_compiler(args.root.resolve(), args.output.resolve())
            return 0
        custom = args.boot or args.linker_script or args.objects or args.entry != "_start"
        require(not (custom or args.emit == "object") or (args.command == "build" and args.target == "kernel"), "native kernel options require build --target kernel")
        require(not custom or args.linker_script, "custom linking requires --linker-script")
        require(args.emit != "object" or not custom, "object emission does not accept link inputs")
        require(re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", args.entry), "invalid entry symbol")
        for path in [args.boot, args.linker_script, *args.objects]:
            require(path is None or path.is_file(), f"missing native input: {path}")
        runner = Runner(args)
        if args.command == "test":
            runner.accept(args)
        else:
            require(args.source is not None, "build requires a source")
            result = runner.build(args.source.resolve(), "program", args.target, object_only=args.emit == "object",
                                  boot_source=args.boot.resolve() if args.boot else None,
                                  linker_script=args.linker_script.resolve() if args.linker_script else None,
                                  objects=[path.resolve() for path in args.objects], entry=args.entry)
            runner.passed("build")
            print(result)
        return 0
    except (seed.SeedError, OSError, ValueError, KeyError, TypeError, struct.error) as error:
        if runner:
            runner.report["failure"] = str(error)
            runner.save()
        elif not existed and args.output.is_dir():
            report_path = args.output / "report.json"
            report = seed.read_json(report_path) if report_path.exists() else {}
            report["failure"] = str(error)
            report_path.write_text(json.dumps(report, indent=2)+"\n")
        location = f"Artifacts retained: {args.output.resolve()}" if args.output.exists() else "No artifacts created"
        print(f"gsl: {error}\n{location}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
