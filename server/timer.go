package main

import (
	"time"
)

type TimerState struct {
	Running   bool `json:"running"`
	TimeLeft  int  `json:"timeLeft"`
	TotalTime int  `json:"totalTime"`
}

type TimerManager struct {
	Hub      *Hub
	State    TimerState
	ticker   *time.Ticker
	stopChan chan bool
}

func NewTimerManager(hub *Hub) *TimerManager {
	return &TimerManager{
		Hub:      hub,
		stopChan: make(chan bool),
		State:    TimerState{Running: false, TimeLeft: 0},
	}
}

// Start resumes the timer from current TimeLeft
func (tm *TimerManager) Start() {
	if tm.State.Running {
		return
	}
	if tm.State.TimeLeft <= 0 {
		return 
	}

	tm.State.Running = true
	tm.ticker = time.NewTicker(1 * time.Second)
	
	tm.broadcastState()

	go func() {
		for {
			select {
			case <-tm.ticker.C:
				if tm.State.TimeLeft > 0 {
					tm.State.TimeLeft--
					tm.broadcastState()
				} else {
					tm.Pause()
				}
			case <-tm.stopChan:
				return
			}
		}
	}()
}

// Pause stops the ticker but keeps the TimeLeft
func (tm *TimerManager) Pause() {
	if tm.State.Running {
		tm.State.Running = false
		if tm.ticker != nil {
			tm.ticker.Stop()
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