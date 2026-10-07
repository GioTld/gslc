"""Compare generated integer programs against Python arithmetic, independently of LLVM."""
import itertools
import json
import signal

import seed

TYPES = [(f"{prefix}{bits}", bits, prefix == "i")
         for bits in [8, 16, 32, 64] for prefix in ["u", "i"]] + [
             ("usize", 64, False), ("isize", 64, True)]
OPERATORS = ["+", "-", "*", "/", "%", "&", "|", "^", "<<", ">>",
             "==", "!=", "<", "<=", ">", ">="]
PRELUDE = '''import "std/fs"
func emit(value: u64) {
    unsafe { write(1 as u64, (&value) as *u8, 8 as usize); }
}
'''


class ArithmeticTrap(Exception):
    pass


def integer(value, bits, signed):
    value %= 2 ** bits
    if signed and value >= 2 ** (bits - 1):
        value -= 2 ** bits
    return value


def evaluate(op, a, b, bits, signed):
    if op in ["/", "%"]:
        if b == 0 or (signed and a == -(2 ** (bits - 1)) and b == -1):
            raise ArithmeticTrap()
        quotient = abs(a) // abs(b)
        if (a < 0) != (b < 0):
            quotient = -quotient
        result = quotient if op == "/" else a - quotient * b
    elif op in ["<<", ">>"]:
        if b < 0 or b >= bits:
            raise ArithmeticTrap()
        result = a << b if op == "<<" else a >> b
    elif op in ["==", "!=", "<", "<=", ">", ">="]:
        return {"==": a == b, "!=": a != b, "<": a < b,
                "<=": a <= b, ">": a > b, ">=": a >= b}[op]
    else:
        result = {"+": lambda: a + b, "-": lambda: a - b, "*": lambda: a * b,
                  "&": lambda: a & b, "|": lambda: a | b, "^": lambda: a ^ b}[op]()
    return integer(result, bits, signed)


def literal(value, type_name, bits):
    return f"({value % (2 ** bits)} as u{bits}) as {type_name}"


def integer_program(type_name, bits, signed):
    functions, calls, results, cases, traps = [], [], [], [], []
    values = sorted({integer(x, bits, signed) for x in
                     [0, 1, 2, 2 ** (bits - 1) - 1, 2 ** (bits - 1), 2 ** bits - 1]})
    for index, op in enumerate(OPERATORS):
        boolean = op in ["==", "!=", "<", "<=", ">", ">="]
        body = f"if a {op} b {{ return 1; }} return 0;" if boolean else f"return (a {op} b) as u64;"
        function = f"func op{index}(a: {type_name}, b: {type_name}) u64 {{ {body} }}\n"
        functions.append(function)
        right_values = [-1, 0, 1, bits - 1, bits] if op in ["<<", ">>"] and signed else ([0, 1, bits - 1, bits] if op in ["<<", ">>"] else values)
        trapped = set()
        for a, b in itertools.product(values, right_values):
            call = f"emit(op{index}({literal(a, type_name, bits)}, {literal(b, type_name, bits)}));"
            case = {"type": type_name, "operation": op, "left": a, "right": b}
            try:
                value = evaluate(op, a, b, bits, signed)
            except ArithmeticTrap:
                reason = "zero" if b == 0 else "overflow" if op in ["/", "%"] else "negative" if b < 0 else "width"
                if reason not in trapped:
                    traps.append((f"{type_name}-op{index}-{reason}", PRELUDE + function + "func main() u32 {" + call + "return 0;}", case))
                    trapped.add(reason)
                continue
            calls.append(call)
            results.append(int(value) % (2 ** 64))
            cases.append(case)
    for op, name in [("-", "negate"), ("~", "complement")]:
        functions.append(f"func {name}(value: {type_name}) u64 {{return ({op}value) as u64;}}\n")
        for value in values:
            calls.append(f"emit({name}({literal(value, type_name, bits)}));")
            results.append(integer(-value if op == "-" else ~value, bits, signed) % (2 ** 64))
            cases.append({"type": type_name, "operation": name, "value": value})
    for target, target_bits, target_signed in TYPES:
        functions.append(f"func cast_{target}(value: {type_name}) u64 {{return (value as {target}) as u64;}}\n")
        for value in values:
            calls.append(f"emit(cast_{target}({literal(value, type_name, bits)}));")
            results.append(integer(value, target_bits, target_signed) % (2 ** 64))
            cases.append({"type": type_name, "operation": "cast", "target": target, "value": value})
    program = PRELUDE + "".join(functions) + "func main() u32 {\n" + "\n".join(calls) + "\nreturn 0;}\n"
    return program, results, cases, traps


def check_semantics(runner):
    work = runner.work / "semantics"
    work.mkdir()
    groups = []
    traps = []
    for type_name, bits, signed in TYPES:
        program, expected, cases, failures = integer_program(type_name, bits, signed)
        groups.append((type_name, program, expected, cases))
        traps.extend(failures)
    effects = runner.root / "test/bootstrap/acceptance/evaluation_order.gsl"
    groups.append(("evaluation-order", effects.read_text(), [0, 0, 1, 123, 77, 12343, 77], []))
    observations = 0
    for name, program, expected, cases in groups:
        source = work / (name + ".gsl")
        source.write_text(program)
        expected_bytes = b"".join(value.to_bytes(8, "little") for value in expected)
        (work / (name + ".expected.bin")).write_bytes(expected_bytes)
        (work / (name + ".cases.json")).write_text(json.dumps(cases, indent=2) + "\n")
        for optimization in ["-O0", "-O2"]:
            binary = runner.build(source, "semantics/" + name + optimization, "hosted", optimization)
            result = runner.run([binary], timeout=5)
            (work / (name + optimization + ".actual.bin")).write_bytes(result.stdout)
            if result.stdout != expected_bytes or result.stderr:
                raise seed.SeedError(f"semantic mismatch: {source}: {optimization}; see expected/actual bytes and cases")
            runner.passed("semantics-" + name + optimization)
        observations += len(expected)
    for name, program, case in traps:
        source = work / (name + ".gsl")
        source.write_text(program)
        (work / (name + ".case.json")).write_text(json.dumps(case) + "\n")
        for optimization in ["-O0", "-O2"]:
            binary = runner.build(source, "semantics/" + name + optimization, "hosted", optimization)
            result = runner.run([binary], status=-signal.SIGILL, timeout=5)
            if result.stdout or result.stderr:
                raise seed.SeedError(f"trap followed a visible side effect: {source}")
            runner.passed("semantics-trap-" + name + optimization)
    runner.report["semantic_results"] = {"values_per_optimization": observations,
                                          "trap_programs": len(traps), "optimizations": ["-O0", "-O2"]}
    runner.save()
