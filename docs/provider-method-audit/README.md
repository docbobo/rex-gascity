# Provider method audit

This directory holds one report per `runtime-herdr-core` provider method in
[`docs/provider-herdr-method-io.md`](../provider-herdr-method-io.md). Each report
compares the method's documented registration, inputs, outputs, failure cases,
conflicts, and unverified items against the current `gc-runtime-rex`
implementation.

The compiled summary is [`docs/provider-herdr-method-deviations.md`](../provider-herdr-method-deviations.md).

Report format:

- verdict: `match`, `deviations`, or `not implemented`
- deviations table: doc claim, current implementation, behavioral impact, severity
- matches: important conformant behavior
- unverified: items code cannot settle

The audit is read-only. It does not change production code or parity verdicts.
