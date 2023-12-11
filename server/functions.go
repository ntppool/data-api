package server

import (
	"context"
	"database/sql"
	"errors"
	"net/netip"
	"strconv"
	"time"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/data-api/ntpdb"
)

func (srv *Server) FindServer(ctx context.Context, serverID string) (ntpdb.Server, error) {
	log := logger.Setup()
	ctx, span := tracing.Tracer().Start(ctx, "FindServer")
	defer span.End()
	q := ntpdb.NewWrappedQuerier(ntpdb.New(srv.db))

	var serverData ntpdb.Server
	var dberr error
	if id, err := strconv.Atoi(serverID); id > 0 && err == nil {
		serverData, dberr = q.GetServerByID(ctx, uint32(id))
	} else {
		ip, err := netip.ParseAddr(serverID)
		if err != nil || !ip.IsValid() {
			return ntpdb.Server{}, nil // 404 error
		}
		serverData, dberr = q.GetServerByIP(ctx, ip.String())
	}
	if dberr != nil {
		if !errors.Is(dberr, sql.ErrNoRows) {
			log.Error("could not query server id", "err", dberr)
			return serverData, dberr
		}
	}

	if serverData.ID == 0 || (serverData.DeletionOn.Valid && serverData.DeletionOn.Time.Before(time.Now().Add(-1*time.Hour*24*30*24))) {
		// no data and no error to produce 404 errors
		return ntpdb.Server{}, nil
	}

	return serverData, nil
}
