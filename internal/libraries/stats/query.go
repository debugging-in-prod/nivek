package stats

import (
	"fmt"
	"strings"
)

// SQL builders. Kept as pure functions (query string + args) so the exact SQL
// is unit-tested without a database; store.go only executes them.

// placeholders returns "$start, $start+1, ..." for n values.
func placeholders(start, n int) string {
	p := make([]string, n)
	for i := range p {
		p[i] = fmt.Sprintf("$%d", start+i)
	}
	return strings.Join(p, ", ")
}

func homeArgs(home []string) []any {
	args := make([]any, 0, len(home))
	for _, h := range home {
		args = append(args, strings.ToLower(h))
	}
	return args
}

// ActiveChatsQuery counts distinct logins that are opted in, plus the home
// channels. Reads only nivek.users (small: one row per signed-up channel).
func ActiveChatsQuery(home []string) (string, []any) {
	q := `SELECT COUNT(*) FROM (
	SELECT lower(twitch_login) AS login
	  FROM nivek.users
	 WHERE bot_opt_in = TRUE
	   AND COALESCE(twitch_login, '') <> ''`
	if len(home) > 0 {
		q += `
	UNION
	SELECT login FROM (VALUES ` + valuesRows(1, len(home)) + `) AS home(login)`
	}
	q += `
) AS active`
	return q, homeArgs(home)
}

// valuesRows returns "($start), ($start+1), ..." for a VALUES list.
func valuesRows(start, n int) string {
	p := make([]string, n)
	for i := range p {
		p[i] = fmt.Sprintf("($%d::text)", start+i)
	}
	return strings.Join(p, ", ")
}

// TopChannelsQuery returns the channels with the most command uses of all
// time. Opted-out channels are excluded: a channel counts only while its users
// row has bot_opt_in = TRUE, or it is a home channel.
//
// Cost: one pass over nivek.command_usage, which holds one row per (channel,
// trigger, UTC day) actually used, so it grows by at most channels x triggers
// rows a day. The GROUP BY channel_twitch_id can be served in order by the
// primary key, whose leading column it is; the join is by users' twitch_id
// and LIMIT caps the output. PublicService runs it at most once per CacheTTL.
func TopChannelsQuery(home []string, limit int) (string, []any) {
	args := []any{}
	homeCond := "FALSE"
	if len(home) > 0 {
		homeCond = "t.channel_login IN (" + placeholders(1, len(home)) + ")"
		args = append(args, homeArgs(home)...)
	}
	args = append(args, limit)
	q := `WITH totals AS (
	SELECT channel_twitch_id,
	       MAX(channel_login) AS channel_login,
	       SUM(uses)::bigint  AS uses
	  FROM nivek.command_usage
	 GROUP BY channel_twitch_id
)
SELECT COALESCE(NULLIF(u.twitch_display_name, ''), NULLIF(u.twitch_login, ''), t.channel_login) AS display_name,
       t.uses
  FROM totals t
  LEFT JOIN nivek.users u ON u.twitch_id = t.channel_twitch_id
 WHERE (u.bot_opt_in IS TRUE OR ` + homeCond + `)
   AND COALESCE(NULLIF(u.twitch_display_name, ''), NULLIF(u.twitch_login, ''), t.channel_login) <> ''
 ORDER BY t.uses DESC, display_name ASC
 LIMIT $` + fmt.Sprint(len(args))
	return q, args
}

// TopFishersQuery returns the chatters with the highest !fish score summed
// across every channel. fish_score stores the chatter's lowercased Twitch
// login (public); PublicService swaps it for the Twitch display name. There
// is deliberately no opt-out filter: a fisher's score counts no matter which
// channel they fished in, including channels that later opted out.
func TopFishersQuery(limit int) (string, []any) {
	q := `SELECT chattername AS display_name,
       SUM(score)::bigint AS score,
       SUM(jsonb_array_length(fish))::bigint AS catches
  FROM nivek.fish_score
 WHERE chattername <> ''
 GROUP BY chattername
HAVING SUM(score) > 0
 ORDER BY score DESC, display_name ASC
 LIMIT $1`
	return q, []any{limit}
}

// UpsertUsageQuery builds one multi-row upsert for rows, which must already be
// normalized and de-duplicated (MergeUsage). uses is added to the existing
// count, clamped to the INTEGER range so a counter can never overflow into an
// error that would drop the whole batch.
func UpsertUsageQuery(rows []UsageCount) (string, []any) {
	const cols = 5
	values := make([]string, len(rows))
	args := make([]any, 0, len(rows)*cols)
	for i, r := range rows {
		b := i*cols + 1
		values[i] = fmt.Sprintf("($%d, $%d, $%d, $%d::date, $%d)", b, b+1, b+2, b+3, b+4)
		args = append(args, r.ChannelTwitchID, r.ChannelLogin, r.Trigger, r.Day, r.Uses)
	}
	q := `INSERT INTO nivek.command_usage AS cu (channel_twitch_id, channel_login, trigger, day, uses)
VALUES ` + strings.Join(values, ",\n       ") + `
ON CONFLICT (channel_twitch_id, trigger, day) DO UPDATE
   SET uses = LEAST(cu.uses::bigint + EXCLUDED.uses, 2147483647)::integer,
       channel_login = CASE WHEN EXCLUDED.channel_login <> '' THEN EXCLUDED.channel_login ELSE cu.channel_login END,
       updated_at = now()`
	return q, args
}
