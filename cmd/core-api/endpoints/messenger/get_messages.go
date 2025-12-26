package messenger

import (
	"net/http"

	"github.com/labstack/echo/v4"
	messagerSvc "github.com/tim-the-toolman-taylor/nivek/internal/libraries/messager"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
)

func NewGetMessagesEndpoint(nivek nivek.NivekService) echo.HandlerFunc {
	return func(c echo.Context) error {
		messagerService := messagerSvc.NewService(nivek)
		messages, err := messagerService.GetMessages()
		if err != nil {
			nivek.Logger().Errorf("[Messager] failed to create new message %s", err.Error())
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "failed to create message",
			})
		}

		return c.JSON(http.StatusOK, messages)
	}
}
