package spark

import (
	"regexp"
	"strconv"
	"strings"
)

func ParseTPS(resp string) float64 {
	tpsRegex := regexp.MustCompile(`TPS:\s*([\d.]+)|Tick Rate:\s*([\d.]+)`)
	matches := tpsRegex.FindStringSubmatch(resp)
	if len(matches) >= 3 {
		if matches[1] != "" {
			return parseFloat(matches[1])
		}
		return parseFloat(matches[2])
	}
	return 20.0
}

func ParseMemoryUsed(resp string) int64 {
	usedRegex := regexp.MustCompile(`Memory Used:\s*(\d+)\s*(?:MB|MiB)|(\d+)\s*(?:MB|MiB)\s*/`)
	matches := usedRegex.FindStringSubmatch(resp)
	if len(matches) >= 3 {
		if matches[1] != "" {
			return parseInt(matches[1])
		}
		if matches[2] != "" {
			return parseInt(matches[2])
		}
	}

	lines := strings.Split(resp, "\n")
	for _, line := range lines {
		lowerLine := strings.ToLower(line)
		if strings.Contains(lowerLine, "memory") {
			slashIdx := strings.Index(line, "/")
			if slashIdx != -1 {
				beforeSlash := strings.TrimSpace(line[:slashIdx])
				parts := strings.Fields(beforeSlash)
				for _, part := range parts {
					val := parseInt(part)
					if val > 0 {
						return val
					}
				}
			}
		}
	}
	return 0
}

func ParseMemoryMax(resp string) int64 {
	maxRegex := regexp.MustCompile(`Memory Max:\s*(\d+)\s*(?:MB|MiB)|/\s*(\d+)\s*(?:MB|MiB)`)
	matches := maxRegex.FindStringSubmatch(resp)
	if len(matches) >= 3 {
		if matches[1] != "" {
			return parseInt(matches[1])
		}
		if matches[2] != "" {
			return parseInt(matches[2])
		}
	}

	lines := strings.Split(resp, "\n")
	for _, line := range lines {
		slashIdx := strings.Index(line, "/")
		if slashIdx != -1 {
			afterSlash := strings.TrimSpace(line[slashIdx+1:])
			parts := strings.Fields(afterSlash)
			for _, part := range parts {
				val := parseInt(part)
				if val > 0 {
					return val
				}
			}
		}
	}
	return 0
}

func ParseCPU(resp string) float64 {
	cpuRegex := regexp.MustCompile(`CPU:\s*([\d.]+)%|CPU\s*:\s*([\d.]+)%`)
	matches := cpuRegex.FindStringSubmatch(resp)
	if len(matches) >= 3 {
		if matches[1] != "" {
			return parseFloat(matches[1])
		}
		return parseFloat(matches[2])
	}
	return 0.0
}

func ExtractProfileURL(resp string) string {
	urlRegex := regexp.MustCompile(`https://spark\.lucko\.me/[a-zA-Z0-9\-]+`)
	matches := urlRegex.FindString(resp)
	if matches != "" {
		return matches
	}
	return ""
}

func StripColorCodes(input string) string {
	colorRegex := regexp.MustCompile(`§[0-9a-fk-or]`)
	return colorRegex.ReplaceAllString(input, "")
}

func parseFloat(s string) float64 {
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0.0
	}
	return val
}

func parseInt(s string) int64 {
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return val
}
