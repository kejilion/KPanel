package ai

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

const maxContextAttachmentBytes = 8 << 20
const maxRetrySourceRuns = 64

var (
	ErrContextAttachmentLimit  = errors.New("本轮及重试来源的附件无法在一次处理预算内完整发送（附件预算 8 MiB）；原内容已保留，请新建会话分批发送，每批结束后再发送下一批")
	ErrContextAttachmentSource = errors.New("无法确认重试附件的完整来源；原内容已保留，请新建会话重新选择文件并分批发送")
)

type ContextSnapshot struct {
	Messages         []Message
	Summary          string
	UserMessageCount int
}

func (s *Store) ContextForRun(ctx context.Context, run Run, window int) (ContextSnapshot, error) {
	return s.contextSnapshot(ctx, run.SessionID, run.ID, window, false)
}

func (s *Store) ContextTextMessages(ctx context.Context, sessionID string, window int) ([]Message, error) {
	snapshot, err := s.contextSnapshot(ctx, sessionID, "", window, true)
	return snapshot.Messages, err
}

func (s *Store) contextSnapshot(ctx context.Context, sessionID, runID string, window int, textOnly bool) (ContextSnapshot, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ContextSnapshot{}, err
	}
	defer tx.Rollback()
	var summary, cursor, userID string
	if err := tx.QueryRowContext(ctx, `SELECT summary,summary_cursor,user_id FROM sessions WHERE id=?`, sessionID).Scan(&summary, &cursor, &userID); err != nil {
		return ContextSnapshot{}, err
	}
	var userCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE session_id=? AND role='user' AND tool_call_id=''`, sessionID).Scan(&userCount); err != nil {
		return ContextSnapshot{}, err
	}
	required := map[string]int{}
	if !textOnly && runID != "" {
		required, err = contextRequiredAttachments(ctx, tx, sessionID, userID, runID)
		if err != nil {
			return ContextSnapshot{}, err
		}
	}
	originalCursor := cursor
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE session_id=? AND
		(?='' OR (created_at,id)>(SELECT created_at,id FROM messages WHERE id=? AND session_id=?))`, sessionID, cursor, cursor, sessionID).Scan(&count); err != nil {
		return ContextSnapshot{}, err
	}
	summaryLimit := 8000
	if window > 0 && window*4/5 < summaryLimit {
		summaryLimit = window * 4 / 5
	}
	if summaryLimit < 512 {
		summaryLimit = 512
	}
	for count > 200 {
		batch, err := readContextBatch(ctx, tx, sessionID, cursor, min(count-200, 100), true)
		if err != nil {
			return ContextSnapshot{}, err
		}
		if len(batch) == 0 {
			return ContextSnapshot{}, errors.New("AI context summary cursor did not advance")
		}
		summary = appendContextSummary(summary, batch, summaryLimit)
		cursor = batch[len(batch)-1].ID
		count -= len(batch)
	}
	messages, err := readContextBatch(ctx, tx, sessionID, cursor, 200, textOnly)
	if err != nil {
		return ContextSnapshot{}, err
	}
	if window >= 1024 {
		requiredChars := 0
		for _, message := range messages {
			if _, ok := required[message.ID]; ok {
				requiredChars += contextMessageChars(message)
			}
		}
		if requiredChars > int(float64(window)*0.7*4) {
			return ContextSnapshot{}, ErrContextAttachmentLimit
		}
		chars := len(summary)
		for _, message := range messages {
			chars += contextMessageChars(message)
		}
		if chars > int(float64(window)*0.7*4) {
			keepChars, split, running := max(int(float64(window)*0.45*4)-len(summary), 512), len(messages), 0
			for index := len(messages) - 1; index >= 0; index-- {
				running += contextMessageChars(messages[index])
				if running > keepChars {
					split = index + 1
					break
				}
			}
			if split > 0 && split < len(messages) {
				summary = appendContextSummary(summary, messages[:split], summaryLimit)
				cursor = messages[split-1].ID
				messages = messages[split:]
			}
		}
	}
	if !textOnly {
		if err := hydrateContextAttachments(ctx, tx, sessionID, messages, required); err != nil {
			return ContextSnapshot{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return ContextSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return ContextSnapshot{}, err
	}
	if cursor != originalCursor {
		// Do not upgrade a read transaction after another user message arrives.
		// The original snapshot count also goes to the run's completion CAS.
		_, err := s.db.ExecContext(ctx, `UPDATE sessions SET summary=?,summary_cursor=?,updated_at=? WHERE id=? AND summary_cursor=?
			AND (SELECT COUNT(*) FROM messages WHERE session_id=? AND role='user' AND tool_call_id='')=?`, summary, cursor, millis(s.now()), sessionID, originalCursor, sessionID, userCount)
		if err != nil {
			return ContextSnapshot{}, err
		}
	}
	return ContextSnapshot{Messages: messages, Summary: summary, UserMessageCount: userCount}, nil
}

func contextMessageChars(message Message) int {
	chars := len(message.Content)
	for _, attachment := range message.Attachments {
		if attachment.Kind == "text" {
			chars += attachment.Size
		} else {
			chars += 16_000
		}
	}
	return chars
}

func readContextBatch(ctx context.Context, tx *sql.Tx, sessionID, cursor string, limit int, textOnly bool) ([]Message, error) {
	attachmentColumn, lengthColumn := "X''", "0"
	if !textOnly {
		lengthColumn = "length(CAST(attachments_json AS BLOB))"
		attachmentColumn = attachmentMetadataProjectionSQL()
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,run_id,role,content,tool_call_id,provider_id,provider_name,model_id,model_name,created_at,`+attachmentColumn+`,`+lengthColumn+` FROM messages WHERE session_id=? AND
		(?='' OR (created_at,id)>(SELECT created_at,id FROM messages WHERE id=? AND session_id=?)) ORDER BY created_at ASC,id ASC LIMIT ?`, sessionID, cursor, cursor, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Message, 0, limit)
	for rows.Next() {
		var item Message
		var data sql.RawBytes
		var encodedBytes int64
		var created int64
		if err := rows.Scan(&item.ID, &item.RunID, &item.Role, &item.Content, &item.ToolCallID, &item.ProviderID, &item.ProviderName, &item.ModelID, &item.ModelName, &created, &data, &encodedBytes); err != nil {
			return nil, err
		}
		if encodedBytes > maxAttachmentReadBytes {
			return nil, errors.New("message attachment record exceeds the read limit")
		}
		item.SessionID = sessionID
		item.CreatedAt = fromMillis(created)
		item.Attachments, err = decodeAttachmentMetadataProjection(data)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func contextRequiredAttachments(ctx context.Context, tx *sql.Tx, sessionID, userID, runID string) (map[string]int, error) {
	seen, chain := map[string]bool{}, []string{}
	unknown := false
	for runID != "" {
		if seen[runID] || len(chain) >= maxRetrySourceRuns {
			return nil, ErrContextAttachmentSource
		}
		seen[runID] = true
		chain = append(chain, runID)
		var parent sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT retry_of FROM runs WHERE id=? AND session_id=? AND user_id=?`, runID, sessionID, userID).Scan(&parent)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrContextAttachmentSource
		}
		if err != nil {
			return nil, err
		}
		if !parent.Valid {
			unknown = true
			break
		}
		runID = parent.String
	}
	args := []any{sessionID}
	where := ""
	if !unknown {
		marks := make([]string, len(chain))
		for i, id := range chain {
			marks[i] = "?"
			args = append(args, id)
		}
		where = " AND run_id IN (" + strings.Join(marks, ",") + ")"
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,`+attachmentMetadataProjectionSQL()+`,
		length(CAST(attachments_json AS BLOB)) FROM messages WHERE session_id=? AND role='user' AND tool_call_id='' AND length(attachments_json)>0`+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	required, bytes := map[string]int{}, 0
	for rows.Next() {
		var id string
		var data sql.RawBytes
		var encodedBytes int64
		if err := rows.Scan(&id, &data, &encodedBytes); err != nil {
			return nil, err
		}
		if encodedBytes > maxAttachmentReadBytes {
			return nil, ErrContextAttachmentLimit
		}
		attachments, err := decodeAttachmentMetadataProjection(data)
		if err != nil {
			return nil, err
		}
		if len(attachments) == 0 {
			continue
		}
		if unknown {
			return nil, ErrContextAttachmentSource
		}
		size := 0
		for _, attachment := range attachments {
			size += attachment.Size
		}
		bytes += size
		if bytes > maxContextAttachmentBytes || len(required) >= 200 {
			return nil, ErrContextAttachmentLimit
		}
		required[id] = size
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return required, nil
}

func hydrateContextAttachments(ctx context.Context, tx *sql.Tx, sessionID string, messages []Message, required map[string]int) error {
	remaining, present := maxContextAttachmentBytes, 0
	selected := map[string]bool{}
	for i := range messages {
		message := &messages[i]
		if size, ok := required[message.ID]; ok {
			remaining -= size
			present++
			message.RequiredAttachments = true
			selected[message.ID] = true
		}
	}
	if present != len(required) || remaining < 0 {
		return ErrContextAttachmentLimit
	}
	for i := len(messages) - 1; i >= 0; i-- {
		message := &messages[i]
		if selected[message.ID] || len(message.Attachments) == 0 {
			continue
		}
		size := 0
		for _, attachment := range message.Attachments {
			size += attachment.Size
		}
		if size <= remaining {
			selected[message.ID] = true
			remaining -= size
		} else {
			message.Attachments = nil
			message.Content += "\n[Earlier attachments were not sent in this request because of the attachment processing budget. Original attachments are retained in the conversation.]"
		}
	}
	for i := range messages {
		message := &messages[i]
		if !selected[message.ID] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		var data []byte
		if err := tx.QueryRowContext(ctx, `SELECT attachments_json FROM messages WHERE session_id=? AND id=?`, sessionID, message.ID).Scan(&data); err != nil {
			return err
		}
		attachments, err := decodeAttachments(data)
		if err != nil {
			return err
		}
		message.Attachments = attachments
	}
	return nil
}
