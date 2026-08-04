# Contributing

Contributions that improve correctness, reproducibility, observability, or the
documented evidence boundary are welcome.

## Development Workflow

1. Create a branch from `main`.
2. Keep changes scoped and add tests for behavioral changes.
3. Run the full repository gate in Ubuntu or WSL2:

   ```bash
   make verify
   ```

4. Update documentation, version locks, and third-party notices when a runtime
   dependency or pinned artifact changes.
5. Open a pull request that explains the behavior, validation performed, and
   evidence level supported by the change.

## Evidence Rules

- Do not describe fake-backend or single-accelerator results as multi-endpoint
  production performance evidence.
- Keep H1-H3 benefit claims at the L3 gate until two independent real
  accelerator endpoints have been tested.
- Record immutable versions, workload definitions, and clean-commit status for
  committed evidence.
- Do not commit model weights, raw private prompts, credentials, Authorization
  headers, host-specific paths, or large raw benchmark output.

Report security issues through the private process in `SECURITY.md`, not through
a public issue.

By contributing, you agree that your contribution is licensed under the Apache
License 2.0 included in this repository.
