package db

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/keithah/stint/internal/services"
)

func (s *Store) insertHeartbeatsWithCopyStaging(ctx context.Context, userID uuid.UUID, heartbeats []services.Heartbeat) ([]HeartbeatInsertResult, error) {
	results := make([]HeartbeatInsertResult, len(heartbeats))
	if len(heartbeats) == 0 {
		return results, nil
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		CREATE TEMP TABLE tmp_heartbeat_ingest (
			row_index int NOT NULL,
			entity text NOT NULL,
			type text NOT NULL,
			category text,
			time double precision NOT NULL,
			project text,
			branch text,
			language text,
			machine_name text,
			plugin text,
			plugin_version text,
			editor text,
			editor_version text,
			operating_system text,
			architecture text,
			dependencies text,
			lines int,
			line_number int,
			cursor_pos int,
			is_write boolean NOT NULL,
			ai_line_changes int,
			human_line_changes int,
			ai_session text,
			ai_input_tokens int,
			ai_output_tokens int,
			ai_prompt_length int,
			ai_subscription_plan text,
			ai_model text,
			ai_provider text,
			ai_agent text,
			ai_agent_version text,
			ai_agent_complexity text,
			commit_hash text,
			metadata text NOT NULL,
			raw_payload text NOT NULL
		) ON COMMIT DROP`); err != nil {
		return nil, err
	}

	copyRows := make([][]any, 0, len(heartbeats))
	for i, heartbeat := range heartbeats {
		if heartbeat.Type == "" {
			heartbeat.Type = "file"
		}
		results[i].Heartbeat = heartbeat
		copyRows = append(copyRows, []any{
			i, heartbeat.Entity, heartbeat.Type, nullEmpty(heartbeat.Category), heartbeat.Time,
			nullEmpty(heartbeat.Project), nullEmpty(heartbeat.Branch), nullEmpty(heartbeat.Language), nullEmpty(heartbeat.MachineName),
			nullEmpty(heartbeat.Plugin), nullEmpty(heartbeat.PluginVersion), nullEmpty(heartbeat.Editor), nullEmpty(heartbeat.EditorVersion),
			nullEmpty(heartbeat.OperatingSystem), nullEmpty(heartbeat.Architecture), nullEmpty(heartbeat.Dependencies),
			nullableInt(heartbeat.Lines), nullableInt(heartbeat.LineNumber), nullableInt(heartbeat.CursorPosition), heartbeat.IsWrite,
			nullableInt(heartbeat.AILineChanges), nullableInt(heartbeat.HumanLineChanges), nullEmpty(heartbeat.AISession),
			nullableInt(heartbeat.AIInputTokens), nullableInt(heartbeat.AIOutputTokens), nullableInt(heartbeat.AIPromptLength),
			nullEmpty(heartbeat.AISubscriptionPlan), nullEmpty(heartbeat.AIModel), nullEmpty(heartbeat.AIProvider), nullEmpty(heartbeat.AIAgent),
			nullEmpty(heartbeat.AIAgentVersion), nullEmpty(heartbeat.AIAgentComplexity), nullEmpty(heartbeat.CommitHash),
			jsonMapArg(heartbeat.Metadata), jsonMapArg(heartbeat.RawPayload),
		})
	}

	if _, err := tx.CopyFrom(ctx, pgx.Identifier{"tmp_heartbeat_ingest"}, []string{
		"row_index", "entity", "type", "category", "time", "project", "branch", "language", "machine_name",
		"plugin", "plugin_version", "editor", "editor_version", "operating_system", "architecture", "dependencies",
		"lines", "line_number", "cursor_pos", "is_write", "ai_line_changes", "human_line_changes", "ai_session",
		"ai_input_tokens", "ai_output_tokens", "ai_prompt_length", "ai_subscription_plan", "ai_model", "ai_provider",
		"ai_agent", "ai_agent_version", "ai_agent_complexity", "commit_hash", "metadata", "raw_payload",
	}, pgx.CopyFromRows(copyRows)); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO machine_names (user_id, name, last_seen_at)
		SELECT $1, machine_name, now()
		FROM tmp_heartbeat_ingest
		WHERE machine_name IS NOT NULL
		GROUP BY machine_name
		ON CONFLICT (user_id, name) DO UPDATE SET last_seen_at = now()`, userID); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO projects (user_id, name, first_heartbeat_at, last_heartbeat_at)
		SELECT $1, project, to_timestamp(min(time)), to_timestamp(max(time))
		FROM tmp_heartbeat_ingest
		WHERE project IS NOT NULL
		GROUP BY project
		ON CONFLICT (user_id, name) DO UPDATE SET
			last_heartbeat_at = GREATEST(projects.last_heartbeat_at, EXCLUDED.last_heartbeat_at),
			first_heartbeat_at = LEAST(projects.first_heartbeat_at, EXCLUDED.first_heartbeat_at)`, userID); err != nil {
		return nil, err
	}

	rows, err := tx.Query(ctx, `
		WITH ranked AS (
			SELECT *,
				row_number() OVER (PARTITION BY entity, time ORDER BY row_index) AS conflict_rank
			FROM tmp_heartbeat_ingest
		),
		source AS (
			SELECT * FROM ranked WHERE conflict_rank = 1
		),
		inserted AS (
			INSERT INTO heartbeats (
				user_id, entity, type, category, time, project, branch, language, machine_name_id,
				machine_name, plugin, plugin_version, editor, editor_version, operating_system, architecture,
				dependencies, lines, line_number, cursor_pos, is_write, ai_line_changes, human_line_changes,
				ai_session, ai_input_tokens, ai_output_tokens, ai_prompt_length, ai_subscription_plan,
				ai_model, ai_provider, ai_agent, ai_agent_version, ai_agent_complexity, commit_hash, metadata, raw_payload
			)
			SELECT
				$1, source.entity, source.type, source.category, source.time, source.project, source.branch, source.language, machine_names.id,
				source.machine_name, source.plugin, source.plugin_version, source.editor, source.editor_version, source.operating_system, source.architecture,
				source.dependencies, source.lines, source.line_number, source.cursor_pos, source.is_write, source.ai_line_changes, source.human_line_changes,
				source.ai_session, source.ai_input_tokens, source.ai_output_tokens, source.ai_prompt_length, source.ai_subscription_plan,
				source.ai_model, source.ai_provider, source.ai_agent, source.ai_agent_version, source.ai_agent_complexity, source.commit_hash,
				source.metadata::jsonb, source.raw_payload::jsonb
			FROM source
			LEFT JOIN machine_names ON machine_names.user_id = $1 AND machine_names.name = source.machine_name
			ON CONFLICT DO NOTHING
			RETURNING id, entity, time
		)
		SELECT source.row_index, inserted.id
		FROM source
		JOIN inserted ON inserted.entity = source.entity AND inserted.time = source.time
		ORDER BY source.row_index`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	storedAny := false
	for rows.Next() {
		var rowIndex int
		var id uuid.UUID
		if err := rows.Scan(&rowIndex, &id); err != nil {
			return nil, err
		}
		results[rowIndex].Heartbeat.ID = id.String()
		results[rowIndex].Stored = true
		storedAny = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range results {
		if !results[i].Stored && results[i].Err == nil {
			results[i].Duplicate = true
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	if storedAny {
		if err := s.MarkStatsStale(ctx, userID); err != nil {
			return nil, err
		}
	}
	return results, nil
}
