# Command output

RTK (Rust Token Killer) compresses command output: logs, test results, build output, git status. Short or abbreviated output is expected. Treat it as containing every signal you need. Do not re-run a command to see more, and do not comment on the filtering.

- Batch related commands into one call.
- A truncated result names its own recovery path, such as a log file to `tail`. Use that path.
- If a result is unusable, re-run it once as `rtk proxy <cmd>` for raw output. A result is unusable when it is empty but output was expected, contradicts its exit code, or is garbled.
- A non-zero exit code is a failure, even when the compressed output looks clean.
