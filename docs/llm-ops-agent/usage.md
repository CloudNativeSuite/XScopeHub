# Usage

Build the daemon:

```bash
go build -o xopsagent ./cmd
```

Run with a configuration file (default `/etc/XOpsAgent.yaml`):

```bash
./xopsagent --config /etc/XOpsAgent.yaml
```

A sample configuration is provided at `configs/XOpsAgent.yaml`.
The agent will start an HTTP server exposing the module endpoints described in [API Reference](api.md).

For guidance on running the test suite and verifying the service locally, see [Testing](testing.md).
