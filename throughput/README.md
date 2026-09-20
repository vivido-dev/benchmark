# Terminal Throughput Benchmark

A standalone terminal throughput and parser benchmark tool, extracted from Kitty's internal benchmarking suite and enhanced to run natively on **Windows (including Windows Terminal and PowerShell)** as well as **Linux and macOS**.

## How It Works

The benchmark sends large streams of text and escape sequences directly to the controlling terminal device (`CONIN$`/`CONOUT$` on Windows, `/dev/tty` on Unix) in raw mode. It uses synchronized output sequences (`\x1b[?2026h`/`l`) to suppress rendering during tests, focusing on parser throughput.

To accurately measure when processing finishes, the tool emits a terminal status query (`\x1b[5n`) at the end of each benchmark run and times how long it takes for the terminal to parse all sent data and return the response (`\x1b[0n`).

## Benchmarks Included

- **`ascii`**: Rapid throughput of printable ASCII characters and newlines/tabs (~2 MB per rep).
- **`unicode`**: High-density Unicode text including CJK lorem ipsum, symbols, emojis, and combining marks.
- **`unique_unicode`**: 256K unique multi-codepoint Unicode grapheme clusters with 3 combining characters each.
- **`csi`**: Dense mix of ANSI / CSI escape sequences (cursor movements, SGR colors, line editing, etc.).
- **`images`**: Chunked Kitty Graphics Protocol APC sequences (`\x1b_G...`). On terminals with Kitty graphics support (e.g. WezTerm, Ghostty, Kitty), tests graphics transmission; on others (e.g. Windows Terminal), stress-tests APC parsing and sequence skipping.
- **`long_escape_codes`**: Long OSC escape sequences (8 KB payload per sequence).

## Building & Running

### Requirements
- [Go 1.22+](https://go.dev/dl/)

### Run Directly
```sh
go run .
```

### Build Binary
```sh
# On Windows PowerShell:
go build -o benchmark.exe .
.\benchmark.exe

# On Linux / macOS:
go build -o benchmark .
./benchmark
```

## Options & Flags

```
Usage: benchmark [options] [optional benchmark to run ...]

Options:
  --repetitions int
        The number of repetitions of each benchmark (default 500)
  --with-scrollback
        Use the main screen instead of the alt screen so speed of scrollback is also tested
  --render
        Allow rendering of the data sent during tests (note: rendering is asynchronous in modern terminals)

Available benchmarks:
  ascii, unicode, unique_unicode, csi, images, long_escape_codes
```

### Examples

Run all benchmarks with default 500 repetitions:
```powershell
.\benchmark.exe
```

Run only ASCII and Unicode benchmarks with 50 repetitions:
```powershell
.\benchmark.exe --repetitions 50 ascii unicode
```

Test with main screen scrollback:
```powershell
.\benchmark.exe --with-scrollback --repetitions 20 ascii
```

Enable live rendering during benchmark:
```powershell
.\benchmark.exe --render --repetitions 10 csi
```
