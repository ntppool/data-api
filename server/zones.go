package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
	"go.ntppool.org/data-api/ntpdb"
)

func (srv *Server) zoneCounts(c echo.Context) error {
	log := logger.Setup()
	ctx, span := tracing.Tracer().Start(c.Request().Context(), "zoneCounts")
	defer span.End()

	// just cache for a short time by default
	c.Response().Header().Set("Cache-Control", "public,max-age=240")
	c.Response().Header().Set("Access-Control-Allow-Origin", "*")
	c.Response().Header().Del("Vary")

	q := ntpdb.NewWrappedQuerier(ntpdb.New(srv.db))

	zone, err := q.GetZoneByName(ctx, c.Param("zone_name"))
	if err != nil || zone.ID == 0 {
		if errors.Is(err, sql.ErrNoRows) {
			return c.String(http.StatusNotFound, "Not found")
		}
		log.ErrorContext(ctx, "could not query for zone", "err", err)
		span.RecordError(err)
		return echo.NewHTTPError(http.StatusInternalServerError, "internal error")
	}

	counts, err := q.GetZoneCounts(ctx, zone.ID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.ErrorContext(ctx, "get counts", "err", err)
			span.RecordError(err)
			return c.String(http.StatusInternalServerError, "internal error")
		}
	}

	type historyEntry struct {
		D  string `json:"d"`  // date
		Ts int    `json:"ts"` // epoch timestamp
		Rc int    `json:"rc"` // count registered
		Ac int    `json:"ac"` // count active
		W  int    `json:"w"`  // netspeed active
		Iv string `json:"iv"` // ip version
	}

	rv := struct {
		History []historyEntry `json:"history"`
	}{}

	skipCount := 0.0
	limit := 0

	if limitParam := c.QueryParam("limit"); len(limitParam) > 0 {
		if limitInt, err := strconv.Atoi(limitParam); err == nil && limitInt > 0 {
			limit = limitInt
		}
	}

	var mostRecentDate int64 = -1
	if limit > 0 {
		count := 0
		dates := map[int64]bool{}
		for _, c := range counts {
			ep := c.Date.Unix()
			if _, ok := dates[ep]; !ok {
				count++
				dates[ep] = true
				mostRecentDate = ep
			}
		}
		if limit < count {
			if limit > 1 {
				skipCount = float64(count) / float64(limit-1)
			} else {
				// skip everything and use the special logic that we always include the most recent date
				skipCount = float64(count) + 1

			}
		}

		log.DebugContext(ctx, "mod", "count", count, "limit", limit, "mod", count%limit, "skipCount", skipCount)
		// log.Info("limit plan", "date count", count, "limit", limit, "skipCount", skipCount)
	}

	toSkip := 0.0
	if limit == 1 {
		toSkip = skipCount // we just want to look for the last entry
	}
	lastDate := int64(0)
	lastSkip := int64(0)
	skipThreshold := 0.5
	for _, c := range counts {
		cDate := c.Date.Unix()
		if (toSkip <= skipThreshold && cDate != lastSkip) ||
			lastDate == cDate ||
			mostRecentDate == cDate {
			// log.Info("adding date", "date", c.Date.Format(time.DateOnly))
			rv.History = append(rv.History, historyEntry{
				D:  c.Date.Format(time.DateOnly),
				Ts: int(cDate),
				Ac: int(c.CountActive),
				Rc: int(c.CountRegistered),
				W:  int(c.NetspeedActive),
				Iv: string(c.IpVersion),
			})
			lastDate = cDate
		} else {
			// log.Info("skipping date", "date", c.Date.Format(time.DateOnly))
			if lastSkip == cDate {
				continue
			}
			toSkip--
			lastSkip = cDate
			continue
		}
		if toSkip <= skipThreshold && skipCount > 0 {
			toSkip += skipCount
		}

	}

	if limit > 0 {
		count := 0
		dates := map[int]bool{}
		for _, c := range rv.History {
			ep := c.Ts
			if _, ok := dates[ep]; !ok {
				count++
				dates[ep] = true
			}
		}
		log.DebugContext(ctx, "result counts", "skipCount", skipCount, "limit", limit, "got", count)
	}

	c.Response().Header().Set("Cache-Control", "s-maxage=28800, max-age=7200")
	return c.JSON(http.StatusOK, rv)

}
