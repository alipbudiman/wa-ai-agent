package eventsrunner

import (
	"sync"
	"time"
)

type QRLoginState struct {
	State     string `json:"state"`
	Code      string `json:"code,omitempty"`
	Message   string `json:"message,omitempty"`
	UpdatedAt string `json:"updated_at"`
}

type qrLoginStateStore struct {
	mu    sync.RWMutex
	state QRLoginState
}

var globalQRLoginState = &qrLoginStateStore{
	state: QRLoginState{
		State:     "idle",
		Message:   "bot runner is not started yet",
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	},
}

func setQRLoginState(state, code, message string) {
	globalQRLoginState.mu.Lock()
	defer globalQRLoginState.mu.Unlock()

	globalQRLoginState.state = QRLoginState{
		State:     state,
		Code:      code,
		Message:   message,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

func GetQRLoginState() QRLoginState {
	globalQRLoginState.mu.RLock()
	defer globalQRLoginState.mu.RUnlock()

	return globalQRLoginState.state
}
