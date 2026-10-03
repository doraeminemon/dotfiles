# Command output

## Environment & Tooling Constraints

- **Rust Token Killer (RTK) Active:** The terminal environment utilizes RTK to aggressively intercept, strip, and compress verbose tool/command outputs (logs, test results, build outputs, and Git status).
- **Output Truncation is Expected:** If tool or terminal responses look highly minimalist, abbreviated, or missing typical verbose details, **do not question it, re-query, or assume an error occurred**. It is filtered by design to optimize context window efficiency.
- **Do Not Debug the Output Pipeline:** Avoid commenting on, asking about, or attempting to troubleshoot missing terminal context unless explicitly asked.
- **If bypass is needed:** Add "Run RTK_DISABLED=1 pnpm test" or "Check git diff using RTK_DISABLED=1 git diff"
- Command output here is condensed to save tokens, keeping every signal and
  dropping costly noise. Treat it as the complete result: run commands
  normally, and batch related commands into one call to avoid extra turns.
  Truncated results state their recovery path in their own output. Re-run a
  command as `rtk proxy <cmd>` only when its result is unusable: empty when
  output was clearly expected, contradicting its exit code, or garbled.
