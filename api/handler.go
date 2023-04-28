package api

import (
	"encoding/json"
	"fmt"
	"github.com/danthegoodman1/Firescroll/partitions"
	"github.com/labstack/echo/v4"
	"github.com/samber/lo"
	"github.com/twmb/franz-go/pkg/kgo"
	"net/http"
)

func (s *HTTPServer) operationHandler(c echo.Context) error {
	operation := partitions.Operation(c.Param("op"))
	switch operation {
	case partitions.OperationPut, partitions.OperationDelete:
		return s.handleMutation(c)
	case partitions.OperationGet:
		return s.handleGet(c)
	default:
		return c.String(http.StatusBadRequest, fmt.Sprintf("unknown operation '%s'", operation))
	}
}

type MutationReq struct {
	Records []partitions.RecordMutation `validate:"min=1"`
}

func (s *HTTPServer) handleMutation(c echo.Context) error {
	var reqBody MutationReq
	if err := ValidateRequest(c, &reqBody); err != nil {
		return c.String(http.StatusBadRequest, err.Error())
	}

	// TODO: Group and publish by partition
	err := s.lc.Client.ProduceSync(c.Request().Context(), lo.Map(reqBody.Records, func(item partitions.RecordMutation, index int) *kgo.Record {
		item.Mutation = partitions.Operation(c.Param("op"))
		jsonB, err := json.Marshal(item)
		if err != nil {
			// TODO: handle without lo package
			logger.Fatal().Err(err).Msg("error marshalling json, exiting")
		}
		return &kgo.Record{
			Key:     []byte(item.Pk),
			Value:   jsonB,
			Topic:   s.lc.MutationTopic,
			Context: c.Request().Context(),
		}
	})...).FirstErr()
	if err != nil {
		return fmt.Errorf("error in ProduceSync: %w", err)
	}

	return c.String(http.StatusOK, "ok")
}

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
