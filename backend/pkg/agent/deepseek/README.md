# DeepseekGo-ReactAgent
ReAct agent module to be implemented in Golang

Go React Agent with DeepSeek

This project implements a ReAct (Reasoning + Acting) agent in Go, powered by the DeepSeek API. The agent follows an iterative loop of reasoning, acting (calling tools), and observing results until it reaches a final answer.

## Features
ReAct loop: The agent thinks step by step, decides which tool to use, executes it, and incorporates observations.

Tool integration: Define custom tools in a tools.json file. The agent can invoke them dynamically.

DeepSeek API: Uses DeepSeek's chat completions with JSON mode for structured outputs.

Retry logic: Automatically retries API calls on failure.

Conversation memory: Maintains the full interaction history.

## Folder Structure

.
├── deepseek.go      # DeepSeek API client (oneshot calls)
├── react.go         # Agent implementation (ReAct loop)
├── tools.go         # Tool loading and formatting
└── README.md        # This file

## Prerequisites
Go 1.18+ (for generics, though not heavily used)

A DeepSeek API key 

tools.json defining the available tools (see below)


## Deepseek.go

In this file I have created standard functions to easily process API calls to Deepseek.



### 1. DeepseekOneShot

This function is created for the purpose of sending one shot calls to deepseek API

#### Params:

#### Algorithm:
1. Load environment variables from .env file
2. Extract DEEPSEEKAPIKEY from environment
3. Construct ChatTemplate struct:
   - Model: "deepseek-chat"
   - Messages: [{role: "system", content: systemMessage}, {role: "user", content: userMessage}]
   - Stream: false
   - Temperature: user-provided
   - MaxTokens: user-provided
4. Marshal struct to JSON
5. Create HTTP POST request to deepseekURL (/beta/v1/chat/completions)
6. Set headers: Content-Type: application/json, Authorization: Bearer <apiKey>
7. Execute request with 30-second timeout
8. Check status code; if not 200, read body and return error
9. Decode JSON response into ChatResponse struct
10. Validate: ensure at least one choice exists
11. Return the content of the first choice's message
simple one shot call to deepseek api,

### 2. DeepseekOneshotJSON

This function returns JSON object, make sure to mention the exact object required in the system prompt and provide examples

#### Params:

| Param         | Type        | Description                                                                                                                                                          |
| ------------- | ----------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `messages`    | `[]Message` | Slice of conversation history (including system prompt with JSON schema instructions). The final message should explicitly request JSON output matching your schema. |
| `temperature` | `float64`   | Sampling temperature. Lower values (0.1-0.2) recommended for deterministic JSON structure adherence.                                                       |
| `maxTokens`   | `int`       | Maximum tokens for the response. **Critical:** If too low, JSON may be truncated mid-object, causing parse failures.                                                 |


#### Algorithm:
1. Load environment variables from .env file
2. Validate DEEPSEEKAPIKEY exists
3. Construct ChatTemplate:
   - Model: "deepseek-chat"
   - Messages: provided slice (must contain schema instructions)
   - Stream: false
   - Temperature: user-provided
   - MaxTokens: user-provided
   - ResponseFormat: {Type: "json_object"}  // Forces valid JSON syntax only
4. Marshal payload to JSON
5. Create HTTP POST request to beta endpoint
6. Set Authorization header with Bearer token
7. Execute with 120-second timeout (extended for complex generation)
8. Read full response body as bytes
9. Trim whitespace and validate non-empty
10. Unmarshal into ChatResponse struct
11. Validate response choices exist
12. Check FinishReason:
    - If "length": return error indicating truncation (maxTokens insufficient)
13. Trim content and validate non-empty
14. Return raw JSON string (caller must unmarshal into specific struct)

### 3. DeepseekOneShotMemory:

same exact function like the deepseekoneshot, I literally copy pasted the algorithm but this takes memory as parameters. This is Perfect when we need to take a memory array as input rather than system and userprompt. 

THIS FUNCTION IS GREAT IF WE WANT TO TAKE MEMORY FROM ONE AGENT TO ANOTHER OR MANIPULATE THE MEMORY IN ANY WAY.

## 4. DeepseekMemoryLoop

This function is DeepseekOneShotMemory but in an endless loop, perfect to test in the terminal.


## ReAct.go 


In this file I have implemented the ReAct (Reasoning + Acting) agent logic using struct methods to maintain state across iterations. The ReAct loop structure and tool parsing logic are hard-coded—the SystemPrompt is used purely to describe the agent's role/persona (e.g., "customer support agent", "code review agent", "research assistant"), while the reasoning framework remains standardized.

### 1. Agent Struct 

| Field          | Type                                      | Description                                                                                                                         |
| -------------- | ----------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| `SystemPrompt` | `string`                                  | **Agent role description** (e.g., "You are a customer support agent..."). Defines persona/task type only—ReAct logic is hard-coded. |
| `UserPrompt`   | `string`                                  | The specific user query or problem to solve.                                                                                        |
| `Memory`       | `[]Message`                               | Conversation history tracking all reasoning steps, actions, and observations.                                                       |
| `Tools`        | `[]Tool`                                  | Available tools loaded from `tools.json` (used for LLM descriptions).                                                               |
| `Registry`     | `map[string]func(string) (string, error)` | **Runtime tool implementations**. Must be populated manually with functions matching the tool names from `tools.json`.              |


### 2. Agent Response Struct

Standardized JSON structure that the LLM returns to indicate its reasoning and next action. This format is enforced by the hard-coded system prompt template in oneloop(). 

What iam trying to achieve is have a json output with parameters Reason-Act-Observe so that I can easily manipulate those parameters. Its efficient to have schema guided reasoning.

| Field         | Type     | Description                                                              |                     |                  |
| ------------- | -------- | ------------------------------------------------------------------------ | ------------------- | ---------------- |
| `Reasoning`   | `string` | Step-by-step thought process explaining why the agent chose this action. |                     |                  |
| `Act`         | `string` | Action to take: either \`tool\_name                                      | arguments`or`finish | final\_answer\`. |
| `Observation` | `string` | Result from tool execution (populated after the act is processed).       |                     |                  |

### oneloop() function

Executes a single iteration of the ReAct cycle: constructs the system prompt with tool descriptions (from tools.json), queries the LLM, and parses the structured response. The ReAct reasoning framework is hard-coded—only the agent persona varies via SystemPrompt.

#### Algorithm

1. Load tool descriptions from tools.json using ToolsToLLMString()
2. Construct hard-coded ReAct system prompt template combining:
3. SystemPrompt (user-provided agent role description)
4. Available tools list from tools.json
5. Fixed JSON schema requirements: {"reasoning": "...", "act": "...", "observation": ""}
6. Build message slice: [system prompt] + a.Memory
7. Attempt up to 3 API calls with DeepseekOneshotJSON:
8. Temperature: 0.1 (react should have minimal creativity from the model)
9. MaxTokens: 64000 (thinking model)
10. Sleep 30 seconds between retries on failure
11. Parse raw JSON response into AgentResponse struct
12. Return parsed response or cumulative error after max retries

### run() function

The main execution engine that runs the hard-coded ReAct loop until completion or max iterations. Handles the standardized cycle: Reasoning → Acting (tool call) → Observing → Repeating.

#### Algorithm

1. Initialize a.Memory with user prompt as first message (role: "user")
2. Set maxIterations to prevent infinite loops
3. For each iteration (0 to maxIterations):
  1. Call a.oneloop() to get agent's reasoning and action
  2. Check completion: If resp.Act starts with finish|:
  3. Extract final answer from suffix
  4. Set resp.Observation to final answer
  5. Return response immediately
  6. Parse tool call: Split resp.Act on first | character
  7. Validate format (must contain exactly one separator)
  9. Extract toolName and toolArgs
  10. Execute tool: Lookup toolName in a.Registry
  11. If found: call function with toolArgs, capture result or error
  12. Update memory: Append two messages:
  13. Sleep 2 seconds to prevent rate limiting

### PrintConversation() function

simple function to print the entire conversation upon the end of the loop . This is good to either test in the terminal or create logs from it. It will clearly mention system_prompt, userMessage and Assistant alongside the exact tools called etc.


## Tools.go functions

In this file I have implemented the tool loading and formatting system that bridges the JSON tool definitions with the LLM readable format required by the ReAct agent.

I am not gonna describe the entire algorithm since they are simple but this file takes in a json file in the following format:

KEEP NOTE ITS A LIST OF KEY VALUE PAIRS!!!!
```
    {
        "type": "function",
        "function": {
            "name": "read_file",
            "description": "Read content from a file",
            "parameters": {
                "type": "object",
                "properties": {
                    "filepath": {
                        "type": "string",
                        "description": "Path to the file to read"
                    }
                },
                "required": ["filepath"]
            }
        }
    }
```

then it transforms this data structure into a string with the following format:

```
Tool: read_file
Description: Read content from a file
Parameters:
  - filepath: string (required)
    Path to the file to read

```

This is done for 2 reasons:

1. to save tokens since we will get rid of unnecessary symbols
2. Since all my API calls take systemprompt and userprompt in the form of string, I want to have functions that can transform the tools data structure into a simple string. I also feel the LLM will understand it better.
