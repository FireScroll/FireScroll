package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/danthegoodman1/FireScroll/gossip"
	"github.com/danthegoodman1/FireScroll/internal"
	"github.com/danthegoodman1/FireScroll/partitions"
	"github.com/danthegoodman1/FireScroll/utils"
	"github.com/labstack/echo/v4"
	"github.com/samber/lo"
	"github.com/twmb/franz-go/pkg/kgo"
	"golang.org/x/sync/errgroup"
	"io"
	"net/http"
	"time"
)

var (
	ErrHighStatusCode = errors.New("high status code")
)

func (s *HTTPServer) operationHandler(c echo.Context) error {
	st := time.Now()
	operation := partitions.Operation(c.Param("op"))
	var err error
	switch operation {
	case partitions.OperationPut, partitions.OperationDelete:
		err = s.handleMutation(c)
	case partitions.OperationGet:
		err = s.handleGet(c)
	case partitions.OperationList:
		err = s.handleList(c)
	default:
		err = c.String(http.StatusBadRequest, fmt.Sprintf("unknown operation '%s'", operation))
	}
	if err == nil {
		internal.Metric_HTTPLatenciesMicro.With(map[string]string{
			"operation": string(operation),
		}).Observe(float64(time.Since(st).Microseconds()))
	}
	return err
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
	nMS := time.Now().UnixMilli()
	partMap := map[int32][]*kgo.Record{}
	for _, mut := range reqBody.Records {
		mut.TsMs = nMS
		mut.Mutation = partitions.Operation(c.Param("op"))
		// If we have an `if`, verify it works
		if mut.If != nil {
			err := partitions.VerifyIfStatement(*mut.If)
			if err != nil {
				return c.String(http.StatusBadRequest, fmt.Sprintf("error compiling `if` statement `%s`: %s", *mut.If, err))
			}
		}
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
		_, exists := partMap[partID]
		if !exists {
			partMap[partID] = []*kgo.Record{record}
			continue
		}
		partMap[partID] = append(partMap[partID], record)
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
	Records []partitions.RecordKey `validate:"min=1"`
}
type GetRes struct {
	Records []partitions.Record
}

func (s *HTTPServer) handleGet(c echo.Context) error {
	var reqBody GetReq
	if err := ValidateRequest(c, &reqBody); err != nil {
		return c.String(http.StatusBadRequest, err.Error())
	}

	localPartitions := s.pm.GetPartitionIDs()

	localKeys := map[int32][]partitions.RecordKey{}
	remoteKeys := map[int32][]partitions.RecordKey{}

	for _, key := range reqBody.Records {
		part := utils.GetPartition(key.Pk)
		logger.Debug().Msgf("using partition %d for %+v", part, key)
		if lo.Contains(localPartitions, part) {
			_, exists := localKeys[part]
			if !exists {
				localKeys[part] = []partitions.RecordKey{key}
				continue
			}
			localKeys[part] = append(localKeys[part], key)
			continue
		}
		_, exists := remoteKeys[part]
		if !exists {
			remoteKeys[part] = []partitions.RecordKey{key}
			continue
		}
		remoteKeys[part] = append(remoteKeys[part], key)
	}

	results := make(chan []partitions.Record, len(localKeys)+len(remoteKeys))
	g := &errgroup.Group{}
	g.Go(func() error {
		r, err := s.pm.ReadRecords(localKeys)
		if err != nil {
			return err
		}
		results <- r
		return nil
	})
	for p, k := range remoteKeys {
		// TODO: optimize to send only a single request per remote partition
		partition := p
		keys := k
		g.Go(func() error {
			addr, err := s.gm.GetRandomRemotePartition(partition)
			if errors.Is(err, gossip.ErrNoRemotePartitions) {
				logger.Warn().Msgf("did not get any remote addresses for partition %d, it probably shut down and the partition has not been reassigned yet", partition)
				return nil
			} else if err != nil {
				return fmt.Errorf("error in GetRandomRemotePartition for partition %d: %w", partition, err)
			}
			r, err := s.doRemoteOperation(c.Request().Context(), addr, partitions.OperationGet, GetReq{Records: keys})
			if err != nil {
				return fmt.Errorf("error in getRemoteRecords for partition %d: %w", partition, err)
			}
			results <- r
			return nil
		})
	}
	err := g.Wait()
	if err != nil {
		return fmt.Errorf("error getting records: %w", err)
	}
	close(results)
	res := GetRes{
		Records: []partitions.Record{}, // no null return
	}

	for result := range results {
		res.Records = append(res.Records, result...)
	}

	return c.JSON(http.StatusOK, res)
}

func (s *HTTPServer) doRemoteOperation(ctx context.Context, addr string, op partitions.Operation, body any) ([]partitions.Record, error) {
	// TODO: encoders for encoding and decoding of bodies so no double allocation
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("error in json.Marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://%s/records/%s", addr, op), bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("error in http.NewRequestWithContext: %w", err)
	}
	req.Header.Add("content-type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error in http.Do: %w", err)
	}

	resBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("error in io.ReadAll: %w", err)
	}
	if res.StatusCode > 299 {
		return nil, fmt.Errorf("got high status code %d from addr %s: %s %w", res.StatusCode, addr, string(resBytes), ErrHighStatusCode)
	}
	var getRes GetRes
	err = json.Unmarshal(resBytes, &getRes)
	if err != nil {
		return nil, fmt.Errorf("error in json.Unmarshal: %w", err)
	}
	return getRes.Records, nil
}

type ListReq struct {
	Pk       string  `json:"pk"`
	SkPrefix string  `json:"sk_prefix"`
	Limit    int64   `json:"limit" validate:"gte=0"`
	If       *string `json:"if"`
	IfStop   bool    `json:"if_stop"`
}

func (s *HTTPServer) handleList(c echo.Context) error {
	var reqBody ListReq
	if err := ValidateRequest(c, &reqBody); err != nil {
		return c.String(http.StatusBadRequest, err.Error())
	}

	localPartitions := s.pm.GetPartitionIDs()
	part := utils.GetPartition(reqBody.Pk)
	var listRes []partitions.Record
	var err error
	if reqBody.If != nil {
		// Verify it's valid
		err := partitions.VerifyIfStatement(*reqBody.If)
		if err != nil {
			return c.String(http.StatusBadRequest, fmt.Sprintf("error compiling `if` statement `%s`: %s", *reqBody.If, err))
		}
	}
	if lo.Contains(localPartitions, part) {
		listRes, err = s.pm.ListRecords(part, reqBody.Pk, reqBody.SkPrefix, reqBody.Limit, reqBody.If, reqBody.IfStop)
		if err != nil {
			return fmt.Errorf("error in pm.ListRecords: %w", err)
		}
	} else {
		addr, err := s.gm.GetRandomRemotePartition(part)
		if err != nil {
			return fmt.Errorf("error in GetRandomRemotePartition: %w", err)
		}
		listRes, err = s.doRemoteOperation(c.Request().Context(), addr, partitions.OperationList, reqBody)
	}

	res := GetRes{
		Records: []partitions.Record{}, // no null return
	}
	res.Records = append(res.Records, listRes...)

	return c.JSON(http.StatusOK, res)
}
