# Why AnyCLI?

## The Problem: MCP Is Overkill for Most Agent Workflows

The Model Context Protocol (MCP) was designed as a universal bridge between AI agents and external tools. In practice, it introduces significant overhead that hurts agent performance in the majority of developer-facing use cases.

### The Numbers Don't Lie

Benchmarks from [ScaleKit](https://www.scalekit.com/blog/mcp-vs-cli-use) comparing MCP vs CLI on identical tasks (Claude Sonnet, GitHub operations):

| Metric | CLI | MCP | Difference |
|--------|-----|-----|------------|
| Token consumption (single task) | 1,365 | 44,026 | **32x more** |
| Success rate | 100% (25/25) | 72% (18/25) | **28% less reliable** |
| Monthly cost (10K operations) | ~$3.20 | ~$55.20 | **17x more expensive** |

The root cause: **schema bloat**. A standard GitHub MCP server exposes 43 tools. Every agent interaction carries the full schema for all 43 tools — even when only 1-2 are needed. Connecting a GitHub MCP server dumps ~55,000 tokens into context before the agent does anything useful.

## The Thesis: CLI Is the Natural Interface for Agents

Peter Steinberger, creator of [OpenClaw](https://github.com/openclaw/openclaw) (190K+ GitHub stars) and now at OpenAI, put it bluntly:

> "MCP were a mistake. Bash is better."

His core arguments — and the broader community consensus — form the foundation of AnyCLI:

### 1. LLMs Already Know CLIs

LLMs are trained on massive corpora that include extensive CLI usage — `git`, `curl`, `jq`, `grep`, `docker`, `kubectl`, and thousands more. The model already knows how to use these tools with **zero schema tokens consumed**. MCP forces the agent to learn a new protocol from scratch on every interaction.

### 2. `--help` Is the Only Schema You Need

An agent runs `tool --help` and gets a zero-noise, high-density prompt — the documentation is the interface. No separate schema definition, no protocol negotiation, no capability handshake. Just a well-structured help text that the agent can immediately act on.

> "Agents are really, really good at calling CLIs — actually much better than calling MCPs."
> — Peter Steinberger

### 3. Unix Philosophy = Agent Chain-of-Thought

The Unix principle of "do one thing and do it well" maps perfectly onto agent reasoning:

```bash
log-fetch --days 7 | grep "ERROR" | feishu-send
```

Atomic CLI tools composed through pipes are the natural orchestration primitive for agents. Each step is inspectable, debuggable, and independently testable.

### 4. Mechanical Reliability

Once a CLI command works, it executes reliably hundreds of times without requiring additional inference or context. MCP connections suffer from TCP timeouts, server crashes, and protocol-level failures that the agent must reason about and recover from.

### 5. Human-Agent Shared Interface

> "CLIs work for both humans and AI agents — we can run, debug, and understand them."
> — Peter Steinberger

CLI commands are transparent. You can copy-paste what the agent ran, debug it yourself, pipe it to other tools, and verify the output. MCP interactions are opaque protocol exchanges that require specialized tooling to inspect.

## What MCP Gets Right (And Where CLI Falls Short)

We acknowledge MCP's strengths — these inform AnyCLI's design:

| Concern | MCP's Answer | AnyCLI's Approach |
|---------|-------------|-------------------|
| **Tool discovery** | Standardized capability registry | CLI registry with searchable metadata |
| **Security & permissions** | Protocol-level authorization | Scoped execution policies, no full shell access |
| **Cross-platform portability** | Mount servers in any MCP-compatible host | Generate agent-ready CLI wrappers for any platform |

## Design Principles for Agent-Native CLIs

Drawing from Peter Steinberger and the broader community, AnyCLI follows these principles:

1. **Structured output by default** — Always support `--json` / `--yaml` for machine-readable output
2. **Rich `--help` text with examples** — LLMs learn by imitation; examples are the best prompt
3. **No interactive input** — Support `-y` / `--yes` flags; agents cannot type into prompts
4. **Composable via stdin/stdout** — Embrace pipes; each tool is a building block
5. **Progressive disclosure** — Start simple, let the agent discover advanced features via `--help`
6. **Predictable exit codes** — 0 for success, non-zero for failure; agents need clear signals

## The Vision

AnyCLI wraps authenticated cloud service CLIs/APIs into agent-friendly interfaces — no MCP server needed, no schema bloat, no protocol overhead. Install, authenticate, and let agents use tools they already know.

## References

- [Peekaboo 2.0 – Free the CLI from its MCP shackles | Peter Steinberger](https://steipete.me/posts/2025/peekaboo-2-freeing-the-cli-from-its-mcp-shackles)
- [MCP vs CLI: Benchmarking AI Agent Cost & Reliability | ScaleKit](https://www.scalekit.com/blog/mcp-vs-cli-use)
- [CLI 才是 AI 连接世界的终极接口 | Tony Bai](https://tonybai.com/2026/02/04/openclaw-author-cli-ultimate-agent-interface-vs-mcp/)
- [Why CLIs Beat MCP for AI Agents | Medium](https://medium.com/@rentierdigital/why-clis-beat-mcp-for-ai-agents-and-how-to-build-your-own-cli-army-6c27b0aec969)
- [Why CLI Tools Are Beating MCP for AI Agents](https://jannikreinhard.com/2026/02/22/why-cli-tools-are-beating-mcp-for-ai-agents/)
- [CLI-Anything | HKUDS](https://github.com/HKUDS/CLI-Anything)
