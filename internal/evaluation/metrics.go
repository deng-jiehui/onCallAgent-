package evaluation

import (
	"math"
	"strings"
	"unicode"
)

type Score struct {
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
	F1        float64 `json:"f1"`
}

func RecallAtK(retrieved, relevant []string, k int) float64 {
	relevantSet := stringSet(relevant)
	if len(relevantSet) == 0 || k <= 0 {
		return 0
	}
	ranked := uniqueStrings(retrieved)
	if len(ranked) > k {
		ranked = ranked[:k]
	}
	hits := 0
	for _, id := range ranked {
		if _, ok := relevantSet[id]; ok {
			hits++
		}
	}
	return float64(hits) / float64(len(relevantSet))
}

func MRR(retrieved, relevant []string) float64 {
	relevantSet := stringSet(relevant)
	if len(relevantSet) == 0 {
		return 0
	}
	for index, id := range uniqueStrings(retrieved) {
		if _, ok := relevantSet[id]; ok {
			return 1 / float64(index+1)
		}
	}
	return 0
}

func NDCGAtK(retrieved, relevant []string, k int) float64 {
	relevantSet := stringSet(relevant)
	if len(relevantSet) == 0 || k <= 0 {
		return 0
	}
	ranked := uniqueStrings(retrieved)
	if len(ranked) > k {
		ranked = ranked[:k]
	}
	dcg := 0.0
	for index, id := range ranked {
		if _, ok := relevantSet[id]; ok {
			dcg += 1 / math.Log2(float64(index+2))
		}
	}
	idealCount := len(relevantSet)
	if idealCount > k {
		idealCount = k
	}
	idcg := 0.0
	for index := 0; index < idealCount; index++ {
		idcg += 1 / math.Log2(float64(index+2))
	}
	if idcg == 0 {
		return 0
	}
	return dcg / idcg
}

func ExactMatch(answer, reference string) float64 {
	if strings.Join(tokenize(answer), " ") == strings.Join(tokenize(reference), " ") {
		return 1
	}
	return 0
}

func TokenF1(answer, reference string) Score {
	actual := tokenize(answer)
	expected := tokenize(reference)
	if len(actual) == 0 && len(expected) == 0 {
		return Score{Precision: 1, Recall: 1, F1: 1}
	}
	if len(actual) == 0 || len(expected) == 0 {
		return Score{}
	}
	actualCounts := tokenCounts(actual)
	expectedCounts := tokenCounts(expected)
	overlap := 0
	for token, count := range actualCounts {
		if expectedCount := expectedCounts[token]; expectedCount < count {
			overlap += expectedCount
		} else {
			overlap += count
		}
	}
	precision := float64(overlap) / float64(len(actual))
	recall := float64(overlap) / float64(len(expected))
	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}
	return Score{Precision: precision, Recall: recall, F1: f1}
}

func ToolSetAccuracy(actual, expected []string) float64 {
	actualSet := stringSet(actual)
	expectedSet := stringSet(expected)
	if len(actualSet) != len(expectedSet) {
		return 0
	}
	for value := range actualSet {
		if _, ok := expectedSet[value]; !ok {
			return 0
		}
	}
	return 1
}

func tokenize(value string) []string {
	var tokens []string
	var word []rune
	flush := func() {
		if len(word) > 0 {
			tokens = append(tokens, strings.ToLower(string(word)))
			word = word[:0]
		}
	}
	for _, r := range value {
		switch {
		case unicode.Is(unicode.Han, r):
			flush()
			tokens = append(tokens, string(r))
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			word = append(word, unicode.ToLower(r))
		default:
			flush()
		}
	}
	flush()
	return tokens
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func tokenCounts(values []string) map[string]int {
	result := make(map[string]int, len(values))
	for _, value := range values {
		result[value]++
	}
	return result
}
