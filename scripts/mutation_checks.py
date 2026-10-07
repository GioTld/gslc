"""Deterministic malformed-source checks; generated reproducers stay in the report directory."""
import json
import re

import seed

FIXTURES = ["test/bootstrap/acceptance/logical.gsl",
            "test/fixtures/arrays.gsl", "test/fixtures/packed.gsl"]
TOKEN = re.compile(rb'"(?:\\.|[^"\\])*"|[A-Za-z_][A-Za-z_0-9]*|[0-9]+|[^\s]')


def mutations(source):
    yield "original", source
    tokens = list(TOKEN.finditer(source))
    # Evenly spaced positions keep the corpus bounded as fixtures grow.
    positions = sorted({i * (len(tokens) - 1) // 15 for i in range(16)})
    for index in positions:
        token = tokens[index]
        start, end = token.span()
        yield f"truncate-{index}", source[:start]
        yield f"delete-{index}", source[:start] + source[end:]
        yield f"replace-{index}", source[:start] + b"@" + source[end:]
    for suffix in [b'"', b"/*", b"(", b"}"]:
        yield "suffix-" + suffix.hex(), source + suffix


def check_mutations(runner):
    work = runner.work / "mutations"
    work.mkdir()
    counts = {"accepted": 0, "rejected": 0}
    for fixture in FIXTURES:
        source_bytes = (runner.root / fixture).read_bytes()
        for label, data in mutations(source_bytes):
            name = fixture.rsplit("/", 1)[-1][:-4] + "-" + label
            source, output = work / (name + ".gsl"), work / (name + ".ll")
            source.write_bytes(data)
            outcomes = []
            for compiler in [runner.compiler, runner.compare]:
                output.write_bytes(b"preserved")
                result = runner.run([compiler, "--diagnostics=json", source, output],
                                    status=None, timeout=5)
                if result.returncode not in [0, 1, 2, 4]:
                    raise seed.SeedError(f"mutation failed unexpectedly: {source}: {result.returncode}")
                if label == "original" and result.returncode != 0:
                    raise seed.SeedError(f"unmodified control fixture failed: {source}")
                if result.stderr:
                    raise seed.SeedError(f"unexpected mutation stderr: {source}")
                contents = output.read_bytes()
                if result.returncode:
                    if contents != b"preserved":
                        raise seed.SeedError(f"rejected mutation changed output: {source}")
                    try:
                        diagnostic = json.loads(result.stdout)
                        valid = (diagnostic["code"].startswith("E") and
                                 isinstance(diagnostic["message"], str) and
                                 0 <= diagnostic["span"]["start"] <= diagnostic["span"]["end"] <= len(data))
                    except (ValueError, KeyError, TypeError, AttributeError):
                        valid = False
                    if not valid:
                        raise seed.SeedError(f"invalid mutation diagnostic: {source}")
                elif result.stdout or contents == b"preserved":
                    raise seed.SeedError(f"invalid mutation success: {source}")
                outcomes.append((result.returncode, result.stdout, contents))
            if outcomes[0] != outcomes[1]:
                raise seed.SeedError(f"compiler generations disagree on mutation: {source}")
            if outcomes[0][0] == 0:
                runner.run([runner.clang, "-Wno-override-module", "-c", output, "-o", work / "verified.o"], timeout=10)
                counts["accepted"] += 1
            else:
                counts["rejected"] += 1
            runner.passed("mutation-" + name)
    runner.report["mutations"] = counts
    runner.save()
