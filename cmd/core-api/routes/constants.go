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

	GetAutoShoutChatters   = "/auto-shout"
	PostAutoShoutChatter   = "/auto-shout"
	DeleteAutoShoutChatter = "/auto-shout/:id/"
)
