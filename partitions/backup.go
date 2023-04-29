package partitions

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/danthegoodman1/Firescroll/utils"
	"golang.org/x/sync/errgroup"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

var (
	ErrBackupNotFound    = errors.New("backup not found")
	ErrRemoteOffsetLower = errors.New("remote offset lower than local")
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
	if p.LastOffset == nil {
		// nothing to backup
		return nil
	}
	uploader := s3manager.NewUploader(p.s3Session)
	pr, pw := io.Pipe()
	key := fmt.Sprintf("firescroll/%s/%d-%d.bdb", p.Namespace, p.ID, *p.LastOffset)
	logger.Debug().Msgf("backup partition %d to S3 path %s", p.ID, key)
	g := errgroup.Group{}
	g.Go(func() error {
		defer pw.Close()
		_, err := p.DB.Backup(pw, 0)
		return err
	})
	g.Go(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(utils.Env_BackupTimeoutSec))
		defer cancel()
		_, err := uploader.UploadWithContext(ctx, &s3manager.UploadInput{
			Bucket: &utils.Env_BackupS3Bucket,
			Key:    &key,
			Body:   pr,
		})
		return err
	})
	return g.Wait()
}

func (p *Partition) getLatestBackupOffset(ctx context.Context) (int64, string, error) {
	// List the backups
	keyPrefix := fmt.Sprintf("firescroll/%s/%d-", p.Namespace, p.ID)
	logger.Debug().Msgf("listing S3 object with prefix %s", keyPrefix)
	params := &s3.ListObjectsV2Input{
		Bucket: &utils.Env_BackupS3Bucket,
		Prefix: &keyPrefix,
	}
	svc := s3.New(p.s3Session)
	newestBackup := ""
	err := svc.ListObjectsV2PagesWithContext(ctx, params, func(output *s3.ListObjectsV2Output, _ bool) bool {
		// TODO: might need some nil and length checking here
		if len(output.Contents) == 0 {
			return true
		}
		newestBackup = *output.Contents[len(output.Contents)-1].Key
		return true
	})
	if err != nil {
		return 0, "", fmt.Errorf("error in ListObjectsV2PagesWithContext: %w", err)
	}

	if newestBackup == "" {
		// Expected error
		return 0, "", ErrBackupNotFound
	}

	parts := strings.Split(newestBackup, "/")
	offsetString := strings.Split(strings.Split(parts[len(parts)-1], "-")[1], ".bdb")[0]
	offset, err := strconv.ParseInt(offsetString, 10, 64)

	return offset, newestBackup, nil
}

func (p *Partition) checkAndRestoreFromS3() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*120)
	defer cancel()
	offset, newestBackup, err := p.getLatestBackupOffset(ctx)
	if err != nil {
		return fmt.Errorf("error in getLatestBackupOffset: %w", err)
	}

	logger.Debug().Msgf("S3 backup detected for partition %d at offset %d", p.ID, offset)
	if p.LastOffset != nil && offset <= *p.LastOffset {
		logger.Debug().Msgf("remote offset for partition %d lower than or equal to local offset, using local", p.ID)
		// Expected error
		return ErrRemoteOffsetLower
	}

	logger.Debug().Msgf("deleting local DB for partition %d to restore backup from S3", p.ID)
	partitionPath := getPartitionPath(p.ID)

	restorePath := path.Join(partitionPath, "restore.bdb")
	f, err := os.Create(restorePath)
	defer f.Close()

	// Download the backup
	downloader := s3manager.NewDownloader(p.s3Session)
	_, err = downloader.DownloadWithContext(ctx, f, &s3.GetObjectInput{
		Bucket: &utils.Env_BackupS3Bucket,
		Key:    &newestBackup,
	})
	if err != nil {
		return fmt.Errorf("error in downloader.DownloadWithContext: %w", err)
	}

	err = p.DB.Load(f, 1)
	if err != nil {
		return fmt.Errorf("error in p.DB.Load: %w", err)
	}

	defer os.Remove(restorePath)
	p.LastOffset = &offset
	logger.Debug().Msgf("restored partition %d from S3 at offset %d", p.ID, offset)

	return nil
}
