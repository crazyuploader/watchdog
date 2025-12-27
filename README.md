# Telnyx Balance Watchdog

A lightweight Go-based service that monitors your Telnyx balance and sends notifications via a custom webhook (Apprise-compatible) when the balance falls below a configured threshold.

## Features

- Automated monitoring of Telnyx balance
- Apprise-compatible notifications
- Configurable via `config.yaml` or environment variables

## Prerequisites

- Go 1.25+

## Installation

```bash
go build -o watchdog ./cmd
```

## Usage

Run the watchdog:

```bash
./watchdog
```

Specify a custom config file:

```bash
./watchdog --config path/to/config.yaml
```

## License

MIT
