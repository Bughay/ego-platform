# `react.go` — the DeepSeek ReAct agent engine

This file implements a **ReAct** (Reason + Act) agent loop. The agent talks to a
DeepSeek model, and on every turn the model returns a tiny JSON object that says
either *"call this tool with these args"* or *"I'm finished, here's the answer"*.
The engine executes requested tools, feeds the results back to the model as
observations, and repeats until the model finishes or a hard iteration cap is
hit.

It drives the EgoLifter trainer bot (`pkg/agent/workflows`) and the standalone
`agent-cli`. The LLM transport itself lives in `deepseek.go`
(`DeepseekOneshotJSON`, `DeepseekOneshotMemory`); the tool catalog rendering
lives in `tools.go` (`ToolsToLLMStringFromData`).

---

## Constants

| Constant | Value | Meaning |
|---|---|---|
| `maxAgentIterations` | 100 | Hard cap on reason→act cycles in one `Run`. |
| `iterationDelay` | 2s | Pause between successful iterations of the loop. |
| `retryDelay` | 5s | Backoff between failed attempts of a single step. |
| `maxRetries` | 3 | How many times one step is retried before giving up. |
| `memoryWindowSize` | 20 | Sliding-window size used by `trimMemory`. |

## Types

- **`AgentResponse{ Reasoning string; Act string }`** — the JSON contract the
  model must emit each turn. `Act` is either `"tool_name|args"` or
  `"finish|final answer"`.
- **`Agent{ ... }`** — the runtime config and state for one run:
  - `Model`, `MaxTokens` — which model and token budget.
  - `SystemPrompt` — the task description (e.g. the EgoLifter prompt).
  - `UserPrompt` — the current user request.
  - `Memory []Message` — the running conversation; **may be pre-seeded by the
    caller** with prior turns so the agent has context across messages.
  - `Thinking bool` — when true, the text-generation call runs as a DeepSeek
    reasoning ("thinking") model. The JSON repair call (`DeepseekOneshotJSON`) is
    always non-thinking regardless, since json_object mode breaks under reasoning.
  - `Tools []Tool` — parsed tool schemas (informational on the struct).
  - `Registry map[string]func(string) (string, error)` — maps a tool name to the
    Go function that actually executes it.
  - `SchemaData []byte` / `Path string` — the tool catalog JSON, embedded
    (preferred) or loaded from a file path (fallback).
- **`React{ Conversation []AgentResponse }`** — a small holder type (vestigial;
  not used by the loop).

---

## Functions

### `sleepCtx(ctx, d) error`
A sleep that can be cancelled. Used so the retry/iteration backoffs don't block
for the full delay if the context is cancelled.

```text
function sleepCtx(ctx, d):
    start timer for duration d
    wait for whichever happens first:
        ctx is cancelled  -> return ctx.Err()
        timer fires       -> return nil
```

### `(a *Agent) trimMemory()`
Caps unbounded memory growth during a long loop. Always keeps the first 2
messages (system prompt + first user message) and the most recent
`memoryWindowSize` messages; drops the middle.

```text
function trimMemory():
    head = 2
    if len(Memory) <= head + memoryWindowSize:
        return                      # nothing to trim
    keep first `head` messages
    keep last `memoryWindowSize` messages
    Memory = head ++ tail           # middle discarded
```

### `(a *Agent) convertTextToJSON(ctx, text) (string, error)`
A **repair step**, used only when the model's own text output did not already
parse as JSON. It runs a second model call whose only job is to re-express that
free text as the strict `{reasoning, act}` JSON shape, using the tool schemas as
a guide. (It is not the primary path — `attempt` parses the text directly first.)

```text
function convertTextToJSON(ctx, text):
    toolsStr = render tool catalog from SchemaData
    jsonMessages = [
        system: "convert the following text into this exact JSON schema
                 {reasoning, act}, here are the tool schemas: " + toolsStr,
        user:   text,
    ]
    result = DeepseekOneshotJSON(model, jsonMessages, temp=0.1, maxTokens)
    return result
```

### `stripCodeFence(s) string`
Removes a leading ```` ```json ```` / ```` ``` ```` fence and the trailing
```` ``` ```` that models sometimes wrap around JSON, returning the inner text.

```text
function stripCodeFence(s):
    s = trim(s)
    if s starts with "```":
        drop everything up to and including the first newline
        drop everything from the last "```" onward
    return trim(s)
```

### `parseAgentResponse(raw) (*AgentResponse, error)`
Turns raw model output into a typed `AgentResponse`: strip any code fence, then
JSON-unmarshal.

```text
function parseAgentResponse(raw):
    cleaned = stripCodeFence(raw)
    resp = jsonUnmarshal(cleaned) as AgentResponse
    if error: return error
    return resp
```

### `(a *Agent) attempt(ctx, messages) (*AgentResponse, error)`
**One step's worth of model interaction.** Text-first: generate plain text, then
try to parse it directly as the `{reasoning, act}` JSON. The text model almost
always emits that object already, and parsing it directly preserves a long final
answer **verbatim** instead of losing it in a re-encode. Only when the text is
NOT valid JSON does it fall back to `convertTextToJSON` (which needs `SchemaData`).
Real transport/API/context errors propagate so `oneloop` can retry the whole step.

```text
function attempt(ctx, messages):
    text = DeepseekOneshotMemory(model, messages, temp=0.1, maxTokens, a.Thinking)
    if error: return error                        # propagated for oneloop to retry

    resp = parseAgentResponse(text)               # try the text as-is
    if parsed ok: return resp                      # lossless happy path

    if no SchemaData:
        return error "text not parseable and no schema for conversion"
    converted = convertTextToJSON(ctx, text)      # repair step
    if error: return error
    return parseAgentResponse(converted)
```

### `(a *Agent) oneloop(ctx, messages) (*AgentResponse, error)`
**Retry wrapper** around `attempt`. Retries up to `maxRetries` times with
`retryDelay` backoff, but bails immediately if the context was cancelled/expired
(no point burning retries).

```text
function oneloop(ctx, messages):
    for i in 0 .. maxRetries-1:
        resp, err = attempt(ctx, messages)
        if err == nil: return resp
        if err is Canceled or DeadlineExceeded: return err   # don't retry
        log the failure
        if sleepCtx(ctx, retryDelay) cancelled: return err
    return error "failed after maxRetries"
```

### `(r *AgentResponse) FinishAnswer() string`
Convenience accessor: returns the text after the `"finish|"` prefix of a finished
response.

```text
function FinishAnswer():
    return Act with leading "finish|" removed
```

### `(a *Agent) Run(ctx) (*AgentResponse, error)`
**The main ReAct loop.** Builds the full system prompt (task + injected tool
catalog + the required JSON format), seeds memory, then iterates: ask the model,
parse its `act`, either finish or execute a tool and feed the observation back.

```text
function Run(ctx):
    # 1. Build the system prompt
    toolsDesc = render tool catalog (from SchemaData, else from Path)
    fullSystemPrompt =
        "You are a ReAct agent...
         The Task you will be solving: " + SystemPrompt + "
         Available tools: " + toolsDesc + "
         You must respond in this exact JSON format: {reasoning, act}
         use tool_name|args OR finish|answer"

    # 2. Seed memory: system prompt in front, current user prompt at the end,
    #    wrapping any prior turns the caller already placed in Memory.
    Memory = [system: fullSystemPrompt] ++ Memory(existing) ++ [user: UserPrompt]

    # 3. Iterate
    for step in 0 .. maxAgentIterations-1:
        if ctx cancelled: return ctx.Err()

        resp = oneloop(ctx, Memory)            # reason + act, with retries/fallback

        if resp.Act starts with "finish|":
            if FinishAnswer() is empty:        # guard: never return a blank reply
                resp.Act = "finish|" + resp.Reasoning
            return resp                        # DONE

        parts = split resp.Act on "|" (into name, args)
        if not exactly 2 parts:                # malformed action
            append assistant(resp) + user("Observation: invalid action format")
            trimMemory(); sleepCtx(iterationDelay); continue

        toolName, toolArgs = parts
        observation = "Tool not found: " + toolName
        if toolName in Registry:
            result, err = Registry[toolName](toolArgs)
            observation = err ? ("Error: " + err) : result

        # feed the cycle back into memory
        append assistant("Reasoning: ... Act: ...") + user("Observation: " + observation)
        trimMemory()
        sleepCtx(ctx, iterationDelay)

    return error "max iterations reached"
```

### `(a *Agent) PrintConversation()` / `(a *Agent) PrintMemory()`
Debug helpers that dump `Memory` to stdout — `PrintConversation` formats each
message by role; `PrintMemory` prints each message with its index. Neither
affects the loop.

---

## End-to-end workflow

A single call to `Run` plays out like this:

```text
                ┌─────────────────────────────────────────────┐
                │ Run: build fullSystemPrompt (task + tools +  │
                │ JSON format) and seed Memory:                │
                │   [system] + caller's Memory + [user prompt] │
                └─────────────────────────────────────────────┘
                                   │
                                   ▼
        ┌──────────────────── loop (≤ maxAgentIterations) ───────────────────┐
        │                                                                     │
        │  oneloop ──► attempt                                               │
        │     ▲           │                                                   │
        │     │           ├─ DeepseekOneshotMemory → plain text               │
        │     │           ├─ parse text directly ──► AgentResponse (lossless) │
        │     │           └─ if not JSON: convertTextToJSON → parse (repair)  │
        │     │                (any error ─► return err)                      │
        │     │                                                               │
        │     └─ retry ≤ maxRetries with retryDelay backoff (bail on cancel)  │
        │                 │                                                    │
        │                 ▼                                                    │
        │        inspect AgentResponse.Act                                    │
        │           │                                                         │
        │           ├─ "finish|answer"  ─► (empty? use reasoning) ─► RETURN   │
        │           │                                                         │
        │           └─ "tool_name|args" ─► Registry[tool](args)               │
        │                                      │                              │
        │                                      ▼                              │
        │              append assistant turn + "Observation: <result>"        │
        │              to Memory, trimMemory(), sleep iterationDelay          │
        │                                      │                              │
        └──────────────────────────────────────┘ (next iteration)            │
                                   │
                                   ▼
              max iterations reached ──► RETURN error
```

**Logic in words:**

1. **Prompt assembly.** `Run` composes the full system prompt from three parts:
   the task (`SystemPrompt`), the rendered tool catalog (so the model knows
   exactly what tools/args exist — this is why the EgoLifter prompt itself stays
   lean and does *not* re-list tools), and the strict `{reasoning, act}` JSON
   format spec.

2. **Memory seeding.** Memory becomes `[system] + (any prior turns the caller
   pre-loaded) + [current user prompt]`. A fresh agent (empty `Memory`) collapses
   to the classic `[system, user]`; a chat-backed caller (the bot) pre-seeds the
   conversation so the agent remembers earlier turns.

3. **Reason → Act.** Each iteration asks the model (via `oneloop` → `attempt`):
   generate a plain-text answer, then parse it directly as `{reasoning, act}`.
   The text model almost always emits that object already, so the full answer is
   preserved verbatim; only when the text isn't valid JSON does `attempt` run
   `convertTextToJSON` to repair it. The model returns its reasoning plus an `act`.

4. **Act dispatch.** If `act` is `finish|...`, the loop returns the answer
   (callers use `FinishAnswer()` to strip the prefix; an empty answer falls back
   to the reasoning so the reply is never blank). Otherwise `act` is split into
   `tool_name|args`; the tool is looked up in `Registry` and executed, and its
   result (or error, or "tool not found") becomes an **Observation**.

5. **Observe → repeat.** The assistant's turn and the observation are appended to
   `Memory`, `trimMemory` keeps memory bounded, and the loop continues so the
   model can reason about the new observation.

**Resilience** keeps this robust against a flaky LLM:

- **Text-first, parse-direct** (`attempt`): generate plain text, then parse it
  directly — this avoids asking the model to emit JSON under constraints (which
  returned empty often) and preserves a long final answer verbatim. Only an
  unparseable message triggers the `convertTextToJSON` repair call.
- **Never-blank finish** (`Run`): if a finish answer comes back empty, it falls
  back to the reasoning text so the user always sees a reply.
- **Retry with backoff** (`oneloop`): if a step fails (transient transport/API
  error, empty body, or unparseable output that even repair can't fix), the whole
  step is retried up to `maxRetries` with `retryDelay` between tries.
- **Context awareness** (`sleepCtx`, ctx checks): cancellation/timeout short-
  circuits sleeps, retries, and the main loop immediately rather than running to
  completion.

**Two termination conditions:** the model emits `finish|...` (success), or the
loop exhausts `maxAgentIterations` (returns an error).
