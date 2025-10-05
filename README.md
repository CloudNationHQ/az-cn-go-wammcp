# wamcp [![Go Reference](https://pkg.go.dev/badge/github.com/cloudnationhq/ac-cn-wam-msp.svg)](https://pkg.go.dev/github.com/cloudnationhq/ac-cn-wam-msp)

An MCP (Model Context Protocol) server that provides comprehensive knowledge about CloudNation's Terraform modules for Azure infrastructure.

## Features

**Module Discovery**

List and search all available Terraform modules

**Code Search**

Search across all module code for specific patterns

**Module Analysis**

Get detailed info on variables, outputs, and resources

**Pattern Comparison**

Compare code patterns (like dynamic blocks) across modules

**Example Access**

Retrieve usage examples for any module

**Variable Extraction**

Get complete variable definitions with types and defaults

**GitHub Sync**

Automatically syncs and indexes modules from GitHub repositories into a local SQLite database for fast queries

## Prerequisites

Go 1.23.0 or later

SQLite (with FTS5 support - included in most modern installations)

GitHub Personal Access Token (optional, for higher rate limits)

## Configuration

**Server flags**

The server accepts command-line flags for configuration:

--org - GitHub organization name (default: "cloudnationhq")

--token - GitHub personal access token (optional)

--db - Path to SQLite database file (default: "index.db")

**Adding to AI agents**

To use this MCP server with AI agents like claude, opencode, codex or other compatible ones, add it to their configuration file:

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

The token is optional and only requires `repo → public_repo` rights.

## Build from source

make build

## Example Queries

**Once configured, you can ask any agentic agent that supports additional mcp servers:**

List all network related modules

Get module info for vnet and show required variables

Generate example usage for storage and private link

Compare the pattern dynamic block identity across all modules and show the inconsistencies and flavours

Search for the resource nat rules in the virtual wan module

Search for the dynamic block delegation in the vnet module

Get module info for keyvault and show all resources and how they relate

List all module examples for automation accounts

## Direct Database Access

The indexed data is stored in a SQLite database file, which you can query also directly:

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

MIT Licensed. See [LICENSE](https://github.com/cloudnationhq/terraform-azure-vnet/blob/main/LICENSE) for full details.
