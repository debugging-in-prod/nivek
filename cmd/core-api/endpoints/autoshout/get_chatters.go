package autoshout

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
)

func NewGetAutoShoutChattersEndpoint(nivek nivek.NivekService) echo.HandlerFunc {
	return func(c echo.Context) error {
		//user, err := utilities.GetUserFromContext(c)
		//if err != nil {
		//	return c.JSON(http.StatusUnauthorized, map[string]string{
		//		"error": "internal server error",
		//	})
		//}

		// these debug lines aren't printing

		fmt.Println("~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~")
		fmt.Println("hi momx44")
		fmt.Println("~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~")

		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})

		//autoShoutService := autoShoutSvc.NewService(nivek)
		//chatters, errChat := autoShoutService.GetAutoShoutChatters(user.Username)
		//if errChat != nil {
		//	return c.JSON(http.StatusInternalServerError, map[string]string{
		//		"error": fmt.Sprintf(
		//			"error fetching auto shout chatter for user [%s]: %s",
		//			user.Username, errChat.Error(),
		//		),
		//	})
		//}
		//
		//return c.JSON(http.StatusOK, chatters)
	}
}
