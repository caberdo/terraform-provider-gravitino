package models

import (
	"strconv"
	"strings"
)

type VersionResponse struct {
	Code    int     `json:"code"`
	Version Version `json:"version"`
}

type Version struct {
	Version     string `json:"version"`
	CompileDate string `json:"compileDate"`
	GitCommit   string `json:"gitCommit"`
}

// ParseVersion extracts the numeric major, minor and patch components of a
// Gravitino server version. Gravitino reports plain semantic versions ("1.3.1")
// and build-suffixed ones ("1.3.1-incubating", "1.4.0-SNAPSHOT"); the suffix
// after the third component is ignored. ok is false when the version does not
// start with three dot-separated numbers.
func ParseVersion(version string) (major, minor, patch int, ok bool) {
	core, _, _ := strings.Cut(version, "-")
	core, _, _ = strings.Cut(core, "+")

	fields := strings.Split(core, ".")
	if len(fields) != 3 {
		return 0, 0, 0, false
	}

	nums := [3]int{}
	for i, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 {
			return 0, 0, 0, false
		}
		nums[i] = n
	}

	return nums[0], nums[1], nums[2], true
}
