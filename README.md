# Watchdog - Monitoring Framework

A lightweight Go-based framework for monitoring various services and sending notifications via custom webhooks (Apprise-compatible) when configured thresholds are breached.

## Features

- Framework for adding monitoring tasks
- Currently supports:
  - Telnyx balance monitoring
  - GitHub PR review monitoring (stale PR detection)
  - External contributor PR monitoring
- Apprise-compatible notifications
- Configurable via `config.yaml` or environment variables
- Easy to extend with new monitoring tasks

## Prerequisites

- Go 1.25+

## Installation

```bash
go build -o watchdog ./cmd
```

## Usage

Run the watchdog framework:

```bash
./watchdog
```

Specify a custom config file:

```bash
./watchdog --config path/to/config.yaml
```

## Configuration

See `sample_config.yaml` for all configuration options.

### External Contributor PR Monitoring

The `external_contributor_prs` task monitors repositories for PRs created by external contributors (users not in the org members list). This is useful for:

- Tracking contributions from the community
- Ensuring external PRs get attention
- Avoiding missing review requests from contributors

Example configuration:

```yaml
tasks:
  external_contributor_prs:
    interval: "60m"
    notification_cooldown: "24h"
    repositories:
      - owner: "SigNoz"
        repo: "signoz.io"
        org_members:
          - "member1"
          - "member2"
        pr_lookback_days: 7
```

## License

[MIT](LICENSE)
