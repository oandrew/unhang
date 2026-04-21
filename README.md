# unhang

A reliable escape hatch for hung TUI/CLI applications. Press `^]` three times to forcefully terminate locked commands (like VMs or SSH sessions) and unstick your terminal.

Inspired by `systemd-nspawn`, `unhang` wraps your target command in a pseudo-terminal (PTY) and listens for the `^]` (Ctrl+]) key sequence. If you press `^]` three times rapidly (within 500ms of each other), `unhang` intercepts the sequence and terminates the application.

This is highly useful for commands that can become unresponsive and ignore `Ctrl+C` (SIGINT)—such as VMs (`crosvm`, `cloud-hypervisor`), SSH sessions, container managers or misbehaving scripts—saving you from having to open a new terminal pane to manually `kill` the process.

## Features

- **Triple-Escape Sequence**: Press `^] ^] ^]` rapidly to trigger termination.
- **Graceful & Forceful Kill**: Sends `SIGTERM` on the first trigger, and `SIGKILL` if triggered a second time.
- **Custom Kill Commands**: Optionally run a custom shell script to handle termination using the `-c` flag.
- **Timeout Fallback**: Automatically send a `SIGKILL` after a defined timeout using the `-k` flag.

## Installation

```sh
go install github.com/oandrew/unhang@latest
```

## Usage

Simply prefix your command with `unhang`:

```sh
unhang qemu-system-x86_64 -m 1G ...
```

If the VM locks up, just press `Ctrl+]` three times in quick succession to terminate it.

### Command-Line Arguments

* `-c <command>`: Custom bash command to execute when the escape sequence is triggered. The environment variable `$TARGET_PID` will be populated with the child process ID.
* `-k <seconds>`: Kill timeout. If set to `>= 0`, `unhang` will automatically send a `SIGKILL` to the process this many seconds after the initial `SIGTERM` or custom command.

### Examples

**Standard Usage (SIGTERM)**
```sh
unhang crosvm run  ...
```

**With Kill Timeout**
Send `SIGTERM`, wait 5 seconds, then send `SIGKILL` if still running:
```sh
unhang -k 5 podman run ...
```

**Custom Kill Command**
Use crosvm's native stop command via its control socket:
```sh
unhang -c 'crosvm stop /tmp/crosvm-$TARGET_PID.sock' crosvm run -s /tmp/ ...
```

## Why?

When working with virtualization tools or embedded emulators, it's common for the guest or the emulator itself to lock up. Since these tools often put the terminal in raw mode and consume all input (including `Ctrl+C`), you are usually forced to switch to another terminal and run `killall qemu-system-x86_64`. `unhang` solves this by running the process in a PTY and parsing the input stream for the escape sequence before passing it to the child process.
