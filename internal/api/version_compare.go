package api

import (
	"regexp"
	"strconv"
	"strings"
)

type semVersion struct {
	major      int
	minor      int
	patch      int
	prerelease []string
}

var semVersionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)
var numericIdentifierPattern = regexp.MustCompile(`^\d+$`)

func parseSemVersion(raw string) (semVersion, bool) {
	m := semVersionPattern.FindStringSubmatch(strings.TrimSpace(raw))
	if len(m) == 0 {
		return semVersion{}, false
	}

	major, err := strconv.Atoi(m[1])
	if err != nil {
		return semVersion{}, false
	}
	minor, err := strconv.Atoi(m[2])
	if err != nil {
		return semVersion{}, false
	}
	patch, err := strconv.Atoi(m[3])
	if err != nil {
		return semVersion{}, false
	}

	var prerelease []string
	if m[4] != "" {
		for _, token := range strings.Split(m[4], ".") {
			if token == "" {
				continue
			}
			prerelease = append(prerelease, token)
		}
	}

	return semVersion{
		major:      major,
		minor:      minor,
		patch:      patch,
		prerelease: prerelease,
	}, true
}

func compareSemVersion(a, b semVersion) int {
	if a.major != b.major {
		return compareInt(a.major, b.major)
	}
	if a.minor != b.minor {
		return compareInt(a.minor, b.minor)
	}
	if a.patch != b.patch {
		return compareInt(a.patch, b.patch)
	}
	return comparePrerelease(a.prerelease, b.prerelease)
}

func comparePrerelease(a, b []string) int {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	if len(a) == 0 {
		return 1
	}
	if len(b) == 0 {
		return -1
	}

	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		ai := a[i]
		bi := b[i]
		aNum := numericIdentifierPattern.MatchString(ai)
		bNum := numericIdentifierPattern.MatchString(bi)

		var cmp int
		switch {
		case aNum && bNum:
			av, _ := strconv.Atoi(ai)
			bv, _ := strconv.Atoi(bi)
			cmp = compareInt(av, bv)
		case aNum && !bNum:
			cmp = -1
		case !aNum && bNum:
			cmp = 1
		default:
			cmp = strings.Compare(ai, bi)
		}

		if cmp != 0 {
			return cmp
		}
	}

	return compareInt(len(a), len(b))
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func prereleaseStage(v semVersion) string {
	if len(v.prerelease) == 0 {
		return ""
	}
	return strings.ToLower(v.prerelease[0])
}

func isRcToBetaMigration(current, latest semVersion) bool {
	if current.major != latest.major ||
		current.minor != latest.minor ||
		current.patch != latest.patch {
		return false
	}
	return prereleaseStage(current) == "rc" && prereleaseStage(latest) == "beta"
}

func hasVersionUpdate(current, latest string) bool {
	currentStr := trimVersionPrefix(current)
	latestStr := trimVersionPrefix(latest)
	if currentStr == "0.1.0-dev" {
		return false
	}

	currentVer, currentOK := parseSemVersion(currentStr)
	latestVer, latestOK := parseSemVersion(latestStr)
	if !currentOK || !latestOK {
		return currentStr != latestStr
	}

	// Historical migration rule: allow 0.1.0-rc.* users to move to 0.1.0-beta.*
	// while keeping semver ordering for all other comparisons.
	if isRcToBetaMigration(currentVer, latestVer) {
		return true
	}
	return compareSemVersion(latestVer, currentVer) > 0
}

func trimVersionPrefix(s string) string {
	return strings.TrimPrefix(strings.TrimSpace(s), "v")
}
