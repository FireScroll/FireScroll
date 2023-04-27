package api

import (
	"fmt"
	"github.com/danthegoodman1/FanoutDB/partitions"
	"github.com/labstack/echo/v4"
	"net/http"
)

func operationHandler(c echo.Context) error {
	operation := partitions.Operation(c.Param("op"))
	switch operation {
	case partitions.OperationPut, partitions.OperationDelete, partitions.OperationBatch:
		return handleMutation(c)
	case partitions.OperationGet:
		return handleGet(c)
	default:
		return c.String(http.StatusBadRequest, fmt.Sprintf("unknown operation '%s'", operation))
	}
}

func handleMutation(c echo.Context) error {

}

func handleGet(c echo.Context) error {

}
