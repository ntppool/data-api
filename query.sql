-- name: GetZoneStatsData :many
SELECT zc.date, z.name, zc.ip_version, count_active, count_registered, netspeed_active
FROM zone_server_counts zc USE INDEX (date_idx)
  INNER JOIN zones z
    ON(zc.zone_id=z.id)
  WHERE date IN (SELECT max(date) from zone_server_counts)
ORDER BY name;


-- name: GetServerNetspeed :one
select netspeed from servers where ip = ?;

-- name: GetZoneStatsV2 :many
select zone_name, netspeed_active+0 as netspeed_active FROM (
SELECT
	z.name as zone_name,
	SUM(
		IF (deletion_on IS NULL AND score_raw > 10,
			netspeed,
		  0
    )
	) AS netspeed_active
FROM
	servers s
	INNER JOIN server_zones sz ON (sz.server_id = s.id)
	INNER JOIN zones z ON (z.id = sz.zone_id)
  INNER JOIN (
    select zone_id, s.ip_version
    from server_zones sz
      inner join servers s on (s.id=sz.server_id)
    where s.ip=?
  ) as srvz on (srvz.zone_id=z.id AND srvz.ip_version=s.ip_version)
WHERE
	(deletion_on IS NULL OR deletion_on > NOW())
	AND in_pool = 1
	AND netspeed > 0
GROUP BY z.name)
AS server_netspeed;

-- name: GetServerByID :one
select * from servers
where
  id = ?;

-- name: GetServerByIP :one
select * from servers
where
  ip = sqlc.arg(ip);

-- name: GetMonitorByName :one
select * from monitors
where
  tls_name like sqlc.arg('tls_name')
  order by id
  limit 1;

-- name: GetMonitorsByID :many
select * from monitors
where id in (sqlc.slice('MonitorIDs'));

-- name: GetServerScores :many
select
    m.id, m.name, m.tls_name, m.location, m.type,
    ss.score_raw, ss.score_ts, ss.status
  from server_scores ss
    inner join monitors m
      on (m.id=ss.monitor_id)
where
  server_id = ? AND
  monitor_id in (sqlc.slice('MonitorIDs'));

-- name: GetServerLogScores :many
select * from log_scores
where
  server_id = ?
  order by ts desc
  limit ?;

-- name: GetServerLogScoresByMonitorID :many
select * from log_scores
where
  server_id = ? AND
  monitor_id = ?
  order by ts desc
  limit ?;

-- name: GetZoneByName :one
select * from zones
where
  name = sqlc.arg(name);

-- name: GetZoneCounts :many
select * from zone_server_counts
  where zone_id = ?
  order by date;
