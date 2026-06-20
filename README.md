# Pi Web Gateway

Browser UI for local Pi sessions.

## Requirements

- [mise](https://mise.jdx.dev/)
- Pi CLI available on `PATH`

## Setup

```sh
mise trust
mise install
mise run setup
```

The setup task installs Ruby dependencies and creates a local gateway config file at `~/.config/pi-web-gateway/env` if needed. When `PI_GATEWAY_ADMIN_PASSWORD` is missing, setup generates a random admin password there and prints the file path. You can change the gateway admin password by editing that file.

## Run the gateway

```sh
mise run start
```

The gateway listens on <http://localhost:4567>.

By default, the gateway binds to `0.0.0.0`. To bind it only to a specific address, such as your Tailscale IP, pass the host as an argument:

```sh
mise run start 100.x.y.z
```

You can also configure the host or port with environment variables:

```sh
PI_GATEWAY_HOST=100.x.y.z mise run start
PI_GATEWAY_PORT=4568 mise run start
```

## Development server

```sh
mise run dev
```

## Tests

```sh
mise run test
```
