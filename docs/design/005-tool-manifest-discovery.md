# Tool Manifest Discovery

**Date:** 2026-07-15
**Status:** Accepted
**Scope:** Add deterministic, public discovery for AnyCLI's embedded tool set and validate the definition-to-executor contract generically.

## Problem

AnyCLI embeds tool definitions and separately registers built-in service implementations. Before this change, consumers could only probe one guessed name with `definitions.LoadBundled`, while tests repeated a handwritten list of tools and injection details. Adding a provider therefore required updating several parallel lists, and a service definition could compile without a registered executor.

## Decisions

1. `definitions.ListBundled` enumerates embedded JSON definitions, verifies that each filename matches its declared name, and returns them in deterministic name order.
2. The public package exposes `ListTools() ([]ToolManifest, error)`. A manifest contains only the tool name, execution kind, description, and required credential field names. It does not expose credentials or make definitions consumer-supplied.
3. Discovery validates the execution contract:
   - service definitions must have a registered in-process implementation;
   - CLI definitions must declare a binary;
   - unknown execution kinds fail explicitly.
4. Generic tests sweep the discovered set. Provider-specific tests retain literal assertions only for provider-specific wire contracts.

## Resulting extension path

A new built-in REST provider adds one embedded definition, one service package, and one registration. Discovery, generic validation, and consumer contract checks pick it up without another handwritten tool inventory.

The embeddable boundary from designs 002 and 003 is unchanged: hosts still supply only credentials and an optional cache, never definitions or service implementations.
