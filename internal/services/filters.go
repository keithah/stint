package services

func FilterWritesOnly(heartbeats []Heartbeat, writesOnly bool) []Heartbeat {
	if !writesOnly {
		return heartbeats
	}
	filtered := make([]Heartbeat, 0, len(heartbeats))
	for _, heartbeat := range heartbeats {
		if heartbeat.IsWrite {
			filtered = append(filtered, heartbeat)
		}
	}
	return filtered
}
