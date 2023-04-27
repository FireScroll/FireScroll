package api

import (
	"fmt"
	"github.com/danthegoodman1/FanoutDB/partitions"
	"github.com/labstack/echo/v4"
	"net/http"
)

func (s *HTTPServer) operationHandler(c echo.Context) error {
	operation := partitions.Operation(c.Param("op"))
	switch operation {
	//case partitions.OperationPut, partitions.OperationDelete, partitions.OperationBatch:
	//	return handleMutation(c)
	case partitions.OperationGet:
		return s.handleGet(c)
	default:
		return c.String(http.StatusBadRequest, fmt.Sprintf("unknown operation '%s'", operation))
	}
}

//func handleMutation(c echo.Context) error {
//
//}

type GetReq struct {
	Keys []partitions.RecordKey `validate:"min=1"`
}
type GetRes struct {
	Results []partitions.Record
}

func (s *HTTPServer) handleGet(c echo.Context) error {
	var reqBody GetReq
	if err := ValidateRequest(c, &reqBody); err != nil {
		return c.String(http.StatusBadRequest, err.Error())
	}
	results, err := s.pm.ReadRecords(c.Request().Context(), reqBody.Keys)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, GetRes{Results: results})
}
