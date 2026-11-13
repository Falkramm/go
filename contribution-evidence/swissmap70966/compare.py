#!/usr/bin/env python3
"""Recompute the 16 recorded baseline/patched comparisons with benchstat."""

import argparse
import json
import pathlib
import subprocess


parser = argparse.ArgumentParser()
parser.add_argument("--benchstat", default="benchstat")
arguments = parser.parse_args()
root = pathlib.Path(__file__).resolve().parent
plan = json.loads((root / "plan.json").read_text())
for pair in plan["sample_pairs"]:
    print(f"\n{pair['phase']} / {pair['name']}\n", flush=True)
    subprocess.run(
        [arguments.benchstat, pair["baseline"], pair["patched"]],
        cwd=root,
        check=True,
    )
