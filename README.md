# lua-mapper

A tiny line-oriented stream mapper. It reads lines from stdin, extracts
**named capture groups** with a Go regular expression and prints a template
with the captured values substituted in. It runs until stdin reaches EOF or
you press **Ctrl+C**.

```
stdin lines ──▶ regex named groups ──▶ ${name} template ──▶ stdout
```

No dependencies, single static binary.

## Build

```sh
go build -o lua-mapper .
# or
make build
```

## Usage

```sh
lua-mapper -expr REGEX -tmpl TEMPLATE
```

| Flag    | Description                                                  |
| ------- | ------------------------------------------------------------ |
| `-expr` | Regular expression containing named capture groups (required) |
| `-tmpl` | Output template; `${name}` is replaced by the capture (required) |
| `-v`    | Report lines that do not match on stderr                      |

Named groups use Go's regexp syntax:

- `(?P<name>...)`
- `(?<name>...)` (Go 1.22 and newer)

## Examples

Basic mapping:

```sh
printf 'john doe\njane roe\n' |
  lua-mapper \
    -expr '^(?P<name>\w+)\s+(?P<surname>\w+)$' \
    -tmpl '{"name":"${name}","surname":"${surname}"}'
```

Output:

```json
{"name":"john","surname":"doe"}
{"name":"jane","surname":"roe"}
```

Interactive use (keeps running until Ctrl+C):

```sh
$ lua-mapper -expr '^(?P<key>\w+)=(?P<value>.*)$' -tmpl '${key} -> ${value}'
a=1
a -> 1
b=2
b -> 2
^C
```

Following a log stream:

```sh
tail -f app.log |
  lua-mapper -expr '^(?P<time>\S+) (?P<level>\w+) (?P<msg>.*)$' \
             -tmpl '${time} [${level}] ${msg}'
```

## Semantics

- Each input line is tested against `-expr`; the **first match** on that line
  is used. Anchor with `^...$` to require the whole line.
- Lines with no match are **skipped**. Run with `-v` to log them to stderr.
- A named group that matched nothing (for example an optional group) is
  replaced with an empty string.
- Template references to groups that do not exist in `-expr` are rejected at
  startup.
- `$$` produces a literal `$`, so `$${name}` outputs `${name}`.
- A trailing newline is added unless the rendered template already ends with
  one.
- Input lines may be arbitrarily long; CRLF line endings are handled.
- Output is buffered for throughput and flushed per line when stdout is a
  terminal, so interactive output appears immediately.
- Ctrl+C / SIGTERM stop the loop cleanly, flushing pending output.

Quote templates with single quotes in the shell so `${...}` is not expanded by
the shell.

## Development

```sh
make test   # go test ./...
make vet    # go vet ./...
make fmt    # gofmt
```
