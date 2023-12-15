package ntpdb

import "time"

func (s *Server) DeletionAge(dur time.Duration) bool {
	if !s.DeletionOn.Valid {
		return false
	}
	if dur > 0 {
		dur = dur * -1
	}
	if s.DeletionOn.Time.Before(time.Now().Add(dur)) {
		return true
	}
	return false
}
