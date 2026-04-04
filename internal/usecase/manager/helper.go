package manager

import "time"

func (m *Manager) getCurrentTime() int64 {
	return time.Now().Unix()
}
