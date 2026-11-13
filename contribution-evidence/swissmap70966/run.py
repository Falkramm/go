#!/usr/bin/env python3
"""Build and rerun the recorded SwissMap workloads using two built Go trees."""

import argparse
import collections
import hashlib
import json
import os
import pathlib
import re
import resource
import subprocess
import time


ROOT = pathlib.Path(__file__).resolve().parent
PLAN = json.loads((ROOT / "plan.json").read_text())
SUMMARY = json.loads((ROOT / "summary.json").read_text())
parser = argparse.ArgumentParser()
parser.add_argument("--baseline", required=True, type=pathlib.Path)
parser.add_argument("--patched", required=True, type=pathlib.Path)
parser.add_argument("--out", required=True, type=pathlib.Path)
parser.add_argument("--cpu", type=int, default=2)
parser.add_argument("--phase", choices=["all", "main", "prespecified", "fixed"], default="all")
parser.add_argument("--suite", choices=[suite["name"] for suite in PLAN["main"]["suites"]])
parser.add_argument("--skip-native386", action="store_true")
parser.add_argument("--build-only", action="store_true")
arguments = parser.parse_args()
TREES = {name: getattr(arguments, name).resolve() for name in ("baseline", "patched")}
OUTPUT = arguments.out.resolve()
OUTPUT.mkdir(parents=True, exist_ok=True)
(OUTPUT / "bin").mkdir(exist_ok=True)
BUILDS = {}
RECORDS = []


def save_records():
    (OUTPUT / "measurements.json").write_text(json.dumps(RECORDS, indent=2) + "\n")


def samples(text):
    result = []
    for line in text.splitlines():
        fields = line.split()
        if fields and fields[0].startswith("Benchmark") and len(fields) >= 4 and fields[1].isdigit():
            result.append({
                "name": re.sub(r"-\d+$", "", fields[0]),
                "iterations": int(fields[1]),
                "metrics": {fields[index]: float(fields[index - 1]) for index in range(3, len(fields), 2)},
            })
    return result


def cpu_snapshot():
    prefix = f"cpu{arguments.cpu} "
    line = next(line for line in pathlib.Path("/proc/stat").read_text().splitlines() if line.startswith(prefix))
    return [int(value) for value in line.split()[1:9]]


def runtime_environment():
    environment = os.environ.copy()
    environment.update(PLAN["environment"])
    return environment


def build(variant, artifact):
    name = artifact["name"]
    tree = TREES[variant]
    binary = OUTPUT / "bin" / f"{variant}-{name}.test"
    environment = os.environ.copy()
    environment.update({
        "GOROOT": str(tree), "GOENV": "off", "GOTOOLCHAIN": "local", "GOFLAGS": "", "GOWORK": "off",
        "GOMAXPROCS": "2", "GOOS": "linux", "GOARCH": artifact["goarch"], "GOAMD64": artifact["goamd64"],
        "GO386": "sse2", "GOEXPERIMENT": artifact["goexperiment"], "CGO_ENABLED": artifact["cgo_enabled"],
        "GO111MODULE": "off" if "probe" in artifact else "on",
    })
    cwd = ROOT / "probes" / artifact["probe"] if "probe" in artifact else tree / "src" / artifact["package"]
    command = [str(tree / "bin/go"), "test", "-p=2", "-c", "-o", str(binary)]
    if artifact.get("probe") == "stack":
        command.append("-gcflags=-m=2")
    if "package" in artifact:
        command.append(artifact["package"])
    with (OUTPUT / f"build-{variant}-{name}.txt").open("w") as output:
        subprocess.run(command, cwd=cwd, env=environment, stdout=output, stderr=subprocess.STDOUT, check=True)
    if artifact.get("probe") == "stack":
        subprocess.run([str(binary), "-test.run=^TestStackMapAllocations$", "-test.count=1"], env=environment, check=True)
    BUILDS[variant, name] = {"binary": binary, "cwd": cwd, "sha256": hashlib.sha256(binary.read_bytes()).hexdigest()}


def execute(variant, artifact, pattern, count, duration, extra, phase, suite, batch):
    built = BUILDS[variant, artifact]
    command = [
        "taskset", "-c", str(arguments.cpu), str(built["binary"]), "-test.run=^$", "-test.bench=" + pattern,
        "-test.benchmem", "-test.cpu=1", f"-test.count={count}", "-test.benchtime=" + duration, *extra,
    ]
    before_cpu = cpu_snapshot()
    before_usage = resource.getrusage(resource.RUSAGE_CHILDREN)
    before_wall = time.monotonic()
    completed = subprocess.run(command, cwd=built["cwd"], env=runtime_environment(), stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    elapsed = time.monotonic() - before_wall
    after_usage = resource.getrusage(resource.RUSAGE_CHILDREN)
    ticks = [end - start for start, end in zip(before_cpu, cpu_snapshot())]
    assert completed.returncode == 0 and completed.stdout.rstrip().endswith("PASS"), completed.stdout
    user = after_usage.ru_utime - before_usage.ru_utime
    system = after_usage.ru_stime - before_usage.ru_stime
    record = {
        "phase": phase, "suite": suite, "variant": variant, "batch": batch, "artifact": artifact,
        "pattern": pattern, "count": count, "duration": duration, "extra_arguments": extra,
        "exit_code": completed.returncode, "process_wall_seconds": elapsed,
        "process_user_seconds": user, "process_system_seconds": system, "process_cpu_seconds": user + system,
        "cpu_tick_deltas": ticks, "cpu_steal_percent": 100 * ticks[7] / sum(ticks) if sum(ticks) else 0,
        "observations": samples(completed.stdout),
    }
    RECORDS.append(record)
    save_records()
    return completed.stdout, record


def append(path, text):
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a") as output:
        output.write(text)


def run_main(suites):
    for suite in suites:
        print("RUN main / " + suite["name"], flush=True)
        for variant in TREES:
            (OUTPUT / f"main-{suite['name']}-{variant}.txt").write_text("")
        for batch, variant in enumerate(PLAN["main"]["order"], 1):
            for run in suite["runs"]:
                text, record = execute(variant, run["artifact"], run["pattern"], 3, run["duration"], run["extra_arguments"], "main", suite["name"], batch)
                assert collections.Counter(row["name"] for row in record["observations"]) == {name: 3 for name in run["expected_names"]}
                append(OUTPUT / f"main-{suite['name']}-{variant}.txt", text)


def run_prespecified(cases):
    for case in cases:
        print("RUN prespecified / " + case["name"], flush=True)
        for variant in TREES:
            (OUTPUT / f"prespecified-{case['name']}-{variant}.txt").write_text("")
        for batch, variant in enumerate(PLAN["prespecified"]["order"], 1):
            for request in case["patterns"]:
                text, record = execute(variant, case["artifact"], request["pattern"], 5, "1s", case["extra_arguments"], "prespecified", case["name"], batch)
                assert collections.Counter(row["name"] for row in record["observations"]) == {name: 5 for name in request["expected_names"]}
                append(OUTPUT / f"prespecified-{case['name']}-{variant}.txt", text)


def run_fixed(cases):
    for case in cases:
        print("RUN fixed / " + case["name"], flush=True)
        artifact = "stack-386" if case["goarch"] == "386" else "stack"
        for variant in TREES:
            (OUTPUT / f"fixed-{case['name']}-{variant}.txt").write_text("")
        for repeat in range(3):
            for variant in TREES:
                execute(variant, artifact, "^$", 1, "1x", [], "empty-harness", case["name"], repeat + 1)
        for cycle in range(5):
            for position, variant in enumerate(case["order"], 1):
                _, record = execute(variant, artifact, "^BenchmarkStackMap$/^" + case["workload"] + "$", 1, "20000000x", [], "fixed", case["name"], cycle * 4 + position)
                assert len(record["observations"]) == 1
                observation = record["observations"][0]
                assert observation["iterations"] == 20_000_000
                assert observation["metrics"]["B/op"] == observation["metrics"]["allocs/op"] == 0
                values = {
                    "process_cpu": record["process_cpu_seconds"] * 1e9 / 20_000_000,
                    "process_wall": record["process_wall_seconds"] * 1e9 / 20_000_000,
                    "benchmark_wall": observation["metrics"]["ns/op"],
                }
                lines = [f"BenchmarkFixedStack{case['workload']}/{metric} 20000000 {value:.9f} ns/op" for metric, value in values.items()]
                append(OUTPUT / f"fixed-{case['name']}-{variant}.txt", "\n".join(lines) + "\n")


def main():
    for name, expected in SUMMARY["modified_source_sha256"].items():
        assert hashlib.sha256((TREES["patched"] / name).read_bytes()).hexdigest() == expected, name
    for name, expected in SUMMARY["baseline_source_sha256"].items():
        assert hashlib.sha256((TREES["baseline"] / name).read_bytes()).hexdigest() == expected, name
    configuration = {
        "tested_base": PLAN["tested_base"], "full_patch_sha256": PLAN["full_patch_sha256"],
        "cpu_affinity": [arguments.cpu], "runtime_environment": PLAN["environment"],
        "phase": arguments.phase, "main_suite": arguments.suite,
        "skip_native386": arguments.skip_native386,
        "go_versions": {variant: subprocess.check_output([str(tree / "bin/go"), "version"], text=True).strip() for variant, tree in TREES.items()},
    }
    suites = [suite for suite in PLAN["main"]["suites"] if arguments.suite is None or suite["name"] == arguments.suite] if arguments.phase in {"all", "main"} else []
    cases = PLAN["prespecified"]["cases"] if arguments.phase in {"all", "prespecified"} else []
    fixed = [case for case in PLAN["fixed"] if not (arguments.skip_native386 and case["goarch"] == "386")] if arguments.phase in {"all", "fixed"} else []
    needed = {run["artifact"] for suite in suites for run in suite["runs"]} | {case["artifact"] for case in cases}
    needed.update("stack-386" if case["goarch"] == "386" else "stack" for case in fixed)
    artifacts = list(PLAN["artifacts"]) + [{"name": "stack-386", "probe": "stack", "goarch": "386", "goamd64": "v1", "goexperiment": "", "cgo_enabled": "0"}]
    for variant in TREES:
        for artifact in artifacts:
            if artifact["name"] in needed:
                print("BUILD " + variant + " / " + artifact["name"], flush=True)
                build(variant, artifact)
    configuration["executable_sha256"] = {f"{variant}/{artifact}": built["sha256"] for (variant, artifact), built in BUILDS.items()}
    (OUTPUT / "configuration.json").write_text(json.dumps(configuration, indent=2) + "\n")
    if arguments.build_only:
        return
    run_main(suites)
    run_prespecified(cases)
    run_fixed(fixed)
    for built in BUILDS.values():
        assert hashlib.sha256(built["binary"].read_bytes()).hexdigest() == built["sha256"]
    print("SELECTED CAMPAIGNS COMPLETE", flush=True)


if __name__ == "__main__":
    main()
