// License: GPLv3 Copyright: 2023, Kovid Goyal, <kovid at kovidgoyal.net>

package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"
)

type terminal interface {
	WriteAllString(data string) error
	ReadWithTimeout(buf []byte, timeout time.Duration) (int, error)
	RestoreAndClose()
}

type Options struct {
	Repetitions    int
	WithScrollback bool
	Render         bool
}

const reset = "\x1b]\x1b\\\x1bc"
const ascii_printable = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ  `~!@#$%^&*()_+-=[]{}\\|;:'\",<.>/?"
const control_chars = "\n\t"
const chinese_lorem_ipsum = "旦海司有幼雞讀松鼻種比門真目怪少：扒裝虎怕您跑綠蝶黃，位香法士錯乙音造活羽詞坡村目園尺封鳥朋；法松夕點我冬停雪因科對只貓息加黃住蝶，明鴨乾春呢風乙時昔孝助？小紅女父故去。" +
	"飯躲裝個哥害共買去隻把氣年，己你校跟飛百拉！快石牙飽知唱想土人吹象毛吉每浪四又連見、欠耍外豆雞秋鼻。住步帶。" +
	"打六申幾麼：或皮又荷隻乙犬孝習秋還何氣；幾裏活打能花是入海乙山節會。種第共後陽沒喜姐三拍弟海肖，行知走亮包，他字幾，的木卜流旦乙左杯根毛。" +
	"您皮買身苦八手牛目地止哥彩第合麻讀午。原朋河乾種果「才波久住這香松」兄主衣快他玉坐要羽和亭但小山吉也吃耳怕，也爪斗斥可害朋許波怎祖葉卜。" +
	"行花兩耍許車丟學「示想百吃門高事」不耳見室九星枝買裝，枝十新央發旁品丁青給，科房火；事出出孝肉古：北裝愛升幸百東鼻到從會故北「可休笑物勿三游細斗」娘蛋占犬。我羊波雨跳風。" +
	"牛大燈兆新七馬，叫這牙後戶耳、荷北吃穿停植身玩間告或西丟再呢，他禾七愛干寺服石安：他次唱息它坐屋父見這衣發現來，苗會開條弓世者吃英定豆哭；跳風掃叫美神。" +
	"寸再了耍休壯植己，燈錯和，蝶幾欠雞定和愛，司紅後弓第樹會金拉快喝夕見往，半瓜日邊出讀雞苦歌許開；發火院爸乙；四帶亮錯鳥洋個讀。"
const misc_unicode = "‘’“”‹›«»‚„ 😀😛😇😈😉😍😎😮👍👎 —–§¶†‡©®™ →⇒•·°±−×÷¼½½¾" +
	"…µ¢£€¿¡¨´¸ˆ˜ ÀÁÂÃÄÅÆÇÈÉÊË ÌÍÎÏÐÑÒÓÔÕÖØ ŒŠÙÚÛÜÝŸÞßàá âãäåæçèéêëìí" +
	"îïðñòóôõöøœš ùúûüýÿþªºαΩ∞ ū̀n̂o᷵H̨a̠b̡͓̐c̡͓̐X̡͓̐"

var opts Options

func terminalSetState(alternateScreen bool) string {
	var sb strings.Builder
	if alternateScreen {
		sb.WriteString("\x1b7")
	}
	sb.WriteString("\x1b[?s\x1b[*x")
	// reset IRM, DECKM, DECSCNM, BRACKETED_PASTE, mouse tracking modes
	sb.WriteString("\x1b[4l\x1b[?1l\x1b[?5l\x1b[?2004l\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1005l\x1b[?1006l")
	// set DECARM, DECAWM, DECTCEM
	sb.WriteString("\x1b[?8h\x1b[?7h\x1b[?25h")
	if alternateScreen {
		sb.WriteString("\x1b[?1049h\x1b[H\x1b[2J")
	}
	sb.WriteString("\x1b[>u")
	// hide cursor
	sb.WriteString("\x1b[?25l")
	return sb.String()
}

func terminalResetState(alternateScreen bool) string {
	var sb strings.Builder
	sb.WriteString("\x1b[<u")
	if alternateScreen {
		sb.WriteString("\x1b[?1049l")
	} else {
		sb.WriteString("\x1b7")
	}
	sb.WriteString("\x1b[?r\x1b8")
	// show cursor
	sb.WriteString("\x1b[?25h")
	// reset
	sb.WriteString(reset)
	return sb.String()
}

func benchmark_data(description string, data string, opts Options) (duration time.Duration, sent_data_size int, reps int, err error) {
	term, err := openControllingTerm()
	if err != nil {
		return 0, 0, 0, err
	}
	defer term.RestoreAndClose()

	alternateScreen := !opts.WithScrollback

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-sigChan:
			_ = term.WriteAllString(terminalResetState(alternateScreen))
			term.RestoreAndClose()
			os.Exit(1)
		case <-done:
		}
	}()

	write_with_retry := func(data string) error {
		return term.WriteAllString(data)
	}

	if err = write_with_retry(terminalSetState(alternateScreen)); err != nil {
		return
	}
	defer func() {
		_ = write_with_retry(terminalResetState(alternateScreen))
	}()

	const count = 3
	const clear_screen = "\x1b[m\x1b[H\x1b[2J"
	desc := clear_screen + "Running: " + description + "\r\n"
	const pause_rendering = "\x1b[?2026h"
	const resume_rendering = "\x1b[?2026l"

	if !opts.Render {
		if err = write_with_retry(desc + pause_rendering); err != nil {
			return
		}
	}

	start := time.Now()
	end_of_loop_reset := desc
	if !opts.Render {
		end_of_loop_reset += resume_rendering + pause_rendering
	}
	for reps < opts.Repetitions {
		if err = write_with_retry(data); err != nil {
			return
		}
		sent_data_size += len(data)
		reps += 1
		if err = write_with_retry(end_of_loop_reset); err != nil {
			return
		}
	}

	finalize := clear_screen + "Waiting for response indicating parsing finished\r\n"
	if !opts.Render {
		finalize += resume_rendering
	}
	finalize += "\x1b[6n" + strings.Repeat("\x1b[5n", count)
	if err = write_with_retry(finalize); err != nil {
		return
	}

	q := []byte(strings.Repeat("\x1b[0n", count))
	var read_data []byte
	buf := make([]byte, 8192)
	deadline := time.Now().Add(30 * time.Second)
	for !bytes.Contains(read_data, q) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return duration, sent_data_size, reps, fmt.Errorf("timed out waiting for terminal response after 30s (read %d bytes: %q)", len(read_data), string(read_data))
		}
		n, rerr := term.ReadWithTimeout(buf, remaining)
		if rerr != nil {
			if errors.Is(rerr, os.ErrDeadlineExceeded) {
				return duration, sent_data_size, reps, fmt.Errorf("timed out waiting for terminal response after 30s (read %d bytes: %q)", len(read_data), string(read_data))
			}
			return duration, sent_data_size, reps, fmt.Errorf("read error: %w", rerr)
		}
		if n > 0 {
			if bytes.Contains(buf[:n], []byte{3}) {
				_ = write_with_retry(terminalResetState(alternateScreen))
				term.RestoreAndClose()
				os.Exit(1)
			}
			read_data = append(read_data, buf[:n]...)
		}
	}
	duration = time.Since(start)
	return
}

func random_string_of_bytes(n int, alphabet string) string {
	b := make([]byte, n)
	al := len(alphabet)
	for i := range n {
		b[i] = alphabet[rand.IntN(al)]
	}
	return string(b)
}

type result struct {
	desc        string
	data_sz     int
	duration    time.Duration
	repetitions int
}

func simple_ascii() (r result, err error) {
	const desc = "Only ASCII chars"
	data := random_string_of_bytes(1024*1024, ascii_printable+control_chars)
	duration, data_sz, reps, err := benchmark_data(desc, data, opts)
	if err != nil {
		return result{}, err
	}
	return result{desc, data_sz, duration, reps}, nil
}

func unicode() (r result, err error) {
	const desc = "Unicode chars"
	data := strings.Repeat(chinese_lorem_ipsum+misc_unicode+control_chars, 576)
	duration, data_sz, reps, err := benchmark_data(desc, data, opts)
	if err != nil {
		return result{}, err
	}
	return result{desc, data_sz, duration, reps}, nil
}

func unique_unicode() (r result, err error) {
	const cell_count = 144 * 1024
	const combining_count = 0x70
	var data strings.Builder
	data.Grow(cell_count * 10)
	for i := range cell_count {
		q := i
		data.WriteByte('a')
		for range 3 {
			data.WriteRune(rune(0x300 + q%combining_count))
			q /= combining_count
		}
	}
	const desc = "Unique multi-codepoint Unicode cells"
	duration, data_sz, reps, err := benchmark_data(desc, data.String(), opts)
	if err != nil {
		return result{}, err
	}
	return result{desc, data_sz, duration, reps}, nil
}

func ascii_with_csi() (r result, err error) {
	const sz = 1024*1024 + 17
	out := make([]byte, 0, sz+48)
	chunk := ""
	for len(out) < sz {
		q := rand.IntN(100)
		switch {
		case (q < 10):
			chunk = random_string_of_bytes(rand.IntN(72)+1, ascii_printable+control_chars)
		case (10 <= q && q < 30):
			chunk = "\x1b[m\x1b[?1h\x1b[H"
		case (30 <= q && q < 40):
			chunk = "\x1b[1;2;3;4:3;31m"
		case (40 <= q && q < 50):
			chunk = "\x1b[38:5:24;48:2:125:136:147m"
		case (50 <= q && q < 60):
			chunk = "\x1b[58;5;44;2m"
		case (60 <= q && q < 80):
			chunk = "\x1b[m\x1b[10A\x1b[3E\x1b[2K"
		case (80 <= q && q < 100):
			chunk = "\x1b[39m\x1b[10`a\x1b[100b\x1b[?1l"
		}
		out = append(out, chunk...)
	}
	out = append(out, "\x1b[m"...)
	const desc = "CSI codes with few chars"
	duration, data_sz, reps, err := benchmark_data(desc, string(out), opts)
	if err != nil {
		return result{}, err
	}
	return result{desc, data_sz, duration, reps}, nil
}

func images() (r result, err error) {
	const dim = 1024
	payload := make([]byte, 4*dim*dim)
	b64 := base64.RawStdEncoding.EncodeToString(payload)
	var b strings.Builder
	b.Grow(8 * dim * dim)

	const chunkSize = 128 * 1024
	first := true
	for len(b64) > 0 {
		chunk := b64
		more := 0
		if len(b64) > chunkSize {
			chunk = b64[:chunkSize]
			b64 = b64[chunkSize:]
			more = 1
		} else {
			b64 = ""
		}

		if first {
			fmt.Fprintf(&b, "\x1b_Ga=t,q=2,f=32,s=%d,v=%d,i=12345,m=%d;%s\x1b\\", dim, dim, more, chunk)
			first = false
		} else {
			fmt.Fprintf(&b, "\x1b_Ga=t,q=2,m=%d;%s\x1b\\", more, chunk)
		}
	}

	// Delete command
	b.WriteString("\x1b_Ga=d,q=2,d=I,i=12345\x1b\\")

	data := b.String()
	const desc = "Images"
	duration, data_sz, reps, err := benchmark_data(desc, data, opts)
	if err != nil {
		return result{}, err
	}
	return result{desc, data_sz, duration, reps}, nil
}

func long_escape_codes() (r result, err error) {
	data := random_string_of_bytes(8024, ascii_printable)
	// OSC 6 is document reporting or XTerm special color which terminals ignore after parsing
	data = strings.Repeat("\x1b]6;"+data+"\x07", 1024)
	const desc = "Long escape codes"
	duration, data_sz, reps, err := benchmark_data(desc, data, opts)
	if err != nil {
		return result{}, err
	}
	return result{desc, data_sz, duration, reps}, nil
}

var divs = []time.Duration{
	time.Duration(1), time.Duration(10), time.Duration(100), time.Duration(1000)}

func round(d time.Duration, digits int) time.Duration {
	switch {
	case d > time.Second:
		d = d.Round(time.Second / divs[digits])
	case d > time.Millisecond:
		d = d.Round(time.Millisecond / divs[digits])
	case d > time.Microsecond:
		d = d.Round(time.Microsecond / divs[digits])
	}
	return d
}

func present_result(r result, col_width int) {
	rate := float64(r.data_sz) / r.duration.Seconds()
	rate /= 1024. * 1024.
	f := fmt.Sprintf("%%-%ds", col_width)
	fmt.Printf("  "+f+" : %-10v @ \x1b[32m%-7.1f\x1b[m MB/s\n", r.desc, round(r.duration, 2), rate)
}

func all_benchmarks() []string {
	return []string{
		"ascii", "unicode", "unique_unicode", "csi", "images", "long_escape_codes",
	}
}

func warmup_data() string {
	num_colors := 1000 + rand.IntN(1001)
	type rgb struct{ r, g, b uint8 }
	colors := make([]rgb, num_colors)
	for i := range colors {
		colors[i] = rgb{uint8(rand.IntN(256)), uint8(rand.IntN(256)), uint8(rand.IntN(256))}
	}
	colored_text := ascii_printable + control_chars + chinese_lorem_ipsum
	var b strings.Builder
	b.Grow(len(colored_text)*20 + len(misc_unicode) + 8)
	color_idx := 0
	for _, ch := range colored_text {
		c := colors[color_idx%num_colors]
		fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm%c", c.r, c.g, c.b, ch)
		color_idx++
	}
	b.WriteString("\x1b[m")
	b.WriteString(misc_unicode)
	return b.String()
}

func runBenchmarks(args []string) (err error) {
	if len(args) == 0 {
		args = all_benchmarks()
	}
	var results []result
	var r result
	// First warm up the terminal by getting it to render all chars so that font rendering
	// time is not polluting the benchmarks.
	w := Options{Repetitions: 1}
	if _, _, _, err = benchmark_data("Warmup", warmup_data(), w); err != nil {
		return err
	}
	time.Sleep(time.Second / 2)

	if slices.Index(args, "ascii") >= 0 {
		if r, err = simple_ascii(); err != nil {
			return err
		}
		results = append(results, r)
	}

	if slices.Index(args, "unicode") >= 0 {
		if r, err = unicode(); err != nil {
			return err
		}
		results = append(results, r)
	}

	if slices.Index(args, "unique_unicode") >= 0 {
		if r, err = unique_unicode(); err != nil {
			return err
		}
		results = append(results, r)
	}

	if slices.Index(args, "csi") >= 0 {
		if r, err = ascii_with_csi(); err != nil {
			return err
		}
		results = append(results, r)
	}

	if slices.Index(args, "long_escape_codes") >= 0 {
		if r, err = long_escape_codes(); err != nil {
			return err
		}
		results = append(results, r)
	}

	if slices.Index(args, "images") >= 0 {
		if r, err = images(); err != nil {
			return err
		}
		results = append(results, r)
	}

	fmt.Print(reset)
	fmt.Println(
		"These results measure the time it takes the terminal to fully parse all the data sent to it.")
	if opts.Render {
		fmt.Println("Note that not all data transmitted will be displayed as input parsing is typically asynchronous with rendering in high performance terminals.")
	} else {
		fmt.Println("Note that \x1b[31mrendering is suppressed\x1b[m (if the terminal supports the synchronized output escape code) to better benchmark parser performance. Use the --render flag to enable rendering.")
	}
	fmt.Println()
	fmt.Println("Results:")
	mlen := 10
	for _, r := range results {
		mlen = max(mlen, len(r.desc))
	}
	for _, r := range results {
		present_result(r, mlen)
	}
	return
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options] [optional benchmark to run ...]\n\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), "Benchmarking works by sending large amounts of data to the terminal device")
		fmt.Fprintln(flag.CommandLine.Output(), "and waiting for the terminal to process the data and respond to queries sent to it.")
		fmt.Fprintln(flag.CommandLine.Output(), "By default rendering is suppressed during benchmarking to focus on parser performance.")
		fmt.Fprintln(flag.CommandLine.Output(), "Use the --render flag to enable it.\n")
		fmt.Fprintln(flag.CommandLine.Output(), "Options:")
		flag.PrintDefaults()
		fmt.Fprintln(flag.CommandLine.Output(), "\nAvailable benchmarks:")
		fmt.Fprintf(flag.CommandLine.Output(), "  %s\n", strings.Join(all_benchmarks(), ", "))
	}

	flag.IntVar(&opts.Repetitions, "repetitions", 500, "The number of repetitions of each benchmark")
	flag.BoolVar(&opts.WithScrollback, "with-scrollback", false, "Use the main screen instead of the alt screen so speed of scrollback is also tested")
	flag.BoolVar(&opts.Render, "render", false, "Allow rendering of the data sent during tests. Note that modern terminals render asynchronously, so timings do not generally reflect render performance.")

	flag.Parse()

	opts.Repetitions = max(1, opts.Repetitions)

	args := flag.Args()
	validBenchmarks := all_benchmarks()
	for _, arg := range args {
		if !slices.Contains(validBenchmarks, arg) {
			fmt.Fprintf(os.Stderr, "Unknown benchmark: %s\nValid benchmarks are: %s\n", arg, strings.Join(validBenchmarks, ", "))
			os.Exit(1)
		}
	}

	if err := runBenchmarks(args); err != nil {
		fmt.Fprintf(os.Stderr, "Benchmark failed: %v\n", err)
		os.Exit(1)
	}
}
