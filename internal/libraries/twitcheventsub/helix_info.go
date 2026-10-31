package twitcheventsub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Read-only Helix lookups backing the bot's info commands (!uptime, !game,
// !title, !accountage, !followage). Every call honors ctx, so callers bound
// them with a short timeout.

// helixBaseURL is a var (not a const) so tests can point it at an httptest
// server.
var helixBaseURL = "https://api.twitch.tv/helix"

// ErrFollowsUnavailable means Helix would not reveal follow data for the
// channel: the token lacks moderator:read:followers, or the bot isn't a
// moderator there (Twitch then returns only the follower total).
var ErrFollowsUnavailable = errors.New("follow data unavailable for this channel")

// StreamInfo is the subset of Get Streams used for !uptime.
type StreamInfo struct {
	Live      bool
	StartedAt time.Time
}

// ChannelInfo is the subset of Get Channel Information used for !game/!title.
type ChannelInfo struct {
	GameName string `json:"game_name"`
	Title    string `json:"title"`
}

// UserInfo is the subset of Get Users used for !accountage / !followage.
type UserInfo struct {
	ID          string    `json:"id"`
	Login       string    `json:"login"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
}

// FollowInfo is the result of a Get Channel Followers lookup for one user.
type FollowInfo struct {
	Following  bool
	FollowedAt time.Time
}

// GetStream reports whether broadcasterID is live and since when. An offline
// channel is {Live: false} with a nil error.
func (c *clientImpl) GetStream(ctx context.Context, broadcasterID string) (StreamInfo, error) {
	if broadcasterID == "" {
		return StreamInfo{}, errors.New("broadcaster id is required")
	}
	q := url.Values{"user_id": {broadcasterID}}
	body, status, err := c.doAppGet(ctx, helixBaseURL+"/streams?"+q.Encode())
	if err != nil {
		return StreamInfo{}, err
	}
	if status != http.StatusOK {
		return StreamInfo{}, fmt.Errorf("get streams returned %d: %s", status, truncateBody(body))
	}
	return parseStreamInfo(body)
}

func parseStreamInfo(body []byte) (StreamInfo, error) {
	var parsed struct {
		Data []struct {
			Type      string    `json:"type"`
			StartedAt time.Time `json:"started_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return StreamInfo{}, fmt.Errorf("decode streams response: %w", err)
	}
	if len(parsed.Data) == 0 || parsed.Data[0].StartedAt.IsZero() {
		return StreamInfo{}, nil
	}
	return StreamInfo{Live: true, StartedAt: parsed.Data[0].StartedAt}, nil
}

// GetLastStreamEnd estimates when broadcasterID's most recent stream ended
// from their newest archive VOD (created_at + duration). found is false (nil
// error) when there is no archive to go on: VODs disabled, expired or deleted.
// Only meaningful while the channel is offline — during a live stream the
// newest archive is the in-progress one.
func (c *clientImpl) GetLastStreamEnd(ctx context.Context, broadcasterID string) (time.Time, bool, error) {
	if broadcasterID == "" {
		return time.Time{}, false, errors.New("broadcaster id is required")
	}
	q := url.Values{"user_id": {broadcasterID}, "type": {"archive"}, "sort": {"time"}, "first": {"1"}}
	body, status, err := c.doAppGet(ctx, helixBaseURL+"/videos?"+q.Encode())
	if err != nil {
		return time.Time{}, false, err
	}
	if status == http.StatusNotFound {
		// Helix answers 404 when the user has no videos at all.
		return time.Time{}, false, nil
	}
	if status != http.StatusOK {
		return time.Time{}, false, fmt.Errorf("get videos returned %d: %s", status, truncateBody(body))
	}
	return parseLastStreamEnd(body)
}

func parseLastStreamEnd(body []byte) (time.Time, bool, error) {
	var parsed struct {
		Data []struct {
			CreatedAt time.Time `json:"created_at"`
			Duration  string    `json:"duration"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return time.Time{}, false, fmt.Errorf("decode videos response: %w", err)
	}
	if len(parsed.Data) == 0 || parsed.Data[0].CreatedAt.IsZero() {
		return time.Time{}, false, nil
	}
	// Helix durations look like "3h8m33s" / "45m2s" / "9s", which
	// time.ParseDuration accepts as-is.
	d, err := time.ParseDuration(parsed.Data[0].Duration)
	if err != nil || d < 0 {
		return time.Time{}, false, nil
	}
	return parsed.Data[0].CreatedAt.Add(d), true, nil
}

// GetChannelInfo returns the channel's current category and title.
func (c *clientImpl) GetChannelInfo(ctx context.Context, broadcasterID string) (ChannelInfo, error) {
	if broadcasterID == "" {
		return ChannelInfo{}, errors.New("broadcaster id is required")
	}
	q := url.Values{"broadcaster_id": {broadcasterID}}
	body, status, err := c.doAppGet(ctx, helixBaseURL+"/channels?"+q.Encode())
	if err != nil {
		return ChannelInfo{}, err
	}
	if status != http.StatusOK {
		return ChannelInfo{}, fmt.Errorf("get channels returned %d: %s", status, truncateBody(body))
	}
	var parsed struct {
		Data []ChannelInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ChannelInfo{}, fmt.Errorf("decode channels response: %w", err)
	}
	if len(parsed.Data) == 0 {
		return ChannelInfo{}, errors.New("get channels returned empty data")
	}
	return parsed.Data[0], nil
}

// GetUserByLogin looks a user up by login. found is false (nil error) when no
// such user exists.
func (c *clientImpl) GetUserByLogin(ctx context.Context, login string) (UserInfo, bool, error) {
	if login == "" {
		return UserInfo{}, false, errors.New("login is required")
	}
	q := url.Values{"login": {login}}
	body, status, err := c.doAppGet(ctx, helixBaseURL+"/users?"+q.Encode())
	if err != nil {
		return UserInfo{}, false, err
	}
	if status == http.StatusBadRequest {
		// Helix rejects malformed logins with 400; treat as "no such user".
		return UserInfo{}, false, nil
	}
	if status != http.StatusOK {
		return UserInfo{}, false, fmt.Errorf("get users returned %d: %s", status, truncateBody(body))
	}
	var parsed struct {
		Data []UserInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return UserInfo{}, false, fmt.Errorf("decode users response: %w", err)
	}
	if len(parsed.Data) == 0 {
		return UserInfo{}, false, nil
	}
	return parsed.Data[0], true, nil
}

// GetChannelFollow reports whether userID follows broadcasterID. Get Channel
// Followers needs a USER token with moderator:read:followers whose user is the
// broadcaster or one of its moderators (app tokens are rejected), so the
// caller passes the bot's user token. Any auth failure, or a response with no
// data array (Twitch's "not a moderator" shape), is ErrFollowsUnavailable.
func (c *clientImpl) GetChannelFollow(ctx context.Context, userToken, broadcasterID, userID string) (FollowInfo, error) {
	if broadcasterID == "" || userID == "" {
		return FollowInfo{}, errors.New("broadcaster id and user id are required")
	}
	if userToken == "" {
		return FollowInfo{}, ErrFollowsUnavailable
	}
	q := url.Values{"broadcaster_id": {broadcasterID}, "user_id": {userID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, helixBaseURL+"/channels/followers?"+q.Encode(), nil)
	if err != nil {
		return FollowInfo{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+userToken)
	req.Header.Set("Client-Id", c.cfg.ClientID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return FollowInfo{}, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return FollowInfo{}, fmt.Errorf("read response: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return FollowInfo{}, fmt.Errorf("%w: status %d", ErrFollowsUnavailable, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return FollowInfo{}, fmt.Errorf("get channel followers returned %d: %s", resp.StatusCode, truncateBody(body))
	}
	return parseFollowInfo(body)
}

func parseFollowInfo(body []byte) (FollowInfo, error) {
	var parsed struct {
		// Pointer so a missing/null "data" (only the total was returned, i.e.
		// no moderator access) is distinguishable from an empty list (the user
		// genuinely doesn't follow).
		Data *[]struct {
			FollowedAt time.Time `json:"followed_at"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return FollowInfo{}, fmt.Errorf("decode followers response: %w", err)
	}
	if parsed.Data == nil {
		return FollowInfo{}, ErrFollowsUnavailable
	}
	if len(*parsed.Data) == 0 {
		return FollowInfo{Following: false}, nil
	}
	return FollowInfo{Following: true, FollowedAt: (*parsed.Data)[0].FollowedAt}, nil
}

func truncateBody(b []byte) string {
	const max = 300
	if len(b) > max {
		return string(b[:max]) + "..."
	}
	return string(b)
}
