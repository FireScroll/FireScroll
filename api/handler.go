package api

import (
	"encoding/json"
	"fmt"
	"github.com/danthegoodman1/Firescroll/partitions"
	"github.com/danthegoodman1/Firescroll/utils"
	"github.com/labstack/echo/v4"
	"github.com/twmb/franz-go/pkg/kgo"
	"golang.org/x/sync/errgroup"
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

	// Break up and produce by partition
	partMap := map[int32][]*kgo.Record{}
	for _, mut := range reqBody.Records {
		mut.Mutation = partitions.Operation(c.Param("op"))
		jsonB, err := json.Marshal(mut)
		if err != nil {
			logger.Fatal().Err(err).Msg("error marshalling json, exiting")
		}
		record := &kgo.Record{
			Key:     []byte(mut.Pk),
			Value:   jsonB,
			Topic:   s.lc.MutationTopic,
			Context: c.Request().Context(),
		}
		partID := utils.GetPartition(mut.Pk)
		part, exists := partMap[partID]
		if !exists {
			partMap[partID] = []*kgo.Record{record}
			continue
		}
		part = append(part, record)
	}
	g := errgroup.Group{}
	for partID, records := range partMap {
		r := records // var reuse protection
		p := partID
		g.Go(func() error {
			logger.Debug().Msgf("producing %d items for partition %d", len(r), p)
			return s.lc.Client.ProduceSync(c.Request().Context(), r...).FirstErr()
		})
	}
	err := g.Wait()
	if err != nil {
		return fmt.Errorf("error in produce group: %w", err)
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
