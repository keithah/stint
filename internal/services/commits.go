package services

import (
	"sort"
	"time"
)

func ComputeProjectCommits(heartbeats []Heartbeat, project, branch string, timeout time.Duration) []CommitSummary {
	groups := map[string][]Heartbeat{}
	for _, heartbeat := range heartbeats {
		hash := normalizeCommitHash(heartbeat)
		if hash == "" || heartbeat.Project != project {
			continue
		}
		if branch != "" && heartbeat.Branch != branch {
			continue
		}
		groups[hash] = append(groups[hash], heartbeat)
	}

	rows := make([]CommitSummary, 0, len(groups))
	for hash, items := range groups {
		sortedItems := SortedHeartbeats(items)
		total := sumDurations(ComputeDurationsFromSorted(sortedItems, timeout, "commit"))
		last := sortedItems[len(sortedItems)-1]
		commitTime := time.Unix(int64(last.Time), 0).UTC().Format(time.RFC3339)
		rows = append(rows, CommitSummary{
			ID:                            hash,
			Hash:                          hash,
			TruncatedHash:                 truncateCommitHash(hash),
			Branch:                        last.Branch,
			Ref:                           commitRef(last.Branch),
			TotalSeconds:                  total,
			HumanReadableTotal:            HumanDuration(total),
			HumanReadableTotalWithSeconds: HumanDuration(total),
			CreatedAt:                     commitTime,
			AuthorDate:                    commitTime,
			CommitterDate:                 commitTime,
			LastHeartbeatAt:               last.Time,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].LastHeartbeatAt == rows[j].LastHeartbeatAt {
			return rows[i].Hash < rows[j].Hash
		}
		return rows[i].LastHeartbeatAt > rows[j].LastHeartbeatAt
	})
	return rows
}

func normalizeCommitHash(heartbeat Heartbeat) string {
	if heartbeat.CommitHash != "" {
		return heartbeat.CommitHash
	}
	return heartbeat.Revision
}

func truncateCommitHash(hash string) string {
	if len(hash) <= 7 {
		return hash
	}
	return hash[:7]
}

func commitRef(branch string) string {
	if branch == "" {
		return ""
	}
	return "refs/heads/" + branch
}
