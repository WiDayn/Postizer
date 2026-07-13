package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type backupState struct {
	LastAttempt  time.Time `json:"last_attempt,omitempty"`
	LastSuccess  time.Time `json:"last_success,omitempty"`
	LastFilename string    `json:"last_filename,omitempty"`
	LastBytes    int64     `json:"last_bytes,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
}

func (s *server) statePath() string { return filepath.Join(s.dataDir, "state.json") }

func (s *server) loadState() (backupState, error) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.loadStateLocked()
}

func (s *server) loadStateLocked() (backupState, error) {
	body, err := os.ReadFile(s.statePath())
	if os.IsNotExist(err) {
		return backupState{}, nil
	}
	if err != nil {
		return backupState{}, err
	}
	var state backupState
	if err := json.Unmarshal(body, &state); err != nil {
		return backupState{}, fmt.Errorf("读取备份状态: %w", err)
	}
	return state, nil
}

func (s *server) updateState(update func(*backupState)) error {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	state, err := s.loadStateLocked()
	if err != nil {
		return err
	}
	update(&state)
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	temporary := s.statePath() + ".tmp"
	if err := os.WriteFile(temporary, append(body, '\n'), 0600); err != nil {
		return err
	}
	defer os.Remove(temporary)
	return replaceFile(temporary, s.statePath())
}

func (s *server) backupDue(cfg config, now time.Time) bool {
	state, err := s.loadState()
	if err != nil || state.LastSuccess.IsZero() {
		return true
	}
	return !now.Before(state.LastSuccess.Add(time.Duration(cfg.IntervalHours) * time.Hour))
}
