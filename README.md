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

Compare code patterns (e.g., dynamic blocks, lifecycle, resource types) across modules. Uses HCL AST for tighter, block‑aware matches when patterns like `resource "..."`, `dynamic "..."`, or `lifecycle` are used; falls back to text search otherwise.

**Example Access**

Retrieve usage examples per module, including example file contents

**Variable Extraction**

Extract complete variable definitions including types, defaults, and sensitivity

**Short-name Aliases**

Use short module names (e.g., `vnet`, `kv`, `pe`, `agw`) instead of full names (e.g., `terraform-azure-vnet`). Aliases are auto-generated from module names, submodules, and tags during sync and work with all tools that accept `module_name`.

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

## Example Prompts

**Once configured, you can ask any agentic agent that supports additional MCP servers:**

`Networking`

Show all networking modules grouped by category (core networking, connectivity, name resolution, private access, security, ingress/egress, monitoring), including submodules.

List modules related to vnet based on shared tags/resources, and include brief descriptions.

Regarding the previous question, how does evh relate to vnet?

`Module Info`

Show module info for vnet and highlight only the required variables.

Show module info for kv and list all resources it creates.

`Examples`

List all examples for terraform-azure-aa.

List the examples for sa, then open the example that demonstrates a private endpoint (private link) and show all files.

For terraform-azure-func, list examples and open the private-endpoint example in full code.

`AST Pattern Compare (block‑aware)`

Compare dynamic "identity" across modules and show one example per unique pattern, with full code.

Compare resource "azurerm_private_endpoint" across all modules and show full resource blocks.

Show lifecycle blocks that set ignore_changes (pattern: lifecycle has:ignore_changes) with full code.

In vnet, show dynamic "delegation" blocks with full code, and summarize the service_delegation name/actions.

`Focused Code Queries`

Search code for key vault/keyvault access_policy and show matching files and snippets.

Find modules that use for_each = merge(flatten(...)) and list the module names and file paths.

In vwan and vgw, show resource "azurerm_vpn_gateway_nat_rule" with full blocks.

In vnet, show dynamic "delegation" with full blocks.

`Sync and Maintenance`

Run a full sync of all modules and report the job ID; then show the sync status for that job ID.

Run an incremental sync (updates only) and report the job ID; then show the sync status for that job ID.

**Tips**
```
For AST mode, include quotes around types/labels in the pattern:
  resource "azurerm_...", dynamic "identity", lifecycle
Add attribute filters with has: to narrow results:
  resource "azurerm_" has:lifecycle.ignore_changes
  dynamic "identity" has:identity_ids
Use show_full_blocks: true when you want the exact HCL code, or leave it false for a compact table.
```

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
