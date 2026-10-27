package main

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Query completion is parsed-terminal latency, independent of GPU presentation.
func measureLatency(samples int) error {
	term, err := openControllingTerm()
	if err != nil {
		return err
	}
	defer term.RestoreAndClose()
	for _, loaded := range []bool{false, true} {
		values := make([]float64, 0, samples)
		for range samples {
			payload := "\x1b[5n"
			if loaded {
				payload = strings.Repeat("abcdefghijklmnopqrstuvwx\r\n", 4096) + payload
			}
			start := time.Now()
			if err := term.WriteAllString(payload); err != nil {
				return err
			}
			deadline := time.Now().Add(5 * time.Second)
			var response []byte
			buffer := make([]byte, 128)
			for !bytes.Contains(response, []byte("\x1b[0n")) {
				remaining := time.Until(deadline)
				if remaining <= 0 {
					return fmt.Errorf("query response timed out")
				}
				n, err := term.ReadWithTimeout(buffer, remaining)
				if err != nil {
					return err
				}
				response = append(response, buffer[:n]...)
				if len(response) > 4096 {
					return fmt.Errorf("unexpected query response size")
				}
			}
			values = append(values, float64(time.Since(start).Nanoseconds())/1e6)
		}
		slices.Sort(values)
		fmt.Printf("LATENCY loaded=%v samples=%d p50_ms=%.6f p99_ms=%.6f max_ms=%.6f\n", loaded, samples, values[len(values)/2], values[(len(values)-1)*99/100], values[len(values)-1])
	}
	return nil
}
