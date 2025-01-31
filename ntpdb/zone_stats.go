package ntpdb

import (
	"context"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/common/tracing"
)

type ZoneStats []ZoneStat

type ZoneStat struct {
	CC string
	V4 float64
	V6 float64
}

func GetZoneStats(ctx context.Context, q Querier) (*ZoneStats, error) {
	log := logger.Setup()
	ctx, span := tracing.Tracer().Start(ctx, "GetZoneStats")
	defer span.End()

	zoneStatsRows, err := q.GetZoneStatsData(ctx)
	if err != nil {
		return nil, err
	}

	var (
		total4 float64
		total6 float64
	)

	// todo: consider how many servers and if they are managed
	// by the same account, in the same ASN, etc.

	type counts struct {
		Netspeed4    uint64
		Netspeed6    uint64
		PercentTotal struct {
			V4 float64
			V6 float64
		}
	}

	ccs := map[string]*counts{}

	for _, r := range zoneStatsRows {
		if r.Name == "." {
			switch r.IpVersion {
			case "v4":
				total4 = float64(r.NetspeedActive)
			case "v6":
				total6 = float64(r.NetspeedActive)
			}
			continue
		}

		if _, ok := ccs[r.Name]; !ok {
			ccs[r.Name] = &counts{}
		}

		c := ccs[r.Name]

		switch r.IpVersion {
		case "v4":
			c.Netspeed4 = uint64(r.NetspeedActive)
		case "v6":
			c.Netspeed6 = uint64(r.NetspeedActive)
		}

	}

	data := ZoneStats{}
	for name, cc := range ccs {

		log.InfoContext(ctx, "zone stats cc", "name", name)

		cc.PercentTotal.V4 = (100 / total4) * float64(cc.Netspeed4)
		cc.PercentTotal.V6 = (100 / total6) * float64(cc.Netspeed6)

		data = append(data, ZoneStat{
			CC: name,
			V4: cc.PercentTotal.V4,
			V6: cc.PercentTotal.V6,
		})
	}

	return &data, nil
}
