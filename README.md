# rdq

**RDS Data Query** -- A CLI for querying Aurora databases via the RDS Data API.

> [!WARNING]
> This project is under active development. Features and APIs may change without notice.

## Overview

`rdq` is a Go CLI tool that wraps the AWS RDS Data API (`execute-statement`, `batch-execute-statement`) with a friendlier interface. It aims to solve common pain points when working with Aurora databases:

- The AWS CLI `rds-data` subcommands require many arguments and are verbose for daily use
- SQL result output in raw JSON is hard to read
- Connection setup (cluster ARN, secret ARN, database name) is tedious to specify each time

Since the RDS Data API operates over HTTPS, `rdq` requires no VPC or security group configuration -- just valid AWS credentials and IAM permissions.

## Features

- **Direct SQL execution** -- Run SQL statements with human-readable table output
- **Natural language to SQL** -- Convert natural language queries to SQL via Amazon Bedrock
- **Interactive TUI mode** -- Default mode with integrated `exec` and `ask` functionality
  - SQL keyword, table name, and column name autocomplete
  - Seamless mode switching between SQL and natural language input
  - Query history with search
  - Connection info status bar
- **Fuzzy search** -- Interactively select AWS Profiles and RDS clusters with incremental search
- **Profile management** -- Save and reuse connection info (cluster ARN, secret ARN, database name)
- **Schema caching** -- Cache table/column metadata for autocomplete and improved Bedrock SQL generation

## Installation

```bash
go install github.com/Tocyuki/rdq/cmd/rdq@latest
```

Or build from source:

```bash
git clone https://github.com/Tocyuki/rdq.git
cd rdq
go build -o rdq ./cmd/rdq/
```

## Usage

### Global Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--profile` | `-p` | AWS profile to use (falls back to `AWS_PROFILE` env var) |
| `--debug` | `-d` | Enable debug output |

### Execute SQL

```bash
rdq exec "SELECT * FROM users WHERE id = 1"
```

### Natural Language Query

```bash
rdq ask "show all users who registered in the last week"
```

### Interactive TUI Mode (Default)

Launch by running `rdq` with no arguments:

```bash
rdq
```

```
+-rdq------------------------------------------------------------+
| Profile: my-project  Cluster: my-aurora-cluster  DB: mydb      |
+----------------------------------------------------------------+
| > SELECT * FROM us_                                            |
|   +----------------------------+                               |
|   | users                      |                               |
|   | user_sessions              |                               |
|   | user_preferences           |                               |
|   +----------------------------+                               |
|                                                                |
| [Tab] Autocomplete  [Ctrl+A] Ask mode  [Ctrl+E] Exec mode     |
+----------------------------------------------------------------+
```

## AWS Profile Resolution

`rdq` resolves the AWS profile in the following order:

1. `--profile` / `-p` flag (highest priority)
2. `AWS_PROFILE` environment variable
3. Interactive fuzzy search from `~/.aws/config` profiles (in TUI mode)

## Prerequisites

- AWS credentials configured (`~/.aws/credentials`, SSO, or environment variables)
- IAM permissions for:
  - **RDS Data API** -- `rds-data:ExecuteStatement`, `rds-data:BatchExecuteStatement`, `rds-data:BeginTransaction`, `rds-data:CommitTransaction`, `rds-data:RollbackTransaction`
  - **Secrets Manager** -- `secretsmanager:GetSecretValue`
  - **Bedrock** -- `bedrock:InvokeModel` (required for the `ask` subcommand)
- An Aurora cluster with the [Data API enabled](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/data-api.html)

## Tech Stack

| Component | Technology |
|-----------|------------|
| Language | Go |
| AWS SDK | [AWS SDK for Go v2](https://github.com/aws/aws-sdk-go-v2) |
| CLI Framework | [Kong](https://github.com/alecthomas/kong) |
| TUI Framework | [Bubble Tea](https://github.com/charmbracelet/bubbletea) + [Bubbles](https://github.com/charmbracelet/bubbles) |
| Fuzzy Search | [go-fuzzyfinder](https://github.com/ktr0731/go-fuzzyfinder) |
| LLM Integration | [Amazon Bedrock](https://aws.amazon.com/bedrock/) (via AWS SDK) |

## Development

```bash
git clone https://github.com/Tocyuki/rdq.git
cd rdq
go build -o rdq ./cmd/rdq/
./rdq --help
```

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
