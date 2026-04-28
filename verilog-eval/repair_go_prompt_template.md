# Repair Go Prompt Template

Use this prompt when repairing an existing MyGO Go file.

## Universal Repair Prompt

```text
Repair the existing MyGO Go file so that it matches the reference behavior exactly.

Hard requirements:
- Return one complete Go file only.
- Keep `package main`.
- Keep exactly one `func TopModule(...)`.
- Keep outputs as package-level `out_*` globals only.
- Preserve the existing module interface and output names exactly.

Code quality requirements:
- The repaired Go must use common, standard, RTL-like structure.
- Do not use weird tricks, ad-hoc hacks, obscure patterns, or test-specific special cases.
- Do not write code that only tries to game the testbench.
- The structure must be easy to read and look like a normal hardware design written in MyGO.

Synthesizability requirements:
- The design must be synthesizable.
- Use a conventional hardware structure:
  1. package-level registers/state for persistent sequential state,
  2. combinational next-state / next-value logic,
  3. clocked state updates on the correct edge,
  4. straightforward output logic.
- Do not use dynamic allocation, maps, recursion, goroutines, interfaces, structs-as-ports, pointers for outputs, or any software-style workaround.
- Do not depend on non-synthesizable behavior, implicit simulator quirks, or undefined values.
- Do not use unusual coding patterns unless they are clearly required by the circuit itself.

Style requirements:
- Prefer simple conditionals, small helper temporaries, and explicit state transitions.
- Prefer clear package-level register names for sequential state.
- For sequential designs, do not keep persistent state in local variables inside `TopModule`.
- For combinational designs, do not introduce fake sequential state.
- For vector logic, preserve exact bit ordering, boundary behavior, and wraparound rules.

Behavior requirements:
- Match reset semantics exactly.
- Match clock-edge semantics exactly.
- Match Moore/Mealy timing exactly when relevant.
- Match all counters, rollover behavior, shift behavior, and state transitions exactly.

Return one complete repaired Go file only.
```

## Recommended Validation Prompt Suffix

Append this text after the case-specific instructions:

```text
The repair is only successful if it passes recheck with zero mismatches, no compile failures, and no simulation timeout.
```
