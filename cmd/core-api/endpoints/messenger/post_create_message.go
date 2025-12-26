package messenger

import (
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	messagerSvc "github.com/tim-the-toolman-taylor/nivek/internal/libraries/messager"
	"github.com/tim-the-toolman-taylor/nivek/internal/libraries/nivek"
)

func NewCreateMesageEndpoint(nivek nivek.NivekService) echo.HandlerFunc {
	return func(c echo.Context) error {
		var newMessage messagerSvc.Message
		if err := c.Bind(&newMessage); err != nil {
			nivek.Logger().Errorf("[Messager] failed to read request body: %w", err)
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "failed to read request body",
			})
		}

		newMessage.CreatedAt = time.Now()
		newMessage.UpdatedAt = time.Now()

		messagerService := messagerSvc.NewService(nivek)
		if err := messagerService.CreateMessage(newMessage); err != nil {
			nivek.Logger().Errorf("[Messager] failed to create new message %s", err.Error())
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "failed to create message",
			})
		}

		return c.JSON(http.StatusOK, map[string]string{})
	}
}
