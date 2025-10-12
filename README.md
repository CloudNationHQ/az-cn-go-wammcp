# wammcp

An MCP (Model Context Protocol) server that indexes, analyzes, and serves cloudnation's terraform modules for azure on demand to MCP compatible AI agents.

## Features

**Module Discovery**

List and search all available Terraform modules with fast, FTS-backed lookups

**Code Search**

Search across all module code (any .tf file) for patterns, resources, or free text

**Module Analysis**

Get detailed info on variables, outputs, resources, and examples in one response

**Pattern Comparison**

Compare code patterns (e.g., dynamic blocks, lifecycle, identity) across modules

**Example Access**

Retrieve usage examples per module, including example file contents

**Variable Extraction**

Extract complete variable definitions including types, defaults, and sensitivity

**GitHub Sync**

Syncs and indexes modules from GitHub into a local SQLite database for fast queries. Supports incremental updates and parallel syncing with rate‑limit awareness for larger orgs.

## Prerequisites

Go 1.23.0 or later

SQLite (with FTS5 support - included in most modern installations)

GitHub Personal Access Token (optional, for higher rate limits) with `repo → public_repo` rights.

## Configuration

**Server flags**

The server accepts command-line flags for configuration:

--org - GitHub organization name (default: "cloudnationhq")

--token - GitHub personal access token (optional; improves rate limits)

--db - Path to SQLite database file (default: "index.db")

**Adding to AI agents**

To use this MCP server with AI agents (Claude CLI, Copilot, Codex CLI, or other MCP-compatible clients), add it to their configuration file:

```json
{
  "mcpServers": {
    "az-cn-wam": {
      "command": "/path/to/az-cn-wam-mcp",
      "args": ["--org", "cloudnationhq", "--token", "YOUR_TOKEN"]
    }
  }
}
```

## Build from source

make build

## Example Queries

**Once configured, you can ask any agentic agent that supports additional MCP servers:**

Search all network related modules.

Show module info for vnet and show required variables.

Show example usage for storage with private link.

Compare dynamic "identity" across all modules and show only the ones that are different in code.

Search inside the vwan module for any resources that manage nat rules and show it.

Search code for dynamic "delegation" in vnet and show it.

Show module info for keyvault and list all resources.

List all examples for automation account.

Search code for validation { in the private endpoint module.

Search code for for_each = merge(flatten and name the modules.

Sync modules.

Sync updates modules.

## Notes

It might be quicker to use a low reasoning model for the sample queries above, then switch to a higher one for interpreting, planning, or debugging.

GitHub token is optional; without it, syncing still works but may hit lower API rate limits. Pass `--token` to raise limits.

Initial full sync takes ~20 seconds on first run. It is optimized via gitHub tarball archives and a bounded worker pool (rate‑limit aware).

Deleting the database file `index.db` will cause a full rebuild the next time the tool gets called.

Archived, private and empty repositories will be skipped by default.

## Direct Database Access

The indexed data is stored in a SQLite database file with FTS5 enabled. You can query it directly for ad‑hoc inspection:

`sqlite3 index.db "SELECT name, description FROM modules LIMIT 10"`

`sqlite3 index.db "SELECT name FROM modules WHERE name LIKE '%storage%'"`

`sqlite3 index.db "
  SELECT m.name, r.resource_name
  FROM modules m
  JOIN module_resources r ON m.id = r.module_id
  WHERE r.resource_type = 'azurerm_storage_account'"
`

## Contributors

We welcome contributions from the community! Whether it's reporting a bug, suggesting a new feature, or submitting a pull request, your input is highly valued.

For more information, please see our contribution [guidelines](./CONTRIBUTING.md). <br><br>

<a href="https://github.com/cloudnationhq/ac-cn-wam-mcp/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=cloudnationhq/ac-cn-wam-mcp" />
</a>

## License

MIT Licensed. See [LICENSE](./LICENSE) for full details.
