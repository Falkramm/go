# SwissMap zero-empty review evidence

This experiment encodes empty SwissMap control bytes as zero and updates the compiler, runtime, and GDB reader. The tested implementation uses byte-typed H2 and scalar first-free selection for small uint64-key maps. [Go issue #70966](https://github.com/golang/go/issues/70966)

The tested upstream base is `ddc6bf091828c267719f2a3b504460f8a1d2866e`. Both toolchains reported `go1.28-devel_ddc6bf09 Fri Nov 13 09:49:21 2026 -0800`. The bootstrap toolchain was Go 1.27.4.

The complete tested patch SHA256 is `3972d28a4b25633f6b499a07be57bc5eb269de23179485bd8f76d2bf4c7293a0`. The patch includes the compiler/runtime changes, control-byte tests, and GDB compatibility change. [Complete patch](zero-empty-swissmap.patch)

## Included evidence

- Standalone custom benchmarks cover capacity-hinted creation, growth, update, hit/miss lookup, and delete/insert for three key/value layouts. [Custom probe](probes/custom/map_benchmark_test.go)
- Five stack-map workloads check results and zero allocations for one, six, and eight uint64 entries, plus short and long string keys. [Stack probe](probes/stack/stack_test.go)
- The independent lifecycle probe uses a linear association-list oracle. It covers growth/splitting, deletion holes and replacement, clear, clone independence, indirect keys/values, map-only GC roots, iteration mutation, nil/interface keys, NaN, and signed zero. The small-uint64 test exercises all 511 deletion patterns across sizes zero through eight. [Lifecycle probe](probes/lifecycle/lifecycle_test.go)
- Raw main and prespecified samples retain every benchmark value and package/CPU header. Fixed-work files retain process CPU, process wall, and benchmark-loop wall metrics. [Sampling plan](plan.json)
- The machine-readable summary records all reported comparisons, source/probe SHA256 values, allocation ranges, and sampling limitations. Per-command and per-process numeric measurements retain CPU-time and steal diagnostics. [Summary](summary.json) · [Measurements](measurements.json)
- The local Delve companion and its tested scope are available for release coordination. [Companion evidence](coordination/README.md)

## Recorded sampling

The main series contains 80 workloads and 960 observations. Each suite used sequential baseline/patched/patched/baseline batches, with three observations per batch and six per implementation. Custom, application, and stack samples used 500 ms. Existing runtime samples used 300 ms. Build-time configurations include GOAMD64 v3, v4, and `nomapsplitgroup` v1. JSON, HTML template, and parser benchmarks use v1.

Six repeats were selected from earlier byte-H2 measurements before the final main series. They used the same four-batch order, five one-second observations per batch, and ten observations per implementation, giving 120 observations.

The native386 six-entry stack case and two amd64 followups each used five baseline/patched/patched/baseline cycles. Each process executed exactly 20,000,000 iterations, giving ten processes per implementation. The three fixed-work cases contain 60 benchmark processes. Each process contributes three correlated metric rows. Three empty-harness processes per implementation were recorded separately for each case. Process CPU includes startup, testing harness, and finalization.

The amd64 StringShort and Uint64Eight followups were selected after the final main series. StringShort had a +12.95% main signal with p = .041. Its fresh fixed-work repeat measured CPU −0.78% with p = .912 and benchmark wall −0.74% with p = .869. Uint64Eight had a +6.79% main point estimate with p = .132. Its fixed-work repeat measured CPU −2.74% with p = .052. The six-entry prespecified repeat measured +0.11% with p = .591. Native386 six-entry CPU measured −5.06% with p = .002.

The six capacity-65536 string-keyed 64-byte-value and 32-byte-keyed 256-byte-value creation cases improved by 33.27–47.51%, each with p = .002 and n = 6. All 80 main and six prespecified median allocation counts matched. Every baseline zero-allocation workload retained zero allocations.

One patched v3 uint64 growth sample recorded 47 allocs/op and 296092 B/op. Every baseline sample and the other five patched samples recorded 48 allocs/op and 296120 B/op. Both medians matched. Randomized map growth can vary counts. Parser and template B/op ranges are retained in the summary.

## Hardware and statistical scope

Measurements ran on a Linux/amd64 VM reporting Intel Xeon Gold 6238R at 2.20 GHz. Processes were pinned to CPU2 with `GOMAXPROCS=1`, `GOGC=100`, `GOMEMLIMIT=off`, and empty `GODEBUG`. Builds, correctness tests, and debugger work completed before timing.

Weighted CPU2 steal was 20.19% during the main series, 25.02% during prespecified repeats, 24.50% during native386, and 24.87–26.04% during amd64 followups. Per-workload hypothesis tests are unadjusted for screening multiplicity. VM variation produces broad intervals. Broad intervals leave performance equivalence unresolved. Performance coverage is native Linux/amd64 and the additional native linux/386 case. Recorded executable hashes identify the tested builds, and local build paths affect executable metadata.

## Recompute medians and p-values

The recorded tool was `golang.org/x/perf/cmd/benchstat` from `v0.0.0-20261009192801-be2c69fb417e`, built with Go 1.27.4. Install that version and recompute all 16 raw-file pairs.

```sh
BENCH_TOOLS=$(mktemp -d)
GOBIN="$BENCH_TOOLS" go install golang.org/x/perf/cmd/benchstat@v0.0.0-20261009192801-be2c69fb417e
python3 compare.py --benchstat "$BENCH_TOOLS/benchstat"
```

One pair can be inspected directly.

```sh
"$BENCH_TOOLS/benchstat" samples/main/v3-custom-baseline.txt samples/main/v3-custom-patched.txt
```

The 32 sample files contain 1080 direct main/repeat observations and 180 fixed-work metric rows derived from 60 processes. Probe source bytes preserve the tested SHA256 values. `manifest.json` records every public file identity.

## Build clean baseline and patched toolchains

These commands start in this evidence directory with Go 1.27.4 available as `go`. Linux/amd64 builds require the usual Go C toolchain. The full v4 suite requires a CPU supporting GOAMD64 v4 instructions. Native386 execution requires Linux IA32 support. Python 3 and `taskset` support the measurement runner.

```sh
EVIDENCE_DIR=$(pwd)
BENCH_WORK=$(mktemp -d)
BOOTSTRAP_ROOT=$(go env GOROOT)
GO_BASE=ddc6bf091828c267719f2a3b504460f8a1d2866e

git clone https://github.com/golang/go.git "$BENCH_WORK/go-baseline"
git -C "$BENCH_WORK/go-baseline" checkout --detach "$GO_BASE"
git clone "$BENCH_WORK/go-baseline" "$BENCH_WORK/go-patched"
git -C "$BENCH_WORK/go-patched" checkout --detach "$GO_BASE"
git -C "$BENCH_WORK/go-patched" apply --check "$EVIDENCE_DIR/zero-empty-swissmap.patch"
git -C "$BENCH_WORK/go-patched" apply "$EVIDENCE_DIR/zero-empty-swissmap.patch"

(
    cd "$BENCH_WORK/go-baseline/src"
    env -u GOROOT GOROOT_BOOTSTRAP="$BOOTSTRAP_ROOT" GOMAXPROCS=4 GOENV=off GOTOOLCHAIN=local GOFLAGS= GOEXPERIMENT= GODEBUG= GOOS=linux GOARCH=amd64 GOAMD64=v1 ./make.bash
)
(
    cd "$BENCH_WORK/go-patched/src"
    env -u GOROOT GOROOT_BOOTSTRAP="$BOOTSTRAP_ROOT" GOMAXPROCS=6 GOENV=off GOTOOLCHAIN=local GOFLAGS= GOEXPERIMENT= GODEBUG= GOOS=linux GOARCH=amd64 GOAMD64=v1 ./make.bash
)
```

The source hashes in `summary.json` permit checking the tested pristine and patched source files. Compiler and executable build flags are recorded in `plan.json` and implemented by `run.py`.

## Run the standalone correctness probes

```sh
for variant in baseline patched
do
    (
        cd "$EVIDENCE_DIR/probes/lifecycle"
        GO111MODULE=off GOMAXPROCS=4 "$BENCH_WORK/go-$variant/bin/go" test -count=3
        GO111MODULE=off GOMAXPROCS=4 GOEXPERIMENT=nomapsplitgroup "$BENCH_WORK/go-$variant/bin/go" test -count=3
    )
    (
        cd "$EVIDENCE_DIR/probes/stack"
        GO111MODULE=off "$BENCH_WORK/go-$variant/bin/go" test -run='^TestStackMapAllocations$' -count=1 -v
    )
done

(
    cd "$EVIDENCE_DIR/probes/lifecycle"
    GO111MODULE=off GOMAXPROCS=4 "$BENCH_WORK/go-patched/bin/go" test -race -count=1
)
```

The patched tree includes its control-byte and map regression tests. The standard whole-tree entry point is available after building.

```sh
(
    cd "$BENCH_WORK/go-patched/src"
    GOMAXPROCS=6 ./run.bash --no-rebuild
)
```

## Rerun the recorded benchmark selection

`run.py` first compiles the required artifacts using each supplied toolchain with `GOMAXPROCS=2` and `-p=2`. It then runs the selected campaigns serially. The compiled main/repeat binaries use CGO_ENABLED=1. The native386 pair uses GO386=sse2 and CGO_ENABLED=0. All execution uses `-test.run=^$`, `-test.benchmem`, and `-test.cpu=1`. Existing runtime cases pass `-mapbench`. The parser runs from the matching tree's `src/go/parser` directory and reads its `../printer/nodes.go` fixture.

```sh
python3 run.py --baseline "$BENCH_WORK/go-baseline" --patched "$BENCH_WORK/go-patched" --out "$BENCH_WORK/rerun" --cpu 2
```

`--phase` accepts `main`, `prespecified`, and `fixed`. `--suite` selects one main suite, and `--skip-native386` selects the remaining supported cases. `--cpu` chooses an available CPU for affinity. Exact regexes, expected names, durations, order, and sample counts are recorded in the portable plan. [Plan](plan.json)

```sh
python3 run.py --baseline "$BENCH_WORK/go-baseline" --patched "$BENCH_WORK/go-patched" --out "$BENCH_WORK/v3-custom" --cpu 2 --phase main --suite v3-custom
```
