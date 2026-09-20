# Terminal Throughput Benchmark

A standalone terminal throughput and parser benchmark tool, extracted from Kitty's internal benchmarking suite and enhanced to run natively on **Windows (including Windows Terminal and PowerShell)** as well as **Linux and macOS**.

## How It Works

The benchmark sends large streams of text and escape sequences directly to the controlling terminal device (`CONIN$`/`CONOUT$` on Windows, `/dev/tty` on Unix) in raw mode. It uses synchronized output sequences (`\x1b[?2026h`/`l`) to suppress rendering during tests, focusing on parser throughput.

To accurately measure when processing finishes, the tool emits a terminal status query (`\x1b[5n`) at the end of each benchmark run and times how long it takes for the terminal to parse all sent data and return the response (`\x1b[0n`).

## Benchmarks Included

- **`ascii`**: Rapid throughput of printable ASCII characters and newlines/tabs (~1 MB per rep).
- **`unicode`**: High-density Unicode text including CJK lorem ipsum, symbols, emojis, and combining marks (~1 MB per rep).
- **`unique_unicode`**: 144K unique multi-codepoint Unicode grapheme clusters with 3 combining characters each (~1 MB per rep).
- **`csi`**: Dense mix of ANSI / CSI escape sequences (cursor movements, SGR colors, line editing, etc., ~1 MB per rep).
- **`images`**: Chunked Kitty Graphics Protocol APC sequences (`\x1b_G...`). On terminals with Kitty graphics support (e.g. WezTerm, Ghostty, Kitty), tests graphics transmission; on others (e.g. Windows Terminal), stress-tests APC parsing and sequence skipping.
- **`long_escape_codes`**: Long OSC escape sequences (8 KB payload per sequence).

## Windows Adaptation & Architecture Notes

### 1. Payload Sized to ~1 MB (Synchronized Output Compliance)
- **Change**: Reduced the repetition payload from ~2.097 MB (`1024*2048 + 13` bytes) to ~1 MB (`1024*1024` bytes) across text benchmarks.
- **Rationale**: Terminals adhering to the terminal-wg synchronized output specification (e.g. Alacritty, Vivido / `vvte`) enforce a strict 2 MiB safety limit (`SYNC_BUFFER_SIZE = 0x20_0000`, 2,097,152 bytes) on `\x1b[?2026h` (DECSET 2026) blocks to guard against denial-of-service memory exhaustion attacks from child processes. The original Unix Kitty benchmark used 2,097,165 bytes (> 2 MiB), which exceeded this limit on every single repetition. This triggered an intentional security abort in compliant terminals: flushing the buffer, deallocating back to 64 KB, waking up the event loop, and repainting the GUI 500 times in a tight loop. Capping payloads to ~1 MB allows DECSET 2026 to suppress rendering as intended across all terminals.

### 2. Windows `CONOUT$` Write Chunk Size Increased to 1 MB
- **Change**: In `term_windows.go`, increased `WriteAllString` chunking from 64 KB (`64 * 1024`) to 1 MB (`1024 * 1024`).
- **Rationale**: On Windows, console writes to `CONOUT$` cross the console driver boundary into the Windows Pseudoconsole host (ConPTY / `conhost.exe`). Chunking high-throughput data at 64 KB incurred tens of thousands of redundant LPC/syscall transitions. Increasing the chunk cap to 1 MB aligns write calls with the benchmark payload size and substantially minimizes driver transition overhead.

### 3. Understanding ConPTY vs Emulator Performance
- **ConPTY Passthrough vs Inbox Host**: Windows Terminal ships with a bundled `OpenConsole.exe` supporting ConPTY Passthrough mode (`PSEUDOCONSOLE_PASSTHROUGH`), streaming raw VT sequences with zero intermediate console buffer emulation. Third-party terminals using the system `CreatePseudoConsole` run through the Windows inbox `conhost.exe`, which maintains an internal 2D character buffer for legacy console API compatibility, translating character output twice (once inside `conhost.exe` and once inside the terminal engine).
- **Escape Sequence (CSI) Throughput**: While raw ASCII streaming is constrained by the Windows ConPTY pipeline, escape-heavy workloads (cursor repositioning, SGR color changes, line clears) reflect actual TUI application performance (`htop`, `nvim`, `helix`). In CSI benchmarks, optimized parser engines (such as Vivido's `vvte`) process escape sequences up to **13× faster** than Windows Terminal, which exhibits severe parser stalls on dense CSI sequences.

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
