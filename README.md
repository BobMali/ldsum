# ldsum

Verify a file against an expected checksum.

The file comes from a local path or a URL, and the expected checksum is either
given inline or read from a checksum file — a `SHA256SUMS` published alongside
a release, say. `ldsum` prints whether the computed digest matches and exits
non-zero when it does not, so it drops into a script.

> **Status:** `sum` computes checksums for local files and directories.
> `verify` works with local files, given a checksum inline or read from a
> checksum file. URL input is not yet implemented.

## Usage

### Compute a checksum

```sh
ldsum sum dist.tar.gz
ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad  dist.tar.gz
```

The default is the GNU coreutils text format, so the output is a `SHA256SUMS`
line. Four formats are available, and they are mutually exclusive:

| Flag | Output |
|------|--------|
| `--text`, `-t` | `<digest>  <path>` — the default |
| `--binary`, `-b` | `<digest> *<path>` |
| `--tag` | `SHA256 (<path>) = <digest>` |
| `--bare` | `<digest>` alone, for `d=$(ldsum sum --bare f)` |

A path containing a backslash or a newline is escaped the way coreutils does
it: the line gains a leading `\`, a backslash becomes `\\`, and a newline
becomes `\n`. Tagged format has no such convention, so such a path is an error
there.

`--algo` picks the algorithm, defaulting to sha256:

```sh
ldsum sum --algo sha512 dist.tar.gz
```

Directories are walked only with `-r`, which is deliberate — hashing a tree is
too large an action to start because an argument happened to be a directory:

```sh
ldsum sum -r ./dist > SHA256SUMS
```

`-o` writes the lines to a file instead, truncating whatever is there:

```sh
ldsum sum -r ./dist -o SHA256SUMS
```

It behaves like the redirection above, which includes the one hazard: an output
file inside the tree being walked exists by the time the walk reaches it, and is
summed into its own listing. Write it outside the tree, or name the paths.

A symlink named as an argument is followed — naming it is how you ask for it.
Symlinks found by walking are not, so no file is summed twice and no cycle can
occur. Skipped entries are silent unless `-v` names them on stderr.

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

### Verify against a checksum file

`--sums-file`, or `-c`, reads the expected checksums from a file:

```sh
ldsum verify -c SHA256SUMS
dist.tar.gz: OK
docs.pdf: OK
```

The format is recognised per line, so a file may mix them: the GNU text
(`<digest>  <path>`) and binary (`<digest> *<path>`) formats, the BSD tagged
format (`SHA256 (<path>) = <digest>`), and a file holding nothing but a
digest. Escaped paths are unescaped, so anything `ldsum sum` writes can be
read straight back.

Entries resolve relative to **the checksum file's directory**, not the working
directory — a published `SHA256SUMS` sits beside the files it describes, so
that is what its paths mean:

```sh
ldsum verify -c ~/downloads/SHA256SUMS      # no need to cd first
```

An entry spelled as an absolute path is used as it stands, since it already
says where its file is, so a listing `ldsum sum` wrote from absolute paths
reads back wherever it is kept:

```sh
ldsum sum /srv/dist/app.tar.gz > SUMS
ldsum verify -c SUMS
```

This is the main place `ldsum` differs from `sha256sum -c`, which resolves
against the working directory.

Naming files checks only those entries, in the order given:

```sh
ldsum verify -c SHA256SUMS dist.tar.gz
```

A file that holds a bare digest names nothing, so name the file yourself:

```sh
ldsum verify -c dist.tar.gz.sha256 dist.tar.gz
```

A symlink listed in a checksum file is followed, the same as one named on the
command line. The reason differs, though: `ldsum sum` follows a named symlink
because naming it is how you asked for it, and under `-c` it is the file doing
the naming rather than you.

A mismatch does not stop the run: every file is reported and the summary goes
to stderr.

```sh
ldsum verify -c SHA256SUMS
dist.tar.gz: OK
docs.pdf: FAILED
expected: 0000000000000000000000000000000000000000000000000000000000000000
actual:   ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad
ldsum: 1 of 2 files failed
```

Lines that are not checksums are named on stderr and skipped; a file with no
usable lines is an error. `--algo` and `--sums-file` cannot be combined —
the file says which algorithm each entry uses.

## Exit codes

Both subcommands exit so they drop straight into a script:

| Code | Meaning |
|------|---------|
| 0 | every digest matched, or every file was summed |
| 1 | a digest did not match, or a file being verified is missing |
| 2 | the command could not be carried out: wrong argument count, bad checksum, unknown algorithm, an unreadable file, an unusable checksum file, an output file that cannot be written, a directory without `-r` |

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
