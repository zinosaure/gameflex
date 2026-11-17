package gamestatus

import (
	"sync"
	"time"
)

// Memory — un Backend en mémoire : les tests des kernels, et un hôte sans base.
type Memory struct {
	mu       sync.Mutex
	consoles map[string]Console
}

// NewMemory — un Backend vide.
func NewMemory() *Memory {
	return &Memory{consoles: map[string]Console{}}
}

func (memory *Memory) Console(consoleID string) (Console, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	console := memory.consoles[consoleID]
	copied := Console{Status: console.Status, By: console.By, At: console.At}
	if len(console.Games) > 0 {
		copied.Games = map[string][]Report{}
		for gameID, reports := range console.Games {
			copied.Games[gameID] = append([]Report(nil), reports...)
		}
	}

	return copied, nil
}

func (memory *Memory) AddReport(consoleID, gameID string, report Report) error {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	console := memory.consoles[consoleID]
	if console.Games == nil {
		console.Games = map[string][]Report{}
	}
	console.Games[gameID] = append(console.Games[gameID], report)
	memory.consoles[consoleID] = console

	return nil
}

func (memory *Memory) SetConsole(consoleID, status, by string, at time.Time) error {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	console := memory.consoles[consoleID]
	stamp := at.UTC()
	console.Status, console.By, console.At = status, by, &stamp
	memory.consoles[consoleID] = console

	return nil
}

func (memory *Memory) Statuses() (map[string]string, error) {
	memory.mu.Lock()
	defer memory.mu.Unlock()
	statuses := map[string]string{}
	for consoleID, console := range memory.consoles {
		if console.Status != "" {
			statuses[consoleID] = console.Status
		}
	}

	return statuses, nil
}
