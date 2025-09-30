# WAM MCP Server

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

## Example Queries

Once configured, you can ask any agentic agent that supports additional mcp servers:

Show me all networking modules

What are the required inputs for the VNet module

Generate example usage for storage and private link

Show me the inconsistencies of the dynamic block identity in all modules

Show only the nat rules resource in the virtual wan module in main.tf

Show me the delegation dynamic block in the virtual network module

What resources are in the keyvault module and how do they relate

Show all available module usage in the automation account module
