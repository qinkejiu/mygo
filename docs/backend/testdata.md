# Backend Testdata Reference

The files under `internal/backend/testdata/` are minimal SystemVerilog fixtures that
exercise specific backend behaviors. They are intentionally tiny so unit tests can
copy or mutate them without invoking CIRCT.

| File | Purpose |
| ---- | ------- |
| `design_inline_fifo.sv` | Top-level design that already contains a concrete FIFO module. Used to verify that `stripAndWriteFifos` removes inline FIFOs and emits aux files instead. |
| `design_fifo_with_attrs.sv` | Similar to the inline design but with annotation comments that ensure attribute-driven stripping still behaves. |
| `fifo_impl_concrete.sv` | Simple fully elaborated FIFO implementation copied verbatim when tests only need an auxiliary file without templating. |
| `fifo_impl_external_stub.sv` | Represents a user-supplied FIFO body from outside the repo; backend tests copy it to confirm external sources remain untouched. |
| `fifo_impl_template_parametric.sv` | Parameterized FIFO that includes the `// mygo:fifo_template` marker. Unit tests feed it through the backend to ensure wrapper generation produces concrete modules (e.g., `mygo_fifo_i32_d1`). |

When adding new fixtures, keep names descriptive (`<role>_<detail>.sv`) and update this
table so contributors know which test exercises which behavior.
