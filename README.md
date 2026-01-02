# Watchdog - Monitoring Framework

A lightweight Go-based framework for monitoring various services and sending notifications via custom webhooks (Apprise-compatible) when configured thresholds are breached.

## Features

- Framework for adding monitoring tasks
- Currently supports Telnyx balance monitoring
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

## License

[MIT](LICENSE)
