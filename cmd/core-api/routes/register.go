package routes

import (
	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/cmd/core-api/endpoints"
	"github.com/tim-the-toolman-taylor/nivek/cmd/core-api/endpoints/autoshout"
	"github.com/tim-the-toolman-taylor/nivek/cmd/core-api/endpoints/bot"
	"github.com/tim-the-toolman-taylor/nivek/cmd/core-api/endpoints/df"
	"github.com/tim-the-toolman-taylor/nivek/cmd/core-api/endpoints/fishing"
	"github.com/tim-the-toolman-taylor/nivek/cmd/core-api/endpoints/messenger"
	"github.com/tim-the-toolman-taylor/nivek/cmd/core-api/endpoints/task"
	"github.com/tim-the-toolman-taylor/nivek/cmd/core-api/endpoints/user"
	"github.com/tim-the-toolman-taylor/nivek/cmd/core-api/endpoints/user/auth"
	"github.com/tim-the-toolman-taylor/nivek/cmd/core-api/endpoints/weather"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivekmiddleware"
)

// RegisterRoutes attaches the API handlers to the given router group. It takes
// an *echo.Group (rather than *echo.Echo) so the API can be mounted under a
// path prefix — e.g. e.Group("/api") — leaving the root free for the static
// SPA served by middleware.Static (Traefik routes both to the same container).
func RegisterRoutes(nivek nivek.NivekService, e *echo.Group) {

	//
	// Hello World
	e.GET(HelloWorld, endpoints.NewIndexEndpoint(nivek))

	//
	// Auth — Twitch OAuth is the only signup/login path. /start redirects to
	// Twitch with a CSRF state cookie; /callback exchanges the code, fetches
	// the user's Twitch profile, find-or-creates a row, issues our JWT, and
	// 302s back to the frontend with the token in the URL fragment.
	e.GET(GetTwitchStart, auth.NewTwitchStartEndpoint(nivek))
	e.GET(GetTwitchCallback, auth.NewTwitchCallbackEndpoint(nivek))

	//
	// Secure routes:
	e.POST(PostLogout, auth.NewLogoutEndpoint(nivek),
		nivekmiddleware.NewJWTMiddleware(nivek).Middleware(),
	)

	e.POST(PostFetchUserData, user.NewGetProfileEndpoint(nivek),
		nivekmiddleware.NewJWTMiddleware(nivek).Middleware(),
	)

	e.GET(GetUserTasks, task.NewGetUserTasksEndpoint(nivek),
		nivekmiddleware.NewJWTMiddleware(nivek).Middleware(),
	)

	e.POST(PostCreateUserTask, task.NewPostCreateUserTaskEndpoint(nivek),
		nivekmiddleware.NewJWTMiddleware(nivek).Middleware(),
	)

  e.POST(
    TwitchWebhookSubscriptionRequest,
    endpoints.NewTwitchEventSubEndpoint(nivek),
  )

	// weather
	e.POST(PostWeather, weather.NewGetWeatherEndpoint(nivek),
		nivekmiddleware.NewJWTMiddleware(nivek).Middleware(),
	)

	// fishing
	e.GET(GetFishingScore, fishing.NewGetFishingScoreEndpoint(nivek),
		nivekmiddleware.NewJWTMiddleware(nivek).Middleware(),
	)

	// auto shout
	e.GET(GetAutoShoutChatters, autoshout.NewGetAutoShoutChattersEndpoint(nivek),
		nivekmiddleware.NewJWTMiddleware(nivek).Middleware(),
	)
	e.POST(PostCreateAutoShoutChatter, autoshout.NewCreateAutoShoutChatterEndpoint(nivek),
		nivekmiddleware.NewJWTMiddleware(nivek).Middleware(),
	)
	e.POST(PostUpdateAutoShoutChatter, autoshout.NewUpdateAutoShoutChatterEndpoint(nivek),
		nivekmiddleware.NewJWTMiddleware(nivek).Middleware(),
	)
	e.DELETE(DeleteAutoShoutChatter, autoshout.NewDeleteAutoShoutChatterEndpoint(nivek),
		nivekmiddleware.NewJWTMiddleware(nivek).Middleware(),
	)

	// messager
	e.POST(PostCreateMessage, messenger.NewCreateMesageEndpoint(nivek),
		nivekmiddleware.NewJWTMiddleware(nivek).Middleware(),
	)
	e.GET(GetMessages, messenger.NewGetMessagesEndpoint(nivek),
		nivekmiddleware.NewJWTMiddleware(nivek).Middleware(),
	)

	// DF dashboard (GET public, POST HMAC-authed in the handler)
	e.GET(GetDFSnapshot, df.NewGetSnapshotEndpoint(nivek))
	e.POST(PostDFSnapshot, df.NewPostSnapshotEndpoint(nivek))

	// twitch-bot RPC. HMAC-authed via BOT_API_HMAC_KEY (hex). See
	// nivekmiddleware.NewHMACMiddleware for the canonical-string format the
	// bot signs.
	botAuth := nivekmiddleware.NewHMACMiddleware("BOT_API_HMAC_KEY")
	e.GET(GetBotChannels, bot.NewGetChannelsEndpoint(nivek), botAuth)
  e.GET(GetActiveChannels, bot.NewGetActiveChannelsEndpoint(nivek), botAuth)
	e.POST(PostBotBreadIncrement, bot.NewPostBreadIncrementEndpoint(nivek), botAuth)
	e.GET(GetBotBreadTotal, bot.NewGetBreadTotalEndpoint(nivek), botAuth)
	e.POST(PostBotLurkMessage, bot.NewPostLurkMessageEndpoint(nivek), botAuth)
	e.POST(PostBotFishGo, bot.NewPostFishGoEndpoint(nivek), botAuth)
}
