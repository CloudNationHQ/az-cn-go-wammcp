# Wam MCP Server

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

List all network related modules

Get module info for vnet and show required variables

Generate example usage for storage and private link

Compare the pattern dynamic block identity across all modules and show the inconsistencies and flavours

Search for the resource nat rules in the virtual wan module

Search for the dynamic block delegation in the vnet module

Get module info for keyvault and show all resources and how they relate

List all module examples for automation accounts
