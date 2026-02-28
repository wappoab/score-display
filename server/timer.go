package main

import (
	"sync"
	"time"
)

type TimerState struct {
	Running   bool `json:"running"`
	TimeLeft  int  `json:"timeLeft"`
	TotalTime int  `json:"totalTime"`
}

type TimerManager struct {
	Hub              *Hub
	State            TimerState
	ticker           *time.Ticker
	stopChan         chan bool
	mu               sync.Mutex
	goroutineRunning bool
}

func NewTimerManager(hub *Hub) *TimerManager {
	return &TimerManager{
		Hub:              hub,
		stopChan:         make(chan bool, 1),
		State:            TimerState{Running: false, TimeLeft: 0},
		goroutineRunning: false,
	}
}

// Start resumes the timer from current TimeLeft
func (tm *TimerManager) Start() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.State.Running || tm.goroutineRunning {
		return
	}
	if tm.State.TimeLeft <= 0 {
		return
	}

	tm.State.Running = true
	tm.goroutineRunning = true
	tm.ticker = time.NewTicker(1 * time.Second)

	tm.broadcastState()

	go func() {
		defer func() {
			tm.mu.Lock()
			tm.goroutineRunning = false
			tm.mu.Unlock()
		}()

		for {
			select {
			case <-tm.ticker.C:
				tm.mu.Lock()
				if tm.State.TimeLeft > 0 {
					tm.State.TimeLeft--
					tm.broadcastState()
				} else {
					tm.mu.Unlock()
					tm.Pause()
					return
				}
				tm.mu.Unlock()
			case <-tm.stopChan:
				return
			}
		}
	}()
}

// Pause stops the ticker but keeps the TimeLeft
func (tm *TimerManager) Pause() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.State.Running {
		tm.State.Running = false
		if tm.ticker != nil {
			tm.ticker.Stop()
			tm.ticker = nil
		}
		// Non-blocking send to stopChan
		select {
		case tm.stopChan <- true:
		default:
		}
		tm.broadcastState()
	}
}

// Reset sets the timer to a new duration and stops it
func (tm *TimerManager) Reset(seconds int) {
	tm.Pause()
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.State.TotalTime = seconds
	tm.State.TimeLeft = seconds
	tm.broadcastState()
}

func (tm *TimerManager) broadcastState() {
	tm.Hub.BroadcastJSON(struct {
		Type    string     `json:"type"`
		Payload TimerState `json:"payload"`
	}{
		Type:    "timer_update",
		Payload: tm.State,
	})
}