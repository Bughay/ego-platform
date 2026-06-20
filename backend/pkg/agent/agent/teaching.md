# Teaching note: unifying two backends the Go way

This package lets the rest of the app talk to **either** the DeepSeek agent or
the Grok agent through one tiny API, choosing the backend at runtime. It is a
compact, real example of how Go does polymorphism: **small interfaces + struct
embedding (composition)**, not inheritance. Read the four source files alongside
this note — `agent.go`, `deepseek.go`, `grok.go`, `compose.go`.

---

## 1. The problem

`pkg/agent/deepseek` and `pkg/agent/grok` each expose a ReAct agent with the
*same shape*:

```go
type Agent struct { Model, SystemPrompt, UserPrompt string; Memory []Message; /* … */ }
func (a *Agent) Run(ctx context.Context) (*AgentResponse, error)
func (r *AgentResponse) FinishAnswer() string
```

They behave identically, but they are **different concrete types in different
packages**. Without a unifying layer, every caller has to `import` a specific
provider and hard-code its types — so switching providers (or supporting both)
means editing every call site. We want one entry point and one type to depend
on.

---

## 2. Why composition, not inheritance

Go deliberately has **no classes and no inheritance**. There is no
`class GrokAgent extends BaseAgent`. Instead Go gives you two tools:

- **Interfaces** — a *set of method signatures*. Any type that has those methods
  satisfies the interface. There is no `implements` keyword: satisfaction is
  **implicit / structural**. (That is why `deepseek` and `grok` don't import
  this package, yet their wrapped agents satisfy our interface.)
- **Embedding** — putting one type *inside* another so the inner type's exported
  fields and methods are **promoted** onto the outer type. This is "has-a" reuse
  (composition), the idiomatic stand-in for "is-a" inheritance.

The Go proverb: **"Prefer composition over inheritance."** This package is that
proverb in miniature.

---

## 3. The interface: keep it tiny

`agent.go`:

```go
type Agent interface {
    Run(ctx context.Context) (string, error)
}
```

Another proverb: **"The bigger the interface, the weaker the abstraction."** The
only thing every backend must do is *run to completion and return an answer*, so
that is the only method on the interface. Callers depend on this one method and
nothing else — not on `*deepseek.AgentResponse`, not on `FinishAnswer()`, not on
which provider is underneath.

---

## 4. Struct embedding + method shadowing (the heart of it)

`deepseek.go`:

```go
type deepSeekAgent struct {
    *deepseek.Agent          // <-- embedded: no field name. This is composition.
}

func (a *deepSeekAgent) Run(ctx context.Context) (string, error) {
    resp, err := a.Agent.Run(ctx) // call the embedded agent's Run
    if err != nil {
        return "", err
    }
    return resp.FinishAnswer(), nil // adapt its result to the interface
}
```

Two things are happening:

**Promotion.** Because `*deepseek.Agent` is embedded (written with no field
name), all of its exported fields and methods are *promoted* to `deepSeekAgent`.
That is why the test can read `a.Model`, `a.MaxTokens`, etc. directly — those
come from the embedded agent. Inside our methods we can still reach the inner
value explicitly as `a.Agent` (the field's implicit name is the type's base
name, `Agent`).

**Shadowing — and why it's mandatory here.** Embedding *also* promotes
`deepseek.Agent`'s own method:

```go
func (a *deepseek.Agent) Run(ctx) (*deepseek.AgentResponse, error)
```

That promoted `Run` returns `*deepseek.AgentResponse`, which does **not** match
our interface's `Run(ctx) (string, error)`. So we declare our own `Run` on
`*deepSeekAgent`. A method declared directly on the outer type (depth 0)
**shadows** a promoted method of the same name (depth 1) — ours wins, with no
ambiguity. Our `Run` calls the inner one and converts `*AgentResponse` →
`string` via `FinishAnswer()`. That conversion is exactly the adapter's job.

> If we had *not* written our own `Run`, `*deepSeekAgent` would only have the
> promoted `Run` (wrong signature) and would fail to satisfy `Agent`.

`grok.go` is the same pattern against `*grok.Agent`. Adding a third backend =
one more ~15-line adapter; nothing else changes. That is the payoff of
composition.

### Pointer receivers matter
We embed `*deepseek.Agent` (a pointer) and use a pointer receiver
`func (a *deepSeekAgent)`. `deepseek.Agent.Run` has a pointer receiver, so it is
only in the method set of `*deepseek.Agent`, not `deepseek.Agent`. Keeping
everything on the pointer keeps the method sets lined up and lets the loop
mutate `Memory` in place.

---

## 5. The provider-neutral `Config` — and why `Message` needs conversion

`agent.go` defines neutral input types so callers never name a provider type:

```go
type Message struct { Role, Content string }

type Config struct {
    Model, SystemPrompt, UserPrompt string
    Memory     []Message
    Thinking   bool
    Registry   map[string]func(string) (string, error)
    SchemaData []byte
    MaxTokens  int
}
```

Each constructor copies `Config` into the provider's `Agent`. Most fields pass
through **untouched** — but `Memory` must be **converted per provider**. Here is
the precise reason, because it is a genuinely instructive corner of Go's type
system:

- `agent.Message`, `deepseek.Message`, and `grok.Message` are three **distinct
  *defined* (named) types**. Go only lets you assign between two defined types if
  they are the *same* type — identical-looking fields are **not** enough. So
  `var d deepseek.Message = someAgentMessage` does not compile.
- They are not even structurally identical: the providers' `Message` has extra
  `ToolCalls []ToolCall` / `ToolCallID` fields (and `ToolCall` is itself a
  provider-local type). The neutral `Message` omits them on purpose — those are
  filled by the provider's `Run` loop during a tool call, never by a caller
  seeding history.
- And it's a **slice**: `[]agent.Message` → `[]deepseek.Message` is never a
  single assignment or conversion in Go. You must allocate a new slice and copy
  element by element:

```go
func toDeepSeekMessages(in []Message) []deepseek.Message {
    if in == nil { return nil }
    out := make([]deepseek.Message, len(in))
    for i, m := range in {
        out[i] = deepseek.Message{Role: m.Role, Content: m.Content}
    }
    return out
}
```

**Contrast — why the other fields *don't* need conversion:**

- `Registry` has type `map[string]func(string)(string,error)`. That is an
  **unnamed composite type** built only from predeclared types; it has no package
  identity, so it is literally the same type in `agent`, `deepseek`, and `grok`
  and assigns directly.
- `Model`, `MaxTokens`, `Thinking`, `SchemaData` are `string`/`int`/`bool`/`[]byte`
  — the same everywhere.
- `Tools` is skipped entirely: both providers parse the same `SchemaData` JSON
  into their *own* `Tool` type inside `Run`, so we hand over `[]byte` and never
  touch a named `Tool` type.

The lesson: a **named, package-local struct** is the one thing that cannot cross
a package boundary for free — so it is the one thing the adapter converts.

---

## 6. The factory + the `Provider` enum

`agent.go`:

```go
type Provider string
const ( DeepSeek Provider = "deepseek"; Grok Provider = "grok" )

func New(p Provider, cfg Config) (Agent, error) {
    switch p {
    case DeepSeek: return NewDeepSeek(cfg), nil
    case Grok:     return NewGrok(cfg), nil
    default:       return nil, fmt.Errorf("agent: unknown provider %q", p)
    }
}
```

`New` is runtime selection — pass a `Provider` you read from config or a request
and get back an `Agent`. Note the return type is the **interface**, not a
concrete adapter. The usual Go advice is "accept interfaces, return concrete
types," but a *factory whose whole job is to hide the concrete type* is the
standard exception: returning `Agent` is the point.

---

## 7. Composition of *behaviors*: `fallbackAgent`

Once behavior is an interface, you compose it. `compose.go`:

```go
type fallbackAgent struct {
    primary   Agent
    secondary Agent
}

func NewFallback(primary, secondary Agent) Agent { /* … */ }

func (a *fallbackAgent) Run(ctx context.Context) (string, error) {
    if answer, err := a.primary.Run(ctx); err == nil {
        return answer, nil
    }
    return a.secondary.Run(ctx) // (with context checks + combined error)
}
```

`fallbackAgent` is built *out of* `Agent`s and is *itself* an `Agent`. So
`NewFallback(NewGrok(cfg), NewDeepSeek(cfg))` rides out a provider outage, and
because the result is an `Agent` you can nest or wrap it further. This is
composition the same way Unix pipes compose commands — each piece implements one
small contract, and you snap them together. (It is also the decorator/adapter
pattern: a wrapper that satisfies the same interface it consumes.)

---

## 8. Compile-time interface checks

Each adapter file has a line like:

```go
var _ Agent = (*deepSeekAgent)(nil)
```

This asserts at **compile time** that `*deepSeekAgent` satisfies `Agent`,
assigning a typed `nil` to the blank identifier (so nothing is allocated or
kept). If a method signature ever drifts, the build breaks *here*, with a clear
message, instead of at some far-away call site. A cheap, idiomatic safety net.

---

## 9. How to extend it

- **Add a provider:** create `openai.go` with an `openAIAgent struct{ *openai.Agent }`,
  a shadowing `Run`, a `NewOpenAI`, a `toOpenAIMessages`, and a
  `var _ Agent = (*openAIAgent)(nil)`; then add one `case` to `New`.
- **Add a capability:** if every provider should also do a single-shot
  completion, widen the interface (e.g. `Complete(ctx, prompt) (string, error)`)
  and implement it on each adapter. Keep additions minimal — a wide interface is
  a weak one. The current scope is intentionally just the ReAct `Run`.

---

## 10. Usage

```go
cfg := agent.Config{
    Model:        "grok-4.3",
    SystemPrompt: "You are a fitness assistant.",
    UserPrompt:   "Log a push day: bench 60kg x8.",
    Registry:     egotools.EgolifterFunctions(ctx, svc, userID), // same map type both providers accept
    SchemaData:   egotools.SchemaJSON,                            // each provider parses its own tools
    MaxTokens:    4096,
    Thinking:     true,
}

a, err := agent.New(agent.Grok, cfg) // or agent.DeepSeek — only this line changes
if err != nil {
    return err
}
answer, err := a.Run(ctx)
```

Swapping `agent.Grok` for `agent.DeepSeek` is the *only* change needed to switch
backends — every other line stays the same. That is the whole goal, and Go's
composition model is what makes it this small.
