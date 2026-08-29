# ldsum

Verify a file against an expected checksum.

The file comes from a local path or a URL, and the expected checksum is either
given inline or read from a checksum file — a `SHA256SUMS` published alongside
a release, say. `ldsum` prints whether the computed digest matches and exits
non-zero when it does not, so it drops into a script.

> **Status:** The `verify` command works with local files and inline checksums.
> URL input and checksum-file input are not yet implemented.

## Usage

Verify a file against a checksum:

```sh
ldsum verify dist.tar.gz ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad
dist.tar.gz: OK
```

When the digests differ, the expected and actual values go to stderr:

```sh
ldsum verify dist.tar.gz 0000000000000000000000000000000000000000000000000000000000000000
dist.tar.gz: FAILED
expected: 0000000000000000000000000000000000000000000000000000000000000000
actual:   ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad
```

The algorithm comes from the length of the checksum — 64 hex characters is
sha256, 128 is sha512. Name it explicitly with `--algo` when you want the
length checked too:

```sh
ldsum verify --algo sha256 dist.tar.gz ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad
```

Exit codes, so it drops into a script:

| Code | Meaning |
|------|---------|
| 0 | the digest matched |
| 1 | the digest did not match, or the file is missing |
| 2 | the command was wrong: bad checksum, unknown algorithm, unreadable file |

## Requirements

- Go 1.27 or newer
- `git` and `bash` — the hook tests shell out to both

## Setup

```sh
git clone https://github.com/BobMali/ldsum.git
cd ldsum
git config core.hooksPath githooks
go build ./...
go test ./...
```

The `core.hooksPath` line is the one step that is easy to miss. It is
repository-local config, so a clone does not carry it, and without it the
`commit-msg` hook silently never runs.

## Commit messages

Commits follow [Conventional Commits](https://www.conventionalcommits.org),
enforced by `githooks/commit-msg`:

```
<type>(<scope>): <description>
```

Types are `feat` `fix` `docs` `test` `refactor` `perf` `build` `ci` `chore`
`revert`; the optional scopes are `hash` `checksums` `cmd` `run` `ci` `deps`.
The description is lowercase and imperative with no trailing period. The
pattern itself lives in `githooks/conventional-regex.txt`.

A rejected commit is a message problem — fix the message rather than skipping
the hook.

## Development

```sh
go build ./...
go vet ./...
gofmt -l .          # prints nothing when clean
go test ./...
```

All four pass before a change is done.
