package evaluation

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type Case struct {
	ID              string   `json:"id"`
	Question        string   `json:"question"`
	RelevantDocIDs  []string `json:"relevant_doc_ids,omitempty"`
	ReferenceAnswer string   `json:"reference_answer,omitempty"`
	ExpectedTools   []string `json:"expected_tools,omitempty"`
	Tags            []string `json:"tags,omitempty"`
}

type Dataset struct {
	Cases []Case `json:"cases"`
}

func LoadJSONL(reader io.Reader) (Dataset, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	seen := map[string]struct{}{}
	dataset := Dataset{}
	line := 0
	for scanner.Scan() {
		line++
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var item Case
		if err := json.Unmarshal([]byte(text), &item); err != nil {
			return Dataset{}, fmt.Errorf("dataset line %d: %w", line, err)
		}
		item.ID = strings.TrimSpace(item.ID)
		item.Question = strings.TrimSpace(item.Question)
		if item.ID == "" {
			return Dataset{}, fmt.Errorf("dataset line %d: id is required", line)
		}
		if item.Question == "" {
			return Dataset{}, fmt.Errorf("dataset line %d: question is required", line)
		}
		if _, exists := seen[item.ID]; exists {
			return Dataset{}, fmt.Errorf("dataset line %d: duplicate id %q", line, item.ID)
		}
		seen[item.ID] = struct{}{}
		dataset.Cases = append(dataset.Cases, item)
	}
	if err := scanner.Err(); err != nil {
		return Dataset{}, fmt.Errorf("read dataset: %w", err)
	}
	return dataset, nil
}
