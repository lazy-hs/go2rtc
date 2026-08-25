package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const usage = `Version management for go2rtc

Usage:
  go run ./cmd/version show
  go run ./cmd/version check
  go run ./cmd/version next <regular|special|major>
  go run ./cmd/version bump <regular|special|major>
  go run ./cmd/version check-tag <vM.m.p>
  go run ./cmd/version check-transition <old> <new> <regular|special|major>

Aliases:
  regular = minor
  special = patch
`

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type version struct {
	major int
	minor int
	patch int
}

func (v version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.major, v.minor, v.patch)
}

type releaseType string

const (
	releaseRegular releaseType = "regular"
	releaseSpecial releaseType = "special"
	releaseMajor   releaseType = "major"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Print(usage)
		return nil
	}

	switch args[0] {
	case "check-transition":
		if len(args) != 4 {
			return errors.New("usage: check-transition <old> <new> <regular|special|major>")
		}
		oldVersion, err := parseVersion(args[1])
		if err != nil {
			return fmt.Errorf("invalid old version: %w", err)
		}
		newVersion, err := parseVersion(args[2])
		if err != nil {
			return fmt.Errorf("invalid new version: %w", err)
		}
		kind, err := parseReleaseType(args[3])
		if err != nil {
			return err
		}
		return checkTransition(oldVersion, newVersion, kind)
	}

	versionFile, err := findVersionFile()
	if err != nil {
		return err
	}
	current, err := readVersion(versionFile)
	if err != nil {
		return err
	}

	switch args[0] {
	case "show":
		if len(args) != 1 {
			return errors.New("usage: show")
		}
		fmt.Println(current)
		return nil
	case "check":
		if len(args) != 1 {
			return errors.New("usage: check")
		}
		fmt.Printf("VERSION is valid: %s\n", current)
		return nil
	case "next", "bump":
		if len(args) != 2 {
			return fmt.Errorf("usage: %s <regular|special|major>", args[0])
		}
		kind, err := parseReleaseType(args[1])
		if err != nil {
			return err
		}
		next := current.bump(kind)
		if args[0] == "next" {
			fmt.Println(next)
			return nil
		}
		if err := os.WriteFile(versionFile, []byte(next.String()+"\n"), 0o644); err != nil {
			return fmt.Errorf("write VERSION: %w", err)
		}
		fmt.Printf("VERSION updated: %s -> %s (%s)\n", current, next, kind)
		return nil
	case "check-tag":
		if len(args) != 2 {
			return errors.New("usage: check-tag <vM.m.p>")
		}
		expected := "v" + current.String()
		if args[1] != expected {
			return fmt.Errorf("tag %q does not match VERSION; expected %q", args[1], expected)
		}
		fmt.Printf("tag matches VERSION: %s\n", args[1])
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usage)
	}
}

func parseVersion(value string) (version, error) {
	value = strings.TrimSpace(value)
	match := versionPattern.FindStringSubmatch(value)
	if match == nil {
		return version{}, fmt.Errorf("%q must use M.m.p numeric format without leading zeros", value)
	}

	parts := [3]int{}
	for i := range parts {
		parsed, err := strconv.Atoi(match[i+1])
		if err != nil {
			return version{}, fmt.Errorf("parse %q: %w", value, err)
		}
		parts[i] = parsed
	}
	if parts[1] >= 10 {
		return version{}, fmt.Errorf("minor version must be between 0 and 9; roll over to the next major version")
	}
	return version{major: parts[0], minor: parts[1], patch: parts[2]}, nil
}

func parseReleaseType(value string) (releaseType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "regular", "minor":
		return releaseRegular, nil
	case "special", "patch":
		return releaseSpecial, nil
	case "major":
		return releaseMajor, nil
	default:
		return "", fmt.Errorf("unknown release type %q; use regular, special, or major", value)
	}
}

func (v version) bump(kind releaseType) version {
	switch kind {
	case releaseRegular:
		if v.minor == 9 {
			return version{major: v.major + 1}
		}
		return version{major: v.major, minor: v.minor + 1}
	case releaseSpecial:
		return version{major: v.major, minor: v.minor, patch: v.patch + 1}
	case releaseMajor:
		return version{major: v.major + 1}
	default:
		panic("unsupported release type: " + kind)
	}
}

func checkTransition(oldVersion, newVersion version, kind releaseType) error {
	expected := oldVersion.bump(kind)
	if newVersion != expected {
		return fmt.Errorf("invalid %s transition: %s -> %s; expected %s", kind, oldVersion, newVersion, expected)
	}
	fmt.Printf("valid %s transition: %s -> %s\n", kind, oldVersion, newVersion)
	return nil
}

func readVersion(path string) (version, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return version{}, fmt.Errorf("read VERSION: %w", err)
	}
	parsed, err := parseVersion(string(data))
	if err != nil {
		return version{}, fmt.Errorf("invalid VERSION file: %w", err)
	}
	return parsed, nil
}

func findVersionFile() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}

	for {
		versionFile := filepath.Join(dir, "VERSION")
		moduleFile := filepath.Join(dir, "go.mod")
		if fileExists(versionFile) && fileExists(moduleFile) {
			return versionFile, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not find VERSION and go.mod in the current directory or its parents")
		}
		dir = parent
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
