package routes

import (
	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/cmd/core-api/endpoints"
	"github.com/tim-the-toolman-taylor/nivek/cmd/core-api/endpoints/autoshout"
	"github.com/tim-the-toolman-taylor/nivek/cmd/core-api/endpoints/fishing"
	"github.com/tim-the-toolman-taylor/nivek/cmd/core-api/endpoints/task"
	"github.com/tim-the-toolman-taylor/nivek/cmd/core-api/endpoints/user"
	"github.com/tim-the-toolman-taylor/nivek/cmd/core-api/endpoints/user/auth"
	"github.com/tim-the-toolman-taylor/nivek/cmd/core-api/endpoints/weather"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivekmiddleware"
)

func RegisterRoutes(nivek nivek.NivekService, e *echo.Echo) {

	//
	// Hello World
	e.GET(HelloWorld, endpoints.NewIndexEndpoint(nivek))

	e.POST(PostCreateUser, user.NewCreateUserEndpoint(nivek))

	//
	// Login, Signup
	e.POST(PostSignup, auth.NewSignupEndpoint(nivek))
	e.POST(PostLogin, auth.NewLoginEndpoint(nivek))

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
}
