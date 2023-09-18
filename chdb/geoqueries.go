package chdb

// queries to the GeoDNS database

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"sort"
	"time"
)

type flatAPI struct {
	CC   string
	IPv4 float64
	IPv6 float64
}

type UserCountry []flatAPI

func (s UserCountry) Len() int {
	return len(s)
}
func (s UserCountry) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s UserCountry) Less(i, j int) bool {
	return s[i].IPv4 > s[j].IPv4
}

func (d *ClickHouse) UserCountryData(ctx context.Context) (*UserCountry, error) {

	// rows, err := conn.Query(ctx, "select dt,UserCC,Qtype,sum(queries) as queries from by_usercc_1d group by rollup(dt,Qtype,UserCC) order by dt,UserCC,Qtype;")
	rows, err := d.conn.Query(ctx, "select max(dt) as d,UserCC,Qtype,sum(queries) as queries from by_usercc_1d where dt > now() - INTERVAL 4 DAY group by rollup(Qtype,UserCC) order by UserCC,Qtype;")
	if err != nil {
		slog.Error("query error", "err", err)
		return nil, fmt.Errorf("database error")
	}

	type counts struct {
		Count4       uint64
		Count6       uint64
		PercentTotal struct {
			V4 float64
			V6 float64
		}
	}

	type dateCCs struct {
		Date time.Time
		CC   map[string]*counts
	}

	type total struct {
		Date   time.Time
		Counts counts
	}

	data := struct {
		UserCC []dateCCs
		Totals []total
	}{}

	totals := map[time.Time]*counts{}
	ccs := map[time.Time]dateCCs{}

	for rows.Next() {
		var (
			dt            time.Time
			UserCC, Qtype string
			queries       uint64
		)
		if err := rows.Scan(
			&dt,
			&UserCC,
			&Qtype,
			&queries,
		); err != nil {
			log.Fatal(err)
		}

		// tdt, err := time.Parse(time.RFC3339, dt)
		// if err != nil {
		// 	slog.Error("could not parse time", "input", dt, "err", err)
		// }

		var c *counts

		if len(UserCC) > 0 {
			var ok bool

			if _, ok = ccs[dt]; !ok {
				ccs[dt] = dateCCs{
					CC: map[string]*counts{},
				}
			}

			if _, ok = ccs[dt].CC[UserCC]; !ok {
				ccs[dt].CC[UserCC] = &counts{}
			}

			c = ccs[dt].CC[UserCC]

		} else {
			slog.Info("row", "dt", dt, "usercc", UserCC, "qtype", Qtype, "queries", queries)

			if dt.UTC().Equal(time.Unix(0, 0)) {
				continue // we skip the overall total
			}

			if _, ok := totals[dt]; !ok {
				// slog.Info("totals empty", "day", dt)
				totals[dt] = &counts{}
			}

			c = totals[dt]

		}
		switch Qtype {
		case "A":
			c.Count4 = queries
		case "AAAA":
			c.Count6 = queries
		}

		// slog.Info("set c", "c", c)
		// slog.Info("totals", "totals", totals)
	}

	// spew.Dump(totals)

	for d, c := range totals {
		data.Totals = append(data.Totals, total{Date: d, Counts: *c})
	}

	for d, cdata := range ccs {

		totalDay, ok := totals[d]
		if !ok {
			slog.Error("no total for day", "date", d)
			continue
		}
		total4 := float64(totalDay.Count4)
		total6 := float64(totalDay.Count4)

		for _, cc := range cdata.CC {
			cc.PercentTotal.V4 = (100 / total4) * float64(cc.Count4)
			cc.PercentTotal.V6 = (100 / total6) * float64(cc.Count6)
		}

		cdata.Date = d

		data.UserCC = append(data.UserCC, cdata)
	}

	// todo: while we just return one rollup
	// remove this to return data by date

	for _, ucc := range data.UserCC {

		output := UserCountry{}

		for cc, c := range ucc.CC {
			output = append(output, flatAPI{
				CC:   cc,
				IPv4: c.PercentTotal.V4,
				IPv6: c.PercentTotal.V6,
			})
		}

		sort.Sort(output)

		if len(output) > 0 {
			return &output, nil
		}
	}

	return nil, nil
}
