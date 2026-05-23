package routes

const (
	HelloWorld = "/"

	PostCreateUser = "/user"

	PostSignup = "/signup"
	PostLogin  = "/login"

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
