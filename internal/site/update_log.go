package site

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	UpdateLogFile = ".update_log.jsonl"

	UpdateEventDetected  = "update_detected"
	UpdateEventCompleted = "update_completed"
	UpdateEventFailed    = "update_failed"
)

type UpdateLogEntry struct {
	Time    time.Time `json:"time"`
	Event   string    `json:"event"`
	Version string    `json:"version,omitempty"`
	Message string    `json:"message,omitempty"`
}

func AppendUpdateLogEntry(contentRoot string, entry UpdateLogEntry) error {
	contentRoot = strings.TrimSpace(contentRoot)
	if contentRoot == "" {
		return nil
	}
	entry.Event = strings.TrimSpace(entry.Event)
	if entry.Event == "" {
		return nil
	}
	if entry.Time.IsZero() {
		entry.Time = time.Now().UTC()
	}
	entry.Time = entry.Time.UTC()
	entry.Version = strings.TrimSpace(entry.Version)
	entry.Message = trimUpdateLogMessage(entry.Message)

	if err := os.MkdirAll(contentRoot, 0755); err != nil {
		return err
	}
	file, err := os.OpenFile(updateLogPath(contentRoot), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0640)
	if err != nil {
		return err
	}
	defer file.Close()

	body, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(body, '\n')); err != nil {
		return err
	}
	return nil
}

func LoadUpdateLog(contentRoot string, limit int) ([]UpdateLogEntry, error) {
	contentRoot = strings.TrimSpace(contentRoot)
	if contentRoot == "" || limit == 0 {
		return nil, nil
	}
	file, err := os.Open(updateLogPath(contentRoot))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 4096), 1024*1024)
	entries := make([]UpdateLogEntry, 0)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry UpdateLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entry.Event = strings.TrimSpace(entry.Event)
		if entry.Event == "" || entry.Time.IsZero() {
			continue
		}
		entry.Version = strings.TrimSpace(entry.Version)
		entry.Message = trimUpdateLogMessage(entry.Message)
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	for left, right := 0, len(entries)-1; left < right; left, right = left+1, right-1 {
		entries[left], entries[right] = entries[right], entries[left]
	}
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

func updateLogPath(contentRoot string) string {
	return filepath.Join(contentRoot, UpdateLogFile)
}

func trimUpdateLogMessage(message string) string {
	message = strings.TrimSpace(message)
	if len(message) <= 4000 {
		return message
	}
	return strings.TrimSpace(message[:4000]) + "..."
}
