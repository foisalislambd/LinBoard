// Command planrelease prints the release decision for VERSION.md.
// Known tags and GitHub release names are passed with -known (repeatable).
// Non-semver names are ignored. Output is GitHub Actions output syntax.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/foisal/linboard/internal/relversion"
)

func main() {
	file := flag.String("version-file", "VERSION.md", "path to the version file")
	var known []string
	flag.Func("known", "existing tag or release name (repeatable)", func(s string) error {
		known = append(known, s)
		return nil
	})
	flag.Parse()

	raw, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", *file, err)
		os.Exit(1)
	}
	current := strings.TrimSpace(string(raw))
	// VERSION.md is a single version. Extra lines are a mistake.
	if strings.Contains(current, "\n") {
		fmt.Fprintf(os.Stderr, "%s must contain only the version number\n", *file)
		os.Exit(1)
	}

	decision, err := relversion.Decide(current, known)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	release := "false"
	if decision.Release {
		release = "true"
	}
	fmt.Printf("release=%s\n", release)
	fmt.Printf("version=%s\n", decision.Version.String())
	fmt.Printf("tag=%s\n", decision.Version.Tag())
	fmt.Printf("reason=%s\n", decision.Reason)
	fmt.Fprintln(os.Stderr, decision.Reason)
}
