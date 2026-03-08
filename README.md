# Scuta

SID Developer Toolbox. Install once, get everything.

Scuta manages SID's developer CLI tools — installing, updating, and discovering them from a single command.

## Installation

### Homebrew (macOS/Linux)

```bash
brew install sid-technologies/tap/scuta
```

### Go Install

```bash
go install github.com/sid-technologies/scuta@latest
```

### From Source

```bash
git clone https://github.com/sid-technologies/Scuta.git
cd Scuta
make build
./dist/scuta init
```

## Quick Start

```bash
# Set up Scuta on your machine
scuta init

# Install all SID tools
scuta install --all

# Check what's available
scuta list

# Update everything
scuta update
```

## Available Tools

| Tool | Description |
|------|-------------|
| `api-gen` | OpenAPI code generator (Go + TypeScript) |
| `pilum` | Multi-cloud deployment CLI |
| `mcp-gen` | Generate MCP servers from OpenAPI specs |

## Commands

| Command | Description |
|---------|-------------|
| `scuta init` | Setup ~/.scuta/, detect auth, configure PATH |
| `scuta install <tool>` | Install a tool from the registry |
| `scuta install --all` | Install all tools |
| `scuta uninstall <tool>` | Remove a tool |
| `scuta update [tool]` | Update one or all tools |
| `scuta list` | Show all tools + versions + install status |
| `scuta doctor` | Health check (PATH, binaries, state) |
| `scuta self-update` | Update Scuta itself |
| `scuta version` | Print version |

## Global Flags

| Flag | Description |
|------|-------------|
| `--verbose, -v` | Show detailed output |
| `--quiet, -q` | Suppress non-error output |
| `--json` | Output in JSON format |
