package services

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/go-gorp/gorp"
	"github.com/lib/pq"

	"github.com/ovh/cds/engine/api/cache"
	"github.com/ovh/cds/sdk"
	"github.com/ovh/cds/sdk/log"
)

// Pings browses all external services and ping them
func Pings(ctx context.Context, dbFunc func() *gorp.DbMap, ss []sdk.ExternalService) {
	tickPing := time.NewTicker(1 * time.Minute)
	db := dbFunc()
	for {
		select {
		case <-ctx.Done():
			if ctx.Err() != nil {
				log.Error("services.Ping> Exiting scheduler.Cleaner: %v", ctx.Err())
				return
			}
		case <-tickPing.C:
			for _, s := range ss {
				tx, err := db.Begin()
				if err != nil {
					log.Warning("services.Ping> Unable to start transaction")
					continue
				}
				if err := ping(tx, s); err != nil {
					_ = tx.Rollback()
					continue
				}
				if err := tx.Commit(); err != nil {
					_ = tx.Rollback()
				}
			}
		}
	}
}

func ping(db gorp.SqlExecutor, s sdk.ExternalService) error {
	// Select for update
	var serv service
	query := `select * from services where name = $1 for update nowait`
	if err := db.SelectOne(&serv, query, s.Name); err != nil {
		if pqerr, ok := err.(*pq.Error); ok && pqerr.Code == "55P03" {
			log.Debug("services.ping> Unable to lock service %s: %v", s.Name, err)
			return nil
		}
		log.Warning("services.ping> Unable to lock service %s: %v", s.Name, err)
		return err
	}

	mon := sdk.MonitoringStatus{
		Now: time.Now(),
		Lines: []sdk.MonitoringStatusLine{
			{
				Type:      s.Type,
				Component: s.Name,
			},
		},
	}
	var pingURL string
	if s.HealthPort == "0" || s.HealthPort == "" {
		pingURL = s.HealthURL
	} else {
		pingURL = fmt.Sprintf("%s:%s", s.HealthURL, s.HealthPort)
	}
	_, code, err := doRequest(context.Background(), pingURL, "", "GET", s.HealthPath, nil)
	if err != nil || code >= 400 {
		mon.Lines[0].Status = sdk.MonitoringStatusWarn
		mon.Lines[0].Value = "Health: KO"
	} else {
		mon.Lines[0].Status = sdk.MonitoringStatusOK
		mon.Lines[0].Value = "Health: OK"
	}

	serv.LastHeartbeat = time.Now()
	serv.MonitoringStatus = mon
	if _, err := db.Update(&serv); err != nil {
		log.Warning("service.ping> unable to update monitoring status: %v", err)
		return err
	}
	return nil
}

// InitExternal initializes external services
func InitExternal(dbFunc func() *gorp.DbMap, store cache.Store, ss []sdk.ExternalService) error {
	db := dbFunc()
	for _, s := range ss {
		oldSrv, errOldSrv := FindByName(db, s.Name)
		if errOldSrv != nil && errOldSrv != sdk.ErrNotFound {
			return fmt.Errorf("InitExternal> unable to find service %s", s.Name)
		}

		if oldSrv == nil {
			s.Service.LastHeartbeat = time.Now()
			s.Service.Config = s
			if err := Insert(db, &s.Service); err != nil {
				return fmt.Errorf("InitExternal> unable to insert external service: %v", err)
			}
		} else {
			tx, err := db.Begin()
			if err != nil {
				return fmt.Errorf("InitExternal> unable to start transaction: %v", err)
			}
			var serv service
			query := `select * from services where name = $1 for update nowait`
			if err := tx.SelectOne(&serv, query, s.Name); err != nil && err != sql.ErrNoRows {
				log.Warning("services.ping> Unable to lock service %s: %v", s.Name, err)
				_ = tx.Rollback()
				return err
			}
			s.Service.LastHeartbeat = oldSrv.LastHeartbeat
			s.Service.MonitoringStatus = oldSrv.MonitoringStatus
			s.Service.Config = s
			if err := Update(tx, &s.Service); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("InitExternal> unable to update external service: %v", err)
			}
			if err := tx.Commit(); err != nil {
				_ = tx.Rollback()
			}
		}
	}
	return nil
}
