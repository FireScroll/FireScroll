package partitions

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/danthegoodman1/Firescroll/utils"
	"golang.org/x/sync/errgroup"
	"io"
	"time"
)

func (p *Partition) backupInterval() {
	logger.Debug().Msgf("starting backup interval for partition %d", p.ID)
	for {
		select {
		case <-p.BackupTicker.C:
			s := time.Now()
			err := p.runBackup()
			if err != nil {
				logger.Error().Err(err).Msgf("error running backup for partition %d", p.ID)
				continue
			}
			logger.Debug().Msgf("partition %d backup completed in %s", p.ID, time.Since(s))
		case <-p.closeChan:
			logger.Debug().Msgf("backup ticker for partition %d received on close channel, exiting", p.ID)
			return
		}
	}
}

func (p *Partition) runBackup() error {
	uploader := s3manager.NewUploader(p.s3Session)
	pr, pw := io.Pipe()
	key := fmt.Sprintf("firescroll/%s/%d-%d.bdb", p.Namespace, p.ID, p.LastOffset)
	logger.Debug().Msgf("backup partition %d to S3 path %s", p.ID, key)
	g := errgroup.Group{}
	g.Go(func() error {
		defer pw.Close()
		_, err := p.DB.Backup(pw, 0)
		return err
	})
	g.Go(func() error {
		// TODO: ADJUSTABLE CONTEXT, LONGER DEFAULT
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()
		_, err := uploader.UploadWithContext(ctx, &s3manager.UploadInput{
			Bucket: &utils.Env_BackupS3Bucket,
			Key:    &key,
			Body:   pr,
		})
		return err
	})
	fmt.Println("waiting for errgroup to exit")
	return g.Wait()
}

//
//func (p *Partition) restoreFromS3() error {
//	// List the backups
//	logger.Debug().Msgf("listing S3 object with prefix %s", KeyPrefix)
//	keyPrefix := fmt.Sprintf("firescroll/%s/%d-", p.Namespace, p.ID)
//	params := &s3.ListObjectsV2Input{
//		Bucket: &utils.Env_BackupS3Bucket,
//		Prefix: &keyPrefix,
//	}
//	svc := s3.New(p.s3Session)
//	ctx, cancel := context.WithTimeout(context.Background(), time.Second*120)
//	defer cancel()
//	svc.ListObjectsV2PagesWithContext(ctx, params, func(output *s3.ListObjectsV2Output, _ bool) bool {
//
//	})
//	return nil
//}
