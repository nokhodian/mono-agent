package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

// SaveSession inserts or replaces a crawler session for the given platform and
// username. On conflict (same username + platform), the existing row is updated.
func (d *Database) SaveSession(platform, username, cookiesJSON string, expiry time.Time) error {
	_, err := d.DB.Exec(`
		INSERT INTO crawler_sessions (username, platform, cookies_json, expiry)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(username, platform)
		DO UPDATE SET cookies_json = excluded.cookies_json,
		              expiry       = excluded.expiry,
		              when_added   = CURRENT_TIMESTAMP`,
		username, platform, cookiesJSON, expiry.UTC(),
	)
	if err != nil {
		return fmt.Errorf("saving session for %s/%s: %w", platform, username, err)
	}
	return nil
}

// GetSession retrieves a single crawler session by platform and username.
// Returns nil when no matching row exists.
func (d *Database) GetSession(platform, username string) (*Session, error) {
	s := &Session{}
	err := d.DB.QueryRow(`
		SELECT id, username, platform, cookies_json, expiry, when_added, profile_photo
		FROM crawler_sessions
		WHERE platform = ? AND username = ?`, platform, username,
	).Scan(&s.ID, &s.Username, &s.Platform, &s.CookiesJSON, &s.Expiry, &s.WhenAdded, &s.ProfilePhoto)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting session %s/%s: %w", platform, username, err)
	}
	return s, nil
}

// ListSessions returns all crawler sessions ordered by when_added descending.
func (d *Database) ListSessions() ([]*Session, error) {
	rows, err := d.DB.Query(`
		SELECT id, username, platform, cookies_json, expiry, when_added, profile_photo
		FROM crawler_sessions
		ORDER BY when_added DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	defer rows.Close()

	var sessions []*Session
	for rows.Next() {
		s := &Session{}
		if err := rows.Scan(&s.ID, &s.Username, &s.Platform, &s.CookiesJSON, &s.Expiry, &s.WhenAdded, &s.ProfilePhoto); err != nil {
			return nil, fmt.Errorf("scanning session row: %w", err)
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// DeleteSession removes a crawler session by platform and username.
func (d *Database) DeleteSession(platform, username string) error {
	result, err := d.DB.Exec("DELETE FROM crawler_sessions WHERE platform = ? AND username = ?", platform, username)
	if err != nil {
		return fmt.Errorf("deleting session %s/%s: %w", platform, username, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("session %s/%s not found", platform, username)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

// CreateAction inserts a new action row. If the action has no ID one is
// generated automatically.
func (d *Database) CreateAction(action *Action) error {
	if action.ID == "" {
		action.ID = NewID()
	}
	if action.CreatedAt == 0 {
		action.CreatedAt = time.Now().Unix()
	}
	action.State = normalizeState(action.State)

	now := time.Now().UTC()
	action.CreatedAtTS = now
	action.UpdatedAtTS = now

	_, err := d.DB.Exec(`
		INSERT INTO actions (
			id, created_at, title, type, state, disabled, target_platform,
			position, content_subject, content_message, content_blob_urls,
			scheduled_date, execution_interval, start_date, end_date,
			campaign_id, reached_index, keywords, action_execution_count,
			created_at_ts, updated_at_ts
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		action.ID, action.CreatedAt, action.Title, action.Type, action.State,
		boolToInt(action.Disabled), action.TargetPlatform, action.Position,
		nullStr(action.ContentSubject), nullStr(action.ContentMessage), nullStr(action.ContentBlobURLs),
		nullStr(action.ScheduledDate), nullInt(action.ExecutionInterval),
		nullStr(action.StartDate), nullStr(action.EndDate), nullStr(action.CampaignID),
		action.ReachedIndex, nullStr(action.Keywords), action.ActionExecutionCount,
		action.CreatedAtTS, action.UpdatedAtTS,
	)
	if err != nil {
		return fmt.Errorf("creating action %s: %w", action.ID, err)
	}
	return nil
}

// GetAction retrieves a single action by its ID. Returns nil when not found.
func (d *Database) GetAction(id string) (*Action, error) {
	a := &Action{}
	var disabled int
	var contentSubject, contentMessage, contentBlobURLs sql.NullString
	var scheduledDate, startDate, endDate, campaignID, keywords sql.NullString
	var executionInterval sql.NullInt64

	err := d.DB.QueryRow(`
		SELECT id, created_at, title, type, state, disabled, target_platform,
		       position, content_subject, content_message, content_blob_urls,
		       scheduled_date, execution_interval, start_date, end_date,
		       campaign_id, reached_index, keywords, action_execution_count,
		       created_at_ts, updated_at_ts
		FROM actions WHERE id = ?`, id,
	).Scan(
		&a.ID, &a.CreatedAt, &a.Title, &a.Type, &a.State, &disabled,
		&a.TargetPlatform, &a.Position,
		&contentSubject, &contentMessage, &contentBlobURLs,
		&scheduledDate, &executionInterval, &startDate, &endDate,
		&campaignID, &a.ReachedIndex, &keywords, &a.ActionExecutionCount,
		&a.CreatedAtTS, &a.UpdatedAtTS,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting action %s: %w", id, err)
	}

	a.Disabled = disabled != 0
	a.ContentSubject = contentSubject.String
	a.ContentMessage = contentMessage.String
	a.ContentBlobURLs = contentBlobURLs.String
	a.ScheduledDate = scheduledDate.String
	a.ExecutionInterval = int(executionInterval.Int64)
	a.StartDate = startDate.String
	a.EndDate = endDate.String
	a.CampaignID = campaignID.String
	a.Keywords = keywords.String

	return a, nil
}

// ListActions returns actions matching the supplied filters. Pass empty strings
// to skip a filter. limit <= 0 defaults to 100; offset < 0 defaults to 0.
func (d *Database) ListActions(state, actionType, platform string, limit, offset int) ([]*Action, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var conditions []string
	var args []interface{}

	if state != "" {
		conditions = append(conditions, "state = ?")
		args = append(args, state)
	}
	if actionType != "" {
		conditions = append(conditions, "type = ?")
		args = append(args, actionType)
	}
	if platform != "" {
		conditions = append(conditions, "target_platform = ?")
		args = append(args, platform)
	}

	query := "SELECT id, created_at, title, type, state, disabled, target_platform, position, content_subject, content_message, content_blob_urls, scheduled_date, execution_interval, start_date, end_date, campaign_id, reached_index, keywords, action_execution_count, created_at_ts, updated_at_ts FROM actions"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY position ASC, created_at_ts DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	return d.scanActions(query, args...)
}

// UpdateAction updates all mutable fields of an existing action.
func (d *Database) UpdateAction(action *Action) error {
	action.State = normalizeState(action.State)
	action.UpdatedAtTS = time.Now().UTC()

	_, err := d.DB.Exec(`
		UPDATE actions SET
			title = ?, type = ?, state = ?, disabled = ?, target_platform = ?,
			position = ?, content_subject = ?, content_message = ?, content_blob_urls = ?,
			scheduled_date = ?, execution_interval = ?, start_date = ?, end_date = ?,
			campaign_id = ?, reached_index = ?, keywords = ?, action_execution_count = ?,
			updated_at_ts = ?
		WHERE id = ?`,
		action.Title, action.Type, action.State, boolToInt(action.Disabled),
		action.TargetPlatform, action.Position,
		nullStr(action.ContentSubject), nullStr(action.ContentMessage), nullStr(action.ContentBlobURLs),
		nullStr(action.ScheduledDate), nullInt(action.ExecutionInterval),
		nullStr(action.StartDate), nullStr(action.EndDate), nullStr(action.CampaignID),
		action.ReachedIndex, nullStr(action.Keywords), action.ActionExecutionCount,
		action.UpdatedAtTS, action.ID,
	)
	if err != nil {
		return fmt.Errorf("updating action %s: %w", action.ID, err)
	}
	return nil
}

// UpdateActionState sets the state of a single action.
func (d *Database) UpdateActionState(id, state string) error {
	state = normalizeState(state)
	_, err := d.DB.Exec("UPDATE actions SET state = ?, updated_at_ts = ? WHERE id = ?",
		state, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("updating action state %s: %w", id, err)
	}
	return nil
}

// UpdateActionReachedIndex updates the reached_index for an action, which
// tracks how far through a target list the action has progressed.
func (d *Database) UpdateActionReachedIndex(id string, index int) error {
	_, err := d.DB.Exec("UPDATE actions SET reached_index = ?, updated_at_ts = ? WHERE id = ?",
		index, time.Now().UTC(), id)
	if err != nil {
		return fmt.Errorf("updating reached_index for action %s: %w", id, err)
	}
	return nil
}

// DeleteAction removes an action by ID. Associated action_targets are removed
// automatically via ON DELETE CASCADE.
func (d *Database) DeleteAction(id string) error {
	result, err := d.DB.Exec("DELETE FROM actions WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting action %s: %w", id, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("action %s not found", id)
	}
	return nil
}

// GetPendingActions returns all actions in the PENDING state that are not
// disabled, ordered by position.
func (d *Database) GetPendingActions() ([]*Action, error) {
	return d.scanActions(`
		SELECT id, created_at, title, type, state, disabled, target_platform,
		       position, content_subject, content_message, content_blob_urls,
		       scheduled_date, execution_interval, start_date, end_date,
		       campaign_id, reached_index, keywords, action_execution_count,
		       created_at_ts, updated_at_ts
		FROM actions
		WHERE state = 'PENDING' AND disabled = 0
		ORDER BY position ASC, created_at_ts ASC`)
}

// scanActions is an internal helper that executes a query and scans the result
// rows into Action structs.
func (d *Database) scanActions(query string, args ...interface{}) ([]*Action, error) {
	rows, err := d.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying actions: %w", err)
	}
	defer rows.Close()

	var actions []*Action
	for rows.Next() {
		a := &Action{}
		var disabled int
		var contentSubject, contentMessage, contentBlobURLs sql.NullString
		var scheduledDate, startDate, endDate, campaignID, keywords sql.NullString
		var executionInterval sql.NullInt64

		if err := rows.Scan(
			&a.ID, &a.CreatedAt, &a.Title, &a.Type, &a.State, &disabled,
			&a.TargetPlatform, &a.Position,
			&contentSubject, &contentMessage, &contentBlobURLs,
			&scheduledDate, &executionInterval, &startDate, &endDate,
			&campaignID, &a.ReachedIndex, &keywords, &a.ActionExecutionCount,
			&a.CreatedAtTS, &a.UpdatedAtTS,
		); err != nil {
			return nil, fmt.Errorf("scanning action row: %w", err)
		}

		a.Disabled = disabled != 0
		a.ContentSubject = contentSubject.String
		a.ContentMessage = contentMessage.String
		a.ContentBlobURLs = contentBlobURLs.String
		a.ScheduledDate = scheduledDate.String
		a.ExecutionInterval = int(executionInterval.Int64)
		a.StartDate = startDate.String
		a.EndDate = endDate.String
		a.CampaignID = campaignID.String
		a.Keywords = keywords.String

		actions = append(actions, a)
	}
	return actions, rows.Err()
}

// ---------------------------------------------------------------------------
// Action Targets
// ---------------------------------------------------------------------------

// CreateActionTarget inserts a single action target row.
func (d *Database) CreateActionTarget(target *ActionTarget) error {
	if target.ID == "" {
		target.ID = NewID()
	}
	if target.Status == "" {
		target.Status = "PENDING"
	}

	_, err := d.DB.Exec(`
		INSERT INTO action_targets (
			id, action_id, person_id, platform, link, source_type,
			status, last_interacted_at, comment_text, metadata, created_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		target.ID, target.ActionID, nullStr(target.PersonID), target.Platform,
		nullStr(target.Link), nullStr(target.SourceType), target.Status,
		nullStr(target.LastInteractedAt), nullStr(target.CommentText),
		nullStr(target.Metadata), time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("creating action target %s: %w", target.ID, err)
	}
	return nil
}

// ListActionTargets returns targets for a given action, optionally filtered by
// status. Pass an empty status string to return all targets.
func (d *Database) ListActionTargets(actionID string, status string) ([]*ActionTarget, error) {
	query := `
		SELECT id, action_id, person_id, platform, link, source_type,
		       status, last_interacted_at, comment_text, metadata, created_at
		FROM action_targets
		WHERE action_id = ?`
	args := []interface{}{actionID}

	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}
	query += " ORDER BY created_at ASC"

	rows, err := d.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing action targets for %s: %w", actionID, err)
	}
	defer rows.Close()

	var targets []*ActionTarget
	for rows.Next() {
		t := &ActionTarget{}
		var personID, link, sourceType, lastInteracted, commentText, metadata sql.NullString
		if err := rows.Scan(
			&t.ID, &t.ActionID, &personID, &t.Platform, &link, &sourceType,
			&t.Status, &lastInteracted, &commentText, &metadata, &t.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning action target row: %w", err)
		}
		t.PersonID = personID.String
		t.Link = link.String
		t.SourceType = sourceType.String
		t.LastInteractedAt = lastInteracted.String
		t.CommentText = commentText.String
		t.Metadata = metadata.String
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

// UpdateActionTargetStatus updates the status and last_interacted_at timestamp
// of a single action target.
func (d *Database) UpdateActionTargetStatus(id, status string) error {
	_, err := d.DB.Exec(`
		UPDATE action_targets SET status = ?, last_interacted_at = ? WHERE id = ?`,
		status, time.Now().UTC().Format(time.RFC3339), id,
	)
	if err != nil {
		return fmt.Errorf("updating action target status %s: %w", id, err)
	}
	return nil
}

// BatchCreateActionTargets inserts multiple action targets within a single
// transaction.
func (d *Database) BatchCreateActionTargets(targets []*ActionTarget) error {
	if len(targets) == 0 {
		return nil
	}

	tx, err := d.DB.Begin()
	if err != nil {
		return fmt.Errorf("beginning batch target transaction: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO action_targets (
			id, action_id, person_id, platform, link, source_type,
			status, last_interacted_at, comment_text, metadata, created_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("preparing batch target insert: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC()
	for _, t := range targets {
		if t.ID == "" {
			t.ID = NewID()
		}
		if t.Status == "" {
			t.Status = "PENDING"
		}
		if _, err := stmt.Exec(
			t.ID, t.ActionID, nullStr(t.PersonID), t.Platform,
			nullStr(t.Link), nullStr(t.SourceType), t.Status,
			nullStr(t.LastInteractedAt), nullStr(t.CommentText),
			nullStr(t.Metadata), now,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("inserting action target %s: %w", t.ID, err)
		}
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// People
// ---------------------------------------------------------------------------

// UpsertPerson inserts a new person or updates an existing one matched by
// platform_username + platform.
func (d *Database) UpsertPerson(person *Person) error {
	if person.ID == "" {
		person.ID = NewID()
	}
	now := time.Now().UTC()

	_, err := d.DB.Exec(`
		INSERT INTO people (
			id, platform_username, platform, full_name, image_url,
			contact_details, website, content_count, follower_count,
			following_count, introduction, is_verified, category, job_title,
			created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(platform_username, platform)
		DO UPDATE SET
			full_name       = excluded.full_name,
			image_url       = excluded.image_url,
			contact_details = excluded.contact_details,
			website         = excluded.website,
			content_count   = excluded.content_count,
			follower_count  = excluded.follower_count,
			following_count = excluded.following_count,
			introduction    = excluded.introduction,
			is_verified     = excluded.is_verified,
			category        = excluded.category,
			job_title       = excluded.job_title,
			updated_at      = excluded.updated_at`,
		person.ID, person.PlatformUsername, person.Platform,
		nullStr(person.FullName), nullStr(person.ImageURL),
		nullStr(person.ContactDetails), nullStr(person.Website),
		person.ContentCount, nullStr(person.FollowerCount), person.FollowingCount,
		nullStr(person.Introduction), boolToInt(person.IsVerified),
		nullStr(person.Category), nullStr(person.JobTitle),
		now, now,
	)
	if err != nil {
		return fmt.Errorf("upserting person %s/%s: %w", person.Platform, person.PlatformUsername, err)
	}
	return nil
}

// GetPerson retrieves a single person by ID. Returns nil when not found.
func (d *Database) GetPerson(id string) (*Person, error) {
	p := &Person{}
	var fullName, imageURL, contactDetails, website, followerCount sql.NullString
	var introduction, category, jobTitle sql.NullString
	var isVerified int

	err := d.DB.QueryRow(`
		SELECT id, platform_username, platform, full_name, image_url,
		       contact_details, website, content_count, follower_count,
		       following_count, introduction, is_verified, category, job_title,
		       created_at, updated_at
		FROM people WHERE id = ?`, id,
	).Scan(
		&p.ID, &p.PlatformUsername, &p.Platform, &fullName, &imageURL,
		&contactDetails, &website, &p.ContentCount, &followerCount,
		&p.FollowingCount, &introduction, &isVerified, &category, &jobTitle,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting person %s: %w", id, err)
	}

	p.FullName = fullName.String
	p.ImageURL = imageURL.String
	p.ContactDetails = contactDetails.String
	p.Website = website.String
	p.FollowerCount = followerCount.String
	p.Introduction = introduction.String
	p.IsVerified = isVerified != 0
	p.Category = category.String
	p.JobTitle = jobTitle.String

	return p, nil
}

// ListPeople returns people optionally filtered by platform and a search term
// (matched against platform_username and full_name). Pass empty strings to skip
// filters. limit <= 0 defaults to 100.
func (d *Database) ListPeople(platform, search string, limit, offset int) ([]*Person, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var conditions []string
	var args []interface{}

	if platform != "" {
		conditions = append(conditions, "platform = ?")
		args = append(args, platform)
	}
	if search != "" {
		conditions = append(conditions, "(platform_username LIKE ? OR full_name LIKE ?)")
		like := "%" + search + "%"
		args = append(args, like, like)
	}

	query := `
		SELECT id, platform_username, platform, full_name, image_url,
		       contact_details, website, content_count, follower_count,
		       following_count, introduction, is_verified, category, job_title,
		       created_at, updated_at
		FROM people`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY updated_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := d.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing people: %w", err)
	}
	defer rows.Close()

	var people []*Person
	for rows.Next() {
		p := &Person{}
		var fullName, imageURL, contactDetails, website, followerCount sql.NullString
		var introduction, category, jobTitle sql.NullString
		var isVerified int

		if err := rows.Scan(
			&p.ID, &p.PlatformUsername, &p.Platform, &fullName, &imageURL,
			&contactDetails, &website, &p.ContentCount, &followerCount,
			&p.FollowingCount, &introduction, &isVerified, &category, &jobTitle,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning person row: %w", err)
		}

		p.FullName = fullName.String
		p.ImageURL = imageURL.String
		p.ContactDetails = contactDetails.String
		p.Website = website.String
		p.FollowerCount = followerCount.String
		p.Introduction = introduction.String
		p.IsVerified = isVerified != 0
		p.Category = category.String
		p.JobTitle = jobTitle.String

		people = append(people, p)
	}
	return people, rows.Err()
}

// DeletePerson removes a person by ID.
func (d *Database) DeletePerson(id string) error {
	result, err := d.DB.Exec("DELETE FROM people WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting person %s: %w", id, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("person %s not found", id)
	}
	return nil
}

// BatchUpsertPeople inserts or updates multiple people within a single
// transaction.
func (d *Database) BatchUpsertPeople(people []*Person) error {
	if len(people) == 0 {
		return nil
	}

	tx, err := d.DB.Begin()
	if err != nil {
		return fmt.Errorf("beginning batch people transaction: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO people (
			id, platform_username, platform, full_name, image_url,
			contact_details, website, content_count, follower_count,
			following_count, introduction, is_verified, category, job_title,
			created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(platform_username, platform)
		DO UPDATE SET
			full_name       = excluded.full_name,
			image_url       = excluded.image_url,
			contact_details = excluded.contact_details,
			website         = excluded.website,
			content_count   = excluded.content_count,
			follower_count  = excluded.follower_count,
			following_count = excluded.following_count,
			introduction    = excluded.introduction,
			is_verified     = excluded.is_verified,
			category        = excluded.category,
			job_title       = excluded.job_title,
			updated_at      = excluded.updated_at`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("preparing batch people upsert: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC()
	for _, p := range people {
		if p.ID == "" {
			p.ID = NewID()
		}
		if _, err := stmt.Exec(
			p.ID, p.PlatformUsername, p.Platform,
			nullStr(p.FullName), nullStr(p.ImageURL),
			nullStr(p.ContactDetails), nullStr(p.Website),
			p.ContentCount, nullStr(p.FollowerCount), p.FollowingCount,
			nullStr(p.Introduction), boolToInt(p.IsVerified),
			nullStr(p.Category), nullStr(p.JobTitle),
			now, now,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("upserting person %s/%s: %w", p.Platform, p.PlatformUsername, err)
		}
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Social Lists
// ---------------------------------------------------------------------------

// CreateSocialList inserts a new social list.
func (d *Database) CreateSocialList(list *SocialList) error {
	if list.ID == "" {
		list.ID = NewID()
	}
	now := time.Now().UTC()
	list.CreatedAt = now
	list.UpdatedAt = now

	_, err := d.DB.Exec(`
		INSERT INTO social_lists (id, list_type, name, item_count, created_at, updated_at)
		VALUES (?,?,?,?,?,?)`,
		list.ID, nullStr(list.ListType), list.Name, list.ItemCount,
		list.CreatedAt, list.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("creating social list %s: %w", list.ID, err)
	}
	return nil
}

// GetSocialList retrieves a single social list by ID. Returns nil when not
// found.
func (d *Database) GetSocialList(id string) (*SocialList, error) {
	l := &SocialList{}
	var listType sql.NullString
	err := d.DB.QueryRow(`
		SELECT id, list_type, name, item_count, created_at, updated_at
		FROM social_lists WHERE id = ?`, id,
	).Scan(&l.ID, &listType, &l.Name, &l.ItemCount, &l.CreatedAt, &l.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting social list %s: %w", id, err)
	}
	l.ListType = listType.String
	return l, nil
}

// ListSocialLists returns all social lists ordered by creation time.
func (d *Database) ListSocialLists() ([]*SocialList, error) {
	rows, err := d.DB.Query(`
		SELECT id, list_type, name, item_count, created_at, updated_at
		FROM social_lists ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing social lists: %w", err)
	}
	defer rows.Close()

	var lists []*SocialList
	for rows.Next() {
		l := &SocialList{}
		var listType sql.NullString
		if err := rows.Scan(&l.ID, &listType, &l.Name, &l.ItemCount, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning social list row: %w", err)
		}
		l.ListType = listType.String
		lists = append(lists, l)
	}
	return lists, rows.Err()
}

// DeleteSocialList removes a social list by ID. Associated list items are
// removed automatically via ON DELETE CASCADE.
func (d *Database) DeleteSocialList(id string) error {
	result, err := d.DB.Exec("DELETE FROM social_lists WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting social list %s: %w", id, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("social list %s not found", id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Social List Items
// ---------------------------------------------------------------------------

// AddSocialListItem inserts a single item into a social list and increments
// the parent list's item_count.
func (d *Database) AddSocialListItem(item *SocialListItem) error {
	if item.ID == "" {
		item.ID = NewID()
	}

	tx, err := d.DB.Begin()
	if err != nil {
		return fmt.Errorf("beginning add list item transaction: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO social_list_items (
			id, list_id, platform, platform_username, image_url,
			url, full_name, contact_details, follower_count, created_at
		) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		item.ID, item.ListID, item.Platform, item.PlatformUsername,
		nullStr(item.ImageURL), nullStr(item.URL), nullStr(item.FullName),
		nullStr(item.ContactDetails), item.FollowerCount, time.Now().UTC(),
	)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("inserting social list item %s: %w", item.ID, err)
	}

	_, err = tx.Exec("UPDATE social_lists SET item_count = item_count + 1, updated_at = ? WHERE id = ?",
		time.Now().UTC(), item.ListID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("incrementing item_count for list %s: %w", item.ListID, err)
	}

	return tx.Commit()
}

// ListSocialListItems returns all items in a social list.
func (d *Database) ListSocialListItems(listID string) ([]*SocialListItem, error) {
	rows, err := d.DB.Query(`
		SELECT id, list_id, platform, platform_username, image_url,
		       url, full_name, contact_details, follower_count, created_at
		FROM social_list_items
		WHERE list_id = ?
		ORDER BY created_at ASC`, listID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing social list items for %s: %w", listID, err)
	}
	defer rows.Close()

	var items []*SocialListItem
	for rows.Next() {
		it := &SocialListItem{}
		var imageURL, url, fullName, contactDetails sql.NullString
		if err := rows.Scan(
			&it.ID, &it.ListID, &it.Platform, &it.PlatformUsername,
			&imageURL, &url, &fullName, &contactDetails,
			&it.FollowerCount, &it.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning social list item row: %w", err)
		}
		it.ImageURL = imageURL.String
		it.URL = url.String
		it.FullName = fullName.String
		it.ContactDetails = contactDetails.String
		items = append(items, it)
	}
	return items, rows.Err()
}

// BatchAddSocialListItems inserts multiple items into their respective social
// lists within a single transaction and updates the item_count for each
// affected list.
func (d *Database) BatchAddSocialListItems(items []*SocialListItem) error {
	if len(items) == 0 {
		return nil
	}

	tx, err := d.DB.Begin()
	if err != nil {
		return fmt.Errorf("beginning batch list items transaction: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO social_list_items (
			id, list_id, platform, platform_username, image_url,
			url, full_name, contact_details, follower_count, created_at
		) VALUES (?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("preparing batch list item insert: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC()
	listCounts := make(map[string]int)

	for _, it := range items {
		if it.ID == "" {
			it.ID = NewID()
		}
		if _, err := stmt.Exec(
			it.ID, it.ListID, it.Platform, it.PlatformUsername,
			nullStr(it.ImageURL), nullStr(it.URL), nullStr(it.FullName),
			nullStr(it.ContactDetails), it.FollowerCount, now,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("inserting social list item %s: %w", it.ID, err)
		}
		listCounts[it.ListID]++
	}

	// Update item_count for each affected list.
	for listID, count := range listCounts {
		if _, err := tx.Exec(
			"UPDATE social_lists SET item_count = item_count + ?, updated_at = ? WHERE id = ?",
			count, now, listID,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("updating item_count for list %s: %w", listID, err)
		}
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Threads
// ---------------------------------------------------------------------------

// UpsertThread inserts a new thread or updates an existing one matched by
// social_user_id + platform.
func (d *Database) UpsertThread(thread *Thread) error {
	if thread.ID == "" {
		thread.ID = NewID()
	}
	now := time.Now().UTC()

	_, err := d.DB.Exec(`
		INSERT INTO threads (id, social_user_id, platform, metadata, messages, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?)
		ON CONFLICT(social_user_id, platform)
		DO UPDATE SET
			metadata   = excluded.metadata,
			messages   = excluded.messages,
			updated_at = excluded.updated_at`,
		thread.ID, thread.SocialUserID, thread.Platform,
		nullStr(thread.Metadata), nullStr(thread.Messages), now, now,
	)
	if err != nil {
		return fmt.Errorf("upserting thread %s/%s: %w", thread.Platform, thread.SocialUserID, err)
	}
	return nil
}

// GetThread retrieves a single thread by social user ID and platform. Returns
// nil when not found.
func (d *Database) GetThread(socialUserID, platform string) (*Thread, error) {
	t := &Thread{}
	var metadata, messages sql.NullString
	err := d.DB.QueryRow(`
		SELECT id, social_user_id, platform, metadata, messages, created_at, updated_at
		FROM threads
		WHERE social_user_id = ? AND platform = ?`, socialUserID, platform,
	).Scan(&t.ID, &t.SocialUserID, &t.Platform, &metadata, &messages, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting thread %s/%s: %w", platform, socialUserID, err)
	}
	t.Metadata = metadata.String
	t.Messages = messages.String
	return t, nil
}

// ListThreads returns all threads, optionally filtered by platform. Pass an
// empty string to return threads across all platforms.
func (d *Database) ListThreads(platform string) ([]*Thread, error) {
	query := `
		SELECT id, social_user_id, platform, metadata, messages, created_at, updated_at
		FROM threads`
	var args []interface{}
	if platform != "" {
		query += " WHERE platform = ?"
		args = append(args, platform)
	}
	query += " ORDER BY updated_at DESC"

	rows, err := d.DB.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing threads: %w", err)
	}
	defer rows.Close()

	var threads []*Thread
	for rows.Next() {
		t := &Thread{}
		var metadata, messages sql.NullString
		if err := rows.Scan(&t.ID, &t.SocialUserID, &t.Platform, &metadata, &messages, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning thread row: %w", err)
		}
		t.Metadata = metadata.String
		t.Messages = messages.String
		threads = append(threads, t)
	}
	return threads, rows.Err()
}

// ---------------------------------------------------------------------------
// Templates
// ---------------------------------------------------------------------------

// CreateTemplate inserts a new template row. The auto-incremented ID is set on
// the provided struct after insertion.
func (d *Database) CreateTemplate(tmpl *Template) error {
	now := time.Now().UTC()
	result, err := d.DB.Exec(`
		INSERT INTO templates (name, subject, body, metadata, created_at, updated_at)
		VALUES (?,?,?,?,?,?)`,
		tmpl.Name, nullStr(tmpl.Subject), tmpl.Body, nullStr(tmpl.Metadata), now, now,
	)
	if err != nil {
		return fmt.Errorf("creating template: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("getting template ID: %w", err)
	}
	tmpl.ID = int(id)
	tmpl.CreatedAt = now
	tmpl.UpdatedAt = now
	return nil
}

// GetTemplate retrieves a single template by ID. Returns nil when not found.
func (d *Database) GetTemplate(id int) (*Template, error) {
	t := &Template{}
	var subject, metadata sql.NullString
	err := d.DB.QueryRow(`
		SELECT id, name, subject, body, metadata, created_at, updated_at
		FROM templates WHERE id = ?`, id,
	).Scan(&t.ID, &t.Name, &subject, &t.Body, &metadata, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting template %d: %w", id, err)
	}
	t.Subject = subject.String
	t.Metadata = metadata.String
	return t, nil
}

// ListTemplates returns all templates ordered by name.
func (d *Database) ListTemplates() ([]*Template, error) {
	rows, err := d.DB.Query(`
		SELECT id, name, subject, body, metadata, created_at, updated_at
		FROM templates ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("listing templates: %w", err)
	}
	defer rows.Close()

	var templates []*Template
	for rows.Next() {
		t := &Template{}
		var subject, metadata sql.NullString
		if err := rows.Scan(&t.ID, &t.Name, &subject, &t.Body, &metadata, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning template row: %w", err)
		}
		t.Subject = subject.String
		t.Metadata = metadata.String
		templates = append(templates, t)
	}
	return templates, rows.Err()
}

// DeleteTemplate removes a template by ID.
func (d *Database) DeleteTemplate(id int) error {
	result, err := d.DB.Exec("DELETE FROM templates WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("deleting template %d: %w", id, err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("template %d not found", id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Configs
// ---------------------------------------------------------------------------

// SaveConfig inserts or replaces a named configuration entry.
func (d *Database) SaveConfig(name, configData string) error {
	now := time.Now().UTC()
	_, err := d.DB.Exec(`
		INSERT INTO configs (name, config_data, created_at, updated_at)
		VALUES (?,?,?,?)
		ON CONFLICT(name)
		DO UPDATE SET config_data = excluded.config_data,
		              updated_at  = excluded.updated_at`,
		name, configData, now, now,
	)
	if err != nil {
		return fmt.Errorf("saving config %s: %w", name, err)
	}
	return nil
}

// GetConfig retrieves a single configuration entry by name. Returns nil when
// not found.
func (d *Database) GetConfig(name string) (*ConfigEntry, error) {
	c := &ConfigEntry{}
	err := d.DB.QueryRow(`
		SELECT name, config_data, created_at, updated_at
		FROM configs WHERE name = ?`, name,
	).Scan(&c.Name, &c.ConfigData, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting config %s: %w", name, err)
	}
	return c, nil
}

// ListConfigs returns all configuration entries.
func (d *Database) ListConfigs() ([]*ConfigEntry, error) {
	rows, err := d.DB.Query("SELECT name, config_data, created_at, updated_at FROM configs ORDER BY name ASC")
	if err != nil {
		return nil, fmt.Errorf("listing configs: %w", err)
	}
	defer rows.Close()

	var configs []*ConfigEntry
	for rows.Next() {
		c := &ConfigEntry{}
		if err := rows.Scan(&c.Name, &c.ConfigData, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning config row: %w", err)
		}
		configs = append(configs, c)
	}
	return configs, rows.Err()
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

// GetSetting retrieves a single setting value by key. Returns an error if the
// key does not exist.
func (d *Database) GetSetting(key string) (string, error) {
	var value string
	err := d.DB.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("setting %q not found", key)
	}
	if err != nil {
		return "", fmt.Errorf("getting setting %s: %w", key, err)
	}
	return value, nil
}

// SetSetting inserts or replaces a key-value setting.
func (d *Database) SetSetting(key, value string) error {
	_, err := d.DB.Exec(`
		INSERT INTO settings (key, value) VALUES (?,?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("setting %s: %w", key, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// nullStr converts an empty string to a sql.NullString with Valid=false,
// otherwise returns a valid NullString.
func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// nullInt converts a zero int to a sql.NullInt64 with Valid=false, otherwise
// returns a valid NullInt64.
func nullInt(i int) sql.NullInt64 {
	if i == 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(i), Valid: true}
}

// boolToInt converts a Go bool to a SQLite-friendly integer (0 or 1).
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
