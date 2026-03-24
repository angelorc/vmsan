package config

// Levenshtein computes the edit distance between two strings.
func Levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	// Use single-row DP for space efficiency
	prev := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr := make([]int, lb+1)
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(
				curr[j-1]+1,   // insertion
				prev[j]+1,     // deletion
				prev[j-1]+cost, // substitution
			)
		}
		prev = curr
	}

	return prev[lb]
}

// FindClosestMatch returns the best match from candidates within a max
// edit distance of 3, or empty string if no good match exists.
func FindClosestMatch(input string, candidates []string) string {
	const maxDistance = 3
	bestDist := maxDistance + 1
	best := ""

	for _, c := range candidates {
		d := Levenshtein(input, c)
		if d < bestDist {
			bestDist = d
			best = c
		}
	}

	if bestDist > maxDistance {
		return ""
	}
	return best
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
