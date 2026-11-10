// Package relversion decides whether VERSION.md should publish a GitHub release.
package relversion

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is a numeric MAJOR.MINOR.PATCH semver.
type Version struct {
	Major int
	Minor int
	Patch int
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func (v Version) Tag() string {
	return "v" + v.String()
}

// Compare returns -1, 0, or 1.
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		return cmpInt(v.Major, other.Major)
	}
	if v.Minor != other.Minor {
		return cmpInt(v.Minor, other.Minor)
	}
	return cmpInt(v.Patch, other.Patch)
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// Parse accepts "1.2.3" or "v1.2.3". Pre-release and build metadata are rejected.
// Numeric parts must not have leading zeros.
func Parse(raw string) (Version, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return Version{}, fmt.Errorf("empty version")
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("version %q must be MAJOR.MINOR.PATCH", raw)
	}
	nums := [3]int{}
	for i, part := range parts {
		if part == "" || !canonicalNumber(part) {
			return Version{}, fmt.Errorf("version %q has invalid component %q", raw, part)
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return Version{}, fmt.Errorf("version %q has invalid component %q", raw, part)
		}
		nums[i] = n
	}
	return Version{Major: nums[0], Minor: nums[1], Patch: nums[2]}, nil
}

func canonicalNumber(s string) bool {
	if s == "0" {
		return true
	}
	if s[0] == '0' {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Decision is the release workflow outcome for one VERSION.md value.
type Decision struct {
	Release bool
	Version Version
	Reason  string
}

// Decide publishes only when current is strictly greater than every known
// semver tag or release. Non-semver names are ignored. An empty known list
// means nothing has been published yet, so current is released.
func Decide(current string, known []string) (Decision, error) {
	cur, err := Parse(current)
	if err != nil {
		return Decision{}, fmt.Errorf("VERSION.md: %w", err)
	}
	var (
		haveLatest bool
		latest     Version
		ignored    int
	)
	for _, name := range known {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		parsed, err := Parse(name)
		if err != nil {
			ignored++
			continue
		}
		if !haveLatest || parsed.Compare(latest) > 0 {
			latest = parsed
			haveLatest = true
		}
	}
	d := Decision{Version: cur}
	switch {
	case !haveLatest:
		d.Release = true
		if ignored > 0 {
			d.Reason = fmt.Sprintf("no semver release exists yet; publishing %s", cur.Tag())
		} else {
			d.Reason = fmt.Sprintf("no release exists yet; publishing %s", cur.Tag())
		}
	case cur.Compare(latest) > 0:
		d.Release = true
		d.Reason = fmt.Sprintf("%s is greater than %s; publishing", cur.Tag(), latest.Tag())
	default:
		d.Release = false
		d.Reason = fmt.Sprintf("%s is not greater than %s; skipping release", cur.Tag(), latest.Tag())
	}
	return d, nil
}
