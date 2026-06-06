package routes

const (
	HelloWorld = "/"

	// Twitch OAuth — see endpoints/user/auth/twitch.go. /start kicks off the
	// authorize redirect; /callback is what Twitch sends the user back to.
	GetTwitchStart    = "/auth/twitch/start"
	GetTwitchCallback = "/auth/twitch/callback"

	//
	// secure routes
	//

	PostLogout         = "/logout"
	PostFetchUserData  = "/profile"
	GetUserTasks       = "/user/:id/task"
	PostCreateUserTask = "/user/:id/task"
	GetFishingScore    = "/fishing"

	PostWeather = "/weather"

	GetAutoShoutChatters       = "/auto-shout"
	PostCreateAutoShoutChatter = "/auto-shout"
	PostUpdateAutoShoutChatter = "/auto-shout/:id"
	DeleteAutoShoutChatter     = "/auto-shout/:id"

	PostCreateMessage = "/message"
	GetMessages       = "/message"

	// DF dashboard (public, no auth — dashboard is publicly fanned out)
	GetDFSnapshot = "/df/snapshot"
	// DF dashboard ingest from the DFHost pusher (HMAC-authed in handler)
	PostDFSnapshot = "/df/snapshot"
)
