# DeepSeek Spec-to-RTL Workflow

## Purpose

This document records the end-to-end workflow used in this workspace to:

1. read a benchmark prompt and reference Verilog
2. ask an LLM to generate MyGO-compatible Go code
3. compile the generated Go code into Verilog using `mygo`
4. compare the generated Verilog against the provided Verilog testbench

## Dataset Layout

Source dataset:

- [../dataset_spec-to-rtl](../dataset_spec-to-rtl)

Organized as:

- [../dataset_spec-to-rtl/prompts](../dataset_spec-to-rtl/prompts)
- [../dataset_spec-to-rtl/refs](../dataset_spec-to-rtl/refs)
- [../dataset_spec-to-rtl/tests](../dataset_spec-to-rtl/tests)
- [../dataset_spec-to-rtl/misc](../dataset_spec-to-rtl/misc)

Counts:

- prompts: 156
- refs: 156
- tests: 156

## Script Used

Automation script:

- [../scripts/deepseek_spec_to_rtl_eval.py](../scripts/deepseek_spec_to_rtl_eval.py)

What it does for each case:

1. load `*_prompt.txt`
2. load `*_ref.sv`
3. call DeepSeek Chat Completions API
4. write `main.go`
5. compile using:
   - `go run ./cmd/mygo compile -emit=verilog -o <case>.sv <main.go>`
6. rewrite top-level module name from `main` to `TopModule` if needed
7. run:
   - `iverilog -g2012 ...`
   - `vvp ...`
8. collect results into `results.json`

## MyGO DSL Constraints Used In Prompting

The prompt now explicitly constrains the LLM to generate code in the style accepted by the current MyGO compiler.

Core rules:

- file must start with `package main`
- hardware entry must be `func TopModule(...)`
- inputs must be ordinary `TopModule` parameters
- outputs must be package-level globals beginning with `out_`
- `TopModule` must assign to `out_*` globals directly
- no return-value outputs
- no pointer parameters used as ports
- no structs, interfaces, maps, `select`, recursion, or dynamic goroutine patterns
- add `func main() {}` as an empty stub only

Good pattern:

```go
package main

var out_zero bool

func TopModule() {
    out_zero = false
}

func main() {}
```

Bad patterns:

- `func TopModule() bool`
- `func TopModule(out *bool)`
- `type Ports struct { ... }`

## Compiler Fixes Applied So Workflow Can Run

To make the workflow usable, the compiler was repaired so that:

- `TopModule` is auto-detected as the preferred hardware entry
- `TopModule` parameters are exported as input ports
- `out_*` globals are exported as output ports

This was necessary because otherwise generated Verilog used the wrong top module name or exported the wrong interface.

## Current Result Directories

All evaluation outputs are now grouped under:

- [../runs](../runs)

Subdirectories:

- [../runs/pilot_3_cases](../runs/pilot_3_cases)
- [../runs/run_20_cases](../runs/run_20_cases)

## Verified Results So Far

### Pilot: 3 cases

Location:

- [../runs/pilot_3_cases/results.json](../runs/pilot_3_cases/results.json)

Summary:

- total: 3
- equivalent: 3
- pass rate: 100%

Cases:

- `Prob001_zero`
- `Prob002_m2014_q4i`
- `Prob003_step_one`

### Run: first 20 cases

Location:

- [../runs/run_20_cases/results.json](../runs/run_20_cases/results.json)

Summary:

- total: 20
- equivalent: 12
- not equivalent: 5
- Go compile failed: 2
- Icarus compile failed: 1

Equivalent cases:

- `Prob001_zero`
- `Prob002_m2014_q4i`
- `Prob003_step_one`
- `Prob005_notgate`
- `Prob007_wire`
- `Prob008_m2014_q4h`
- `Prob010_mt2015_q4a`
- `Prob011_norgate`
- `Prob012_xnorgate`
- `Prob013_m2014_q4e`
- `Prob014_andgate`
- `Prob019_m2014_q4f`

Failed cases:

- `Prob004_vector2` - not equivalent
- `Prob006_vectorr` - Go compile failed
- `Prob009_popcount3` - Go compile failed
- `Prob015_vector1` - Icarus compile failed
- `Prob016_m2014_q4j` - not equivalent
- `Prob017_mux2to1v` - not equivalent
- `Prob018_mux256to1` - not equivalent
- `Prob020_mt2015_eq2` - not equivalent

## Failure Types Observed

### 1. Go compile failures

Observed in:

- `Prob006_vectorr`
- `Prob009_popcount3`

Pattern:

- packed output width/type mismatch in generated MLIR
- this is likely a compiler/backend issue, not only a prompting issue

### 2. Iverilog compile failures

Observed in:

- `Prob015_vector1`

Pattern:

- multi-output interface not exported with expected port names
- this is often caused by LLM output modeling not matching the benchmark structure

### 3. Behavioral mismatches

Observed in:

- `Prob004_vector2`
- `Prob016_m2014_q4j`
- `Prob017_mux2to1v`
- `Prob018_mux256to1`
- `Prob020_mt2015_eq2`

Pattern:

- code compiles and simulates
- logic does not match reference behavior
- this is primarily LLM generation quality / prompt specificity

## Recommended Next Steps

### Short-term

1. keep using the current DSL-constrained prompt
2. expand from 20 cases to a larger sample only if desired
3. separately investigate the compiler issues for:
   - packed vector outputs
   - multi-output interface emission

### Medium-term

1. refine the prompt further for:
   - multi-bit vectors
   - multi-output modules
   - mux logic
   - add/sub and comparison patterns
2. rerun only failed cases after prompt updates
3. compare pass-rate improvements without rerunning everything immediately

## Reproduction Commands

### Pilot 3 cases

```bash
DEEPSEEK_API_KEY='<key>' python3 scripts/deepseek_spec_to_rtl_eval.py \
  --dataset-dir verilog-eval/spec_to_rtl/dataset/source_data \
  --output-dir verilog-eval/spec_to_rtl/runs/eval_runs/pilot_3_cases \
  --results-json verilog-eval/spec_to_rtl/runs/eval_runs/pilot_3_cases/results.json \
  --model deepseek-chat \
  --api-base https://api.deepseek.com \
  --limit 3 \
  --sleep-seconds 0.5
```

### First 20 cases

```bash
DEEPSEEK_API_KEY='<key>' python3 scripts/deepseek_spec_to_rtl_eval.py \
  --dataset-dir verilog-eval/spec_to_rtl/dataset/source_data \
  --output-dir verilog-eval/spec_to_rtl/runs/eval_runs/run_20_cases \
  --results-json verilog-eval/spec_to_rtl/runs/eval_runs/run_20_cases/results.json \
  --model deepseek-chat \
  --api-base https://api.deepseek.com \
  --limit 20 \
  --sleep-seconds 0.5
```

## Summary

The folder has now been organized and the workflow is stable enough to produce real benchmark results.

At the current state:

- the pipeline works end to end
- the first 3 cases pass
- the first 20 cases achieve 12/20 equivalence
- the remaining failures split cleanly into:
  - compiler/backend issues
  - LLM generation quality issues
