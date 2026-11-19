# portman

A small SSH port-forwarding manager. Run one long-lived daemon that holds your
SSH connections, then add, remove and list local port forwards from the command
line without restarting anything.

```
localhost:3000  ──►  portman daemon  ══ SSH ══►  user@10.0.0.5  ──►  localhost:3000
```

## Features

- **One daemon, many forwards.** Forwards to the same host share one SSH
  connection.
- **Change forwards at runtime.** `portman add` / `rm` / `ls` talk to the
  running daemon over a Unix socket.
- **Load forwards on startup** from a YAML config file.
- **Reconnects automatically.** A dropped SSH connection is removed from the
  pool, and the next incoming connection dials a fresh one.
- **Uses your existing SSH setup.** Authenticates through `ssh-agent` and checks
  host keys against `~/.ssh/known_hosts`.

## Requirements

- Go 1.27+
- A running `ssh-agent` with your key loaded (`SSH_AUTH_SOCK` must be set)
- Each remote host already listed in `~/.ssh/known_hosts` (connect once with
  plain `ssh` to add it)

## Installation

```sh
go install github.com/gvarma28/portman/cmd/portman@latest
```

Or build from source:

```sh
git clone https://github.com/gvarma28/portman.git
cd portman
go build -o portman ./cmd/portman
```

## Configuration

The daemon reads `~/.config/portman/config.yaml` by default. Start from the
example:

```sh
mkdir -p ~/.config/portman
cp config.example.yaml ~/.config/portman/config.yaml
```

```yaml
hosts:
  - host: deploy@10.0.0.5        # user@host, optionally :port (default 22)
    forwards:
      - local: 3000              # port on 127.0.0.1
        remote: 3000             # port on the remote host's localhost
      - local: 8000
        remote: 8080
```

Each forward listens on `127.0.0.1:<local>` and connects to
`localhost:<remote>` as seen from the remote host.

## Usage

Start the daemon (in a separate terminal, or under a process supervisor):

```sh
portman daemon
portman daemon -config /path/to/config.yaml
```

Then manage forwards from any shell:

```sh
# forward local 5432 to port 5432 on the remote host
portman add deploy@10.0.0.5 5432:5432

# list active forwards
portman ls

# remove one or more forwards by local port
portman rm 5432 8000
```

> **Note:** `portman add` currently accepts only `user@<IP address>` (no
> hostnames or custom SSH ports). The config file accepts `user@host[:port]`.

Stop the daemon with `Ctrl+C` or `SIGTERM`; it closes SSH connections and
removes its socket.

## How it works

```
cmd/portman/
  main.go       entry point; dispatches to the daemon or the CLI
  daemon.go     daemon: loads config, serves the control socket
  cli.go        client: sends a command and prints the reply
internal/
  comm/         length-prefixed JSON messages over the Unix socket
  ports/        PortMap: local listeners and the copy loop for each connection
  sshpool/      one cached *ssh.Client per user@host:port
```

- The CLI and daemon talk over `/tmp/portman.sock`. Each message is a 4-byte
  big-endian length followed by a JSON payload.
- For every accepted local connection, the daemon gets a client from the SSH
  pool, opens a `direct-tcpip` channel to the remote port, and copies data both
  ways.
- Removing a forward closes its listener and tears down its active connections.
