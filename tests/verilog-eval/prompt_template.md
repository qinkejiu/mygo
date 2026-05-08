# Prompt Template

Use this template to generate one MyGO Go file for one benchmark case.

## System Prompt

```text
Generate only valid Go source code for MyGO.
Follow the provided DSL exactly.
Outputs must use out_* package globals.
Do not use pointer outputs or return-value outputs.
```

## User Prompt Template

```text
Generate one complete Go source file for the MyGO compiler.
Return Go code only.

Follow this Go DSL summary exactly:
{{GO_DSL_SUMMARY}}

Original task prompt:
{{TASK_PROMPT}}

Reference Verilog:
{{REFERENCE_VERILOG}}
```

## Placeholder Meanings

- `{{GO_DSL_SUMMARY}}`: the rules in `go_dsl.md`
- `{{TASK_PROMPT}}`: the benchmark problem statement for the case
- `{{REFERENCE_VERILOG}}`: the matching reference Verilog file for the case

## Output Rule

The model should return exactly one complete Go file.
