# Observability

This document is the operating guide for Codeplane observability. It is written for
two audiences:

- Humans debugging production or staging issues.
- LLMs and contributors adding or changing instrumentation.

## Goals

Codeplane observability is designed to provide:

- End-to-end trace propagation across HTTP, SSE, async goroutines, and service
  boundaries.
- Low-cardinality metrics that stay safe for Prometheus and remote backends.
- Useful logs with request, trace, workspace, session, and component
  correlation.
- Safe debug endpoints and recent-span inspection without leaking secrets.

## What Exists Today

The main implementation lives in
[`internal/observability/observability.go`](/Users/williamcory/crush/internal/observability/observability.go).

Key helpers:

- `Configure` / `Shutdown`: initialize and stop observability.
- `StartSpan`: create spans with the shared tracer.
- `WithRequestID`, `WithWorkspaceID`, `WithSessionID`, `WithTool`,
  `WithComponent`: attach correlation context.
- `ContextAttrs` and `LogAttrs`: emit logs with correlation IDs and trace IDs.
- `HTTPServerMiddleware`: extracts request context, starts server spans, and
  injects `X-Request-ID` and `X-Trace-ID`.
- `InstrumentedRoundTripper`: starts client spans and propagates headers.
- Redaction helpers:
  `RedactPayload`, `RedactHeaders`, `RedactURLString`.

## Human Workflow

Enable the local debug server in `codeplane.json`:

```json
{
  "$schema": "https://charm.land/codeplane.json",
  "options": {
    "observability": {
      "address": "127.0.0.1:9464",
      "trace_buffer_size": 1024,
      "trace_sample_ratio": 1
    }
  }
}
```

Useful endpoints:

- `/metrics`: Prometheus metrics.
- `/debug/traces`: recent in-memory spans.
- `/debug/vars`: expvar state.
- `/debug/pprof/`: runtime profiling.
- `/debug/observability`: current config and safety state.

Recommended debugging order:

1. Check `/metrics` for elevated error, retry, backlog, or reconnect counters.
2. Inspect `/debug/traces` for the request, workspace, or session path.
3. Use `request_id` or `trace_id` from logs to correlate across layers.
4. Use `pprof` only after confirming the issue is resource- or latency-related.

## LLM And Contributor Rules

When adding instrumentation:

- Prefer `observability.StartSpan` over ad hoc tracing.
- Preserve the current architecture. Do not replace the shared middleware or
  transport stack.
- Propagate context across goroutines, retries, channels, and reconnection
  loops.
- Use `observability.LogAttrs` instead of raw `slog.*` when request or trace
  context exists.
- Use `WithComponent` on background paths so logs and spans stay attributable.
- Keep metrics low-cardinality.

Good metric labels:

- Result categories such as `ok`, `error`, `timeout`, `canceled`,
  `not_found`, `unauthorized`.
- Stable component names such as `workspace_events_client`, `db`, `codeplane`,
  `copilot_oauth`.
- Stable operation names such as `create`, `delete`, `query_row`, `commit`.

Bad metric labels:

- Request IDs, trace IDs, session IDs, workspace IDs, file paths, prompts,
  URLs with unbounded values, raw error strings, provider responses.

Trace attributes may include higher-cardinality identifiers when they are
needed for debugging, but metrics must not.

## Secret Safety

Never emit secrets into logs, traces, metrics, or debug endpoints.

Follow these rules:

- Run URLs through `RedactURLString` before logging.
- Run headers through `RedactHeaders`.
- Run payloads and error bodies through `RedactPayload`.
- Do not attach raw prompts, auth headers, tokens, cookies, API keys, or full
  request bodies as span attributes.
- Prefer event names and bounded status fields over raw payload dumps.

## Instrumentation Checklist

For any new async or network boundary:

1. Start from the incoming `context.Context`.
2. Attach missing correlation context with `WithRequestID`, `WithWorkspaceID`,
   `WithSessionID`, or `WithComponent` when appropriate.
3. Start a span for the lifecycle, not just the transport call.
4. Record retries with bounded reasons.
5. Record duration and final result.
6. Log failures with redacted, structured context.
7. Add a regression test for propagation, reconnect, retry, or redaction.

## Current High-Value Signals

The current metrics and spans cover:

- HTTP server and client requests.
- SQLite connect, migrate, queries, and transactions.
- Pubsub publish, delivery, drops, and subscriber counts.
- SSE connections, events, reconnects, and stream duration.
- Permission backlog, queue delay, request outcome, and lifecycle spans.
- Background job lifecycle and duration.
- Workspace create and delete lifecycle.
- App shutdown result and partial-failure logging.

Background jobs do not use an internal queue. They have a hard concurrency cap;
when the limit is hit the manager rejects the start request and records a
`rejected_limit` lifecycle result instead of queueing hidden work.

## Review Standard

Any observability change should be rejected if it:

- leaks secrets;
- adds high-cardinality metric labels;
- breaks trace propagation on retries or goroutines;
- adds noisy info-level logs on hot paths;
- introduces a second instrumentation path where the shared helper already
  exists.
