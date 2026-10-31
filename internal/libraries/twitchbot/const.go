package twitchbot

// defaultNoReplyLogins is a curated list of official accounts of widely used
// Twitch chat-bot services. The bot never replies to or runs commands for
// these logins, so their automated messages can't start reply loops. It is
// always merged (union, lowercased, deduped) with TWITCH_BOT_NO_REPLY_LOGINS
// and the bot's own login; see buildNoReplySet. Add only official service bot
// accounts here, never a real streamer's or the owner's accounts.
var defaultNoReplyLogins = []string{
	"nightbot",
	"streamelements",
	"streamlabs",
	"moobot",
	"fossabot",
	"wizebot",
	"soundalerts",
	"sery_bot",
	"botisimo",
	"deepbot",
	"phantombot",
	"coebot",
	"vivbot",
	"pretzelrocks",
	"kofistreambot",
	"streamstickers",
	"blerp",
	"lumiastream",
	"songlistbot",
	"stay_hydrated_bot",
	"restreambot",
	"tangiabot",
	"creatisbot",
	"frostytoolsdotcom",
	"own3d",
	"streamcaptainbot",
	"playwithviewersbot",
	"commanderroot",
}
