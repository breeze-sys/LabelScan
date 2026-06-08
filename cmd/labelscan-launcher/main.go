package main

import (
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var defaultURL = "https://labelscan.site"

func main() {
	configURL := firstNonEmpty(
		os.Getenv("LABELSCAN_URL"),
		readSidecarURL(),
		defaultURL,
	)
	targetURL := flag.String("url", configURL, "LabelScan web console URL")
	flag.Parse()

	if err := validateHTTPURL(*targetURL); err != nil {
		exitWithError(err)
	}
	if err := openURL(*targetURL); err != nil {
		exitWithError(fmt.Errorf("open %s: %w", *targetURL, err))
	}
}

func readSidecarURL() string {
	exePath, err := os.Executable()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(exePath), "labelscan.url"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	return ""
}

func validateHTTPURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("URL must start with http:// or https://")
	}
	if parsed.Host == "" {
		return errors.New("URL must include a host")
	}
	return nil
}

func openURL(raw string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", raw).Start()
	case "darwin":
		return exec.Command("open", raw).Start()
	default:
		return exec.Command("xdg-open", raw).Start()
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func exitWithError(err error) {
	_, _ = fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
