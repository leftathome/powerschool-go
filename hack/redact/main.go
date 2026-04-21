package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	in := flag.String("in", "", "path to raw HTML capture")
	out := flag.String("out", "", "path to write redacted HTML (caller must ensure the directory exists)")
	cfg := flag.String("config", "hack/redact.config.json", "path to redaction config (gitignored)")
	flag.Parse()

	if *in == "" || *out == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./hack/redact -in <capture> -out <fixture> [-config <path>]")
		flag.Usage()
		os.Exit(2)
	}

	if err := run(*in, *out, *cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(inPath, outPath, cfgPath string) error {
	c, err := LoadConfig(cfgPath)
	if err != nil {
		return err
	}

	raw, err := os.ReadFile(inPath)
	if err != nil {
		return fmt.Errorf("redact: read input: %w", err)
	}

	redacted, err := Redact(string(raw), c)
	if err != nil {
		return err
	}

	if err := os.WriteFile(outPath, []byte(redacted), 0o644); err != nil {
		return fmt.Errorf("redact: write output: %w", err)
	}
	return nil
}
